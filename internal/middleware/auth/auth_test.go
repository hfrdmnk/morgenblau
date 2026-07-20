package auth

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bluesky-social/indigo/atproto/auth/oauth"
	"github.com/bluesky-social/indigo/atproto/syntax"

	"morgenblau/internal/oauth/cookie"
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

// noopLocker satisfies SessionLocker without serialising, for tests that don't care about concurrency.
type noopLocker struct{}

func (noopLocker) LockSession(syntax.DID, string) func() { return func() {} }

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
		{name: "api/health unauthed", path: "/api/health", method: "GET", wantCode: 200, wantNext: true},
		{name: "oauth-client-metadata", path: "/oauth-client-metadata.json", method: "GET", wantCode: 200, wantNext: true},
		{name: "oauth-jwks", path: "/oauth-jwks.json", method: "GET", wantCode: 200, wantNext: true},
		{name: "oauth/login", path: "/oauth/login", method: "POST", wantCode: 200, wantNext: true},
		{name: "oauth/callback", path: "/oauth/callback", method: "GET", wantCode: 200, wantNext: true},
		{name: "oauth/logout", path: "/oauth/logout", method: "POST", wantCode: 200, wantNext: true},
		{name: "static asset", path: "/assets/index-abc.js", method: "GET", wantCode: 200, wantNext: true},
		{name: "favicon", path: "/favicon.svg", method: "GET", wantCode: 200, wantNext: true},

		{name: "root unauthed", path: "/", method: "GET", wantCode: 200, wantNext: true},
		{name: "login unauthed", path: "/login", method: "GET", wantCode: 200, wantNext: true},

		{name: "root authed", path: "/", method: "GET", authed: true, wantCode: 302, wantNext: false, wantLoc: "/digest"},
		{name: "login authed", path: "/login", method: "GET", authed: true, wantCode: 302, wantNext: false, wantLoc: "/digest"},

		{name: "digest unauthed", path: "/digest", method: "GET", wantCode: 302, wantNext: false, wantLoc: "/login"},
		{name: "sources unauthed", path: "/sources", method: "GET", wantCode: 302, wantNext: false, wantLoc: "/login"},
		{name: "entry unauthed", path: "/entry", method: "GET", wantCode: 302, wantNext: false, wantLoc: "/login"},

		{name: "digest authed", path: "/digest", method: "GET", authed: true, wantCode: 200, wantNext: true},
		{name: "sources authed", path: "/sources", method: "GET", authed: true, wantCode: 200, wantNext: true},
		{name: "entry authed", path: "/entry", method: "GET", authed: true, wantCode: 200, wantNext: true},

		{name: "unknown unauthed", path: "/anything", method: "GET", wantCode: 302, wantNext: false, wantLoc: "/login"},
		{name: "unknown authed", path: "/anything", method: "GET", authed: true, wantCode: 200, wantNext: true},

		// Gated API: 401, frontend needs a status code rather than a redirect.
		{name: "api me unauthed", path: "/api/me", method: "GET", wantCode: 401, wantNext: false},
		{name: "api subscriptions unauthed", path: "/api/subscriptions", method: "GET", wantCode: 401, wantNext: false},

		// Dotted last segments (handles, did:web) must not trip the static-asset heuristic on API paths.
		{name: "api profile handle unauthed", path: "/api/profile/alice.example", method: "GET", wantCode: 401, wantNext: false},
		{name: "api profile did:web unauthed", path: "/api/profiles/did:web:alice.example", method: "GET", wantCode: 401, wantNext: false},
		{name: "api profile handle authed", path: "/api/profile/alice.example", method: "GET", authed: true, wantCode: 200, wantNext: true},

		{name: "api me authed", path: "/api/me", method: "GET", authed: true, wantCode: 200, wantNext: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sealer := newSealer(t)
			resumer := &fakeResumer{sessions: map[string]*oauth.ClientSession{}}
			next := &passthroughNext{}
			m := New(resumer, noopLocker{}, sealer)

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

// bodyReader reads the body and records the error, so a test can observe the MaxBytesReader cap.
type bodyReader struct {
	err error
}

func (b *bodyReader) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	_, b.err = io.ReadAll(r.Body)
	if b.err != nil {
		http.Error(w, "too big", http.StatusRequestEntityTooLarge)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func TestMiddleware_CapsAPIBodySize(t *testing.T) {
	sealer := newSealer(t)
	resumer := &fakeResumer{sessions: map[string]*oauth.ClientSession{}}
	cookie := setSession(t, sealer, resumer, "did:plc:alice", "sid-1")
	m := New(resumer, noopLocker{}, sealer)

	oversized := &bodyReader{}
	bigReq := httptest.NewRequest(http.MethodPost, "/api/subscriptions", strings.NewReader(strings.Repeat("a", (1<<20)+1)))
	bigReq.AddCookie(cookie)
	m(oversized).ServeHTTP(httptest.NewRecorder(), bigReq)
	if oversized.err == nil {
		t.Fatal("expected body read to fail past the cap")
	}
	var maxErr *http.MaxBytesError
	if !errors.As(oversized.err, &maxErr) {
		t.Errorf("read error = %v, want *http.MaxBytesError", oversized.err)
	}

	normal := &bodyReader{}
	okReq := httptest.NewRequest(http.MethodPost, "/api/subscriptions", strings.NewReader(`{"ok":true}`))
	okReq.AddCookie(cookie)
	rr := httptest.NewRecorder()
	m(normal).ServeHTTP(rr, okReq)
	if normal.err != nil {
		t.Errorf("normal body read failed: %v", normal.err)
	}
	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}
}

func TestMiddleware_InjectsSessionIntoContext(t *testing.T) {
	sealer := newSealer(t)
	resumer := &fakeResumer{sessions: map[string]*oauth.ClientSession{}}
	cookie := setSession(t, sealer, resumer, "did:plc:alice", "sid-1")

	next := &passthroughNext{}
	m := New(resumer, noopLocker{}, sealer)
	req := httptest.NewRequest(http.MethodGet, "/digest", nil)
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
	m := New(resumer, noopLocker{}, sealer)

	req := httptest.NewRequest(http.MethodGet, "/digest", nil)
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

func TestMiddleware_ResumeFailure_RedirectsAndClearsCookie(t *testing.T) {
	sealer := newSealer(t)
	resumer := &fakeResumer{sessions: map[string]*oauth.ClientSession{}, err: fmt.Errorf("dead session")}
	setRR := httptest.NewRecorder()
	sealer.Set(setRR, "did:plc:alice", "sid-1")
	cookies := setRR.Result().Cookies()

	next := &passthroughNext{}
	m := New(resumer, noopLocker{}, sealer)

	req := httptest.NewRequest(http.MethodGet, "/digest", nil)
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

// blockingResumer simulates a slow refresh, blocking until released, to assert max concurrent in-flight resumes.
type blockingResumer struct {
	mu               sync.Mutex
	inFlight         int32
	maxInFlight      int32
	release          chan struct{}
	session          *oauth.ClientSession
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

// A mutating request holds the session lock across its handler so two never
// overlap for the same session; otherwise both could refresh and one gets invalid_grant.
func TestMiddleware_MutatingRequestsDoNotOverlapInNext(t *testing.T) {
	sealer := newSealer(t)
	resumer := &fakeResumer{sessions: map[string]*oauth.ClientSession{}}
	cookie := setSession(t, sealer, resumer, "did:plc:alice", "sid-1")
	m := New(resumer, newMemoryLocker(), sealer)

	var inNext atomic.Int32
	var overlapped atomic.Bool
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if inNext.Add(1) > 1 {
			overlapped.Store(true)
		}
		time.Sleep(15 * time.Millisecond)
		inNext.Add(-1)
		w.WriteHeader(http.StatusOK)
	})
	handler := m(next)

	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req := httptest.NewRequest(http.MethodPost, "/api/subscriptions", nil)
			req.AddCookie(cookie)
			handler.ServeHTTP(httptest.NewRecorder(), req)
		}()
	}
	wg.Wait()

	if overlapped.Load() {
		t.Error("mutating handlers for the same session overlapped (lock released before next)")
	}
}

// Read-only requests take no lock, so they never queue behind a slow mutating request for the same session.
func TestMiddleware_GETNotBlockedBySlowMutating(t *testing.T) {
	sealer := newSealer(t)
	resumer := &fakeResumer{sessions: map[string]*oauth.ClientSession{}}
	cookie := setSession(t, sealer, resumer, "did:plc:alice", "sid-1")
	m := New(resumer, newMemoryLocker(), sealer)

	postEntered := make(chan struct{})
	releasePost := make(chan struct{})
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			close(postEntered)
			<-releasePost
		}
		w.WriteHeader(http.StatusOK)
	})
	handler := m(next)

	go func() {
		req := httptest.NewRequest(http.MethodPost, "/api/subscriptions", nil)
		req.AddCookie(cookie)
		handler.ServeHTTP(httptest.NewRecorder(), req)
	}()
	<-postEntered

	getDone := make(chan struct{})
	go func() {
		req := httptest.NewRequest(http.MethodGet, "/api/subscriptions", nil)
		req.AddCookie(cookie)
		handler.ServeHTTP(httptest.NewRecorder(), req)
		close(getDone)
	}()

	select {
	case <-getDone:
	case <-time.After(2 * time.Second):
		close(releasePost)
		t.Fatal("GET blocked behind an in-flight mutating request")
	}
	close(releasePost)
}

// The lock keeps only one concurrent request inside ResumeSession at a time.
func TestMiddleware_LockSerializesMutatingResume(t *testing.T) {
	sealer := newSealer(t)
	did, _ := syntax.ParseDID("did:plc:alice")
	resumer := &blockingResumer{
		release: make(chan struct{}),
		session: &oauth.ClientSession{Data: &oauth.ClientSessionData{AccountDID: did, SessionID: "sid-1"}},
	}
	locker := newMemoryLocker()
	m := New(resumer, locker, sealer)

	setRR := httptest.NewRecorder()
	sealer.Set(setRR, "did:plc:alice", "sid-1")
	cookies := setRR.Result().Cookies()

	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req := httptest.NewRequest(http.MethodPost, "/api/subscriptions", nil)
			for _, c := range cookies {
				req.AddCookie(c)
			}
			m(&passthroughNext{}).ServeHTTP(httptest.NewRecorder(), req)
		}()
	}

	// Poll until the lock has let exactly one goroutine into resumer.
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

// A transient (ctx.Canceled / DeadlineExceeded) error must leave the cookie intact so the next request can retry.
func TestMiddleware_TransientErrorKeepsCookie(t *testing.T) {
	sealer := newSealer(t)
	resumer := &fakeResumer{sessions: map[string]*oauth.ClientSession{}, err: context.Canceled}
	setRR := httptest.NewRecorder()
	sealer.Set(setRR, "did:plc:alice", "sid-1")
	cookies := setRR.Result().Cookies()

	next := &passthroughNext{}
	m := New(resumer, noopLocker{}, sealer)

	req := httptest.NewRequest(http.MethodGet, "/digest", nil)
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

// Follow create/delete write to the PDS like subscriptions/saves/shares, so they need the session lock; list (read) doesn't.
func TestHoldsSessionLock_Follows(t *testing.T) {
	cases := []struct {
		method string
		path   string
		want   bool
	}{
		{http.MethodPost, "/api/follows", true},
		{http.MethodDelete, "/api/follows/3fa", true},
		{http.MethodGet, "/api/follows", false},
	}
	for _, tc := range cases {
		req := httptest.NewRequest(tc.method, tc.path, nil)
		if got := holdsSessionLock(req); got != tc.want {
			t.Errorf("holdsSessionLock(%s %s) = %v, want %v", tc.method, tc.path, got, tc.want)
		}
	}
}
