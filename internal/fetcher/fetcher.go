// Package fetcher hides the polite-HTTP machinery for pulling upstream feeds
// behind a single Fetch method. Worker pool, per-host cap, singleflight,
// conditional GET, redirect cap, body cap, timeouts, and User-Agent are all
// internal — callers see one narrow surface.
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
)

const (
	// WorkerPoolSize bounds concurrent fetches across the whole process.
	// Hardcoded per SPEC fetcher posture.
	WorkerPoolSize = 16
	// PerHostCap limits simultaneous fetches against a single registrable
	// domain — prevents thundering Substack/Medium/WordPress hosts.
	PerHostCap = 2
	// MaxRedirects rejects feeds that bounce too many times. Default mirrors
	// curl's behavior.
	MaxRedirects = 5
	// MaxBodyBytes caps any single response body. Feeds larger than this are
	// almost certainly misconfigured or hostile.
	MaxBodyBytes = 5 << 20 // 5 MiB
	// FetchTimeout governs the whole request, including redirects.
	FetchTimeout = 30 * time.Second
	// UserAgent identifies Morgenblau's fetcher to upstream publishers.
	UserAgent = "Morgenblau/0.1 (+https://morgen.blue/about; bot@morgen.blue) Go-http-client/1.1"
)

// ErrTooManyRedirects signals the redirect chain exceeded MaxRedirects. The
// SPEC backoff schedule treats this like any other transient failure.
var ErrTooManyRedirects = errors.New("fetcher: too many redirects")

// ErrBodyTooLarge signals the upstream response exceeded MaxBodyBytes.
var ErrBodyTooLarge = errors.New("fetcher: response body exceeds cap")

// ErrTimeout signals the fetch exceeded FetchTimeout. Distinct from a generic
// context cancellation so callers can distinguish self-cancellation.
var ErrTimeout = errors.New("fetcher: timeout")

// FeedState is what the caller passes in to enable conditional GET and what
// the caller stores back into the Tier-2 catalog after a successful fetch.
type FeedState struct {
	ETag         string
	LastModified string
}

// Result is what Fetch returns. NotModified means the upstream returned 304
// and the caller can skip parse/upsert work entirely; ETag/LastModified are
// echoed back from the server for re-storage even on 304.
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
	parser  *gofeed.Parser

	hostMu  sync.Mutex
	hostSem map[string]chan struct{} // host → semaphore (capacity PerHostCap)
}

// Option lets tests tweak fetcher knobs (transport, timeout) without exposing
// the entire internals.
type Option func(*Fetcher)

// WithTransport injects a custom http.RoundTripper. Used by tests against
// httptest.Server so we can keep the redirect-aware http.Client we built.
func WithTransport(rt http.RoundTripper) Option {
	return func(f *Fetcher) {
		f.client.Transport = rt
	}
}

// New builds a process-wide fetcher with default knobs.
func New(opts ...Option) *Fetcher {
	f := &Fetcher{
		workers: make(chan struct{}, WorkerPoolSize),
		hostSem: make(map[string]chan struct{}),
		parser:  gofeed.NewParser(),
	}
	f.client = &http.Client{
		Timeout: FetchTimeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= MaxRedirects {
				return ErrTooManyRedirects
			}
			return nil
		},
	}
	for _, opt := range opts {
		opt(f)
	}
	return f
}

// Fetch issues a polite, conditional GET against rawURL and returns either
// a parsed feed or a NotModified signal. Two concurrent calls for the same
// URL collapse to one upstream hit (singleflight). Per-host cap, body cap,
// and redirect cap are all enforced internally; their failure modes surface
// as typed sentinel errors above.
//
// The state argument is the caller's last-known ETag / Last-Modified for
// this URL (empty strings on first fetch). The returned Result echoes the
// fresh server values to store back.
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

	// Worker-pool admission. The pool bounds total concurrency across all
	// hosts so a flood of low-cost feeds can't starve out other work.
	select {
	case f.workers <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	defer func() { <-f.workers }()

	// Per-host semaphore on the *registrable* domain. Two-feeds-per-host cap
	// matters most for Substack/Medium/etc where one company hosts many feeds.
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
		return out, fmt.Errorf("fetcher: upstream %d", resp.StatusCode)
	}

	// Cap body before parsing — bound peak memory at MaxBodyBytes.
	limited := io.LimitReader(resp.Body, MaxBodyBytes+1)
	body, err := io.ReadAll(limited)
	if err != nil {
		return out, fmt.Errorf("read body: %w", err)
	}
	if int64(len(body)) > MaxBodyBytes {
		return out, ErrBodyTooLarge
	}

	feed, err := f.parser.Parse(bytes.NewReader(body))
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

// registrableDomain returns the registrable domain (eTLD+1) for use as the
// per-host key. Falls back to the raw host if PSL doesn't recognize it
// (e.g. localhost in tests).
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
