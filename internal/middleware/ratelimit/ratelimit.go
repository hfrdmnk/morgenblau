// Package ratelimit caps how often one client may hit a route, for public routes whose requests make the server do remote work.
package ratelimit

import (
	"math"
	"net"
	"net/http"
	"net/netip"
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

// ClientIP trusts forwarded addresses only behind Fly; Cloudflare provenance comes from Fly-Client-IP, never the socket address of Fly's proxy.
func ClientIP(behindFly bool) func(*http.Request) string {
	return func(r *http.Request) string {
		if behindFly {
			if ip := headerIP(r.Header, "Fly-Client-IP"); ip.IsValid() {
				for _, prefix := range cloudflarePrefixes {
					if prefix.Contains(ip) {
						if client := headerIP(r.Header, "CF-Connecting-IP"); client.IsValid() {
							return client.String()
						}
						break
					}
				}
				return ip.String()
			}
		}
		host := r.RemoteAddr
		if h, _, err := net.SplitHostPort(host); err == nil {
			host = h
		}
		return parseIP(host).String()
	}
}

func headerIP(header http.Header, name string) netip.Addr {
	values := header.Values(name)
	if len(values) != 1 {
		return netip.Addr{}
	}
	return parseIP(values[0])
}

func parseIP(value string) netip.Addr {
	ip, err := netip.ParseAddr(value)
	if err != nil || ip.Zone() != "" {
		return netip.Addr{}
	}
	return ip.Unmap()
}

// Maintainers update these with https://www.cloudflare.com/ips-v4 and https://www.cloudflare.com/ips-v6 (checked 2026-10-05); keep request handling independent of network availability.
var cloudflarePrefixes = [...]netip.Prefix{
	netip.MustParsePrefix("173.245.48.0/20"),
	netip.MustParsePrefix("103.21.244.0/22"),
	netip.MustParsePrefix("103.22.200.0/22"),
	netip.MustParsePrefix("103.31.4.0/22"),
	netip.MustParsePrefix("141.101.64.0/18"),
	netip.MustParsePrefix("108.162.192.0/18"),
	netip.MustParsePrefix("190.93.240.0/20"),
	netip.MustParsePrefix("188.114.96.0/20"),
	netip.MustParsePrefix("197.234.240.0/22"),
	netip.MustParsePrefix("198.41.128.0/17"),
	netip.MustParsePrefix("162.158.0.0/15"),
	netip.MustParsePrefix("104.16.0.0/13"),
	netip.MustParsePrefix("104.24.0.0/14"),
	netip.MustParsePrefix("172.64.0.0/13"),
	netip.MustParsePrefix("131.0.72.0/22"),
	netip.MustParsePrefix("2400:cb00::/32"),
	netip.MustParsePrefix("2606:4700::/32"),
	netip.MustParsePrefix("2803:f800::/32"),
	netip.MustParsePrefix("2405:b500::/32"),
	netip.MustParsePrefix("2405:8100::/32"),
	netip.MustParsePrefix("2a06:98c0::/29"),
	netip.MustParsePrefix("2c0f:f248::/32"),
}
