package discovercrawl

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/bluesky-social/indigo/atproto/syntax"

	"morgenblau/internal/database"
	"morgenblau/internal/database/db"
)

const crawlCacheSchema = `
CREATE TABLE discover_crawl_state (
    followed_did TEXT PRIMARY KEY,
    fetched_at   TEXT NOT NULL
);
CREATE TABLE discover_crawl_subscriptions (
    followed_did  TEXT NOT NULL,
    canonical_key TEXT NOT NULL,
    kind          TEXT NOT NULL,
    title         TEXT,
    site_url      TEXT,
    fetched_at    TEXT NOT NULL,
    created_at    TEXT,
    PRIMARY KEY (followed_did, canonical_key)
);
`

func openCrawlCacheTestDB(t *testing.T) *database.DB {
	t.Helper()
	t.Setenv("DB_PATH", filepath.Join(t.TempDir(), "test.db"))
	dbs, err := database.Open()
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { dbs.Close() })
	if _, err := dbs.Writer.ExecContext(context.Background(), crawlCacheSchema); err != nil {
		t.Fatalf("schema: %v", err)
	}
	return dbs
}

type fakeCrawler struct {
	// mu guards the counters: the batch path crawls several DIDs concurrently.
	mu       sync.Mutex
	calls    int
	crawled  []string
	results  []Subscription
	byDID    map[string][]Subscription
	err      error
	errByDID map[string]error
}

func (f *fakeCrawler) Crawl(_ context.Context, did syntax.DID) ([]Subscription, error) {
	f.mu.Lock()
	f.calls++
	f.crawled = append(f.crawled, did.String())
	f.mu.Unlock()
	if err, ok := f.errByDID[did.String()]; ok {
		return nil, err
	}
	if f.err != nil {
		return nil, f.err
	}
	if subs, ok := f.byDID[did.String()]; ok {
		return subs, nil
	}
	return f.results, nil
}

func (f *fakeCrawler) crawledDIDs() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := append([]string(nil), f.crawled...)
	sort.Strings(out)
	return out
}

// countingCacheReader proves the batch path issues one IN-read per table rather than a query per DID.
type countingCacheReader struct {
	CacheReader
	stateReads int
	rowReads   int
}

func (c *countingCacheReader) ListDiscoverCrawlStatesByDids(ctx context.Context, dids []string) ([]db.DiscoverCrawlState, error) {
	c.stateReads++
	return c.CacheReader.ListDiscoverCrawlStatesByDids(ctx, dids)
}

func (c *countingCacheReader) ListDiscoverCrawlSubscriptionsByDids(ctx context.Context, dids []string) ([]db.DiscoverCrawlSubscription, error) {
	c.rowReads++
	return c.CacheReader.ListDiscoverCrawlSubscriptionsByDids(ctx, dids)
}

func seedSubscriptionCache(t *testing.T, dbs *database.DB, did string, fetchedAt time.Time, keys ...string) {
	t.Helper()
	q := db.New(dbs.Writer)
	stamp := fetchedAt.UTC().Format(time.RFC3339)
	for _, key := range keys {
		if err := q.InsertDiscoverCrawlSubscription(context.Background(), db.InsertDiscoverCrawlSubscriptionParams{
			FollowedDid:  did,
			CanonicalKey: key,
			Kind:         "rss",
			FetchedAt:    stamp,
		}); err != nil {
			t.Fatalf("seed subscription: %v", err)
		}
	}
	if err := q.UpsertDiscoverCrawlState(context.Background(), db.UpsertDiscoverCrawlStateParams{FollowedDid: did, FetchedAt: stamp}); err != nil {
		t.Fatalf("seed state: %v", err)
	}
}

func subscriptionKeys(subs []Subscription) []string {
	out := make([]string, 0, len(subs))
	for _, s := range subs {
		out = append(out, s.Key)
	}
	return out
}

func newCachedCrawlerUnderTest(dbs *database.DB, crawler Crawler, ttl time.Duration, now time.Time) *CachedCrawler {
	cc := NewCachedCrawler(crawler, db.New(dbs.Reader), ttl).WithTxRunner(dbs.Writer)
	cc.now = func() time.Time { return now }
	return cc
}

// TestCachedCrawler_WiresReaderWriterAndTTL is the thin instantiation check: the cache-logic
// itself (TTL math, degrade posture) lives in cachedFetch and is covered by its own tests.
func TestCachedCrawler_WiresReaderWriterAndTTL(t *testing.T) {
	dbs := openCrawlCacheTestDB(t)
	crawler := &fakeCrawler{results: []Subscription{{Key: "https://a.example/feed", Kind: "rss", Title: "A"}}}
	start := mustParseTime(t, "2026-07-09T12:00:00Z")
	cc := newCachedCrawlerUnderTest(dbs, crawler, time.Hour, start)
	did, _ := syntax.ParseDID(followedDID)

	got, err := cc.FetchSubscriptions(context.Background(), did)
	if err != nil {
		t.Fatalf("FetchSubscriptions: %v", err)
	}
	if crawler.calls != 1 {
		t.Errorf("crawler.calls = %d, want 1", crawler.calls)
	}
	if len(got) != 1 || got[0].Key != "https://a.example/feed" {
		t.Fatalf("got = %+v", got)
	}

	// Cache actually persisted through the real writer, not just returned in-memory.
	rows, err := db.New(dbs.Reader).ListDiscoverCrawlSubscriptions(context.Background(), followedDID)
	if err != nil {
		t.Fatalf("ListDiscoverCrawlSubscriptions: %v", err)
	}
	if len(rows) != 1 || rows[0].CanonicalKey != "https://a.example/feed" {
		t.Fatalf("cached rows = %+v", rows)
	}

	// The ttl passed to NewCachedCrawler is what gets honored, not some hardcoded value.
	cc.now = func() time.Time { return start.Add(30 * time.Minute) }
	if _, err := cc.FetchSubscriptions(context.Background(), did); err != nil {
		t.Fatalf("second fetch: %v", err)
	}
	if crawler.calls != 1 {
		t.Errorf("crawler.calls = %d, want 1 (TTL hit must not re-crawl)", crawler.calls)
	}
}

func TestCachedCrawler_FetchSubscriptionsBatch(t *testing.T) {
	const (
		didFresh = "did:plc:fresh"
		didStale = "did:plc:stale"
		didCold  = "did:plc:cold"
	)
	now := mustParseTime(t, "2026-07-09T12:00:00Z")

	tests := []struct {
		name           string
		dids           []string
		wantCrawled    []string
		wantKeys       map[string][]string
		wantStateReads int
		wantRowReads   int
	}{
		{
			name:           "all fresh serves the cache without crawling",
			dids:           []string{didFresh},
			wantCrawled:    nil,
			wantKeys:       map[string][]string{didFresh: {"https://fresh.example/feed"}},
			wantStateReads: 1,
			wantRowReads:   1,
		},
		{
			name:           "stale and never-crawled both crawl, no row read needed",
			dids:           []string{didStale, didCold},
			wantCrawled:    []string{didCold, didStale},
			wantKeys:       map[string][]string{didStale: {"https://stale-new.example/feed"}, didCold: {"https://cold.example/feed"}},
			wantStateReads: 1,
			wantRowReads:   0,
		},
		{
			name:           "mixed crawls only the stale half",
			dids:           []string{didFresh, didStale, didCold},
			wantCrawled:    []string{didCold, didStale},
			wantKeys:       map[string][]string{didFresh: {"https://fresh.example/feed"}, didStale: {"https://stale-new.example/feed"}, didCold: {"https://cold.example/feed"}},
			wantStateReads: 1,
			wantRowReads:   1,
		},
		{
			name:           "duplicate dids collapse to one crawl",
			dids:           []string{didCold, didCold},
			wantCrawled:    []string{didCold},
			wantKeys:       map[string][]string{didCold: {"https://cold.example/feed"}},
			wantStateReads: 1,
			wantRowReads:   0,
		},
		{
			name:           "empty dids reads nothing",
			dids:           nil,
			wantCrawled:    nil,
			wantKeys:       map[string][]string{},
			wantStateReads: 0,
			wantRowReads:   0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dbs := openCrawlCacheTestDB(t)
			seedSubscriptionCache(t, dbs, didFresh, now.Add(-30*time.Minute), "https://fresh.example/feed")
			seedSubscriptionCache(t, dbs, didStale, now.Add(-48*time.Hour), "https://stale-old.example/feed")

			crawler := &fakeCrawler{byDID: map[string][]Subscription{
				didStale: {{Key: "https://stale-new.example/feed", Kind: "rss"}},
				didCold:  {{Key: "https://cold.example/feed", Kind: "rss"}},
			}}
			reader := &countingCacheReader{CacheReader: db.New(dbs.Reader)}
			cc := NewCachedCrawler(crawler, reader, time.Hour).WithTxRunner(dbs.Writer)
			cc.now = func() time.Time { return now }

			got := cc.FetchSubscriptionsBatch(context.Background(), tc.dids)

			gotKeys := map[string][]string{}
			for did, subs := range got {
				gotKeys[did] = subscriptionKeys(subs)
			}
			if !reflect.DeepEqual(gotKeys, tc.wantKeys) {
				t.Errorf("keys = %v, want %v", gotKeys, tc.wantKeys)
			}
			if crawled := crawler.crawledDIDs(); !reflect.DeepEqual(crawled, tc.wantCrawled) {
				t.Errorf("crawled = %v, want %v", crawled, tc.wantCrawled)
			}
			if reader.stateReads != tc.wantStateReads {
				t.Errorf("state reads = %d, want %d (one IN-read for the whole fan-out)", reader.stateReads, tc.wantStateReads)
			}
			if reader.rowReads != tc.wantRowReads {
				t.Errorf("row reads = %d, want %d", reader.rowReads, tc.wantRowReads)
			}
		})
	}
}

func TestCachedCrawler_FetchSubscriptionsBatch_OneFailedCrawlDegradesOnlyThatDID(t *testing.T) {
	dbs := openCrawlCacheTestDB(t)
	crawler := &fakeCrawler{
		byDID:    map[string][]Subscription{"did:plc:ok": {{Key: "https://ok.example/feed", Kind: "rss"}}},
		errByDID: map[string]error{"did:plc:broken": errors.New("pds unreachable")},
	}
	cc := newCachedCrawlerUnderTest(dbs, crawler, time.Hour, mustParseTime(t, "2026-07-09T12:00:00Z"))

	got := cc.FetchSubscriptionsBatch(context.Background(), []string{"did:plc:broken", "did:plc:ok"})

	if subs, ok := got["did:plc:broken"]; !ok || subs != nil {
		t.Errorf("broken did = %+v (present=%v), want a nil entry", subs, ok)
	}
	if keys := subscriptionKeys(got["did:plc:ok"]); !reflect.DeepEqual(keys, []string{"https://ok.example/feed"}) {
		t.Errorf("ok did = %v, want its crawl result", keys)
	}
}

func TestCachedCrawler_FetchSubscriptionsBatch_StaleCrawlRefreshesCache(t *testing.T) {
	dbs := openCrawlCacheTestDB(t)
	now := mustParseTime(t, "2026-07-09T12:00:00Z")
	seedSubscriptionCache(t, dbs, followedDID, now.Add(-48*time.Hour), "https://old.example/feed")

	crawler := &fakeCrawler{byDID: map[string][]Subscription{
		followedDID: {{Key: "https://new.example/feed", Kind: "rss"}},
	}}
	cc := newCachedCrawlerUnderTest(dbs, crawler, time.Hour, now)

	if got := cc.FetchSubscriptionsBatch(context.Background(), []string{followedDID}); len(got[followedDID]) != 1 {
		t.Fatalf("got = %+v, want the fresh crawl result", got)
	}

	rows, err := db.New(dbs.Reader).ListDiscoverCrawlSubscriptions(context.Background(), followedDID)
	if err != nil {
		t.Fatalf("ListDiscoverCrawlSubscriptions: %v", err)
	}
	if len(rows) != 1 || rows[0].CanonicalKey != "https://new.example/feed" {
		t.Fatalf("cached rows = %+v, want the batch crawl written through", rows)
	}
}

func TestCachedCrawler_FetchSubscriptionsBatch_BoundsConcurrentCrawls(t *testing.T) {
	dbs := openCrawlCacheTestDB(t)
	crawler := newBlockingCrawler(BatchCrawlFanout)
	cc := newCachedCrawlerUnderTest(dbs, crawler, time.Hour, mustParseTime(t, "2026-07-09T12:00:00Z"))

	dids := make([]string, 0, BatchCrawlFanout*2)
	for i := range BatchCrawlFanout * 2 {
		dids = append(dids, "did:plc:candidate"+string(rune('a'+i)))
	}

	done := make(chan map[string][]Subscription, 1)
	go func() { done <- cc.FetchSubscriptionsBatch(context.Background(), dids) }()

	select {
	case <-crawler.reached:
	case <-time.After(5 * time.Second):
		t.Fatal("never reached the fan-out bound in flight")
	}
	// Every crawl is still blocked, so an unbounded fan-out would pile the remaining dids on within this grace window; a bounded one stays pinned at the limit.
	time.Sleep(100 * time.Millisecond)
	max := crawler.maxInFlight()
	close(crawler.release)

	select {
	case got := <-done:
		if len(got) != len(dids) {
			t.Errorf("got %d entries, want %d", len(got), len(dids))
		}
	case <-time.After(5 * time.Second):
		t.Fatal("batch never returned")
	}
	if max != BatchCrawlFanout {
		t.Errorf("in-flight crawls while blocked = %d, want exactly %d", max, BatchCrawlFanout)
	}
}

// blockingCrawler holds every crawl until release closes, so the test can pin the peak in-flight count the fan-out bound admits.
type blockingCrawler struct {
	mu      sync.Mutex
	current int
	max     int
	target  int
	once    sync.Once
	reached chan struct{}
	release chan struct{}
}

func newBlockingCrawler(target int) *blockingCrawler {
	return &blockingCrawler{target: target, reached: make(chan struct{}), release: make(chan struct{})}
}

func (b *blockingCrawler) Crawl(ctx context.Context, _ syntax.DID) ([]Subscription, error) {
	b.mu.Lock()
	b.current++
	if b.current > b.max {
		b.max = b.current
	}
	atTarget := b.current >= b.target
	b.mu.Unlock()
	if atTarget {
		b.once.Do(func() { close(b.reached) })
	}
	defer func() {
		b.mu.Lock()
		b.current--
		b.mu.Unlock()
	}()
	select {
	case <-b.release:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	return nil, nil
}

func (b *blockingCrawler) maxInFlight() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.max
}

func mustParseTime(t *testing.T, s string) time.Time {
	t.Helper()
	tm, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t.Fatalf("parse %q: %v", s, err)
	}
	return tm
}
