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
	defaultCooldownWindow = 30 * time.Second
	maxCooldownWindow     = 10 * time.Minute
)

type HostCoolingError struct {
	Host  string
	Until time.Time
}

func (e *HostCoolingError) Error() string {
	return fmt.Sprintf("atxrpc: %s is rate-limited until %s", e.Host, e.Until.UTC().Format(time.RFC3339))
}

func IsHostCooling(err error) bool {
	var cooling *HostCoolingError
	return errors.As(err, &cooling)
}

type HostCooldown struct {
	mu    sync.Mutex
	until map[string]time.Time
	now   func() time.Time
}

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

func (c *HostCooldown) Observe(host string, resp *http.Response) {
	if resp == nil || !rateLimited(resp) {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	now := c.clock()
	until := now.Add(cooldownWindow(resp, now))
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
