package sync

import (
	"context"
	"errors"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// fakeFeedLister stands in for *db.Queries.ListAllFeedURLs.
type fakeFeedLister struct {
	urls []string
	err  error
}

func (l fakeFeedLister) ListAllFeedURLs(_ context.Context) ([]string, error) {
	return l.urls, l.err
}

// recordingFetcher records every URL fetched and fails on the ones in failOn.
type recordingFetcher struct {
	mu     sync.Mutex
	seen   []string
	failOn map[string]bool
}

func (f *recordingFetcher) FetchAndStore(_ context.Context, url string) error {
	f.mu.Lock()
	f.seen = append(f.seen, url)
	f.mu.Unlock()
	if f.failOn[url] {
		return errors.New("fetch failed: " + url)
	}
	return nil
}

// peakFetcher tracks the maximum number of concurrent FetchAndStore calls.
type peakFetcher struct {
	inflight int32
	peak     int32
	sleep    time.Duration
}

func (f *peakFetcher) FetchAndStore(_ context.Context, _ string) error {
	n := atomic.AddInt32(&f.inflight, 1)
	for {
		p := atomic.LoadInt32(&f.peak)
		if n <= p || atomic.CompareAndSwapInt32(&f.peak, p, n) {
			break
		}
	}
	time.Sleep(f.sleep)
	atomic.AddInt32(&f.inflight, -1)
	return nil
}

func TestRefreshAll_FetchesEveryFeed(t *testing.T) {
	urls := []string{"https://a/feed", "https://b/feed", "https://c/feed"}
	cf := &countingFetcher{}
	r := NewGlobalRefresher(fakeFeedLister{urls: urls}, cf)

	n, err := r.RefreshAll(context.Background())
	if err != nil {
		t.Fatalf("RefreshAll: %v", err)
	}
	if n != len(urls) {
		t.Errorf("count = %d, want %d", n, len(urls))
	}

	seen := cf.seen()
	if len(seen) != len(urls) {
		t.Fatalf("fetched %d feeds, want %d", len(seen), len(urls))
	}
	got := map[string]bool{}
	for _, u := range seen {
		got[u] = true
	}
	for _, u := range urls {
		if !got[u] {
			t.Errorf("feed %q never fetched", u)
		}
	}
}

func TestRefreshAll_ListErrorPropagates(t *testing.T) {
	cf := &countingFetcher{}
	r := NewGlobalRefresher(fakeFeedLister{err: errors.New("db down")}, cf)

	n, err := r.RefreshAll(context.Background())
	if err == nil {
		t.Fatal("RefreshAll returned nil error; want the list query error")
	}
	if n != 0 {
		t.Errorf("count = %d, want 0", n)
	}
	if got := cf.seen(); len(got) != 0 {
		t.Errorf("fetched %v, want none", got)
	}
}

func TestRefreshAll_EmptyCatalog(t *testing.T) {
	cf := &countingFetcher{}
	r := NewGlobalRefresher(fakeFeedLister{urls: nil}, cf)

	n, err := r.RefreshAll(context.Background())
	if err != nil {
		t.Fatalf("RefreshAll: %v", err)
	}
	if n != 0 {
		t.Errorf("count = %d, want 0", n)
	}
	if got := cf.seen(); len(got) != 0 {
		t.Errorf("fetched %v, want none", got)
	}
}

func TestRefreshAll_PerFeedErrorDoesNotAbort(t *testing.T) {
	urls := []string{"https://a/feed", "https://b/feed", "https://c/feed", "https://d/feed"}
	rf := &recordingFetcher{failOn: map[string]bool{"https://b/feed": true}}
	r := NewGlobalRefresher(fakeFeedLister{urls: urls}, rf)

	n, err := r.RefreshAll(context.Background())
	if err != nil {
		t.Fatalf("RefreshAll: %v", err)
	}
	if n != len(urls) {
		t.Errorf("count = %d, want %d", n, len(urls))
	}

	rf.mu.Lock()
	attempted := len(rf.seen)
	rf.mu.Unlock()
	if attempted != len(urls) {
		t.Errorf("attempted %d feeds, want all %d (a failure aborted the sweep)", attempted, len(urls))
	}
}

func TestRefreshAll_RespectsConcurrencyLimit(t *testing.T) {
	urls := make([]string, globalFetchConcurrency*3)
	for i := range urls {
		urls[i] = "https://feed/" + strconv.Itoa(i)
	}
	pf := &peakFetcher{sleep: 20 * time.Millisecond}
	r := NewGlobalRefresher(fakeFeedLister{urls: urls}, pf)

	if _, err := r.RefreshAll(context.Background()); err != nil {
		t.Fatalf("RefreshAll: %v", err)
	}

	peak := atomic.LoadInt32(&pf.peak)
	if peak > int32(globalFetchConcurrency) {
		t.Errorf("peak concurrency = %d, want <= %d", peak, globalFetchConcurrency)
	}
	if peak < 2 {
		t.Errorf("peak concurrency = %d, want concurrent execution (>= 2)", peak)
	}
}

func TestRefreshAll_ContextCancellationStops(t *testing.T) {
	urls := []string{"https://a/feed", "https://b/feed"}
	bf := newBlockingFetcher() // never released, only ctx can unblock it
	r := NewGlobalRefresher(fakeFeedLister{urls: urls}, bf)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	n, err := r.RefreshAll(ctx)
	if err == nil {
		t.Fatal("RefreshAll returned nil; want context cancellation")
	}
	if n != len(urls) {
		t.Errorf("count = %d, want %d", n, len(urls))
	}
	if atomic.LoadInt32(&bf.ctxErrs) == 0 {
		t.Error("fetcher did not observe ctx cancellation")
	}
}
