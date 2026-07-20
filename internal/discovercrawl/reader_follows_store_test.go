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

const readerFollowCrawlCacheSchema = `
CREATE TABLE discover_crawl_follow_state (
    followed_did TEXT PRIMARY KEY,
    fetched_at   TEXT NOT NULL
);
CREATE TABLE discover_crawl_follows (
    followed_did TEXT NOT NULL,
    subject_did  TEXT NOT NULL,
    fetched_at   TEXT NOT NULL,
    PRIMARY KEY (followed_did, subject_did)
);
`

func openReaderFollowCrawlCacheTestDB(t *testing.T) *database.DB {
	t.Helper()
	t.Setenv("DB_PATH", filepath.Join(t.TempDir(), "test.db"))
	dbs, err := database.Open()
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { dbs.Close() })
	if _, err := dbs.Writer.ExecContext(context.Background(), readerFollowCrawlCacheSchema); err != nil {
		t.Fatalf("schema: %v", err)
	}
	return dbs
}

type fakeReaderFollowCrawler struct {
	calls   int
	results []ReaderNetworkFollow
	err     error
}

func (f *fakeReaderFollowCrawler) CrawlReaderNetworkFollows(context.Context, syntax.DID) ([]ReaderNetworkFollow, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	return f.results, nil
}

func newCachedReaderFollowCrawlerUnderTest(dbs *database.DB, crawler ReaderFollowCrawler, ttl time.Duration, now time.Time) *CachedReaderFollowCrawler {
	cc := NewCachedReaderFollowCrawler(crawler, db.New(dbs.Reader), ttl).WithTxRunner(dbs.Writer)
	cc.now = func() time.Time { return now }
	return cc
}

func TestCachedReaderFollowCrawler_FirstCall_CrawlsAndCaches(t *testing.T) {
	dbs := openReaderFollowCrawlCacheTestDB(t)
	crawler := &fakeReaderFollowCrawler{results: []ReaderNetworkFollow{{DID: "did:plc:alice"}}}
	cc := newCachedReaderFollowCrawlerUnderTest(dbs, crawler, time.Hour, mustParseTime(t, "2026-07-09T12:00:00Z"))

	did, _ := syntax.ParseDID(followedDID)
	got, err := cc.FetchReaderNetworkFollows(context.Background(), did)
	if err != nil {
		t.Fatalf("FetchReaderNetworkFollows: %v", err)
	}
	if crawler.calls != 1 {
		t.Errorf("crawler.calls = %d, want 1", crawler.calls)
	}
	if len(got) != 1 || got[0].DID != "did:plc:alice" {
		t.Fatalf("got = %+v", got)
	}

	rows, err := db.New(dbs.Reader).ListDiscoverCrawlFollows(context.Background(), followedDID)
	if err != nil {
		t.Fatalf("ListDiscoverCrawlFollows: %v", err)
	}
	if len(rows) != 1 || rows[0].SubjectDid != "did:plc:alice" {
		t.Fatalf("cached rows = %+v", rows)
	}
}

func TestCachedReaderFollowCrawler_WithinTTL_ServesCacheWithoutCrawling(t *testing.T) {
	dbs := openReaderFollowCrawlCacheTestDB(t)
	crawler := &fakeReaderFollowCrawler{results: []ReaderNetworkFollow{{DID: "did:plc:alice"}}}
	start := mustParseTime(t, "2026-07-09T12:00:00Z")
	cc := newCachedReaderFollowCrawlerUnderTest(dbs, crawler, time.Hour, start)
	did, _ := syntax.ParseDID(followedDID)

	if _, err := cc.FetchReaderNetworkFollows(context.Background(), did); err != nil {
		t.Fatalf("first fetch: %v", err)
	}

	cc.now = func() time.Time { return start.Add(30 * time.Minute) }
	got, err := cc.FetchReaderNetworkFollows(context.Background(), did)
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

func TestCachedReaderFollowCrawler_PastTTL_RecrawlsAndReplaces(t *testing.T) {
	dbs := openReaderFollowCrawlCacheTestDB(t)
	crawler := &fakeReaderFollowCrawler{results: []ReaderNetworkFollow{{DID: "did:plc:old"}}}
	start := mustParseTime(t, "2026-07-09T12:00:00Z")
	cc := newCachedReaderFollowCrawlerUnderTest(dbs, crawler, time.Hour, start)
	did, _ := syntax.ParseDID(followedDID)

	if _, err := cc.FetchReaderNetworkFollows(context.Background(), did); err != nil {
		t.Fatalf("first fetch: %v", err)
	}

	crawler.results = []ReaderNetworkFollow{{DID: "did:plc:new"}}
	cc.now = func() time.Time { return start.Add(2 * time.Hour) }
	got, err := cc.FetchReaderNetworkFollows(context.Background(), did)
	if err != nil {
		t.Fatalf("second fetch: %v", err)
	}
	if crawler.calls != 2 {
		t.Errorf("crawler.calls = %d, want 2 (stale cache must re-crawl)", crawler.calls)
	}
	if len(got) != 1 || got[0].DID != "did:plc:new" {
		t.Fatalf("got = %+v, want only the new result", got)
	}

	rows, err := db.New(dbs.Reader).ListDiscoverCrawlFollows(context.Background(), followedDID)
	if err != nil {
		t.Fatalf("ListDiscoverCrawlFollows: %v", err)
	}
	if len(rows) != 1 || rows[0].SubjectDid != "did:plc:new" {
		t.Fatalf("cached rows not replaced: %+v", rows)
	}
}

func TestCachedReaderFollowCrawler_CrawlErrorPropagatesAndWritesNothing(t *testing.T) {
	dbs := openReaderFollowCrawlCacheTestDB(t)
	crawler := &fakeReaderFollowCrawler{err: errors.New("pds unreachable")}
	cc := newCachedReaderFollowCrawlerUnderTest(dbs, crawler, time.Hour, mustParseTime(t, "2026-07-09T12:00:00Z"))
	did, _ := syntax.ParseDID(followedDID)

	if _, err := cc.FetchReaderNetworkFollows(context.Background(), did); err == nil {
		t.Fatal("expected error to propagate")
	}

	rows, err := db.New(dbs.Reader).ListDiscoverCrawlFollows(context.Background(), followedDID)
	if err != nil {
		t.Fatalf("ListDiscoverCrawlFollows: %v", err)
	}
	if len(rows) != 0 {
		t.Errorf("rows = %+v, want none written on crawl failure", rows)
	}
}
