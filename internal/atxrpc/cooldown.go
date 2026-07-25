package atxrpc

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"

	"morgenblau/internal/backoff"
)

const (
	// defaultCooldownWindow applies when a rate-limited host names no deadline of its own.
	defaultCooldownWindow = 30 * time.Second
	// maxCooldownWindow caps mistaken or hostile deadlines so one header can't park a host for hours.
	maxCooldownWindow = 10 * time.Minute
)

// HostCoolingError reports a request refused before the network because Host is rate-limited until Until.
type HostCoolingError struct {
	Host  string
	Until time.Time
}

func (e *HostCoolingError) Error() string {
	return fmt.Sprintf("atxrpc: %s is rate-limited until %s", e.Host, e.Until.UTC().Format(time.RFC3339))
}

// IsHostCooling reports whether err is a cooldown refusal, seeing through the *url.Error that http.Client wraps it in.
func IsHostCooling(err error) bool {
	var cooling *HostCoolingError
	return errors.As(err, &cooling)
}

// HostCooldown tracks per-host rate-limit deadlines. The zero value is ready to use.
type HostCooldown struct {
	mu    sync.Mutex
	until map[string]time.Time
	now   func() time.Time // nil means time.Now; injected by tests
}

// Cooling reports whether host is still rate-limited, dropping the entry once its window has passed.
func (c *HostCooldown) Cooling(host string) (time.Time, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	until, ok := c.until[host]
	if !ok {
		return time.Time{}, false
	}
	if !until.After(c.clock()) {
		delete(c.until, host)
		return time.Time{}, false
	}
	return until, true
}

// Observe records a cooldown when resp signals rate limiting, honoring Retry-After, then ratelimit-reset, then the default window.
func (c *HostCooldown) Observe(host string, resp *http.Response) {
	if resp == nil || !rateLimited(resp) {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	now := c.clock()
	until := now.Add(cooldownWindow(resp, now))
	// Requests that raced past the check land later and must not shorten a window the host already asked for.
	if prev, ok := c.until[host]; ok && prev.After(until) {
		return
	}
	if c.until == nil {
		c.until = make(map[string]time.Time)
	}
	c.until[host] = until
}

func (c *HostCooldown) clock() time.Time {
	if c.now != nil {
		return c.now()
	}
	return time.Now()
}

// rateLimited treats 429, 503, and an exhausted ratelimit budget alike; all three mean stop.
func rateLimited(resp *http.Response) bool {
	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode == http.StatusServiceUnavailable {
		return true
	}
	return resp.Header.Get("ratelimit-remaining") == "0"
}

func cooldownWindow(resp *http.Response, now time.Time) time.Duration {
	d, ok := backoff.ParseRetryAfter(resp.Header.Get("Retry-After"), now)
	if !ok {
		d, ok = resetWindow(resp.Header.Get("ratelimit-reset"), now)
	}
	if !ok {
		d = defaultCooldownWindow
	}
	return min(d, maxCooldownWindow)
}

// resetWindow reads a ratelimit-reset unix timestamp; a stale one counts as absent so the caller falls back.
func resetWindow(v string, now time.Time) (time.Duration, bool) {
	secs, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return 0, false
	}
	d := time.Unix(secs, 0).Sub(now)
	if d <= 0 {
		return 0, false
	}
	return d, true
}

// cooldownTransport refuses requests to a cooling host, so a rate-limited PDS stops receiving traffic instead of absorbing retries.
type cooldownTransport struct {
	inner    http.RoundTripper
	cooldown *HostCooldown
}

func (t *cooldownTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	host := req.URL.Host
	if until, cooling := t.cooldown.Cooling(host); cooling {
		return nil, &HostCoolingError{Host: host, Until: until}
	}
	resp, err := t.inner.RoundTrip(req)
	if err != nil {
		return nil, err
	}
	t.cooldown.Observe(host, resp)
	return resp, nil
}
