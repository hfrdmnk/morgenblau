package handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bluesky-social/indigo/atproto/auth/oauth"
	"github.com/bluesky-social/indigo/atproto/syntax"
	"morgenblau/internal/middleware/auth"
)

type recordingLoginSync struct{ calls int }

func (s *recordingLoginSync) StartLoginRefresh(context.Context, syntax.DID, string) (string, error) {
	s.calls++
	return "job-1", nil
}

func TestCallbackAdmissionUsesVerifiedDID(t *testing.T) {
	for _, tc := range []struct {
		name       string
		did        syntax.DID
		logoutErr  error
		wantStatus int
	}{
		{name: "allowed", did: "did:plc:allowed", wantStatus: 302},
		{name: "denied", did: "did:plc:other", wantStatus: 403},
		{name: "denied despite cleanup failure", did: "did:plc:other", logoutErr: errors.New("unavailable"), wantStatus: 403},
	} {
		t.Run(tc.name, func(t *testing.T) {
			app := &fakeApp{callbackSession: &oauth.ClientSessionData{AccountDID: tc.did, SessionID: "sid-1"}, logoutErr: tc.logoutErr}
			sealer := newSealer(t)
			sync := &recordingLoginSync{}
			rr := httptest.NewRecorder()
			CallbackHandler(app, sealer, sync, auth.Allowlist{"did:plc:allowed": {}}).ServeHTTP(rr,
				httptest.NewRequest(http.MethodGet, "/oauth/callback?did=did:plc:allowed&state=s&code=c", nil))
			if rr.Code != tc.wantStatus {
				t.Fatalf("status=%d, want %d", rr.Code, tc.wantStatus)
			}
			if tc.wantStatus == 403 {
				if sync.calls != 0 || len(rr.Result().Cookies()) != 0 || rr.Header().Get("Location") != "" {
					t.Fatal("denied account received cookie, redirect, or sync")
				}
				if app.logoutDID != tc.did || app.logoutSID != "sid-1" {
					t.Fatal("denied OAuth session not cleaned up")
				}
			} else {
				if sync.calls != 1 || app.logoutDID != "" || rr.Header().Get("Location") != "/" {
					t.Fatal("allowed callback did not start sync and redirect")
				}
				check := httptest.NewRequest(http.MethodGet, "/", nil)
				for _, c := range rr.Result().Cookies() {
					check.AddCookie(c)
				}
				did, sid, ok := sealer.Get(check)
				if !ok || did != tc.did.String() || sid != "sid-1" {
					t.Fatal("allowed callback did not seal verified session")
				}
			}
		})
	}
}

func TestAlphaDevLoginCannotBypassAdmission(t *testing.T) {
	sealer := newSealer(t)
	sync := &recordingLoginSync{}
	rr := httptest.NewRecorder()
	DevLoginHandler(&devLoginStub{}, sealer, sync, auth.Allowlist{"did:plc:other": {}}).ServeHTTP(rr,
		httptest.NewRequest(http.MethodPost, "/dev/login", nil))
	if rr.Code != http.StatusForbidden || sync.calls != 0 || len(rr.Result().Cookies()) != 0 {
		t.Fatalf("status=%d sync=%d cookies=%d", rr.Code, sync.calls, len(rr.Result().Cookies()))
	}
}
