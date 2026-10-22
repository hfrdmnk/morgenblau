package atxrpc

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

var testBase = time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)

func fixedClock(t time.Time) func() time.Time {
	return func() time.Time { return t }
}

func headerResponse(status int, headers map[string]string) *http.Response {
	h := http.Header{}
	for k, v := range headers {
		h.Set(k, v)
	}
	return &http.Response{StatusCode: status, Header: h}
}

func TestHostCooldown_Observe(t *testing.T) {
	epoch := func(d time.Duration) string {
		return strconv.FormatInt(testBase.Add(d).Unix(), 10)
	}

	cases := []struct {
		name    string
		status  int
		headers map[string]string
		want    time.Duration // zero means no cooldown
	}{
		{"429 retry-after beats ratelimit-reset", 429, map[string]string{"Retry-After": "60", "ratelimit-reset": epoch(5 * time.Minute)}, time.Minute},
		{"429 ratelimit-reset beats default", 429, map[string]string{"ratelimit-reset": epoch(2 * time.Minute)}, 2 * time.Minute},
		{"429 bare falls back to default", 429, nil, 30 * time.Second},
		{"503 bare falls back to default", 503, nil, 30 * time.Second},
		{"429 retry-after http-date", 429, map[string]string{"Retry-After": testBase.Add(90 * time.Second).UTC().Format(http.TimeFormat)}, 90 * time.Second},
		{"429 retry-after garbage falls back to default", 429, map[string]string{"Retry-After": "soon"}, 30 * time.Second},
		{"429 retry-after in the past falls back to default", 429, map[string]string{"Retry-After": testBase.Add(-time.Hour).UTC().Format(http.TimeFormat)}, 30 * time.Second},
		{"429 stale ratelimit-reset falls back to default", 429, map[string]string{"ratelimit-reset": epoch(-time.Hour)}, 30 * time.Second},
		{"retry-after capped at ten minutes", 429, map[string]string{"Retry-After": "3600"}, 10 * time.Minute},
		{"ratelimit-reset capped at ten minutes", 429, map[string]string{"ratelimit-reset": epoch(time.Hour)}, 10 * time.Minute},
		{"exhausted budget cools even on 200", 200, map[string]string{"ratelimit-remaining": "0", "ratelimit-reset": epoch(45 * time.Second)}, 45 * time.Second},
		{"remaining budget does not cool", 200, map[string]string{"ratelimit-remaining": "5"}, 0},
		{"plain 200 does not cool", 200, nil, 0},
		{"404 does not cool", 404, nil, 0},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := &HostCooldown{now: fixedClock(testBase)}
			c.Observe("api.example.com", headerResponse(tc.status, tc.headers))

			until, cooling := c.Cooling("api.example.com")
			if tc.want == 0 {
				if cooling {
					t.Fatalf("Cooling = true (until %v), want no cooldown", until)
				}
				return
			}
			if !cooling {
				t.Fatal("Cooling = false, want a cooldown")
			}
			if got := until.Sub(testBase); got != tc.want {
				t.Errorf("cooldown = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestHostCooldown_ObserveNeverShortensActiveCooldown(t *testing.T) {
	c := &HostCooldown{now: fixedClock(testBase)}
	c.Observe("api.example.com", headerResponse(429, map[string]string{"Retry-After": "300"}))
	c.Observe("api.example.com", headerResponse(429, nil))

	until, cooling := c.Cooling("api.example.com")
	if !cooling {
		t.Fatal("Cooling = false, want a cooldown")
	}
	if got := until.Sub(testBase); got != 5*time.Minute {
		t.Errorf("cooldown = %v, want the longer 5m window preserved", got)
	}
}

func TestHostCooldown_CoolingLazilyExpires(t *testing.T) {
	now := testBase
	c := &HostCooldown{now: func() time.Time { return now }}
	c.Observe("api.example.com", headerResponse(429, map[string]string{"Retry-After": "60"}))

	if _, cooling := c.Cooling("api.example.com"); !cooling {
		t.Fatal("Cooling = false immediately after Observe, want true")
	}

	now = testBase.Add(61 * time.Second)
	if until, cooling := c.Cooling("api.example.com"); cooling {
		t.Fatalf("Cooling = true (until %v) after the window elapsed, want false", until)
	}
	c.mu.Lock()
	tracked := len(c.until)
	c.mu.Unlock()
	if tracked != 0 {
		t.Errorf("tracked hosts = %d, want the expired entry dropped", tracked)
	}
}

func TestHostCooldown_ScopedPerHost(t *testing.T) {
	c := &HostCooldown{now: fixedClock(testBase)}
	c.Observe("a.example.com", headerResponse(429, nil))

	if _, cooling := c.Cooling("b.example.com"); cooling {
		t.Error("Cooling(b.example.com) = true, want cooldowns scoped to the observed host")
	}
}

func TestHostCooldown_UnknownHostIsNotCooling(t *testing.T) {
	c := &HostCooldown{now: fixedClock(testBase)}
	if _, cooling := c.Cooling("api.example.com"); cooling {
		t.Error("Cooling = true for an unobserved host, want false")
	}
}

func TestCooldownTransport_FailsFastWithoutHittingHost(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.Header().Set("Retry-After", "60")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	client := &http.Client{Transport: &cooldownTransport{inner: http.DefaultTransport, cooldown: &HostCooldown{}}}

	resp, err := client.Get(srv.URL)
	if err != nil {
		t.Fatalf("first Get: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("first status = %d, want 429", resp.StatusCode)
	}

	_, err = client.Get(srv.URL)
	if err == nil {
		t.Fatal("second Get succeeded, want a fail-fast cooling error")
	}
	if !IsHostCooling(err) {
		t.Fatalf("IsHostCooling(%v) = false, want true", err)
	}
	var urlErr *url.Error
	if !errors.As(err, &urlErr) {
		t.Fatalf("second Get error = %T, want the *url.Error wrapping that a real client applies", err)
	}
	if got := hits.Load(); got != 1 {
		t.Errorf("upstream hits = %d, want 1 (second request must not reach the host)", got)
	}
}

func TestCooldownTransport_RecoversAfterWindowElapses(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.Header().Set("Retry-After", "60")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	now := testBase
	client := &http.Client{Transport: &cooldownTransport{
		inner:    http.DefaultTransport,
		cooldown: &HostCooldown{now: func() time.Time { return now }},
	}}

	resp, err := client.Get(srv.URL)
	if err != nil {
		t.Fatalf("first Get: %v", err)
	}
	resp.Body.Close()

	now = testBase.Add(61 * time.Second)
	resp, err = client.Get(srv.URL)
	if err != nil {
		t.Fatalf("Get after the window elapsed: %v", err)
	}
	resp.Body.Close()
	if got := hits.Load(); got != 2 {
		t.Errorf("upstream hits = %d, want 2 once the cooldown expired", got)
	}
}

func TestIsHostCooling_OtherErrors(t *testing.T) {
	if IsHostCooling(nil) {
		t.Error("IsHostCooling(nil) = true, want false")
	}
	if IsHostCooling(http.ErrHandlerTimeout) {
		t.Error("IsHostCooling(unrelated error) = true, want false")
	}
}

func TestHostCooldown_ConcurrentAccess(t *testing.T) {
	c := &HostCooldown{}
	hosts := []string{"a.example.com", "b.example.com", "c.example.com"}

	var wg sync.WaitGroup
	for i := range 50 {
		wg.Add(2)
		host := hosts[i%len(hosts)]
		go func() {
			defer wg.Done()
			c.Observe(host, headerResponse(429, map[string]string{"Retry-After": "1"}))
		}()
		go func() {
			defer wg.Done()
			c.Cooling(host)
		}()
	}
	wg.Wait()
}
