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

const shareCrawlCacheSchema = `
CREATE TABLE discover_crawl_share_state (
    followed_did TEXT PRIMARY KEY,
    fetched_at   TEXT NOT NULL
);
CREATE TABLE discover_crawl_shares (
    followed_did TEXT NOT NULL,
    dedupe_key   TEXT NOT NULL,
    kind         TEXT NOT NULL,
    item_url     TEXT,
    document     TEXT,
    feed_url     TEXT,
    comment      TEXT,
    created_at   TEXT NOT NULL,
    fetched_at   TEXT NOT NULL,
    PRIMARY KEY (followed_did, dedupe_key)
);
`

func openShareCrawlCacheTestDB(t *testing.T) *database.DB {
	t.Helper()
	t.Setenv("DB_PATH", filepath.Join(t.TempDir(), "test.db"))
	dbs, err := database.Open()
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { dbs.Close() })
	if _, err := dbs.Writer.ExecContext(context.Background(), shareCrawlCacheSchema); err != nil {
		t.Fatalf("schema: %v", err)
	}
	return dbs
}

type fakeShareCrawler struct {
	calls   int
	results []Share
	err     error
}

func (f *fakeShareCrawler) CrawlShares(context.Context, syntax.DID) ([]Share, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	return f.results, nil
}

func newCachedShareCrawlerUnderTest(dbs *database.DB, crawler ShareCrawler, ttl time.Duration, now time.Time) *CachedShareCrawler {
	cc := NewCachedShareCrawler(crawler, db.New(dbs.Reader), ttl).WithTxRunner(dbs.Writer)
	cc.now = func() time.Time { return now }
	return cc
}

func TestCachedShareCrawler_FirstCall_CrawlsAndCaches(t *testing.T) {
	dbs := openShareCrawlCacheTestDB(t)
	crawler := &fakeShareCrawler{results: []Share{{Kind: "rss", ItemURL: "https://a.example/post", CreatedAt: "2026-07-01T00:00:00Z"}}}
	cc := newCachedShareCrawlerUnderTest(dbs, crawler, time.Hour, mustParseTime(t, "2026-07-09T12:00:00Z"))

	did, _ := syntax.ParseDID(followedDID)
	got, err := cc.FetchShares(context.Background(), did)
	if err != nil {
		t.Fatalf("FetchShares: %v", err)
	}
	if crawler.calls != 1 {
		t.Errorf("crawler.calls = %d, want 1", crawler.calls)
	}
	if len(got) != 1 || got[0].ItemURL != "https://a.example/post" {
		t.Fatalf("got = %+v", got)
	}

	rows, err := db.New(dbs.Reader).ListDiscoverCrawlShares(context.Background(), followedDID)
	if err != nil {
		t.Fatalf("ListDiscoverCrawlShares: %v", err)
	}
	if len(rows) != 1 || rows[0].DedupeKey != "https://a.example/post" {
		t.Fatalf("cached rows = %+v", rows)
	}
}

func TestCachedShareCrawler_WithinTTL_ServesCacheWithoutCrawling(t *testing.T) {
	dbs := openShareCrawlCacheTestDB(t)
	crawler := &fakeShareCrawler{results: []Share{{Kind: "standardfeed", Document: "at://did:plc:pub/site.standard.document/1", CreatedAt: "2026-07-01T00:00:00Z"}}}
	start := mustParseTime(t, "2026-07-09T12:00:00Z")
	cc := newCachedShareCrawlerUnderTest(dbs, crawler, time.Hour, start)
	did, _ := syntax.ParseDID(followedDID)

	if _, err := cc.FetchShares(context.Background(), did); err != nil {
		t.Fatalf("first fetch: %v", err)
	}

	cc.now = func() time.Time { return start.Add(30 * time.Minute) }
	got, err := cc.FetchShares(context.Background(), did)
	if err != nil {
		t.Fatalf("second fetch: %v", err)
	}
	if crawler.calls != 1 {
		t.Errorf("crawler.calls = %d, want 1 (TTL hit must not re-crawl)", crawler.calls)
	}
	if len(got) != 1 || got[0].Kind != "standardfeed" {
		t.Fatalf("got = %+v, want cached result", got)
	}
}

func TestCachedShareCrawler_PastTTL_RecrawlsAndReplaces(t *testing.T) {
	dbs := openShareCrawlCacheTestDB(t)
	crawler := &fakeShareCrawler{results: []Share{{Kind: "rss", ItemURL: "https://old.example/post", CreatedAt: "2026-07-01T00:00:00Z"}}}
	start := mustParseTime(t, "2026-07-09T12:00:00Z")
	cc := newCachedShareCrawlerUnderTest(dbs, crawler, time.Hour, start)
	did, _ := syntax.ParseDID(followedDID)

	if _, err := cc.FetchShares(context.Background(), did); err != nil {
		t.Fatalf("first fetch: %v", err)
	}

	crawler.results = []Share{{Kind: "rss", ItemURL: "https://new.example/post", CreatedAt: "2026-07-09T00:00:00Z"}}
	cc.now = func() time.Time { return start.Add(2 * time.Hour) }
	got, err := cc.FetchShares(context.Background(), did)
	if err != nil {
		t.Fatalf("second fetch: %v", err)
	}
	if crawler.calls != 2 {
		t.Errorf("crawler.calls = %d, want 2 (stale cache must re-crawl)", crawler.calls)
	}
	if len(got) != 1 || got[0].ItemURL != "https://new.example/post" {
		t.Fatalf("got = %+v, want only the new result", got)
	}

	rows, err := db.New(dbs.Reader).ListDiscoverCrawlShares(context.Background(), followedDID)
	if err != nil {
		t.Fatalf("ListDiscoverCrawlShares: %v", err)
	}
	if len(rows) != 1 || rows[0].DedupeKey != "https://new.example/post" {
		t.Fatalf("cached rows not replaced: %+v", rows)
	}
}

func TestCachedShareCrawler_CrawlErrorPropagatesAndWritesNothing(t *testing.T) {
	dbs := openShareCrawlCacheTestDB(t)
	crawler := &fakeShareCrawler{err: errors.New("pds unreachable")}
	cc := newCachedShareCrawlerUnderTest(dbs, crawler, time.Hour, mustParseTime(t, "2026-07-09T12:00:00Z"))
	did, _ := syntax.ParseDID(followedDID)

	if _, err := cc.FetchShares(context.Background(), did); err == nil {
		t.Fatal("expected error to propagate")
	}

	rows, err := db.New(dbs.Reader).ListDiscoverCrawlShares(context.Background(), followedDID)
	if err != nil {
		t.Fatalf("ListDiscoverCrawlShares: %v", err)
	}
	if len(rows) != 0 {
		t.Errorf("rows = %+v, want none written on crawl failure", rows)
	}
}

func TestCachedShareCrawler_StandardfeedDedupeKeyIsDocument(t *testing.T) {
	dbs := openShareCrawlCacheTestDB(t)
	document := "at://did:plc:pub/site.standard.document/1"
	crawler := &fakeShareCrawler{results: []Share{{Kind: "standardfeed", Document: document, ItemURL: "https://pub.example/a", Comment: "nice", CreatedAt: "2026-07-01T00:00:00Z"}}}
	cc := newCachedShareCrawlerUnderTest(dbs, crawler, time.Hour, mustParseTime(t, "2026-07-09T12:00:00Z"))
	did, _ := syntax.ParseDID(followedDID)

	if _, err := cc.FetchShares(context.Background(), did); err != nil {
		t.Fatalf("FetchShares: %v", err)
	}

	rows, err := db.New(dbs.Reader).ListDiscoverCrawlShares(context.Background(), followedDID)
	if err != nil {
		t.Fatalf("ListDiscoverCrawlShares: %v", err)
	}
	if len(rows) != 1 || rows[0].DedupeKey != document {
		t.Fatalf("rows = %+v, want dedupe_key = document for standardfeed shares", rows)
	}
	if rows[0].Comment == nil || *rows[0].Comment != "nice" {
		t.Errorf("comment = %v, want sidecar comment preserved through cache", rows[0].Comment)
	}
}
