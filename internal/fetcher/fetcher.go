// Package fetcher fetches upstream feeds through a single Fetch method that
// hides worker-pool, per-host, and conditional-GET machinery from callers.
package fetcher

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/mmcdole/gofeed"
	"golang.org/x/net/publicsuffix"
	"golang.org/x/sync/singleflight"

	"morgenblau/internal/backoff"
	"morgenblau/internal/safehttp"
)

const (
	// WorkerPoolSize bounds concurrent fetches process-wide (SPEC fetcher posture).
	WorkerPoolSize = 16
	// PerHostCap limits simultaneous fetches per registrable domain, preventing thundering herds on shared hosts like Substack or Medium.
	PerHostCap = 2
	// MaxRedirects rejects feeds that redirect more than curl's default.
	MaxRedirects = 5
	// MaxBodyBytes caps a single response body; larger feeds are treated as misconfigured or hostile.
	MaxBodyBytes = 5 << 20 // 5 MiB
	// FetchTimeout governs the whole request, including redirects.
	FetchTimeout = 30 * time.Second
	// UserAgent identifies Morgenblau's fetcher to upstream publishers.
	UserAgent = safehttp.UserAgent
)

// ErrTooManyRedirects signals the redirect chain exceeded MaxRedirects; the sync pipeline applies internal/backoff to retries on this and HTTPError.
var ErrTooManyRedirects = errors.New("fetcher: too many redirects")

// ErrBodyTooLarge signals the upstream response exceeded MaxBodyBytes.
var ErrBodyTooLarge = errors.New("fetcher: response body exceeds cap")

// ErrTimeout signals the fetch exceeded FetchTimeout, distinct from generic context cancellation so callers can tell them apart.
var ErrTimeout = errors.New("fetcher: timeout")

// HTTPError reports a non-2xx upstream response; RetryAfter is zero when the header is absent or unparseable.
type HTTPError struct {
	StatusCode int
	RetryAfter time.Duration
}

func (e *HTTPError) Error() string { return fmt.Sprintf("fetcher: upstream %d", e.StatusCode) }

// ParseRetryAfter delegates to backoff, which owns retry-hint parsing for every outbound path.
func ParseRetryAfter(v string, now time.Time) (time.Duration, bool) {
	return backoff.ParseRetryAfter(v, now)
}

// FeedState carries the caller's ETag/Last-Modified for conditional GET and is stored back after a fetch.
type FeedState struct {
	ETag         string
	LastModified string
}

// Result is Fetch's return value. NotModified means the upstream sent 304, so the caller can skip parsing; ETag/LastModified are echoed back for re-storage even then.
type Result struct {
	Feed         *gofeed.Feed
	NotModified  bool
	StatusCode   int
	ETag         string
	LastModified string
}

// Fetcher is the process-wide singleton (one instance per server).
type Fetcher struct {
	client  *http.Client
	workers chan struct{} // capacity = WorkerPoolSize
	sf      singleflight.Group

	hostMu  sync.Mutex
	hostSem map[string]chan struct{} // host → semaphore (capacity PerHostCap)
}

// Option lets tests tweak fetcher knobs without exposing internals.
type Option func(*Fetcher)

// WithTransport swaps the RoundTripper only, so tests keep the safe client's redirect-checking behavior.
func WithTransport(rt http.RoundTripper) Option {
	return func(f *Fetcher) {
		f.client.Transport = rt
	}
}

// WithSafeHTTPOptions threads safehttp options (e.g. WithAllowLoopback) into the client. Test-only.
func WithSafeHTTPOptions(opts ...safehttp.Option) Option {
	return func(f *Fetcher) {
		f.client = buildSafeClient(opts...)
	}
}

// New builds a process-wide fetcher with default knobs.
func New(opts ...Option) *Fetcher {
	f := &Fetcher{
		workers: make(chan struct{}, WorkerPoolSize),
		hostSem: make(map[string]chan struct{}),
		client:  buildSafeClient(),
	}
	for _, opt := range opts {
		opt(f)
	}
	return f
}

// buildSafeClient remaps safehttp's redirect-cap sentinel to ErrTooManyRedirects so callers' errors.Is checks keep working.
func buildSafeClient(opts ...safehttp.Option) *http.Client {
	c := safehttp.NewClient(FetchTimeout, MaxRedirects, opts...)
	inner := c.CheckRedirect
	c.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if err := inner(req, via); err != nil {
			if errors.Is(err, safehttp.ErrTooManyRedirects) {
				return ErrTooManyRedirects
			}
			return err
		}
		return nil
	}
	return c
}

// Fetch issues a conditional GET for rawURL, collapsing concurrent calls to the same URL via singleflight.
func (f *Fetcher) Fetch(ctx context.Context, rawURL string, state FeedState) (*Result, error) {
	v, err, _ := f.sf.Do(rawURL, func() (any, error) {
		return f.fetchOne(ctx, rawURL, state)
	})
	if err != nil {
		return nil, err
	}
	return v.(*Result), nil
}

func (f *Fetcher) fetchOne(ctx context.Context, rawURL string, state FeedState) (*Result, error) {
	host, err := registrableDomain(rawURL)
	if err != nil {
		return nil, fmt.Errorf("parse url: %w", err)
	}

	// Worker-pool admission bounds concurrency across all hosts so cheap feeds can't starve other work.
	select {
	case f.workers <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	defer func() { <-f.workers }()

	// Per-host semaphore keyed by registrable domain matters most for shared hosts like Substack, where one company hosts many feeds.
	sem := f.hostSemaphore(host)
	select {
	case sem <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	defer func() { <-sem }()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("User-Agent", UserAgent)
	req.Header.Set("Accept", "application/atom+xml, application/rss+xml, application/feed+json, application/xml;q=0.9, */*;q=0.5")
	if state.ETag != "" {
		req.Header.Set("If-None-Match", state.ETag)
	}
	if state.LastModified != "" {
		req.Header.Set("If-Modified-Since", state.LastModified)
	}

	resp, err := f.client.Do(req)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, ErrTimeout
		}
		// http.Client wraps CheckRedirect errors in *url.Error.
		var urlErr *url.Error
		if errors.As(err, &urlErr) && errors.Is(urlErr.Err, ErrTooManyRedirects) {
			return nil, ErrTooManyRedirects
		}
		return nil, fmt.Errorf("http get: %w", err)
	}
	defer resp.Body.Close()

	out := &Result{
		StatusCode:   resp.StatusCode,
		ETag:         resp.Header.Get("ETag"),
		LastModified: resp.Header.Get("Last-Modified"),
	}

	if resp.StatusCode == http.StatusNotModified {
		out.NotModified = true
		return out, nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		ra, _ := ParseRetryAfter(resp.Header.Get("Retry-After"), time.Now())
		return out, &HTTPError{StatusCode: resp.StatusCode, RetryAfter: ra}
	}

	// Cap body before parsing, bounding peak memory at MaxBodyBytes.
	limited := io.LimitReader(resp.Body, MaxBodyBytes+1)
	body, err := io.ReadAll(limited)
	if err != nil {
		return out, fmt.Errorf("read body: %w", err)
	}
	if int64(len(body)) > MaxBodyBytes {
		return out, ErrBodyTooLarge
	}

	// gofeed.Parser lazily inits translators via unguarded read-then-write, so a
	// shared parser races under concurrent fetches; use a fresh one each time.
	feed, err := gofeed.NewParser().Parse(bytes.NewReader(body))
	if err != nil {
		return out, fmt.Errorf("parse feed: %w", err)
	}
	out.Feed = feed
	return out, nil
}

func (f *Fetcher) hostSemaphore(host string) chan struct{} {
	f.hostMu.Lock()
	defer f.hostMu.Unlock()
	if sem, ok := f.hostSem[host]; ok {
		return sem
	}
	sem := make(chan struct{}, PerHostCap)
	f.hostSem[host] = sem
	return sem
}

// registrableDomain returns the eTLD+1 for per-host keying, falling back to the raw host if the PSL doesn't recognize it (e.g. localhost in tests).
func registrableDomain(rawURL string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}
	host := u.Hostname()
	if host == "" {
		return "", fmt.Errorf("no host in url: %s", rawURL)
	}
	if strings.Contains(host, ":") || strings.Count(host, ".") == 0 {
		return host, nil
	}
	registrable, err := publicsuffix.EffectiveTLDPlusOne(host)
	if err != nil {
		return host, nil
	}
	return registrable, nil
}
