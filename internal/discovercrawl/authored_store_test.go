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

const authoredCrawlCacheSchema = `
CREATE TABLE discover_crawl_authored_state (
    followed_did TEXT PRIMARY KEY,
    fetched_at   TEXT NOT NULL
);
CREATE TABLE discover_crawl_authored (
    followed_did      TEXT NOT NULL,
    canonical_key      TEXT NOT NULL,
    kind                TEXT NOT NULL,
    title               TEXT,
    site_url            TEXT,
    last_published_at   TEXT,
    fetched_at          TEXT NOT NULL,
    verification        TEXT NOT NULL,
    PRIMARY KEY (followed_did, canonical_key)
);
`

func openAuthoredCrawlCacheTestDB(t *testing.T) *database.DB {
	t.Helper()
	t.Setenv("DB_PATH", filepath.Join(t.TempDir(), "test.db"))
	dbs, err := database.Open()
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { dbs.Close() })
	if _, err := dbs.Writer.ExecContext(context.Background(), authoredCrawlCacheSchema); err != nil {
		t.Fatalf("schema: %v", err)
	}
	return dbs
}

type fakeAuthoredCrawler struct {
	calls   int
	results []AuthoredPublication
	err     error
}

func (f *fakeAuthoredCrawler) CrawlAuthoredPublications(context.Context, syntax.DID) ([]AuthoredPublication, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	return f.results, nil
}

func newCachedAuthoredCrawlerUnderTest(dbs *database.DB, crawler AuthoredCrawler, ttl time.Duration, now time.Time) *CachedAuthoredCrawler {
	cc := NewCachedAuthoredCrawler(crawler, db.New(dbs.Reader), ttl).WithTxRunner(dbs.Writer)
	cc.now = func() time.Time { return now }
	return cc
}

func TestCachedAuthoredCrawler_FirstCall_CrawlsAndCaches(t *testing.T) {
	dbs := openAuthoredCrawlCacheTestDB(t)
	pubURI := "at://" + followedDID + "/site.standard.publication/3p"
	crawler := &fakeAuthoredCrawler{results: []AuthoredPublication{{Key: pubURI, Kind: "standardfeed", Title: "Zine", SiteURL: "https://zine.example", LastPublishedAt: "2026-07-08T00:00:00Z", Verification: verifiedOutcome}}}
	cc := newCachedAuthoredCrawlerUnderTest(dbs, crawler, time.Hour, mustParseTime(t, "2026-07-09T12:00:00Z"))

	did, _ := syntax.ParseDID(followedDID)
	got, err := cc.FetchAuthoredPublications(context.Background(), did)
	if err != nil {
		t.Fatalf("FetchAuthoredPublications: %v", err)
	}
	if crawler.calls != 1 {
		t.Errorf("crawler.calls = %d, want 1", crawler.calls)
	}
	if len(got) != 1 || got[0].Key != pubURI {
		t.Fatalf("got = %+v", got)
	}

	rows, err := db.New(dbs.Reader).ListDiscoverCrawlAuthored(context.Background(), followedDID)
	if err != nil {
		t.Fatalf("ListDiscoverCrawlAuthored: %v", err)
	}
	if len(rows) != 1 || rows[0].CanonicalKey != pubURI || rows[0].Verification != verifiedOutcome {
		t.Fatalf("cached rows = %+v, want the verified outcome persisted", rows)
	}
}

func TestCachedAuthoredCrawler_WithinTTL_ServesCacheWithoutCrawling(t *testing.T) {
	dbs := openAuthoredCrawlCacheTestDB(t)
	pubURI := "at://" + followedDID + "/site.standard.publication/3p"
	crawler := &fakeAuthoredCrawler{results: []AuthoredPublication{{Key: pubURI, Kind: "standardfeed", Title: "Zine", Verification: verifiedOutcome}}}
	start := mustParseTime(t, "2026-07-09T12:00:00Z")
	cc := newCachedAuthoredCrawlerUnderTest(dbs, crawler, time.Hour, start)
	did, _ := syntax.ParseDID(followedDID)

	if _, err := cc.FetchAuthoredPublications(context.Background(), did); err != nil {
		t.Fatalf("first fetch: %v", err)
	}

	cc.now = func() time.Time { return start.Add(30 * time.Minute) }
	got, err := cc.FetchAuthoredPublications(context.Background(), did)
	if err != nil {
		t.Fatalf("second fetch: %v", err)
	}
	if crawler.calls != 1 {
		t.Errorf("crawler.calls = %d, want 1 (TTL hit must not re-crawl)", crawler.calls)
	}
	if len(got) != 1 || got[0].Title != "Zine" {
		t.Fatalf("got = %+v, want cached result", got)
	}
}

func TestCachedAuthoredCrawler_PastTTL_RecrawlsAndReplaces(t *testing.T) {
	dbs := openAuthoredCrawlCacheTestDB(t)
	pubURI := "at://" + followedDID + "/site.standard.publication/old"
	crawler := &fakeAuthoredCrawler{results: []AuthoredPublication{{Key: pubURI, Kind: "standardfeed", Verification: verifiedOutcome}}}
	start := mustParseTime(t, "2026-07-09T12:00:00Z")
	cc := newCachedAuthoredCrawlerUnderTest(dbs, crawler, time.Hour, start)
	did, _ := syntax.ParseDID(followedDID)

	if _, err := cc.FetchAuthoredPublications(context.Background(), did); err != nil {
		t.Fatalf("first fetch: %v", err)
	}

	newPubURI := "at://" + followedDID + "/site.standard.publication/new"
	crawler.results = []AuthoredPublication{{Key: newPubURI, Kind: "standardfeed", Verification: verifiedOutcome}}
	cc.now = func() time.Time { return start.Add(2 * time.Hour) }
	got, err := cc.FetchAuthoredPublications(context.Background(), did)
	if err != nil {
		t.Fatalf("second fetch: %v", err)
	}
	if crawler.calls != 2 {
		t.Errorf("crawler.calls = %d, want 2 (stale cache must re-crawl)", crawler.calls)
	}
	if len(got) != 1 || got[0].Key != newPubURI {
		t.Fatalf("got = %+v, want only the new result", got)
	}
}

// TestCachedAuthoredCrawler_WithinTTL_FiltersOutNonVerifiedCachedRows seeds the cache directly (bypassing a crawl) with one verified and one mismatched row, proving the list query still surfaces every outcome while FetchAuthoredPublications only ever serves the verified one.
func TestCachedAuthoredCrawler_WithinTTL_FiltersOutNonVerifiedCachedRows(t *testing.T) {
	dbs := openAuthoredCrawlCacheTestDB(t)
	ctx := context.Background()
	start := mustParseTime(t, "2026-07-09T12:00:00Z")
	q := db.New(dbs.Writer)

	if err := q.UpsertDiscoverCrawlAuthoredState(ctx, db.UpsertDiscoverCrawlAuthoredStateParams{FollowedDid: followedDID, FetchedAt: start.Format(time.RFC3339)}); err != nil {
		t.Fatalf("seed state: %v", err)
	}
	verifiedURI := "at://" + followedDID + "/site.standard.publication/good"
	mismatchURI := "at://" + followedDID + "/site.standard.publication/bad"
	if err := q.InsertDiscoverCrawlAuthored(ctx, db.InsertDiscoverCrawlAuthoredParams{
		FollowedDid: followedDID, CanonicalKey: verifiedURI, Kind: "standardfeed", FetchedAt: start.Format(time.RFC3339), Verification: verifiedOutcome,
	}); err != nil {
		t.Fatalf("seed verified row: %v", err)
	}
	if err := q.InsertDiscoverCrawlAuthored(ctx, db.InsertDiscoverCrawlAuthoredParams{
		FollowedDid: followedDID, CanonicalKey: mismatchURI, Kind: "standardfeed", FetchedAt: start.Format(time.RFC3339), Verification: "mismatch",
	}); err != nil {
		t.Fatalf("seed mismatch row: %v", err)
	}

	crawler := &fakeAuthoredCrawler{}
	cc := newCachedAuthoredCrawlerUnderTest(dbs, crawler, time.Hour, start.Add(30*time.Minute))
	did, _ := syntax.ParseDID(followedDID)
	got, err := cc.FetchAuthoredPublications(ctx, did)
	if err != nil {
		t.Fatalf("FetchAuthoredPublications: %v", err)
	}
	if crawler.calls != 0 {
		t.Errorf("crawler.calls = %d, want 0 (TTL hit must not re-crawl)", crawler.calls)
	}
	if len(got) != 1 || got[0].Key != verifiedURI {
		t.Fatalf("got = %+v, want only the verified row served", got)
	}

	rows, err := db.New(dbs.Reader).ListDiscoverCrawlAuthored(ctx, followedDID)
	if err != nil {
		t.Fatalf("ListDiscoverCrawlAuthored: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("rows = %+v, want the list query to still surface both outcomes", rows)
	}
}

func TestCachedAuthoredCrawler_CrawlErrorPropagatesAndWritesNothing(t *testing.T) {
	dbs := openAuthoredCrawlCacheTestDB(t)
	crawler := &fakeAuthoredCrawler{err: errors.New("pds unreachable")}
	cc := newCachedAuthoredCrawlerUnderTest(dbs, crawler, time.Hour, mustParseTime(t, "2026-07-09T12:00:00Z"))
	did, _ := syntax.ParseDID(followedDID)

	if _, err := cc.FetchAuthoredPublications(context.Background(), did); err == nil {
		t.Fatal("expected error to propagate")
	}

	rows, err := db.New(dbs.Reader).ListDiscoverCrawlAuthored(context.Background(), followedDID)
	if err != nil {
		t.Fatalf("ListDiscoverCrawlAuthored: %v", err)
	}
	if len(rows) != 0 {
		t.Errorf("rows = %+v, want none written on crawl failure", rows)
	}
}
