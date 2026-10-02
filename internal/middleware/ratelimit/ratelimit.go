// Package ratelimit caps how often one client may hit a route, for public routes whose requests make the server do remote work.
package ratelimit

import (
	"math"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"
)

// Limiter allows each client limit requests per fixed window. Counts reset lazily when a request arrives in a new window, so it needs no background goroutine and holds at most one window of clients.
type Limiter struct {
	limit  int
	window time.Duration
	key    func(*http.Request) string
	now    func() time.Time

	mu     sync.Mutex
	start  time.Time
	counts map[string]int
}

func New(limit int, window time.Duration, key func(*http.Request) string) *Limiter {
	return &Limiter{limit: limit, window: window, key: key, now: time.Now, counts: map[string]int{}}
}

func (l *Limiter) Wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if wait, ok := l.allow(l.key(r)); !ok {
			w.Header().Set("Retry-After", strconv.Itoa(int(math.Ceil(wait.Seconds()))))
			http.Error(w, "too many requests, try again shortly", http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (l *Limiter) allow(key string) (time.Duration, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	if now.Sub(l.start) >= l.window {
		l.start = now
		clear(l.counts)
	}
	if l.counts[key] >= l.limit {
		return l.start.Add(l.window).Sub(now), false
	}
	l.counts[key]++
	return 0, true
}

// ClientIP keys requests by client address. Fly's proxy terminates every connection and sets Fly-Client-IP, so behind Fly the socket address is the proxy's; elsewhere the header is client-controlled and ignored.
func ClientIP(behindFly bool) func(*http.Request) string {
	return func(r *http.Request) string {
		if behindFly {
			if ip := r.Header.Get("Fly-Client-IP"); ip != "" {
				return ip
			}
		}
		host, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			return r.RemoteAddr
		}
		return host
	}
}
