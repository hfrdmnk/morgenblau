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
	WorkerPoolSize = 16
	PerHostCap     = 2
	MaxRedirects   = 5
	MaxBodyBytes   = 5 << 20
	FetchTimeout   = 30 * time.Second
	UserAgent      = safehttp.UserAgent
)

var ErrTooManyRedirects = errors.New("fetcher: too many redirects")

var ErrBodyTooLarge = errors.New("fetcher: response body exceeds cap")

var ErrTimeout = errors.New("fetcher: timeout")

type HTTPError struct {
	StatusCode int
	RetryAfter time.Duration
}

func (e *HTTPError) Error() string { return fmt.Sprintf("fetcher: upstream %d", e.StatusCode) }

func ParseRetryAfter(v string, now time.Time) (time.Duration, bool) {
	return backoff.ParseRetryAfter(v, now)
}

type FeedState struct {
	ETag         string
	LastModified string
}

type Result struct {
	Feed         *gofeed.Feed
	NotModified  bool
	StatusCode   int
	ETag         string
	LastModified string
}

type Fetcher struct {
	client     *http.Client
	localFeeds *localFeeds
	workers    chan struct{}
	sf         singleflight.Group

	hostMu  sync.Mutex
	hostSem map[string]chan struct{}
}

type Option func(*Fetcher)

func WithTransport(rt http.RoundTripper) Option {
	return func(f *Fetcher) {
		f.client.Transport = rt
	}
}

func WithSafeHTTPOptions(opts ...safehttp.Option) Option {
	return func(f *Fetcher) {
		f.client = buildSafeClient(opts...)
	}
}

func New(opts ...Option) *Fetcher {
	f := &Fetcher{
		workers: make(chan struct{}, WorkerPoolSize),
		hostSem: make(map[string]chan struct{}),
		client:  buildSafeClient(),
	}
	for _, opt := range opts {
		opt(f)
	}
	if f.localFeeds != nil {
		f.localFeeds.next = f.client.Transport
		f.client.Transport = *f.localFeeds
	}
	return f
}

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

	select {
	case f.workers <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	defer func() { <-f.workers }()

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

	limited := io.LimitReader(resp.Body, MaxBodyBytes+1)
	body, err := io.ReadAll(limited)
	if err != nil {
		return out, fmt.Errorf("read body: %w", err)
	}
	if int64(len(body)) > MaxBodyBytes {
		return out, ErrBodyTooLarge
	}

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
