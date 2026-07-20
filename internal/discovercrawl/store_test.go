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
	calls   int
	results []Subscription
	err     error
}

func (f *fakeCrawler) Crawl(context.Context, syntax.DID) ([]Subscription, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	return f.results, nil
}

func newCachedCrawlerUnderTest(dbs *database.DB, crawler Crawler, ttl time.Duration, now time.Time) *CachedCrawler {
	cc := NewCachedCrawler(crawler, db.New(dbs.Reader), ttl).WithTxRunner(dbs.Writer)
	cc.now = func() time.Time { return now }
	return cc
}

func TestCachedCrawler_FirstCall_CrawlsAndCaches(t *testing.T) {
	dbs := openCrawlCacheTestDB(t)
	crawler := &fakeCrawler{results: []Subscription{{Key: "https://a.example/feed", Kind: "rss", Title: "A"}}}
	cc := newCachedCrawlerUnderTest(dbs, crawler, time.Hour, mustParseTime(t, "2026-07-09T12:00:00Z"))

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

	// Cache actually persisted, not just returned in-memory.
	rows, err := db.New(dbs.Reader).ListDiscoverCrawlSubscriptions(context.Background(), followedDID)
	if err != nil {
		t.Fatalf("ListDiscoverCrawlSubscriptions: %v", err)
	}
	if len(rows) != 1 || rows[0].CanonicalKey != "https://a.example/feed" {
		t.Fatalf("cached rows = %+v", rows)
	}
}

func TestCachedCrawler_WithinTTL_ServesCacheWithoutCrawling(t *testing.T) {
	dbs := openCrawlCacheTestDB(t)
	crawler := &fakeCrawler{results: []Subscription{{Key: "https://a.example/feed", Kind: "rss", Title: "A"}}}
	start := mustParseTime(t, "2026-07-09T12:00:00Z")
	cc := newCachedCrawlerUnderTest(dbs, crawler, time.Hour, start)
	did, _ := syntax.ParseDID(followedDID)

	if _, err := cc.FetchSubscriptions(context.Background(), did); err != nil {
		t.Fatalf("first fetch: %v", err)
	}

	// 30 minutes later, still within the 1h TTL.
	cc.now = func() time.Time { return start.Add(30 * time.Minute) }
	got, err := cc.FetchSubscriptions(context.Background(), did)
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

func TestCachedCrawler_PastTTL_RecrawlsAndReplaces(t *testing.T) {
	dbs := openCrawlCacheTestDB(t)
	crawler := &fakeCrawler{results: []Subscription{{Key: "https://old.example/feed", Kind: "rss"}}}
	start := mustParseTime(t, "2026-07-09T12:00:00Z")
	cc := newCachedCrawlerUnderTest(dbs, crawler, time.Hour, start)
	did, _ := syntax.ParseDID(followedDID)

	if _, err := cc.FetchSubscriptions(context.Background(), did); err != nil {
		t.Fatalf("first fetch: %v", err)
	}

	// Past the TTL, and the followed person's subscriptions changed.
	crawler.results = []Subscription{{Key: "https://new.example/feed", Kind: "rss"}}
	cc.now = func() time.Time { return start.Add(2 * time.Hour) }
	got, err := cc.FetchSubscriptions(context.Background(), did)
	if err != nil {
		t.Fatalf("second fetch: %v", err)
	}
	if crawler.calls != 2 {
		t.Errorf("crawler.calls = %d, want 2 (stale cache must re-crawl)", crawler.calls)
	}
	if len(got) != 1 || got[0].Key != "https://new.example/feed" {
		t.Fatalf("got = %+v, want only the new result", got)
	}

	rows, err := db.New(dbs.Reader).ListDiscoverCrawlSubscriptions(context.Background(), followedDID)
	if err != nil {
		t.Fatalf("ListDiscoverCrawlSubscriptions: %v", err)
	}
	if len(rows) != 1 || rows[0].CanonicalKey != "https://new.example/feed" {
		t.Fatalf("cached rows not replaced: %+v", rows)
	}
}

func TestCachedCrawler_CrawlErrorPropagatesAndWritesNothing(t *testing.T) {
	dbs := openCrawlCacheTestDB(t)
	crawler := &fakeCrawler{err: errors.New("pds unreachable")}
	cc := newCachedCrawlerUnderTest(dbs, crawler, time.Hour, mustParseTime(t, "2026-07-09T12:00:00Z"))
	did, _ := syntax.ParseDID(followedDID)

	if _, err := cc.FetchSubscriptions(context.Background(), did); err == nil {
		t.Fatal("expected error to propagate")
	}

	rows, err := db.New(dbs.Reader).ListDiscoverCrawlSubscriptions(context.Background(), followedDID)
	if err != nil {
		t.Fatalf("ListDiscoverCrawlSubscriptions: %v", err)
	}
	if len(rows) != 0 {
		t.Errorf("rows = %+v, want none written on crawl failure", rows)
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
