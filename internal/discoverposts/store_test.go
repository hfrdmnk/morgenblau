package discoverposts

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"morgenblau/internal/database"
	"morgenblau/internal/database/db"
)

const sourcePostsCacheSchema = `
CREATE TABLE discover_source_posts_state (
    source_key    TEXT PRIMARY KEY,
    fetched_at    TEXT,
    favicon_url   TEXT,
    failure_count INTEGER NOT NULL DEFAULT 0,
    next_retry_at TEXT,
    favicon_failure_count INTEGER NOT NULL DEFAULT 0,
    favicon_next_retry_at TEXT
);
CREATE TABLE discover_source_posts (
    source_key   TEXT NOT NULL,
    position     INTEGER NOT NULL,
    title        TEXT NOT NULL,
    published_at TEXT,
    url          TEXT,
    post_key     TEXT NOT NULL,
    PRIMARY KEY (source_key, position)
);
CREATE UNIQUE INDEX idx_discover_source_posts_post_key ON discover_source_posts(post_key);
`

const testSourceKey = "https://blog.example/feed.xml"

func openSourcePostsCacheTestDB(t *testing.T) *database.DB {
	t.Helper()
	t.Setenv("DB_PATH", filepath.Join(t.TempDir(), "test.db"))
	dbs, err := database.Open()
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { dbs.Close() })
	if _, err := dbs.Writer.ExecContext(context.Background(), sourcePostsCacheSchema); err != nil {
		t.Fatalf("schema: %v", err)
	}
	return dbs
}

type fakePostsFetcher struct {
	calls  int
	result FetchResult
	err    error
}

func (f *fakePostsFetcher) FetchPosts(context.Context, string) (FetchResult, error) {
	f.calls++
	if f.err != nil {
		return FetchResult{}, f.err
	}
	return f.result, nil
}

// trackingPostsFetcher sleeps briefly per call so concurrent fetches overlap in time, while an atomic
// counter records the peak number of simultaneously in-flight calls (singleflight/semaphore tests).
type trackingPostsFetcher struct {
	result FetchResult
	err    error
	delay  time.Duration

	calls   int32
	current int32
	peak    int32
}

func (f *trackingPostsFetcher) FetchPosts(context.Context, string) (FetchResult, error) {
	atomic.AddInt32(&f.calls, 1)
	cur := atomic.AddInt32(&f.current, 1)
	for {
		p := atomic.LoadInt32(&f.peak)
		if cur <= p || atomic.CompareAndSwapInt32(&f.peak, p, cur) {
			break
		}
	}
	time.Sleep(f.delay)
	atomic.AddInt32(&f.current, -1)

	if f.err != nil {
		return FetchResult{}, f.err
	}
	return f.result, nil
}

func newCachedFetcherUnderTest(dbs *database.DB, fetcher PostsFetcher, ttl time.Duration, now time.Time) *CachedFetcher {
	cf := NewCachedFetcher(fetcher, db.New(dbs.Reader), ttl).WithTxRunner(dbs.Writer)
	cf.now = func() time.Time { return now }
	return cf
}

func TestCachedFetcher_FirstCall_FetchesAndCaches(t *testing.T) {
	dbs := openSourcePostsCacheTestDB(t)
	fetcher := &fakePostsFetcher{result: FetchResult{
		Posts:      []Post{{Title: "Post 1", PublishedAt: "2026-07-01T00:00:00Z", URL: "https://blog.example/1", Key: "key-1"}},
		FaviconURL: "https://blog.example/favicon.ico",
	}}
	cf := newCachedFetcherUnderTest(dbs, fetcher, time.Hour, mustParseTime(t, "2026-07-09T12:00:00Z"))

	got, err := cf.FetchPosts(context.Background(), testSourceKey)
	if err != nil {
		t.Fatalf("FetchPosts: %v", err)
	}
	if fetcher.calls != 1 {
		t.Errorf("fetcher.calls = %d, want 1", fetcher.calls)
	}
	if len(got) != 1 || got[0].Title != "Post 1" || got[0].Key != "key-1" {
		t.Fatalf("got = %+v", got)
	}

	rows, err := db.New(dbs.Reader).ListDiscoverSourcePosts(context.Background(), testSourceKey)
	if err != nil {
		t.Fatalf("ListDiscoverSourcePosts: %v", err)
	}
	if len(rows) != 1 || rows[0].Title != "Post 1" || rows[0].PostKey != "key-1" {
		t.Fatalf("cached rows = %+v", rows)
	}
}

func TestCachedFetcher_WithinTTL_ServesCacheWithoutRefetch(t *testing.T) {
	dbs := openSourcePostsCacheTestDB(t)
	fetcher := &fakePostsFetcher{result: FetchResult{Posts: []Post{{Title: "Post 1", Key: "key-1"}}}}
	start := mustParseTime(t, "2026-07-09T12:00:00Z")
	cf := newCachedFetcherUnderTest(dbs, fetcher, time.Hour, start)

	if _, err := cf.FetchPosts(context.Background(), testSourceKey); err != nil {
		t.Fatalf("first fetch: %v", err)
	}

	cf.now = func() time.Time { return start.Add(30 * time.Minute) }
	got, err := cf.FetchPosts(context.Background(), testSourceKey)
	if err != nil {
		t.Fatalf("second fetch: %v", err)
	}
	if fetcher.calls != 1 {
		t.Errorf("fetcher.calls = %d, want 1 (TTL hit must not refetch)", fetcher.calls)
	}
	if len(got) != 1 || got[0].Title != "Post 1" {
		t.Fatalf("got = %+v, want cached result", got)
	}
}

func TestCachedFetcher_PastTTL_RefetchesAndReplaces(t *testing.T) {
	dbs := openSourcePostsCacheTestDB(t)
	fetcher := &fakePostsFetcher{result: FetchResult{Posts: []Post{{Title: "Old", Key: "key-old"}}}}
	start := mustParseTime(t, "2026-07-09T12:00:00Z")
	cf := newCachedFetcherUnderTest(dbs, fetcher, time.Hour, start)

	if _, err := cf.FetchPosts(context.Background(), testSourceKey); err != nil {
		t.Fatalf("first fetch: %v", err)
	}

	fetcher.result = FetchResult{Posts: []Post{{Title: "New", Key: "key-new"}}}
	cf.now = func() time.Time { return start.Add(2 * time.Hour) }
	got, err := cf.FetchPosts(context.Background(), testSourceKey)
	if err != nil {
		t.Fatalf("second fetch: %v", err)
	}
	if fetcher.calls != 2 {
		t.Errorf("fetcher.calls = %d, want 2 (stale cache must refetch)", fetcher.calls)
	}
	if len(got) != 1 || got[0].Title != "New" {
		t.Fatalf("got = %+v, want only the new result", got)
	}
}

func TestCachedFetcher_WithoutTxRunner_DegradesToFetchAlways(t *testing.T) {
	dbs := openSourcePostsCacheTestDB(t)
	fetcher := &fakePostsFetcher{result: FetchResult{Posts: []Post{{Title: "Post 1", Key: "key-1"}}}}
	cf := NewCachedFetcher(fetcher, db.New(dbs.Reader), time.Hour)
	cf.now = func() time.Time { return mustParseTime(t, "2026-07-09T12:00:00Z") }

	if _, err := cf.FetchPosts(context.Background(), testSourceKey); err != nil {
		t.Fatalf("first fetch: %v", err)
	}
	if _, err := cf.FetchPosts(context.Background(), testSourceKey); err != nil {
		t.Fatalf("second fetch: %v", err)
	}
	if fetcher.calls != 2 {
		t.Errorf("fetcher.calls = %d, want 2 (no tx runner must never serve from cache)", fetcher.calls)
	}
}

func TestCachedFetcher_OrderSurvivesCacheRoundtrip(t *testing.T) {
	dbs := openSourcePostsCacheTestDB(t)
	fetcher := &fakePostsFetcher{result: FetchResult{Posts: []Post{
		{Title: "First", Key: "key-1"}, {Title: "Second", Key: "key-2"}, {Title: "Third", Key: "key-3"},
	}}}
	start := mustParseTime(t, "2026-07-09T12:00:00Z")
	cf := newCachedFetcherUnderTest(dbs, fetcher, time.Hour, start)

	if _, err := cf.FetchPosts(context.Background(), testSourceKey); err != nil {
		t.Fatalf("first fetch: %v", err)
	}

	cf.now = func() time.Time { return start.Add(30 * time.Minute) }
	got, err := cf.FetchPosts(context.Background(), testSourceKey)
	if err != nil {
		t.Fatalf("cached fetch: %v", err)
	}
	want := []string{"First", "Second", "Third"}
	if len(got) != len(want) {
		t.Fatalf("got = %+v, want %d posts", got, len(want))
	}
	for i, title := range want {
		if got[i].Title != title {
			t.Errorf("got[%d].Title = %q, want %q (order must survive the cache roundtrip)", i, got[i].Title, title)
		}
	}
}

func TestCachedFetcher_FaviconURLRoundtrips(t *testing.T) {
	dbs := openSourcePostsCacheTestDB(t)
	fetcher := &fakePostsFetcher{result: FetchResult{
		Posts:      []Post{{Title: "Post 1", Key: "key-1"}},
		FaviconURL: "https://blog.example/favicon.ico",
	}}
	start := mustParseTime(t, "2026-07-09T12:00:00Z")
	cf := newCachedFetcherUnderTest(dbs, fetcher, time.Hour, start)

	if _, err := cf.FetchPosts(context.Background(), testSourceKey); err != nil {
		t.Fatalf("first fetch: %v", err)
	}

	state, err := db.New(dbs.Reader).GetDiscoverSourcePostsState(context.Background(), testSourceKey)
	if err != nil {
		t.Fatalf("GetDiscoverSourcePostsState: %v", err)
	}
	if state.FaviconUrl == nil || *state.FaviconUrl != "https://blog.example/favicon.ico" {
		t.Fatalf("state.FaviconUrl = %v, want the favicon to roundtrip", state.FaviconUrl)
	}

	favicon, err := db.New(dbs.Reader).GetDiscoverSourceFaviconURL(context.Background(), testSourceKey)
	if err != nil {
		t.Fatalf("GetDiscoverSourceFaviconURL: %v", err)
	}
	if favicon == nil || *favicon != "https://blog.example/favicon.ico" {
		t.Fatalf("GetDiscoverSourceFaviconURL = %v, want the favicon fallback lookup to see it too", favicon)
	}
}

func TestCachedFetcher_DuplicateKeyPostsWithinSource_StoreKeepsFirstOccurrence(t *testing.T) {
	dbs := openSourcePostsCacheTestDB(t)
	fetcher := &fakePostsFetcher{result: FetchResult{Posts: []Post{
		{Title: "First", Key: "same-key"},
		{Title: "Second (duplicate key)", Key: "same-key"},
		{Title: "Third", Key: "key-3"},
	}}}
	cf := newCachedFetcherUnderTest(dbs, fetcher, time.Hour, mustParseTime(t, "2026-07-09T12:00:00Z"))

	if _, err := cf.FetchPosts(context.Background(), testSourceKey); err != nil {
		t.Fatalf("FetchPosts: %v, want the duplicate key to be deduped rather than fail the store", err)
	}

	rows, err := db.New(dbs.Reader).ListDiscoverSourcePosts(context.Background(), testSourceKey)
	if err != nil {
		t.Fatalf("ListDiscoverSourcePosts: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("cached rows = %+v, want 2 (duplicate key dropped, first occurrence kept)", rows)
	}
	if rows[0].Title != "First" || rows[1].Title != "Third" {
		t.Fatalf("cached rows = %+v, want [First, Third]", rows)
	}
}

// --- negative cache / backoff / stale-while-error ---

func TestCachedFetcher_FetchFailure_NoPriorState_RecordsFailureAndPropagatesError(t *testing.T) {
	dbs := openSourcePostsCacheTestDB(t)
	fetcher := &fakePostsFetcher{err: errors.New("upstream unreachable")}
	now := mustParseTime(t, "2026-07-09T12:00:00Z")
	cf := newCachedFetcherUnderTest(dbs, fetcher, time.Hour, now)

	if _, err := cf.FetchPosts(context.Background(), testSourceKey); err == nil {
		t.Fatal("expected the fetch error to propagate")
	}

	state, err := db.New(dbs.Reader).GetDiscoverSourcePostsState(context.Background(), testSourceKey)
	if err != nil {
		t.Fatalf("GetDiscoverSourcePostsState: %v", err)
	}
	if state.FailureCount != 1 {
		t.Errorf("FailureCount = %d, want 1", state.FailureCount)
	}
	if state.NextRetryAt == nil {
		t.Fatal("NextRetryAt = nil, want a future retry time")
	}
	nextRetry, perr := time.Parse(time.RFC3339, *state.NextRetryAt)
	if perr != nil || !nextRetry.After(now) {
		t.Errorf("NextRetryAt = %v, want a time after %v", *state.NextRetryAt, now)
	}
	if state.FetchedAt != nil {
		t.Errorf("FetchedAt = %v, want nil (never succeeded)", *state.FetchedAt)
	}
}

func TestCachedFetcher_FetchFailure_AfterPriorSuccess_ServesStaleRowsAndPreservesState(t *testing.T) {
	dbs := openSourcePostsCacheTestDB(t)
	fetcher := &fakePostsFetcher{result: FetchResult{
		Posts:      []Post{{Title: "Post 1", Key: "key-1"}},
		FaviconURL: "https://blog.example/favicon.ico",
	}}
	start := mustParseTime(t, "2026-07-09T12:00:00Z")
	cf := newCachedFetcherUnderTest(dbs, fetcher, time.Hour, start)

	if _, err := cf.FetchPosts(context.Background(), testSourceKey); err != nil {
		t.Fatalf("first fetch: %v", err)
	}

	failAt := start.Add(2 * time.Hour) // past TTL, triggers a live refetch attempt
	cf.now = func() time.Time { return failAt }
	fetcher.err = errors.New("upstream unreachable")

	got, err := cf.FetchPosts(context.Background(), testSourceKey)
	if err != nil {
		t.Fatalf("FetchPosts: %v, want stale-while-error to suppress it", err)
	}
	if len(got) != 1 || got[0].Title != "Post 1" {
		t.Fatalf("got = %+v, want the stale cached post", got)
	}

	state, err := db.New(dbs.Reader).GetDiscoverSourcePostsState(context.Background(), testSourceKey)
	if err != nil {
		t.Fatalf("GetDiscoverSourcePostsState: %v", err)
	}
	if state.FailureCount != 1 {
		t.Errorf("FailureCount = %d, want 1", state.FailureCount)
	}
	wantFetchedAt := start.UTC().Format(time.RFC3339)
	if state.FetchedAt == nil || *state.FetchedAt != wantFetchedAt {
		t.Errorf("FetchedAt = %v, want preserved %q from the prior success", state.FetchedAt, wantFetchedAt)
	}
	if state.FaviconUrl == nil || *state.FaviconUrl != "https://blog.example/favicon.ico" {
		t.Errorf("FaviconUrl = %v, want preserved from the prior success", state.FaviconUrl)
	}
}

func TestCachedFetcher_WithinBackoffWindow_ServesStaleRowsWithoutRefetch(t *testing.T) {
	dbs := openSourcePostsCacheTestDB(t)
	fetcher := &fakePostsFetcher{result: FetchResult{Posts: []Post{{Title: "Post 1", Key: "key-1"}}}}
	start := mustParseTime(t, "2026-07-09T12:00:00Z")
	cf := newCachedFetcherUnderTest(dbs, fetcher, time.Hour, start)

	if _, err := cf.FetchPosts(context.Background(), testSourceKey); err != nil {
		t.Fatalf("first fetch: %v", err)
	}

	failAt := start.Add(2 * time.Hour)
	cf.now = func() time.Time { return failAt }
	fetcher.err = errors.New("upstream unreachable")
	if _, err := cf.FetchPosts(context.Background(), testSourceKey); err != nil {
		t.Fatalf("failing fetch: %v", err)
	}
	if fetcher.calls != 2 {
		t.Fatalf("fetcher.calls = %d, want 2 after the failure", fetcher.calls)
	}

	// postsBackoff's first step is 5 minutes; stay inside it.
	cf.now = func() time.Time { return failAt.Add(time.Minute) }
	got, err := cf.FetchPosts(context.Background(), testSourceKey)
	if err != nil {
		t.Fatalf("FetchPosts: %v", err)
	}
	if fetcher.calls != 2 {
		t.Errorf("fetcher.calls = %d, want 2 (backoff must suppress the network call)", fetcher.calls)
	}
	if len(got) != 1 || got[0].Title != "Post 1" {
		t.Fatalf("got = %+v, want the stale cached post", got)
	}
}

func TestCachedFetcher_WithinBackoffWindow_NeverSucceeded_ServesEmptySlice(t *testing.T) {
	dbs := openSourcePostsCacheTestDB(t)
	fetcher := &fakePostsFetcher{err: errors.New("upstream unreachable")}
	start := mustParseTime(t, "2026-07-09T12:00:00Z")
	cf := newCachedFetcherUnderTest(dbs, fetcher, time.Hour, start)

	if _, err := cf.FetchPosts(context.Background(), testSourceKey); err == nil {
		t.Fatal("expected the first fetch to fail")
	}

	// Still inside the backoff window.
	cf.now = func() time.Time { return start.Add(time.Minute) }
	got, err := cf.FetchPosts(context.Background(), testSourceKey)
	if err != nil {
		t.Fatalf("FetchPosts: %v, want backoff to suppress the error too", err)
	}
	if len(got) != 0 {
		t.Fatalf("got = %+v, want an empty slice", got)
	}
	if fetcher.calls != 1 {
		t.Errorf("fetcher.calls = %d, want 1 (backoff must suppress the retry)", fetcher.calls)
	}
}

func TestCachedFetcher_PastBackoffWindow_RefetchesAgain(t *testing.T) {
	dbs := openSourcePostsCacheTestDB(t)
	fetcher := &fakePostsFetcher{err: errors.New("upstream unreachable")}
	start := mustParseTime(t, "2026-07-09T12:00:00Z")
	cf := newCachedFetcherUnderTest(dbs, fetcher, time.Hour, start)

	if _, err := cf.FetchPosts(context.Background(), testSourceKey); err == nil {
		t.Fatal("expected the first fetch to fail")
	}

	// Past the first backoff step (5 minutes).
	cf.now = func() time.Time { return start.Add(10 * time.Minute) }
	if _, err := cf.FetchPosts(context.Background(), testSourceKey); err == nil {
		t.Fatal("expected the retry to fail too (fetcher still errors)")
	}
	if fetcher.calls != 2 {
		t.Errorf("fetcher.calls = %d, want 2 (past backoff must retry)", fetcher.calls)
	}
}

func TestCachedFetcher_SuccessAfterFailure_ResetsFailureState(t *testing.T) {
	dbs := openSourcePostsCacheTestDB(t)
	fetcher := &fakePostsFetcher{err: errors.New("upstream unreachable")}
	start := mustParseTime(t, "2026-07-09T12:00:00Z")
	cf := newCachedFetcherUnderTest(dbs, fetcher, time.Hour, start)

	if _, err := cf.FetchPosts(context.Background(), testSourceKey); err == nil {
		t.Fatal("expected the first fetch to fail")
	}

	fetcher.err = nil
	fetcher.result = FetchResult{Posts: []Post{{Title: "Recovered", Key: "key-1"}}}
	cf.now = func() time.Time { return start.Add(10 * time.Minute) } // past the first backoff step

	got, err := cf.FetchPosts(context.Background(), testSourceKey)
	if err != nil {
		t.Fatalf("FetchPosts: %v", err)
	}
	if len(got) != 1 || got[0].Title != "Recovered" {
		t.Fatalf("got = %+v, want the fresh recovered post", got)
	}

	state, err := db.New(dbs.Reader).GetDiscoverSourcePostsState(context.Background(), testSourceKey)
	if err != nil {
		t.Fatalf("GetDiscoverSourcePostsState: %v", err)
	}
	if state.FailureCount != 0 {
		t.Errorf("FailureCount = %d, want 0 after recovery", state.FailureCount)
	}
	if state.NextRetryAt != nil {
		t.Errorf("NextRetryAt = %v, want nil after recovery", *state.NextRetryAt)
	}
}

// blockingUntilContextDoneFetcher mimics a real ctx-bound HTTP client: it never returns on its own,
// only when its context is canceled, so it exercises the budget's context.WithTimeout wiring for real.
type blockingUntilContextDoneFetcher struct{}

func (f *blockingUntilContextDoneFetcher) FetchPosts(ctx context.Context, key string) (FetchResult, error) {
	<-ctx.Done()
	return FetchResult{}, ctx.Err()
}

func TestCachedFetcher_LiveFetchExceedsBudget_TreatedAsFailure(t *testing.T) {
	dbs := openSourcePostsCacheTestDB(t)
	fetcher := &blockingUntilContextDoneFetcher{}
	start := mustParseTime(t, "2026-07-09T12:00:00Z")
	cf := newCachedFetcherUnderTest(dbs, fetcher, time.Hour, start)
	cf.fetchBudget = 20 * time.Millisecond

	_, err := cf.FetchPosts(context.Background(), testSourceKey)
	if err == nil {
		t.Fatal("expected the fetch to fail once the budget elapses")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("err = %v, want context.DeadlineExceeded (no special-casing: it's just another fetch error)", err)
	}

	state, serr := db.New(dbs.Reader).GetDiscoverSourcePostsState(context.Background(), testSourceKey)
	if serr != nil {
		t.Fatalf("GetDiscoverSourcePostsState: %v", serr)
	}
	if state.FailureCount != 1 {
		t.Errorf("FailureCount = %d, want 1 (budget timeout must be recorded like any other failure)", state.FailureCount)
	}
}

// --- concurrency: singleflight + fanout cap ---

func TestCachedFetcher_SingleflightCollapsesConcurrentSameKeyFetches(t *testing.T) {
	dbs := openSourcePostsCacheTestDB(t)
	fetcher := &trackingPostsFetcher{
		result: FetchResult{Posts: []Post{{Title: "Post 1", Key: "key-1"}}},
		delay:  50 * time.Millisecond,
	}
	cf := newCachedFetcherUnderTest(dbs, fetcher, time.Hour, mustParseTime(t, "2026-07-09T12:00:00Z"))

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			cf.FetchPosts(context.Background(), testSourceKey)
		}()
	}
	wg.Wait()

	if got := atomic.LoadInt32(&fetcher.calls); got != 1 {
		t.Errorf("fetcher.calls = %d, want 1 (singleflight collapsed)", got)
	}
}

func TestCachedFetcher_ConcurrentDistinctKeys_NeverExceedConcurrencyCap(t *testing.T) {
	dbs := openSourcePostsCacheTestDB(t)
	fetcher := &trackingPostsFetcher{
		result: FetchResult{Posts: []Post{{Title: "Post", Key: "key"}}},
		delay:  50 * time.Millisecond,
	}
	cf := newCachedFetcherUnderTest(dbs, fetcher, time.Hour, mustParseTime(t, "2026-07-09T12:00:00Z"))

	const n = 24
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		key := fmt.Sprintf("https://blog.example/feed-%d.xml", i)
		go func() {
			defer wg.Done()
			cf.FetchPosts(context.Background(), key)
		}()
	}
	wg.Wait()

	if got := atomic.LoadInt32(&fetcher.calls); got != n {
		t.Errorf("fetcher.calls = %d, want %d (distinct keys must never collapse)", got, n)
	}
	if peak := atomic.LoadInt32(&fetcher.peak); peak > postsFetchConcurrencyLimit {
		t.Errorf("peak concurrency = %d, want <= %d", peak, postsFetchConcurrencyLimit)
	} else if peak < 2 {
		t.Errorf("peak concurrency = %d, want concurrent fetches to actually overlap", peak)
	}
}

func mustParseTime(t *testing.T, s string) time.Time {
	t.Helper()
	tm, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t.Fatalf("parse %q: %v", s, err)
	}
	return tm
}
