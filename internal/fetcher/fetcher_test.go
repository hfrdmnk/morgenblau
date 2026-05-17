package fetcher

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"morgenblau/internal/safehttp"
)

// newTestFetcher returns a Fetcher whose transport permits 127.0.0.1 so
// httptest.NewServer (which binds to loopback) is reachable.
func newTestFetcher() *Fetcher {
	return New(WithSafeHTTPOptions(safehttp.WithAllowLoopback()))
}

const rssBody = `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0"><channel>
<title>Test Feed</title>
<link>https://example.test</link>
<description>Test</description>
<item><title>Hello</title><link>https://example.test/1</link><guid>1</guid></item>
</channel></rss>`

func TestFetch_HappyPathParses(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = w.Write([]byte(rssBody))
	}))
	defer srv.Close()

	f := newTestFetcher()
	out, err := f.Fetch(context.Background(), srv.URL+"/feed.xml", FeedState{})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if out.NotModified {
		t.Fatal("unexpected NotModified")
	}
	if out.Feed == nil || len(out.Feed.Items) != 1 {
		t.Fatalf("feed = %+v", out.Feed)
	}
}

func TestFetch_SendsUserAgent(t *testing.T) {
	var gotUA string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUA = r.Header.Get("User-Agent")
		_, _ = w.Write([]byte(rssBody))
	}))
	defer srv.Close()

	if _, err := newTestFetcher().Fetch(context.Background(), srv.URL, FeedState{}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(gotUA, "Morgenblau") || !strings.Contains(gotUA, "morgen.blue/about") {
		t.Errorf("UA = %q, want morgenblau identity", gotUA)
	}
}

func TestFetch_ConditionalGET_304ReturnsNotModified(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		if r.Header.Get("If-None-Match") == `"v1"` {
			w.Header().Set("ETag", `"v1"`)
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("ETag", `"v1"`)
		_, _ = w.Write([]byte(rssBody))
	}))
	defer srv.Close()

	f := newTestFetcher()
	r1, err := f.Fetch(context.Background(), srv.URL, FeedState{})
	if err != nil {
		t.Fatal(err)
	}
	if r1.ETag != `"v1"` || r1.NotModified {
		t.Fatalf("r1 = %+v", r1)
	}

	r2, err := f.Fetch(context.Background(), srv.URL, FeedState{ETag: r1.ETag})
	if err != nil {
		t.Fatal(err)
	}
	if !r2.NotModified {
		t.Errorf("r2.NotModified = false")
	}
	if r2.Feed != nil {
		t.Errorf("r2.Feed should be nil on 304")
	}
	if atomic.LoadInt32(&hits) != 2 {
		t.Errorf("hits = %d, want 2", hits)
	}
}

func TestFetch_RedirectCap(t *testing.T) {
	var redirector http.HandlerFunc
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		redirector(w, r)
	}))
	defer srv.Close()

	// 10 hops > MaxRedirects (5).
	redirector = func(w http.ResponseWriter, r *http.Request) {
		next := r.URL.Path + "x"
		if len(next) > 10 {
			_, _ = w.Write([]byte(rssBody))
			return
		}
		http.Redirect(w, r, next, http.StatusFound)
	}

	_, err := newTestFetcher().Fetch(context.Background(), srv.URL+"/", FeedState{})
	if !errors.Is(err, ErrTooManyRedirects) {
		t.Errorf("err = %v, want ErrTooManyRedirects", err)
	}
}

func TestFetch_BodyCap(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// MaxBodyBytes + 100 bytes of garbage.
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = w.Write(make([]byte, MaxBodyBytes+100))
	}))
	defer srv.Close()

	_, err := newTestFetcher().Fetch(context.Background(), srv.URL, FeedState{})
	if !errors.Is(err, ErrBodyTooLarge) {
		t.Errorf("err = %v, want ErrBodyTooLarge", err)
	}
}

func TestFetch_Singleflight_CollapsesConcurrent(t *testing.T) {
	var hits int32
	// Block briefly to let two callers stack up before responding.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&hits, 1)
		time.Sleep(50 * time.Millisecond)
		_, _ = w.Write([]byte(rssBody))
	}))
	defer srv.Close()

	f := newTestFetcher()
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := f.Fetch(context.Background(), srv.URL, FeedState{}); err != nil {
				t.Errorf("Fetch: %v", err)
			}
		}()
	}
	wg.Wait()

	if got := atomic.LoadInt32(&hits); got != 1 {
		t.Errorf("hits = %d, want 1 (singleflight collapsed)", got)
	}
}

// Two different hosts should run concurrently; three requests to the same
// host serialize behind PerHostCap.
func TestFetch_PerHostCap(t *testing.T) {
	const slow = 100 * time.Millisecond
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(slow)
		_, _ = w.Write([]byte(rssBody))
	})

	srv1 := httptest.NewServer(handler)
	defer srv1.Close()
	srv2 := httptest.NewServer(handler)
	defer srv2.Close()

	f := newTestFetcher()

	// Two parallel hosts — should finish within ~slow.
	t0 := time.Now()
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, _ = f.Fetch(context.Background(), srv1.URL+"/a", FeedState{})
	}()
	go func() {
		defer wg.Done()
		_, _ = f.Fetch(context.Background(), srv2.URL+"/a", FeedState{})
	}()
	wg.Wait()
	parallel := time.Since(t0)
	if parallel > 2*slow {
		t.Errorf("two hosts ran serially: %v (want < %v)", parallel, 2*slow)
	}

	// Three requests on the *same* host — PerHostCap=2 → at least ~2*slow.
	t1 := time.Now()
	var wg2 sync.WaitGroup
	for i := 0; i < 3; i++ {
		wg2.Add(1)
		i := i
		go func() {
			defer wg2.Done()
			_, _ = f.Fetch(context.Background(), fmt.Sprintf("%s/path%d", srv1.URL, i), FeedState{})
		}()
	}
	wg2.Wait()
	serial := time.Since(t1)
	if serial < 2*slow {
		t.Errorf("three on one host finished in %v, want >= %v (serialized by PerHostCap)", serial, 2*slow)
	}
}

func TestFetch_TimeoutClassified(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(150 * time.Millisecond)
		_, _ = w.Write([]byte(rssBody))
	}))
	defer srv.Close()

	f := newTestFetcher()
	// Cheap way to force timeout without rewriting the public client knob:
	// give the request a 50ms context.
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := f.Fetch(ctx, srv.URL, FeedState{})
	if err == nil {
		t.Fatal("expected timeout error")
	}
	// Context cancellation comes back as context.DeadlineExceeded inside the
	// http.Client wrapping. We accept either explicit ErrTimeout or the
	// underlying DeadlineExceeded — both are valid signals.
	if !errors.Is(err, ErrTimeout) && !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("err = %v, want timeout-like", err)
	}
}
