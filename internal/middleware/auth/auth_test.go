package auth

import (
	"context"
	"crypto/rand"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bluesky-social/indigo/atproto/auth/oauth"
	"github.com/bluesky-social/indigo/atproto/syntax"

	"morgenblau/internal/oauth/cookie"
	"morgenblau/internal/routes"
)

// memoryLocker is a small in-memory SessionLocker keyed by (did, sid).
type memoryLocker struct {
	mu    sync.Mutex
	locks map[string]*sync.Mutex
}

func newMemoryLocker() *memoryLocker {
	return &memoryLocker{locks: make(map[string]*sync.Mutex)}
}

func (l *memoryLocker) LockSession(did syntax.DID, sid string) func() {
	key := did.String() + "|" + sid
	l.mu.Lock()
	m, ok := l.locks[key]
	if !ok {
		m = &sync.Mutex{}
		l.locks[key] = m
	}
	l.mu.Unlock()
	m.Lock()
	return m.Unlock
}

// noopLocker satisfies SessionLocker without serialising — for tests that
// don't care about concurrency.
type noopLocker struct{}

func (noopLocker) LockSession(syntax.DID, string) func() { return func() {} }

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

func loadRoutes(t *testing.T) []routes.Route {
	t.Helper()
	rs, err := routes.Load()
	if err != nil {
		t.Fatalf("routes.Load(): %v", err)
	}
	return rs
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
		// Infra allowlist — pass through, no auth required.
		{name: "api/health unauthed", path: "/api/health", method: "GET", wantCode: 200, wantNext: true},
		{name: "oauth-client-metadata", path: "/oauth-client-metadata.json", method: "GET", wantCode: 200, wantNext: true},
		{name: "oauth-jwks", path: "/oauth-jwks.json", method: "GET", wantCode: 200, wantNext: true},
		{name: "oauth/login", path: "/oauth/login", method: "POST", wantCode: 200, wantNext: true},
		{name: "oauth/callback", path: "/oauth/callback", method: "GET", wantCode: 200, wantNext: true},
		{name: "oauth/logout", path: "/oauth/logout", method: "POST", wantCode: 200, wantNext: true},
		{name: "static asset", path: "/assets/index-abc.js", method: "GET", wantCode: 200, wantNext: true},
		{name: "favicon", path: "/favicon.svg", method: "GET", wantCode: 200, wantNext: true},

		// Public product routes — pass when anon.
		{name: "root unauthed", path: "/", method: "GET", wantCode: 200, wantNext: true},
		{name: "login unauthed", path: "/login", method: "GET", wantCode: 200, wantNext: true},

		// Public product routes with authedRedirect — 302 when authed.
		{name: "root authed", path: "/", method: "GET", authed: true, wantCode: 302, wantNext: false, wantLoc: "/consume"},
		{name: "login authed", path: "/login", method: "GET", authed: true, wantCode: 302, wantNext: false, wantLoc: "/consume"},

		// Authed product routes — 302 /login when anon.
		{name: "consume unauthed", path: "/consume", method: "GET", wantCode: 302, wantNext: false, wantLoc: "/login"},
		{name: "sources unauthed", path: "/sources", method: "GET", wantCode: 302, wantNext: false, wantLoc: "/login"},
		{name: "entry unauthed", path: "/entry", method: "GET", wantCode: 302, wantNext: false, wantLoc: "/login"},

		// Authed product routes — pass when authed.
		{name: "consume authed", path: "/consume", method: "GET", authed: true, wantCode: 200, wantNext: true},
		{name: "sources authed", path: "/sources", method: "GET", authed: true, wantCode: 200, wantNext: true},
		{name: "entry authed", path: "/entry", method: "GET", authed: true, wantCode: 200, wantNext: true},

		// Unknown SPA path — gated by default. Anon → /login, authed → pass.
		{name: "unknown unauthed", path: "/anything", method: "GET", wantCode: 302, wantNext: false, wantLoc: "/login"},
		{name: "unknown authed", path: "/anything", method: "GET", authed: true, wantCode: 200, wantNext: true},

		// Gated API — 401 (frontend needs a status code, not a redirect).
		{name: "api me unauthed", path: "/api/me", method: "GET", wantCode: 401, wantNext: false},
		{name: "api subscriptions unauthed", path: "/api/subscriptions", method: "GET", wantCode: 401, wantNext: false},

		// Authed API — pass.
		{name: "api me authed", path: "/api/me", method: "GET", authed: true, wantCode: 200, wantNext: true},
	}

	rs := loadRoutes(t)
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sealer := newSealer(t)
			resumer := &fakeResumer{sessions: map[string]*oauth.ClientSession{}}
			next := &passthroughNext{}
			m := New(resumer, noopLocker{}, sealer, rs)

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
	m := New(resumer, noopLocker{}, sealer, loadRoutes(t))
	// /consume is authed → authed user passes through.
	req := httptest.NewRequest(http.MethodGet, "/consume", nil)
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
	m := New(resumer, noopLocker{}, sealer, loadRoutes(t))

	// Hit a gated path; a garbage cookie should be treated as anon and 302'd.
	req := httptest.NewRequest(http.MethodGet, "/consume", nil)
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
	m := New(resumer, noopLocker{}, sealer, loadRoutes(t))

	req := httptest.NewRequest(http.MethodGet, "/consume", nil)
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

// blockingResumer simulates a slow refresh — first caller blocks on a channel
// until released. Tracks max concurrent in-flight resumes for assertion.
type blockingResumer struct {
	mu              sync.Mutex
	inFlight        int32
	maxInFlight     int32
	release         chan struct{}
	session         *oauth.ClientSession
	totalInvocations int32
}

func (b *blockingResumer) ResumeSession(_ context.Context, _ syntax.DID, _ string) (*oauth.ClientSession, error) {
	atomic.AddInt32(&b.totalInvocations, 1)
	now := atomic.AddInt32(&b.inFlight, 1)
	b.mu.Lock()
	if now > b.maxInFlight {
		b.maxInFlight = now
	}
	b.mu.Unlock()
	<-b.release
	atomic.AddInt32(&b.inFlight, -1)
	return b.session, nil
}

func TestMiddleware_LockSerializesRefresh(t *testing.T) {
	sealer := newSealer(t)
	did, _ := syntax.ParseDID("did:plc:alice")
	resumer := &blockingResumer{
		release: make(chan struct{}),
		session: &oauth.ClientSession{Data: &oauth.ClientSessionData{AccountDID: did, SessionID: "sid-1"}},
	}
	locker := newMemoryLocker()
	m := New(resumer, locker, sealer, loadRoutes(t))

	setRR := httptest.NewRecorder()
	sealer.Set(setRR, "did:plc:alice", "sid-1")
	cookies := setRR.Result().Cookies()

	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req := httptest.NewRequest(http.MethodGet, "/consume", nil)
			for _, c := range cookies {
				req.AddCookie(c)
			}
			m(&passthroughNext{}).ServeHTTP(httptest.NewRecorder(), req)
		}()
	}

	// Give goroutines time to enter — the lock should keep only one in resumer.
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if atomic.LoadInt32(&resumer.inFlight) == 1 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if got := atomic.LoadInt32(&resumer.inFlight); got != 1 {
		t.Fatalf("expected exactly 1 concurrent resume, got %d", got)
	}

	// Release both in turn.
	resumer.release <- struct{}{}
	resumer.release <- struct{}{}
	wg.Wait()

	resumer.mu.Lock()
	defer resumer.mu.Unlock()
	if resumer.maxInFlight != 1 {
		t.Errorf("maxInFlight = %d, want 1 (lock didn't serialise)", resumer.maxInFlight)
	}
	if got := atomic.LoadInt32(&resumer.totalInvocations); got != 2 {
		t.Errorf("totalInvocations = %d, want 2", got)
	}
}

// On a transient (ctx.Canceled / DeadlineExceeded) error, the cookie must
// stay intact so the next request can retry.
func TestMiddleware_TransientErrorKeepsCookie(t *testing.T) {
	sealer := newSealer(t)
	resumer := &fakeResumer{sessions: map[string]*oauth.ClientSession{}, err: context.Canceled}
	setRR := httptest.NewRecorder()
	sealer.Set(setRR, "did:plc:alice", "sid-1")
	cookies := setRR.Result().Cookies()

	next := &passthroughNext{}
	m := New(resumer, noopLocker{}, sealer, loadRoutes(t))

	req := httptest.NewRequest(http.MethodGet, "/consume", nil)
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rr := httptest.NewRecorder()
	m(next).ServeHTTP(rr, req)

	for _, c := range rr.Result().Cookies() {
		if c.Name == "mb_session" && c.MaxAge < 0 {
			t.Error("cookie cleared on transient (ctx.Canceled) error")
		}
	}
}
