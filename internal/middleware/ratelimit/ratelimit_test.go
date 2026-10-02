package ratelimit

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

type clock struct{ t time.Time }

func (c *clock) now() time.Time { return c.t }

func newTestLimiter(limit int, window time.Duration) (*Limiter, *clock) {
	c := &clock{t: time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)}
	l := New(limit, window, ClientIP(false))
	l.now = c.now
	return l, c
}

func serve(h http.Handler, remoteAddr string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(http.MethodPost, "/oauth/login", nil)
	r.RemoteAddr = remoteAddr
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, r)
	return rr
}

var ok = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) })

func TestLimiter_RejectsAClientOverItsLimitWithRetryAfter(t *testing.T) {
	l, _ := newTestLimiter(2, time.Minute)
	h := l.Wrap(ok)

	for i := range 2 {
		if rr := serve(h, "192.0.2.1:1000"); rr.Code != http.StatusNoContent {
			t.Fatalf("request %d: status = %d, want 204", i+1, rr.Code)
		}
	}
	rr := serve(h, "192.0.2.1:2000")
	if rr.Code != http.StatusTooManyRequests {
		t.Fatalf("over the limit: status = %d, want 429", rr.Code)
	}
	if got := rr.Header().Get("Retry-After"); got != "60" {
		t.Fatalf("Retry-After = %q, want 60", got)
	}
	if rr := serve(h, "192.0.2.2:1000"); rr.Code != http.StatusNoContent {
		t.Fatalf("another client: status = %d, want 204", rr.Code)
	}
}

func TestLimiter_NextWindowStartsFresh(t *testing.T) {
	l, c := newTestLimiter(1, time.Minute)
	h := l.Wrap(ok)

	serve(h, "192.0.2.1:1000")
	c.t = c.t.Add(30 * time.Second)
	rr := serve(h, "192.0.2.1:1000")
	if rr.Code != http.StatusTooManyRequests {
		t.Fatalf("same window: status = %d, want 429", rr.Code)
	}
	if got := rr.Header().Get("Retry-After"); got != "30" {
		t.Fatalf("Retry-After = %q, want 30", got)
	}
	c.t = c.t.Add(30 * time.Second)
	if rr := serve(h, "192.0.2.1:1000"); rr.Code != http.StatusNoContent {
		t.Fatalf("next window: status = %d, want 204", rr.Code)
	}
}

func TestClientIP_TrustsTheFlyHeaderOnlyBehindFly(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/oauth/login", nil)
	r.RemoteAddr = "198.51.100.7:4000"
	r.Header.Set("Fly-Client-IP", "203.0.113.9")

	if got := ClientIP(true)(r); got != "203.0.113.9" {
		t.Fatalf("behind Fly: key = %q, want the Fly-Client-IP header", got)
	}
	if got := ClientIP(false)(r); got != "198.51.100.7" {
		t.Fatalf("off Fly: key = %q, want the socket address; the header is client-controlled there", got)
	}

	r.Header.Del("Fly-Client-IP")
	if got := ClientIP(true)(r); got != "198.51.100.7" {
		t.Fatalf("behind Fly without the header: key = %q, want the socket address", got)
	}
}
