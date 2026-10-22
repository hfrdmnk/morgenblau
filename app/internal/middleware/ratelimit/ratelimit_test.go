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

func TestLimiter_ProxyClientBuckets(t *testing.T) {
	type request struct {
		remote, fly, cf string
		want            int
	}
	tests := []struct {
		name      string
		behindFly bool
		requests  []request
	}{
		{"clients on one edge", true, []request{
			{"192.0.2.1:1000", "173.245.48.1", "203.0.113.1", 204},
			{"192.0.2.1:1000", "173.245.48.1", "203.0.113.2", 204},
			{"192.0.2.1:2000", "173.245.48.1", "203.0.113.1", 429},
		}},
		{"client across edges and spellings", true, []request{
			{"192.0.2.1:1000", "173.245.48.1", "2001:db8::a", 204},
			{"192.0.2.1:2000", "2606:4700::1", "2001:0DB8:0000:0000:0000:0000:0000:000A", 429},
		}},
		{"mapped client", true, []request{
			{"192.0.2.1:1000", "173.245.48.1", "203.0.113.1", 204},
			{"192.0.2.1:2000", "::ffff:173.245.48.2", "::ffff:cb00:7101", 429},
		}},
		{"direct Fly client cannot forge CF or XFF", true, []request{
			{"173.245.48.1:1000", "203.0.113.9", "198.51.100.1", 204},
			{"173.245.48.1:2000", "::ffff:203.0.113.9", "198.51.100.2", 429},
		}},
		{"off Fly ignores headers", false, []request{
			{"[2001:db8::a]:1000", "173.245.48.1", "203.0.113.1", 204},
			{"[2001:0DB8:0:0:0:0:0:A]:2000", "2606:4700::1", "203.0.113.2", 429},
		}},
		{"direct Fly IPv6 canonicalized", true, []request{
			{"192.0.2.1:1000", "2001:db8::a", "203.0.113.1", 204},
			{"192.0.2.1:2000", "2001:0DB8:0:0:0:0:0:A", "203.0.113.2", 429},
		}},
		{"missing or invalid CF falls back to edge not socket", true, []request{
			{"192.0.2.1:1000", "173.245.48.1", "", 204},
			{"192.0.2.2:2000", "173.245.48.1", "invalid", 429},
			{"192.0.2.1:1000", "173.245.48.2", "", 204},
		}},
		{"off Fly mapped socket", false, []request{
			{"203.0.113.1:1000", "173.245.48.1", "198.51.100.1", 204},
			{"[::ffff:203.0.113.1]:2000", "173.245.48.2", "198.51.100.2", 429},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l, _ := newTestLimiter(1, time.Minute)
			l.key = ClientIP(tt.behindFly)
			h := l.Wrap(ok)
			for i, input := range tt.requests {
				r := httptest.NewRequest(http.MethodPost, "/oauth/login", nil)
				r.RemoteAddr = input.remote
				r.Header.Set("Fly-Client-IP", input.fly)
				r.Header.Set("CF-Connecting-IP", input.cf)
				r.Header.Set("X-Forwarded-For", input.cf)
				rr := httptest.NewRecorder()
				h.ServeHTTP(rr, r)
				if rr.Code != input.want {
					t.Fatalf("request %d: status = %d, want %d", i+1, rr.Code, input.want)
				}
			}
		})
	}
}

func TestLimiter_CloudflareRanges(t *testing.T) {
	tests := []struct {
		edge    string
		trusted bool
	}{
		{"173.245.47.255", false}, {"173.245.48.0", true}, {"173.245.63.255", true}, {"173.245.64.0", false},
		{"103.21.244.1", true}, {"103.22.200.1", true}, {"103.31.4.1", true},
		{"141.101.64.1", true}, {"108.162.192.1", true}, {"190.93.240.1", true},
		{"188.114.96.1", true}, {"197.234.240.1", true}, {"198.41.128.1", true}, {"162.158.0.1", true},
		{"104.15.255.255", false}, {"104.16.0.0", true}, {"104.23.255.255", true},
		{"104.24.0.0", true}, {"104.27.255.255", true}, {"104.28.0.0", false},
		{"172.64.0.1", true}, {"131.0.72.1", true},
		{"2400:cb00::1", true}, {"2606:46ff:ffff:ffff:ffff:ffff:ffff:ffff", false},
		{"2606:4700::", true}, {"2606:4700:ffff:ffff:ffff:ffff:ffff:ffff", true}, {"2606:4701::", false},
		{"2803:f800::1", true}, {"2405:b500::1", true}, {"2405:8100::1", true},
		{"2a06:98bf:ffff:ffff:ffff:ffff:ffff:ffff", false}, {"2a06:98c0::", true},
		{"2a06:98c7:ffff:ffff:ffff:ffff:ffff:ffff", true}, {"2a06:98c8::", false}, {"2c0f:f248::1", true},
	}
	for _, tt := range tests {
		t.Run(tt.edge, func(t *testing.T) {
			l, _ := newTestLimiter(1, time.Minute)
			l.key = ClientIP(true)
			h := l.Wrap(ok)
			for i, client := range []string{"203.0.113.1", "203.0.113.2"} {
				r := httptest.NewRequest(http.MethodPost, "/oauth/login", nil)
				r.RemoteAddr = "192.0.2.1:1000"
				r.Header.Set("Fly-Client-IP", tt.edge)
				r.Header.Set("CF-Connecting-IP", client)
				rr := httptest.NewRecorder()
				h.ServeHTTP(rr, r)
				want := http.StatusNoContent
				if i == 1 && !tt.trusted {
					want = http.StatusTooManyRequests
				}
				if rr.Code != want {
					t.Fatalf("client %s: status = %d, want %d", client, rr.Code, want)
				}
			}
		})
	}
}

func TestLimiter_MalformedProxyHeadersFallBack(t *testing.T) {
	for _, header := range []string{"CF-Connecting-IP", "Fly-Client-IP"} {
		t.Run(header, func(t *testing.T) {
			l, _ := newTestLimiter(1, time.Minute)
			l.key = ClientIP(true)
			h := l.Wrap(ok)
			for i, values := range [][]string{
				nil, {""}, {"not-an-ip"}, {"203.0.113.1:1234"}, {"[2001:db8::1]"},
				{"2001:db8::1%zone"}, {"203.0.113.1, 203.0.113.2"}, {" 203.0.113.1 "},
				{"203.0.113.1", "203.0.113.2"},
			} {
				r := httptest.NewRequest(http.MethodPost, "/oauth/login", nil)
				r.RemoteAddr = "173.245.48.2:1000"
				r.Header.Set("Fly-Client-IP", "173.245.48.1")
				r.Header.Set("CF-Connecting-IP", "203.0.113.1")
				if header == "Fly-Client-IP" && i%2 == 1 {
					r.Header.Set("CF-Connecting-IP", "203.0.113.2")
				}
				r.Header[http.CanonicalHeaderKey(header)] = values
				rr := httptest.NewRecorder()
				h.ServeHTTP(rr, r)
				want := http.StatusTooManyRequests
				if i == 0 {
					want = http.StatusNoContent
				}
				if rr.Code != want {
					t.Fatalf("values %q: status = %d, want %d", values, rr.Code, want)
				}
			}
		})
	}
}
