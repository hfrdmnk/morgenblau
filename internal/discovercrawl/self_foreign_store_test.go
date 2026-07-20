package discovercrawl

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/bluesky-social/indigo/atproto/syntax"

	"morgenblau/internal/database"
	"morgenblau/internal/database/db"
)

const ownForeignCrawlCacheSchema = `
CREATE TABLE discover_crawl_own_foreign_state (
    did        TEXT PRIMARY KEY,
    fetched_at TEXT NOT NULL
);
CREATE TABLE discover_crawl_own_foreign_subscriptions (
    did           TEXT NOT NULL,
    canonical_key TEXT NOT NULL,
    kind          TEXT NOT NULL,
    app           TEXT NOT NULL,
    title         TEXT,
    site_url      TEXT,
    created_at    TEXT,
    fetched_at    TEXT NOT NULL,
    PRIMARY KEY (did, canonical_key)
);
`

func openOwnForeignCrawlCacheTestDB(t *testing.T) *database.DB {
	t.Helper()
	t.Setenv("DB_PATH", filepath.Join(t.TempDir(), "test.db"))
	dbs, err := database.Open()
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { dbs.Close() })
	if _, err := dbs.Writer.ExecContext(context.Background(), ownForeignCrawlCacheSchema); err != nil {
		t.Fatalf("schema: %v", err)
	}
	return dbs
}

type fakeOwnForeignSubscriptionSource struct {
	calls   int
	results []ForeignSubscription
	err     error
}

func (f *fakeOwnForeignSubscriptionSource) CrawlOwnForeignSubscriptions(context.Context, syntax.DID) ([]ForeignSubscription, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	return f.results, nil
}

func newCachedOwnForeignCrawlerUnderTest(dbs *database.DB, crawler OwnForeignSubscriptionSource, ttl time.Duration, now time.Time) *CachedOwnForeignCrawler {
	cc := NewCachedOwnForeignCrawler(crawler, db.New(dbs.Reader), ttl).WithTxRunner(dbs.Writer)
	cc.now = func() time.Time { return now }
	return cc
}

func TestCachedOwnForeignCrawler_FirstCall_CrawlsAndCaches(t *testing.T) {
	dbs := openOwnForeignCrawlCacheTestDB(t)
	crawler := &fakeOwnForeignSubscriptionSource{results: []ForeignSubscription{
		{Subscription: Subscription{Key: "https://a.example/feed", Kind: "rss", Title: "A"}, App: ForeignAppSkyreader},
	}}
	cc := newCachedOwnForeignCrawlerUnderTest(dbs, crawler, time.Hour, mustParseTime(t, "2026-07-09T12:00:00Z"))

	did, _ := syntax.ParseDID(followedDID)
	got, err := cc.CrawlOwnForeignSubscriptions(context.Background(), did)
	if err != nil {
		t.Fatalf("CrawlOwnForeignSubscriptions: %v", err)
	}
	if crawler.calls != 1 {
		t.Errorf("crawler.calls = %d, want 1", crawler.calls)
	}
	if len(got) != 1 || got[0].Key != "https://a.example/feed" || got[0].App != ForeignAppSkyreader {
		t.Fatalf("got = %+v", got)
	}

	rows, err := db.New(dbs.Reader).ListDiscoverCrawlOwnForeignSubscriptions(context.Background(), followedDID)
	if err != nil {
		t.Fatalf("ListDiscoverCrawlOwnForeignSubscriptions: %v", err)
	}
	if len(rows) != 1 || rows[0].CanonicalKey != "https://a.example/feed" || rows[0].App != string(ForeignAppSkyreader) {
		t.Fatalf("cached rows = %+v", rows)
	}
}

func TestCachedOwnForeignCrawler_WithinTTL_ServesCacheWithoutCrawling(t *testing.T) {
	dbs := openOwnForeignCrawlCacheTestDB(t)
	crawler := &fakeOwnForeignSubscriptionSource{results: []ForeignSubscription{
		{Subscription: Subscription{Key: "https://a.example/feed", Kind: "rss", Title: "A"}, App: ForeignAppGlean},
	}}
	start := mustParseTime(t, "2026-07-09T12:00:00Z")
	cc := newCachedOwnForeignCrawlerUnderTest(dbs, crawler, time.Hour, start)
	did, _ := syntax.ParseDID(followedDID)

	if _, err := cc.CrawlOwnForeignSubscriptions(context.Background(), did); err != nil {
		t.Fatalf("first fetch: %v", err)
	}

	// 30 minutes later, still within the 1h TTL.
	cc.now = func() time.Time { return start.Add(30 * time.Minute) }
	got, err := cc.CrawlOwnForeignSubscriptions(context.Background(), did)
	if err != nil {
		t.Fatalf("second fetch: %v", err)
	}
	if crawler.calls != 1 {
		t.Errorf("crawler.calls = %d, want 1 (TTL hit must not re-crawl)", crawler.calls)
	}
	if len(got) != 1 || got[0].Key != "https://a.example/feed" {
		t.Fatalf("got = %+v, want cached result", got)
	}
}

func TestCachedOwnForeignCrawler_PastTTL_RecrawlsAndReplaces(t *testing.T) {
	dbs := openOwnForeignCrawlCacheTestDB(t)
	crawler := &fakeOwnForeignSubscriptionSource{results: []ForeignSubscription{
		{Subscription: Subscription{Key: "https://old.example/feed", Kind: "rss"}, App: ForeignAppSkyreader},
	}}
	start := mustParseTime(t, "2026-07-09T12:00:00Z")
	cc := newCachedOwnForeignCrawlerUnderTest(dbs, crawler, time.Hour, start)
	did, _ := syntax.ParseDID(followedDID)

	if _, err := cc.CrawlOwnForeignSubscriptions(context.Background(), did); err != nil {
		t.Fatalf("first fetch: %v", err)
	}

	// Past the TTL, and the viewer's own foreign subscriptions changed.
	crawler.results = []ForeignSubscription{
		{Subscription: Subscription{Key: "https://new.example/feed", Kind: "rss"}, App: ForeignAppGlean},
	}
	cc.now = func() time.Time { return start.Add(2 * time.Hour) }
	got, err := cc.CrawlOwnForeignSubscriptions(context.Background(), did)
	if err != nil {
		t.Fatalf("second fetch: %v", err)
	}
	if crawler.calls != 2 {
		t.Errorf("crawler.calls = %d, want 2 (stale cache must re-crawl)", crawler.calls)
	}
	if len(got) != 1 || got[0].Key != "https://new.example/feed" || got[0].App != ForeignAppGlean {
		t.Fatalf("got = %+v, want only the new result", got)
	}

	rows, err := db.New(dbs.Reader).ListDiscoverCrawlOwnForeignSubscriptions(context.Background(), followedDID)
	if err != nil {
		t.Fatalf("ListDiscoverCrawlOwnForeignSubscriptions: %v", err)
	}
	if len(rows) != 1 || rows[0].CanonicalKey != "https://new.example/feed" {
		t.Fatalf("cached rows not replaced: %+v", rows)
	}
}

func TestCachedOwnForeignCrawler_CrawlErrorPropagatesAndWritesNothing(t *testing.T) {
	dbs := openOwnForeignCrawlCacheTestDB(t)
	crawler := &fakeOwnForeignSubscriptionSource{err: errors.New("pds unreachable")}
	cc := newCachedOwnForeignCrawlerUnderTest(dbs, crawler, time.Hour, mustParseTime(t, "2026-07-09T12:00:00Z"))
	did, _ := syntax.ParseDID(followedDID)

	if _, err := cc.CrawlOwnForeignSubscriptions(context.Background(), did); err == nil {
		t.Fatal("expected error to propagate")
	}

	rows, err := db.New(dbs.Reader).ListDiscoverCrawlOwnForeignSubscriptions(context.Background(), followedDID)
	if err != nil {
		t.Fatalf("ListDiscoverCrawlOwnForeignSubscriptions: %v", err)
	}
	if len(rows) != 0 {
		t.Errorf("rows = %+v, want none written on crawl failure", rows)
	}
}
