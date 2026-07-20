package sharemeta

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"testing"
	"time"

	"morgenblau/internal/database/db"
)

type fakeEntryReader struct {
	documents map[string]db.GetFeedEntryShareMetadataByDocumentRow
	urls      map[string]db.GetFeedEntryShareMetadataByItemURLRow
}

func noEntries() *fakeEntryReader {
	return &fakeEntryReader{
		documents: map[string]db.GetFeedEntryShareMetadataByDocumentRow{},
		urls:      map[string]db.GetFeedEntryShareMetadataByItemURLRow{},
	}
}

func (f *fakeEntryReader) GetFeedEntryShareMetadataByDocument(_ context.Context, guid string) (db.GetFeedEntryShareMetadataByDocumentRow, error) {
	row, ok := f.documents[guid]
	if !ok {
		return db.GetFeedEntryShareMetadataByDocumentRow{}, sql.ErrNoRows
	}
	return row, nil
}

func (f *fakeEntryReader) GetFeedEntryShareMetadataByItemURL(_ context.Context, url string) (db.GetFeedEntryShareMetadataByItemURLRow, error) {
	row, ok := f.urls[url]
	if !ok {
		return db.GetFeedEntryShareMetadataByItemURLRow{}, sql.ErrNoRows
	}
	return row, nil
}

type fakeCache struct {
	mu     sync.Mutex
	states map[string]db.ShareMetadataCache
}

func newFakeCache() *fakeCache {
	return &fakeCache{states: map[string]db.ShareMetadataCache{}}
}

func (f *fakeCache) GetShareMetadataCache(_ context.Context, key string) (db.ShareMetadataCache, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	state, ok := f.states[key]
	if !ok {
		return db.ShareMetadataCache{}, sql.ErrNoRows
	}
	return state, nil
}

func (f *fakeCache) UpsertShareMetadataSuccess(_ context.Context, arg db.UpsertShareMetadataSuccessParams) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.states[arg.TargetKey] = db.ShareMetadataCache{
		TargetKey: arg.TargetKey,
		Title:     arg.Title,
		TargetUrl: arg.TargetUrl,
		FetchedAt: arg.FetchedAt,
	}
	return nil
}

func (f *fakeCache) RecordShareMetadataFailure(_ context.Context, arg db.RecordShareMetadataFailureParams) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	state := f.states[arg.TargetKey]
	state.TargetKey = arg.TargetKey
	state.FailureCount = arg.FailureCount
	state.NextRetryAt = arg.NextRetryAt
	f.states[arg.TargetKey] = state
	return nil
}

type fakeMetadataFetcher struct {
	mu       sync.Mutex
	metadata Metadata
	err      error
	delay    time.Duration
	calls    int
}

type canceledMetadataFetcher struct {
	calls int
}

func (f *canceledMetadataFetcher) Fetch(context.Context, Target) (Metadata, error) {
	f.calls++
	return Metadata{}, context.Canceled
}

func (f *fakeMetadataFetcher) Fetch(ctx context.Context, _ Target) (Metadata, error) {
	f.mu.Lock()
	f.calls++
	f.mu.Unlock()
	if f.delay > 0 {
		select {
		case <-time.After(f.delay):
		case <-ctx.Done():
			return Metadata{}, ctx.Err()
		}
	}
	return f.metadata, f.err
}

func (f *fakeMetadataFetcher) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

func newResolverUnderTest(entries EntryReader, cache *fakeCache, fetcher MetadataFetcher, now time.Time) *Resolver {
	resolver := NewResolver(entries, cache, fetcher, DefaultTTL)
	resolver.now = func() time.Time { return now }
	resolver.runTx = func(ctx context.Context, fn func(CacheWriter) error) error {
		return fn(cache)
	}
	return resolver
}

func TestResolveMany_PrefersDocumentEntryMetadata(t *testing.T) {
	title := "Cached document"
	entries := noEntries()
	entries.documents[testDocumentURI] = db.GetFeedEntryShareMetadataByDocumentRow{
		Title: &title, EntrySlug: "cached-doc", Url: "https://publication.example/cached",
	}
	fetcher := &fakeMetadataFetcher{err: errors.New("must not fetch")}
	resolver := newResolverUnderTest(entries, newFakeCache(), fetcher, time.Now())

	got := resolver.ResolveMany(context.Background(), []Target{{
		Document: testDocumentURI,
		ItemURL:  "https://sidecar.example/item",
	}})

	if len(got) != 1 || got[0] != (Metadata{
		Title: "Cached document", EntrySlug: "cached-doc", TargetURL: "https://publication.example/cached",
	}) {
		t.Fatalf("got = %+v", got)
	}
	if fetcher.callCount() != 0 {
		t.Errorf("fetch calls = %d, want 0", fetcher.callCount())
	}
}

func TestResolveMany_MatchesRegularItemURLInFeedEntries(t *testing.T) {
	title := "Cached RSS post"
	entries := noEntries()
	entries.urls["https://blog.example/post"] = db.GetFeedEntryShareMetadataByItemURLRow{
		Title: &title, EntrySlug: "cached-rss", Url: "https://blog.example/post",
	}
	fetcher := &fakeMetadataFetcher{err: errors.New("must not fetch")}
	resolver := newResolverUnderTest(entries, newFakeCache(), fetcher, time.Now())

	got := resolver.ResolveMany(context.Background(), []Target{{ItemURL: "https://blog.example/post"}})

	if len(got) != 1 || got[0].Title != title || got[0].EntrySlug != "cached-rss" {
		t.Fatalf("got = %+v", got)
	}
	if fetcher.callCount() != 0 {
		t.Errorf("fetch calls = %d, want 0", fetcher.callCount())
	}
}

func TestResolveMany_FreshMetadataCacheAvoidsLiveFetch(t *testing.T) {
	now := time.Date(2026, 7, 19, 12, 0, 0, 0, time.UTC)
	fetchedAt := now.Add(-time.Hour).Format(time.RFC3339)
	title := "Cached live title"
	targetURL := "https://example.com/post"
	cache := newFakeCache()
	cache.states[targetURL] = db.ShareMetadataCache{
		TargetKey: targetURL, Title: &title, TargetUrl: &targetURL, FetchedAt: &fetchedAt,
	}
	fetcher := &fakeMetadataFetcher{err: errors.New("must not fetch")}
	resolver := newResolverUnderTest(noEntries(), cache, fetcher, now)

	got := resolver.ResolveMany(context.Background(), []Target{{ItemURL: targetURL}})

	if len(got) != 1 || got[0].Title != title {
		t.Fatalf("got = %+v", got)
	}
	if fetcher.callCount() != 0 {
		t.Errorf("fetch calls = %d, want 0", fetcher.callCount())
	}
}

func TestResolveMany_LiveSuccessIsCached(t *testing.T) {
	now := time.Date(2026, 7, 19, 12, 0, 0, 0, time.UTC)
	cache := newFakeCache()
	fetcher := &fakeMetadataFetcher{metadata: Metadata{
		Title: "Fetched title", TargetURL: "https://example.com/final",
	}}
	resolver := newResolverUnderTest(noEntries(), cache, fetcher, now)
	target := Target{ItemURL: "https://example.com/post"}

	first := resolver.ResolveMany(context.Background(), []Target{target})
	second := resolver.ResolveMany(context.Background(), []Target{target})

	if len(first) != 1 || first[0].Title != "Fetched title" || len(second) != 1 || second[0] != first[0] {
		t.Fatalf("first = %+v, second = %+v", first, second)
	}
	if fetcher.callCount() != 1 {
		t.Errorf("fetch calls = %d, want 1", fetcher.callCount())
	}
}

func TestResolveMany_RefreshFailureServesStaleAndStartsBackoff(t *testing.T) {
	now := time.Date(2026, 7, 19, 12, 0, 0, 0, time.UTC)
	oldFetchedAt := now.Add(-48 * time.Hour).Format(time.RFC3339)
	title := "Last known title"
	targetURL := "https://example.com/post"
	cache := newFakeCache()
	cache.states[targetURL] = db.ShareMetadataCache{
		TargetKey: targetURL, Title: &title, TargetUrl: &targetURL, FetchedAt: &oldFetchedAt,
	}
	fetcher := &fakeMetadataFetcher{err: errors.New("upstream down")}
	resolver := newResolverUnderTest(noEntries(), cache, fetcher, now)
	target := Target{ItemURL: targetURL}

	first := resolver.ResolveMany(context.Background(), []Target{target})
	second := resolver.ResolveMany(context.Background(), []Target{target})

	if len(first) != 1 || first[0].Title != title || len(second) != 1 || second[0].Title != title {
		t.Fatalf("first = %+v, second = %+v", first, second)
	}
	if fetcher.callCount() != 1 {
		t.Errorf("fetch calls = %d, want 1 during backoff", fetcher.callCount())
	}
	state, err := cache.GetShareMetadataCache(context.Background(), targetURL)
	if err != nil {
		t.Fatalf("cache state: %v", err)
	}
	wantRetry := now.Add(5 * time.Minute).Format(time.RFC3339)
	if state.FailureCount != 1 || state.NextRetryAt == nil || *state.NextRetryAt != wantRetry {
		t.Errorf("failure state = %+v, want first retry at %s", state, wantRetry)
	}
}

func TestResolveMany_RetryBackoffCapsAtOneDay(t *testing.T) {
	now := time.Date(2026, 7, 19, 12, 0, 0, 0, time.UTC)
	targetURL := "https://example.com/post"
	expiredRetry := now.Add(-time.Minute).Format(time.RFC3339)
	cache := newFakeCache()
	cache.states[targetURL] = db.ShareMetadataCache{
		TargetKey: targetURL, FailureCount: 99, NextRetryAt: &expiredRetry,
	}
	resolver := newResolverUnderTest(
		noEntries(),
		cache,
		&fakeMetadataFetcher{err: errors.New("still unavailable")},
		now,
	)

	resolver.ResolveMany(context.Background(), []Target{{ItemURL: targetURL}})

	state, err := cache.GetShareMetadataCache(context.Background(), targetURL)
	if err != nil {
		t.Fatalf("cache state: %v", err)
	}
	wantRetry := now.Add(24 * time.Hour).Format(time.RFC3339)
	if state.FailureCount != 100 || state.NextRetryAt == nil || *state.NextRetryAt != wantRetry {
		t.Errorf("failure state = %+v, want capped retry at %s", state, wantRetry)
	}
}

func TestResolveMany_CanceledBatchDoesNotStartFailureBackoff(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	cache := newFakeCache()
	fetcher := &canceledMetadataFetcher{}
	resolver := newResolverUnderTest(noEntries(), cache, fetcher, time.Now())

	got := resolver.ResolveMany(ctx, []Target{{ItemURL: "https://example.com/post"}})

	if len(got) != 1 || got[0] != (Metadata{}) {
		t.Fatalf("got = %+v", got)
	}
	if fetcher.calls != 0 {
		t.Errorf("fetch calls = %d, want 0", fetcher.calls)
	}
	if _, err := cache.GetShareMetadataCache(context.Background(), "https://example.com/post"); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("cache error = %v, want no failure state", err)
	}
}

func TestResolveMany_CollapsesConcurrentFetchesForSameTarget(t *testing.T) {
	fetcher := &fakeMetadataFetcher{
		metadata: Metadata{Title: "One fetch"},
		delay:    20 * time.Millisecond,
	}
	resolver := newResolverUnderTest(noEntries(), newFakeCache(), fetcher, time.Now())
	target := Target{ItemURL: "https://example.com/post"}

	var wg sync.WaitGroup
	for range 5 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got := resolver.ResolveMany(context.Background(), []Target{target})
			if len(got) != 1 || got[0].Title != "One fetch" {
				t.Errorf("got = %+v", got)
			}
		}()
	}
	wg.Wait()

	if fetcher.callCount() != 1 {
		t.Errorf("fetch calls = %d, want 1", fetcher.callCount())
	}
}
