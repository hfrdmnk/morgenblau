package auth

import (
	"context"
	"crypto/rand"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bluesky-social/indigo/atproto/auth/oauth"
	"github.com/bluesky-social/indigo/atproto/syntax"

	"morgenblau/internal/oauth/cookie"
)

// fakeResumer satisfies the Resumer interface using a map.
type fakeResumer struct {
	sessions map[string]*oauth.ClientSession
	err      error
}

func (f *fakeResumer) ResumeSession(_ context.Context, did syntax.DID, sid string) (*oauth.ClientSession, error) {
	if f.err != nil {
		return nil, f.err
	}
	key := did.String() + "|" + sid
	if sess, ok := f.sessions[key]; ok {
		return sess, nil
	}
	return nil, fmt.Errorf("not found")
}

func newSealer(t *testing.T) *cookie.Sealer {
	t.Helper()
	key := make([]byte, 32)
	rand.Read(key)
	s, err := cookie.New(key)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// passthroughNext records the request that reached it and returns 200.
type passthroughNext struct {
	hit   bool
	ctx   context.Context
	calls int
}

func (p *passthroughNext) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	p.hit = true
	p.ctx = r.Context()
	p.calls++
	w.WriteHeader(http.StatusOK)
}

// setSession installs an authed session in the resumer and returns the cookie.
func setSession(t *testing.T, sealer *cookie.Sealer, resumer *fakeResumer, did, sid string) *http.Cookie {
	t.Helper()
	rr := httptest.NewRecorder()
	sealer.Set(rr, did, sid)
	cookies := rr.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("sealer.Set produced no cookies")
	}
	parsed, _ := syntax.ParseDID(did)
	resumer.sessions[did+"|"+sid] = &oauth.ClientSession{
		Data: &oauth.ClientSessionData{AccountDID: parsed, SessionID: sid},
	}
	return cookies[0]
}

func TestMiddleware_Table(t *testing.T) {
	type tcase struct {
		name     string
		path     string
		method   string
		authed   bool
		wantCode int
		wantNext bool
		wantLoc  string
	}

	cases := []tcase{
		// Allowlist — pass through, no auth required.
		{name: "api/health unauthed", path: "/api/health", method: "GET", wantCode: 200, wantNext: true},
		{name: "oauth-client-metadata", path: "/oauth-client-metadata.json", method: "GET", wantCode: 200, wantNext: true},
		{name: "oauth-jwks", path: "/oauth-jwks.json", method: "GET", wantCode: 200, wantNext: true},
		{name: "oauth/login", path: "/oauth/login", method: "POST", wantCode: 200, wantNext: true},
		{name: "oauth/callback", path: "/oauth/callback", method: "GET", wantCode: 200, wantNext: true},
		{name: "oauth/logout", path: "/oauth/logout", method: "POST", wantCode: 200, wantNext: true},
		{name: "static asset", path: "/assets/index-abc.js", method: "GET", wantCode: 200, wantNext: true},
		{name: "favicon", path: "/favicon.svg", method: "GET", wantCode: 200, wantNext: true},

		// Login page — public when unauthed, redirects to / when authed.
		{name: "login unauthed", path: "/login", method: "GET", wantCode: 200, wantNext: true},
		{name: "login authed", path: "/login", method: "GET", authed: true, wantCode: 302, wantNext: false, wantLoc: "/"},

		// Gated SPA routes — 302 to /login when unauthed.
		{name: "root unauthed", path: "/", method: "GET", wantCode: 302, wantNext: false, wantLoc: "/login"},
		{name: "sources unauthed", path: "/sources", method: "GET", wantCode: 302, wantNext: false, wantLoc: "/login"},
		{name: "random unauthed", path: "/anything", method: "GET", wantCode: 302, wantNext: false, wantLoc: "/login"},

		// Gated API — 401 (frontend needs a status code, not a redirect).
		{name: "api me unauthed", path: "/api/me", method: "GET", wantCode: 401, wantNext: false},
		{name: "api subscriptions unauthed", path: "/api/subscriptions", method: "GET", wantCode: 401, wantNext: false},

		// Authed access passes through.
		{name: "root authed", path: "/", method: "GET", authed: true, wantCode: 200, wantNext: true},
		{name: "sources authed", path: "/sources", method: "GET", authed: true, wantCode: 200, wantNext: true},
		{name: "api me authed", path: "/api/me", method: "GET", authed: true, wantCode: 200, wantNext: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sealer := newSealer(t)
			resumer := &fakeResumer{sessions: map[string]*oauth.ClientSession{}}
			next := &passthroughNext{}
			m := New(resumer, sealer)

			req := httptest.NewRequest(tc.method, tc.path, nil)
			if tc.authed {
				cookie := setSession(t, sealer, resumer, "did:plc:alice", "sid-1")
				req.AddCookie(cookie)
			}
			rr := httptest.NewRecorder()
			m(next).ServeHTTP(rr, req)

			if rr.Code != tc.wantCode {
				t.Errorf("status = %d, want %d (body: %s)", rr.Code, tc.wantCode, rr.Body.String())
			}
			if next.hit != tc.wantNext {
				t.Errorf("next hit = %v, want %v", next.hit, tc.wantNext)
			}
			if tc.wantLoc != "" && rr.Header().Get("Location") != tc.wantLoc {
				t.Errorf("Location = %q, want %q", rr.Header().Get("Location"), tc.wantLoc)
			}
		})
	}
}

func TestMiddleware_InjectsSessionIntoContext(t *testing.T) {
	sealer := newSealer(t)
	resumer := &fakeResumer{sessions: map[string]*oauth.ClientSession{}}
	cookie := setSession(t, sealer, resumer, "did:plc:alice", "sid-1")

	next := &passthroughNext{}
	m := New(resumer, sealer)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(cookie)
	rr := httptest.NewRecorder()
	m(next).ServeHTTP(rr, req)

	if !next.hit {
		t.Fatal("next not called")
	}
	sess := SessionFromContext(next.ctx)
	if sess == nil {
		t.Fatal("no session in context")
	}
	if sess.Data.AccountDID.String() != "did:plc:alice" {
		t.Errorf("did = %s", sess.Data.AccountDID)
	}
}

func TestMiddleware_InvalidCookie_TreatedAsUnauthed(t *testing.T) {
	sealer := newSealer(t)
	resumer := &fakeResumer{sessions: map[string]*oauth.ClientSession{}}
	next := &passthroughNext{}
	m := New(resumer, sealer)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: "mb_session", Value: "garbage"})
	rr := httptest.NewRecorder()
	m(next).ServeHTTP(rr, req)

	if rr.Code != http.StatusFound {
		t.Errorf("status = %d, want 302", rr.Code)
	}
	if next.hit {
		t.Error("next called despite invalid cookie")
	}
}

// If ResumeSession fails (e.g. session row deleted, refresh token died),
// treat the request as unauthed and clear the stale cookie.
func TestMiddleware_ResumeFailure_RedirectsAndClearsCookie(t *testing.T) {
	sealer := newSealer(t)
	resumer := &fakeResumer{sessions: map[string]*oauth.ClientSession{}, err: fmt.Errorf("dead session")}
	// install cookie pointing at a session the resumer can't find
	setRR := httptest.NewRecorder()
	sealer.Set(setRR, "did:plc:alice", "sid-1")
	cookies := setRR.Result().Cookies()

	next := &passthroughNext{}
	m := New(resumer, sealer)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rr := httptest.NewRecorder()
	m(next).ServeHTTP(rr, req)

	if rr.Code != http.StatusFound {
		t.Errorf("status = %d, want 302", rr.Code)
	}
	if next.hit {
		t.Error("next called despite ResumeSession failure")
	}
	cleared := false
	for _, c := range rr.Result().Cookies() {
		if c.Name == "mb_session" && c.MaxAge < 0 {
			cleared = true
		}
	}
	if !cleared {
		t.Error("stale cookie not cleared on resume failure")
	}
}
