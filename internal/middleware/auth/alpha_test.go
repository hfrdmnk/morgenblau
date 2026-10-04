package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bluesky-social/indigo/atproto/syntax"
	"morgenblau/internal/session"
)

func TestLoadAllowlist(t *testing.T) {
	for _, tc := range []struct {
		name, enabled, dids string
		wantError           bool
		wantRestricted      bool
	}{
		{name: "default unrestricted"},
		{name: "explicitly disabled", enabled: "false", dids: "invalid"},
		{name: "multiple trimmed DIDs", enabled: "true", dids: " did:plc:allowed , did:web:reader.example.com , did:plc:allowed ", wantRestricted: true},
		{name: "empty list", enabled: "true", wantError: true},
		{name: "whitespace list", enabled: "true", dids: "  ", wantError: true},
		{name: "handle instead of DID", enabled: "true", dids: "reader.example", wantError: true},
		{name: "invalid second entry", enabled: "true", dids: "did:plc:allowed,reader.example", wantError: true},
		{name: "trailing comma", enabled: "true", dids: "did:plc:allowed,", wantError: true},
		{name: "invalid flag", enabled: "tru", dids: "did:plc:allowed", wantError: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env := map[string]string{"ALPHA_ENABLED": tc.enabled, "ALPHA_ALLOWED_DIDS": tc.dids}
			allowed, err := LoadAllowlist(func(key string) string { return env[key] })
			if (err != nil) != tc.wantError {
				t.Fatalf("error = %v, wantError = %v", err, tc.wantError)
			}
			if err != nil {
				return
			}
			if !allowed.Allows("did:plc:allowed") || !allowed.Allows("did:web:reader.example.com") {
				t.Fatal("expected listed accounts to be admitted")
			}
			for _, did := range []syntax.DID{"did:plc:other", "did:plc:allowedmore", "did:web:other.example.com"} {
				if got := allowed.Allows(did); got == tc.wantRestricted {
					t.Fatalf("Allows(%q) = %v, restricted = %v", did, got, tc.wantRestricted)
				}
			}
		})
	}
}

type alphaResumer struct{ calls int }

func (r *alphaResumer) ResumeSession(_ context.Context, did syntax.DID, sid string) (*session.Session, error) {
	r.calls++
	return &session.Session{Data: &session.Data{AccountDID: did, SessionID: sid}}, nil
}

func TestAllowlistGatesExistingSessionsBeforeResume(t *testing.T) {
	for _, tc := range []struct {
		path       string
		allowed    bool
		wantStatus int
		wantNext   bool
	}{
		{path: "/api/digest", wantStatus: 401},
		{path: "/digest", wantStatus: 302},
		{path: "/login", wantStatus: 200, wantNext: true},
		{path: "/api/health", wantStatus: 200, wantNext: true},
		{path: "/oauth-client-metadata.json", wantStatus: 200, wantNext: true},
		{path: "/oauth-jwks.json", wantStatus: 200, wantNext: true},
		{path: "/api/digest", allowed: true, wantStatus: 200, wantNext: true},
	} {
		t.Run(tc.path+" allowed="+map[bool]string{true: "true", false: "false"}[tc.allowed], func(t *testing.T) {
			sealer := newSealer(t)
			set := httptest.NewRecorder()
			did := "did:plc:removed"
			if tc.allowed {
				did = "did:plc:allowed"
			}
			sealer.Set(set, did, "existing-session")
			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			req.AddCookie(set.Result().Cookies()[0])
			resumer := &alphaResumer{}
			next := &passthroughNext{}
			rr := httptest.NewRecorder()
			New(resumer, noopLocker{}, sealer, Allowlist{"did:plc:allowed": {}})(next).ServeHTTP(rr, req)
			if rr.Code != tc.wantStatus || next.hit != tc.wantNext {
				t.Fatalf("status=%d next=%v, want status=%d next=%v", rr.Code, next.hit, tc.wantStatus, tc.wantNext)
			}
			if !tc.allowed {
				if resumer.calls != 0 || next.ctx != nil && SessionFromContext(next.ctx) != nil {
					t.Fatal("removed account resumed or reached handler authenticated")
				}
				cookies := rr.Result().Cookies()
				if len(cookies) != 1 || cookies[0].MaxAge >= 0 {
					t.Fatal("removed account cookie not cleared")
				}
			} else if resumer.calls != 1 || SessionFromContext(next.ctx) == nil {
				t.Fatal("allowed account did not resume")
			}
		})
	}
}
