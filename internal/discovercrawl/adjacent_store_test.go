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

const adjacentFollowCrawlCacheSchema = `
CREATE TABLE discover_crawl_adjacent_state (
    did        TEXT PRIMARY KEY,
    fetched_at TEXT NOT NULL
);
CREATE TABLE discover_crawl_adjacent_follows (
    did         TEXT NOT NULL,
    subject_did TEXT NOT NULL,
    network     TEXT NOT NULL,
    fetched_at  TEXT NOT NULL,
    PRIMARY KEY (did, subject_did)
);
`

func openAdjacentFollowCrawlCacheTestDB(t *testing.T) *database.DB {
	t.Helper()
	t.Setenv("DB_PATH", filepath.Join(t.TempDir(), "test.db"))
	dbs, err := database.Open()
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { dbs.Close() })
	if _, err := dbs.Writer.ExecContext(context.Background(), adjacentFollowCrawlCacheSchema); err != nil {
		t.Fatalf("schema: %v", err)
	}
	return dbs
}

type fakeAdjacentFollowSource struct {
	calls   int
	results []AdjacentFollow
	err     error
}

func (f *fakeAdjacentFollowSource) CrawlAdjacentFollows(context.Context, syntax.DID) ([]AdjacentFollow, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	return f.results, nil
}

func newCachedAdjacentFollowCrawlerUnderTest(dbs *database.DB, crawler AdjacentFollowSource, ttl time.Duration, now time.Time) *CachedAdjacentFollowCrawler {
	cc := NewCachedAdjacentFollowCrawler(crawler, db.New(dbs.Reader), ttl).WithTxRunner(dbs.Writer)
	cc.now = func() time.Time { return now }
	return cc
}

func TestCachedAdjacentFollowCrawler_FirstCall_CrawlsAndCaches(t *testing.T) {
	dbs := openAdjacentFollowCrawlCacheTestDB(t)
	crawler := &fakeAdjacentFollowSource{results: []AdjacentFollow{{DID: "did:plc:alice", Network: "bluesky"}}}
	cc := newCachedAdjacentFollowCrawlerUnderTest(dbs, crawler, time.Hour, mustParseTime(t, "2026-07-09T12:00:00Z"))

	did, _ := syntax.ParseDID(followedDID)
	got, err := cc.CrawlAdjacentFollows(context.Background(), did)
	if err != nil {
		t.Fatalf("CrawlAdjacentFollows: %v", err)
	}
	if crawler.calls != 1 {
		t.Errorf("crawler.calls = %d, want 1", crawler.calls)
	}
	if len(got) != 1 || got[0].DID != "did:plc:alice" || got[0].Network != "bluesky" {
		t.Fatalf("got = %+v", got)
	}

	rows, err := db.New(dbs.Reader).ListDiscoverCrawlAdjacentFollows(context.Background(), followedDID)
	if err != nil {
		t.Fatalf("ListDiscoverCrawlAdjacentFollows: %v", err)
	}
	if len(rows) != 1 || rows[0].SubjectDid != "did:plc:alice" || rows[0].Network != "bluesky" {
		t.Fatalf("cached rows = %+v", rows)
	}
}

func TestCachedAdjacentFollowCrawler_WithinTTL_ServesCacheWithoutCrawling(t *testing.T) {
	dbs := openAdjacentFollowCrawlCacheTestDB(t)
	crawler := &fakeAdjacentFollowSource{results: []AdjacentFollow{{DID: "did:plc:alice", Network: "bluesky"}}}
	start := mustParseTime(t, "2026-07-09T12:00:00Z")
	cc := newCachedAdjacentFollowCrawlerUnderTest(dbs, crawler, time.Hour, start)
	did, _ := syntax.ParseDID(followedDID)

	if _, err := cc.CrawlAdjacentFollows(context.Background(), did); err != nil {
		t.Fatalf("first fetch: %v", err)
	}

	// 30 minutes later, still within the 1h TTL.
	cc.now = func() time.Time { return start.Add(30 * time.Minute) }
	got, err := cc.CrawlAdjacentFollows(context.Background(), did)
	if err != nil {
		t.Fatalf("second fetch: %v", err)
	}
	if crawler.calls != 1 {
		t.Errorf("crawler.calls = %d, want 1 (TTL hit must not re-crawl)", crawler.calls)
	}
	if len(got) != 1 || got[0].DID != "did:plc:alice" {
		t.Fatalf("got = %+v, want cached result", got)
	}
}

func TestCachedAdjacentFollowCrawler_PastTTL_RecrawlsAndReplaces(t *testing.T) {
	dbs := openAdjacentFollowCrawlCacheTestDB(t)
	crawler := &fakeAdjacentFollowSource{results: []AdjacentFollow{{DID: "did:plc:old", Network: "bluesky"}}}
	start := mustParseTime(t, "2026-07-09T12:00:00Z")
	cc := newCachedAdjacentFollowCrawlerUnderTest(dbs, crawler, time.Hour, start)
	did, _ := syntax.ParseDID(followedDID)

	if _, err := cc.CrawlAdjacentFollows(context.Background(), did); err != nil {
		t.Fatalf("first fetch: %v", err)
	}

	// Past the TTL, and the viewer's own adjacent follows changed.
	crawler.results = []AdjacentFollow{{DID: "did:plc:new", Network: "tangled"}}
	cc.now = func() time.Time { return start.Add(2 * time.Hour) }
	got, err := cc.CrawlAdjacentFollows(context.Background(), did)
	if err != nil {
		t.Fatalf("second fetch: %v", err)
	}
	if crawler.calls != 2 {
		t.Errorf("crawler.calls = %d, want 2 (stale cache must re-crawl)", crawler.calls)
	}
	if len(got) != 1 || got[0].DID != "did:plc:new" || got[0].Network != "tangled" {
		t.Fatalf("got = %+v, want only the new result", got)
	}

	rows, err := db.New(dbs.Reader).ListDiscoverCrawlAdjacentFollows(context.Background(), followedDID)
	if err != nil {
		t.Fatalf("ListDiscoverCrawlAdjacentFollows: %v", err)
	}
	if len(rows) != 1 || rows[0].SubjectDid != "did:plc:new" {
		t.Fatalf("cached rows not replaced: %+v", rows)
	}
}

func TestCachedAdjacentFollowCrawler_CrawlErrorPropagatesAndWritesNothing(t *testing.T) {
	dbs := openAdjacentFollowCrawlCacheTestDB(t)
	crawler := &fakeAdjacentFollowSource{err: errors.New("pds unreachable")}
	cc := newCachedAdjacentFollowCrawlerUnderTest(dbs, crawler, time.Hour, mustParseTime(t, "2026-07-09T12:00:00Z"))
	did, _ := syntax.ParseDID(followedDID)

	if _, err := cc.CrawlAdjacentFollows(context.Background(), did); err == nil {
		t.Fatal("expected error to propagate")
	}

	rows, err := db.New(dbs.Reader).ListDiscoverCrawlAdjacentFollows(context.Background(), followedDID)
	if err != nil {
		t.Fatalf("ListDiscoverCrawlAdjacentFollows: %v", err)
	}
	if len(rows) != 0 {
		t.Errorf("rows = %+v, want none written on crawl failure", rows)
	}
}
