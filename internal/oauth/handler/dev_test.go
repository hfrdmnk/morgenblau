package handler

import (
	"context"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	"morgenblau/internal/session"
)

type devLoginStub struct {
	err   error
	calls int
}

func (d *devLoginStub) LoginDev(context.Context) (*session.Session, error) {
	d.calls++
	return &session.Session{Data: &session.Data{AccountDID: "did:plc:abcdefghijklmnopqrstuvwx", SessionID: "dev:example"}}, d.err
}

func TestDevLoginHandler(t *testing.T) {
	for _, tc := range []struct {
		name, method, origin string
		failure              bool
		status, calls        int
	}{
		{"availability", "GET", "", false, 200, 0},
		{"login", "POST", "https://app.example.com", false, 204, 1},
		{"agent", "POST", "", false, 204, 1},
		{"cross origin", "POST", "https://other.example.com", false, 403, 0},
		{"failure", "POST", "", true, 502, 1},
		{"wrong method", "DELETE", "", false, 405, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stub := &devLoginStub{}
			if tc.failure {
				stub.err = errors.New("upstream secret")
			}
			sealer := newSealer(t)
			h := DevLoginHandler(stub, sealer, nil)
			r := httptest.NewRequest(tc.method, "https://app.example.com/dev/login", nil)
			if tc.origin != "" {
				r.Header.Set("Origin", tc.origin)
			}
			rr := httptest.NewRecorder()
			h.ServeHTTP(rr, r)
			if rr.Code != tc.status || stub.calls != tc.calls {
				t.Fatalf("status=%d calls=%d", rr.Code, stub.calls)
			}
			if strings.Contains(rr.Body.String(), "secret") {
				t.Fatal("leaked upstream error")
			}
			if tc.status == 204 {
				cookies := rr.Result().Cookies()
				if len(cookies) != 1 || !cookies[0].HttpOnly || !cookies[0].Secure {
					t.Fatal("missing sealed session cookie")
				}
				check := httptest.NewRequest("GET", "/", nil)
				check.AddCookie(cookies[0])
				did, sid, ok := sealer.Get(check)
				if !ok || did != "did:plc:abcdefghijklmnopqrstuvwx" || sid != "dev:example" {
					t.Fatal("wrong session cookie")
				}
			} else if len(rr.Result().Cookies()) != 0 {
				t.Fatal("cookie set on non-login response")
			}
		})
	}
}
