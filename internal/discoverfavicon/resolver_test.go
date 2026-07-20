package discoverfavicon

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"sync"
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
`

const testCandidateKey = "https://blog.example/feed.xml"

// --- fakes for the three site sources ---

type fakeResolutionReader struct {
	mu    sync.Mutex
	sites map[string]string
	err   error
	calls int
}

func noResolutionSites() *fakeResolutionReader {
	return &fakeResolutionReader{sites: map[string]string{}}
}

func (f *fakeResolutionReader) GetDiscoverPublicationResolutionByCanonicalKey(_ context.Context, canonicalKey *string) (db.DiscoverPublicationResolution, error) {
	f.mu.Lock()
	f.calls++
	f.mu.Unlock()
	if f.err != nil {
		return db.DiscoverPublicationResolution{}, f.err
	}
	site, ok := f.sites[*canonicalKey]
	if !ok {
		return db.DiscoverPublicationResolution{}, sql.ErrNoRows
	}
	return db.DiscoverPublicationResolution{SiteUrl: &site}, nil
}

func (f *fakeResolutionReader) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

type fakeSubscriptionReader struct {
	mu    sync.Mutex
	sites map[string]string
	err   error
	calls int
}

func noSubscriptionSites() *fakeSubscriptionReader {
	return &fakeSubscriptionReader{sites: map[string]string{}}
}

func (f *fakeSubscriptionReader) GetDiscoverCrawlSubscriptionSiteURLByKey(_ context.Context, canonicalKey string) (*string, error) {
	f.mu.Lock()
	f.calls++
	f.mu.Unlock()
	if f.err != nil {
		return nil, f.err
	}
	site, ok := f.sites[canonicalKey]
	if !ok {
		return nil, sql.ErrNoRows
	}
	return &site, nil
}

func (f *fakeSubscriptionReader) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

type fakeTrendingReader struct {
	mu    sync.Mutex
	sites map[string]*string
	err   error
	calls int
}

func noTrendingSites() *fakeTrendingReader { return &fakeTrendingReader{sites: map[string]*string{}} }

func (f *fakeTrendingReader) GetDiscoverTrendingSignalTitle(_ context.Context, sourceKey string) (db.GetDiscoverTrendingSignalTitleRow, error) {
	f.mu.Lock()
	f.calls++
	f.mu.Unlock()
	if f.err != nil {
		return db.GetDiscoverTrendingSignalTitleRow{}, f.err
	}
	site, ok := f.sites[sourceKey]
	if !ok {
		return db.GetDiscoverTrendingSignalTitleRow{}, sql.ErrNoRows
	}
	return db.GetDiscoverTrendingSignalTitleRow{SiteUrl: site}, nil
}

func (f *fakeTrendingReader) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

// fakeDiscoverer stubs favicon discovery; delay lets concurrency tests keep several calls in flight at once.
type fakeDiscoverer struct {
	mu    sync.Mutex
	url   string
	err   error
	delay time.Duration
	calls int
}

func (f *fakeDiscoverer) Discover(ctx context.Context, siteURL string) (string, error) {
	f.mu.Lock()
	f.calls++
	f.mu.Unlock()
	if f.delay > 0 {
		select {
		case <-time.After(f.delay):
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
	if f.err != nil {
		return "", f.err
	}
	return f.url, nil
}

func (f *fakeDiscoverer) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

// fakeState is an in-memory StateReader+StateWriter double, mirroring one discover_source_posts_state row.
type fakeState struct {
	mu    sync.Mutex
	state map[string]db.DiscoverSourcePostsState
}

func newFakeState() *fakeState { return &fakeState{state: map[string]db.DiscoverSourcePostsState{}} }

func (f *fakeState) GetDiscoverSourcePostsState(_ context.Context, sourceKey string) (db.DiscoverSourcePostsState, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	s, ok := f.state[sourceKey]
	if !ok {
		return db.DiscoverSourcePostsState{}, sql.ErrNoRows
	}
	return s, nil
}

func (f *fakeState) UpsertDiscoverSourceFaviconURL(_ context.Context, arg db.UpsertDiscoverSourceFaviconURLParams) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	s := f.state[arg.SourceKey]
	s.SourceKey = arg.SourceKey
	s.FaviconUrl = arg.FaviconUrl
	s.FaviconFailureCount = 0
	s.FaviconNextRetryAt = nil
	f.state[arg.SourceKey] = s
	return nil
}

func (f *fakeState) RecordDiscoverSourceFaviconDiscoveryFailure(_ context.Context, arg db.RecordDiscoverSourceFaviconDiscoveryFailureParams) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	s := f.state[arg.SourceKey]
	s.SourceKey = arg.SourceKey
	s.FaviconFailureCount = arg.FaviconFailureCount
	s.FaviconNextRetryAt = arg.FaviconNextRetryAt
	f.state[arg.SourceKey] = s
	return nil
}

// newResolverUnderTest wires the fake state double directly into runTx, bypassing WithTxRunner's *sql.DB
// requirement (same package as resolver.go, so the unexported fields are reachable from tests).
func newResolverUnderTest(resolutions PublicationResolutionReader, subscriptions SubscriptionSiteReader, trending TrendingSiteReader, discoverer FaviconDiscoverer, state *fakeState, now time.Time) *Resolver {
	r := NewResolver(resolutions, subscriptions, trending, discoverer, state)
	r.runTx = func(ctx context.Context, fn func(StateWriter) error) error {
		return fn(state)
	}
	r.now = func() time.Time { return now }
	return r
}

func TestResolve_NoSiteAnywhere_ReturnsEmptyWithoutRecordingBackoff(t *testing.T) {
	resolutions := noResolutionSites()
	subscriptions := noSubscriptionSites()
	trending := noTrendingSites()
	discoverer := &fakeDiscoverer{url: "https://blog.example/favicon.ico"}
	state := newFakeState()
	r := newResolverUnderTest(resolutions, subscriptions, trending, discoverer, state, time.Now())

	got, err := r.Resolve(context.Background(), testCandidateKey)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got != "" {
		t.Errorf("got = %q, want empty (no site known anywhere)", got)
	}
	if discoverer.callCount() != 0 {
		t.Errorf("discoverer.calls = %d, want 0 (nothing to discover)", discoverer.callCount())
	}

	// Second immediate call must re-query all three sources: a "nothing found" outcome is never cached.
	if _, err := r.Resolve(context.Background(), testCandidateKey); err != nil {
		t.Fatalf("second Resolve: %v", err)
	}
	if resolutions.callCount() != 2 || subscriptions.callCount() != 2 || trending.callCount() != 2 {
		t.Errorf("call counts = resolutions:%d subscriptions:%d trending:%d, want 2/2/2 (no backoff recorded)",
			resolutions.callCount(), subscriptions.callCount(), trending.callCount())
	}
}

func TestResolve_ResolutionSite_DiscoveredAndPersisted(t *testing.T) {
	resolutions := &fakeResolutionReader{sites: map[string]string{testCandidateKey: "https://blog.example"}}
	subscriptions := noSubscriptionSites()
	trending := noTrendingSites()
	discoverer := &fakeDiscoverer{url: "https://blog.example/favicon.ico"}
	state := newFakeState()
	r := newResolverUnderTest(resolutions, subscriptions, trending, discoverer, state, time.Now())

	got, err := r.Resolve(context.Background(), testCandidateKey)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got != "https://blog.example/favicon.ico" {
		t.Errorf("got = %q, want the discovered icon", got)
	}
	if subscriptions.callCount() != 0 || trending.callCount() != 0 {
		t.Errorf("subscriptions.calls = %d, trending.calls = %d, want 0/0 (resolution hit must short-circuit)",
			subscriptions.callCount(), trending.callCount())
	}

	stored, err := state.GetDiscoverSourcePostsState(context.Background(), testCandidateKey)
	if err != nil {
		t.Fatalf("GetDiscoverSourcePostsState: %v", err)
	}
	if stored.FaviconUrl == nil || *stored.FaviconUrl != got {
		t.Errorf("stored favicon_url = %v, want %q persisted", stored.FaviconUrl, got)
	}
}

func TestResolve_SubscriptionSite_UsedWhenResolutionMisses(t *testing.T) {
	resolutions := noResolutionSites()
	subscriptions := &fakeSubscriptionReader{sites: map[string]string{testCandidateKey: "https://sub.example"}}
	trending := noTrendingSites()
	discoverer := &fakeDiscoverer{url: "https://sub.example/favicon.ico"}
	state := newFakeState()
	r := newResolverUnderTest(resolutions, subscriptions, trending, discoverer, state, time.Now())

	got, err := r.Resolve(context.Background(), testCandidateKey)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got != "https://sub.example/favicon.ico" {
		t.Errorf("got = %q, want the subscription-sourced icon", got)
	}
	if trending.callCount() != 0 {
		t.Errorf("trending.calls = %d, want 0 (subscription hit must short-circuit)", trending.callCount())
	}
}

func TestResolve_TrendingSite_UsedAsLastFallback(t *testing.T) {
	resolutions := noResolutionSites()
	subscriptions := noSubscriptionSites()
	site := "https://trend.example"
	trending := &fakeTrendingReader{sites: map[string]*string{testCandidateKey: &site}}
	discoverer := &fakeDiscoverer{url: "https://trend.example/favicon.ico"}
	state := newFakeState()
	r := newResolverUnderTest(resolutions, subscriptions, trending, discoverer, state, time.Now())

	got, err := r.Resolve(context.Background(), testCandidateKey)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got != "https://trend.example/favicon.ico" {
		t.Errorf("got = %q, want the trending-sourced icon", got)
	}
}

func TestResolve_TrendingSiteNullOrEmpty_TreatedAsMiss(t *testing.T) {
	resolutions := noResolutionSites()
	subscriptions := noSubscriptionSites()
	empty := ""
	trending := &fakeTrendingReader{sites: map[string]*string{
		testCandidateKey: nil, // NULL site_url
	}}
	discoverer := &fakeDiscoverer{url: "https://unused.example/favicon.ico"}
	state := newFakeState()
	r := newResolverUnderTest(resolutions, subscriptions, trending, discoverer, state, time.Now())

	got, err := r.Resolve(context.Background(), testCandidateKey)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got != "" {
		t.Errorf("got = %q, want empty (NULL trending site_url is a miss)", got)
	}
	if discoverer.callCount() != 0 {
		t.Errorf("discoverer.calls = %d, want 0", discoverer.callCount())
	}

	trending.sites[testCandidateKey] = &empty
	got, err = r.Resolve(context.Background(), testCandidateKey)
	if err != nil {
		t.Fatalf("Resolve (empty site_url): %v", err)
	}
	if got != "" {
		t.Errorf("got = %q, want empty (empty-string trending site_url is a miss)", got)
	}
}

func TestResolve_DiscoveryFailure_RecordsBackoffAndSkipsWithinWindow(t *testing.T) {
	resolutions := &fakeResolutionReader{sites: map[string]string{testCandidateKey: "https://blog.example"}}
	subscriptions := noSubscriptionSites()
	trending := noTrendingSites()
	discoverer := &fakeDiscoverer{err: errors.New("dns lookup failed")}
	state := newFakeState()
	start := time.Now()
	r := newResolverUnderTest(resolutions, subscriptions, trending, discoverer, state, start)

	got, err := r.Resolve(context.Background(), testCandidateKey)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got != "" {
		t.Errorf("got = %q, want empty on discovery failure", got)
	}
	if discoverer.callCount() != 1 {
		t.Fatalf("discoverer.calls = %d, want 1", discoverer.callCount())
	}

	// Still within the backoff window: must skip all lookups entirely.
	r.now = func() time.Time { return start.Add(time.Minute) }
	got, err = r.Resolve(context.Background(), testCandidateKey)
	if err != nil {
		t.Fatalf("second Resolve: %v", err)
	}
	if got != "" {
		t.Errorf("got = %q, want empty (still in backoff)", got)
	}
	if discoverer.callCount() != 1 {
		t.Errorf("discoverer.calls = %d, want 1 (backoff window must skip discovery entirely)", discoverer.callCount())
	}
	if resolutions.callCount() != 1 {
		t.Errorf("resolutions.calls = %d, want 1 (backoff window must skip site lookups too)", resolutions.callCount())
	}
}

func TestResolve_ConcurrentSameKey_CollapsesToOneDiscovery(t *testing.T) {
	resolutions := &fakeResolutionReader{sites: map[string]string{testCandidateKey: "https://blog.example"}}
	subscriptions := noSubscriptionSites()
	trending := noTrendingSites()
	discoverer := &fakeDiscoverer{url: "https://blog.example/favicon.ico", delay: 50 * time.Millisecond}
	state := newFakeState()
	r := newResolverUnderTest(resolutions, subscriptions, trending, discoverer, state, time.Now())

	const n = 20
	var wg sync.WaitGroup
	errs := make([]error, n)
	results := make([]string, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i], errs[i] = r.Resolve(context.Background(), testCandidateKey)
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("Resolve[%d]: %v", i, err)
		}
		if results[i] != "https://blog.example/favicon.ico" {
			t.Errorf("results[%d] = %q, want the discovered icon", i, results[i])
		}
	}
	if discoverer.callCount() != 1 {
		t.Errorf("discoverer.calls = %d, want 1 (concurrent same-key calls must collapse)", discoverer.callCount())
	}
}

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

// TestFaviconLadder_IndependentOfPostsLadder exercises the two new queries against real SQLite: a favicon
// discovery outcome must never touch the posts ladder (fetched_at/failure_count/next_retry_at) and vice versa.
func TestFaviconLadder_IndependentOfPostsLadder(t *testing.T) {
	dbs := openSourcePostsCacheTestDB(t)
	ctx := context.Background()
	w := db.New(dbs.Writer)
	r := db.New(dbs.Reader)

	fetchedAt := "2026-07-09T12:00:00Z"
	faviconURL := "https://blog.example/favicon.ico"
	if err := w.UpsertDiscoverSourcePostsState(ctx, db.UpsertDiscoverSourcePostsStateParams{
		SourceKey: testCandidateKey, FetchedAt: &fetchedAt, FaviconUrl: &faviconURL,
	}); err != nil {
		t.Fatalf("UpsertDiscoverSourcePostsState: %v", err)
	}

	nextRetry := "2026-07-09T13:00:00Z"
	if err := w.RecordDiscoverSourceFaviconDiscoveryFailure(ctx, db.RecordDiscoverSourceFaviconDiscoveryFailureParams{
		SourceKey: testCandidateKey, FaviconFailureCount: 1, FaviconNextRetryAt: &nextRetry,
	}); err != nil {
		t.Fatalf("RecordDiscoverSourceFaviconDiscoveryFailure: %v", err)
	}

	got, err := r.GetDiscoverSourcePostsState(ctx, testCandidateKey)
	if err != nil {
		t.Fatalf("GetDiscoverSourcePostsState: %v", err)
	}
	if got.FetchedAt == nil || *got.FetchedAt != fetchedAt {
		t.Errorf("fetched_at = %v, want untouched %q", got.FetchedAt, fetchedAt)
	}
	if got.FaviconUrl == nil || *got.FaviconUrl != faviconURL {
		t.Errorf("favicon_url = %v, want untouched %q (a discovery failure must not erase a stored icon)", got.FaviconUrl, faviconURL)
	}
	if got.FailureCount != 0 {
		t.Errorf("failure_count (posts ladder) = %d, want 0 untouched", got.FailureCount)
	}
	if got.NextRetryAt != nil {
		t.Errorf("next_retry_at (posts ladder) = %v, want nil untouched", got.NextRetryAt)
	}
	if got.FaviconFailureCount != 1 || got.FaviconNextRetryAt == nil || *got.FaviconNextRetryAt != nextRetry {
		t.Errorf("favicon ladder = (count=%d, retry=%v), want (1, %q)", got.FaviconFailureCount, got.FaviconNextRetryAt, nextRetry)
	}

	// A subsequent discovery success must reset the favicon ladder without touching the posts ladder.
	newIcon := "https://blog.example/new-favicon.ico"
	if err := w.UpsertDiscoverSourceFaviconURL(ctx, db.UpsertDiscoverSourceFaviconURLParams{SourceKey: testCandidateKey, FaviconUrl: &newIcon}); err != nil {
		t.Fatalf("UpsertDiscoverSourceFaviconURL: %v", err)
	}
	got, err = r.GetDiscoverSourcePostsState(ctx, testCandidateKey)
	if err != nil {
		t.Fatalf("GetDiscoverSourcePostsState: %v", err)
	}
	if got.FaviconUrl == nil || *got.FaviconUrl != newIcon {
		t.Errorf("favicon_url = %v, want %q", got.FaviconUrl, newIcon)
	}
	if got.FaviconFailureCount != 0 || got.FaviconNextRetryAt != nil {
		t.Errorf("favicon ladder = (count=%d, retry=%v), want reset to (0, nil)", got.FaviconFailureCount, got.FaviconNextRetryAt)
	}
	if got.FetchedAt == nil || *got.FetchedAt != fetchedAt {
		t.Errorf("fetched_at = %v, want still untouched %q (posts ladder)", got.FetchedAt, fetchedAt)
	}

	// A posts-side failure, in turn, must not clobber the favicon ladder.
	postsRetry := "2026-07-09T14:00:00Z"
	if err := w.RecordDiscoverSourcePostsFailure(ctx, db.RecordDiscoverSourcePostsFailureParams{
		SourceKey: testCandidateKey, FailureCount: 3, NextRetryAt: &postsRetry,
	}); err != nil {
		t.Fatalf("RecordDiscoverSourcePostsFailure: %v", err)
	}
	got, err = r.GetDiscoverSourcePostsState(ctx, testCandidateKey)
	if err != nil {
		t.Fatalf("GetDiscoverSourcePostsState: %v", err)
	}
	if got.FaviconUrl == nil || *got.FaviconUrl != newIcon {
		t.Errorf("favicon_url = %v, want untouched %q (a posts-side failure must not erase a stored icon)", got.FaviconUrl, newIcon)
	}
	if got.FaviconFailureCount != 0 || got.FaviconNextRetryAt != nil {
		t.Errorf("favicon ladder = (count=%d, retry=%v), want untouched (0, nil) by a posts-side failure", got.FaviconFailureCount, got.FaviconNextRetryAt)
	}
}
