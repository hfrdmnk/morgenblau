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
	// mu guards the counters: the batch path crawls several DIDs concurrently.
	mu      sync.Mutex
	calls   int
	crawled []string
	results []AuthoredPublication
	byDID   map[string][]AuthoredPublication
	err     error
}

func (f *fakeAuthoredCrawler) CrawlAuthoredPublications(_ context.Context, did syntax.DID) ([]AuthoredPublication, error) {
	f.mu.Lock()
	f.calls++
	f.crawled = append(f.crawled, did.String())
	f.mu.Unlock()
	if f.err != nil {
		return nil, f.err
	}
	if pubs, ok := f.byDID[did.String()]; ok {
		return pubs, nil
	}
	return f.results, nil
}

func (f *fakeAuthoredCrawler) crawledDIDs() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := append([]string(nil), f.crawled...)
	sort.Strings(out)
	return out
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

func seedAuthoredCache(t *testing.T, dbs *database.DB, did string, fetchedAt time.Time, pubs ...AuthoredPublication) {
	t.Helper()
	q := db.New(dbs.Writer)
	stamp := fetchedAt.UTC().Format(time.RFC3339)
	for _, p := range pubs {
		if err := q.InsertDiscoverCrawlAuthored(context.Background(), db.InsertDiscoverCrawlAuthoredParams{
			FollowedDid:  did,
			CanonicalKey: p.Key,
			Kind:         p.Kind,
			FetchedAt:    stamp,
			Verification: p.Verification,
		}); err != nil {
			t.Fatalf("seed authored: %v", err)
		}
	}
	if err := q.UpsertDiscoverCrawlAuthoredState(context.Background(), db.UpsertDiscoverCrawlAuthoredStateParams{FollowedDid: did, FetchedAt: stamp}); err != nil {
		t.Fatalf("seed authored state: %v", err)
	}
}

func authoredKeys(pubs []AuthoredPublication) []string {
	out := make([]string, 0, len(pubs))
	for _, p := range pubs {
		out = append(out, p.Key)
	}
	return out
}

func TestCachedAuthoredCrawler_FetchAuthoredPublicationsBatch_CachedPathStaysVerifiedOnly(t *testing.T) {
	dbs := openAuthoredCrawlCacheTestDB(t)
	now := mustParseTime(t, "2026-07-09T12:00:00Z")
	seedAuthoredCache(t, dbs, "did:plc:fresh", now.Add(-30*time.Minute),
		AuthoredPublication{Key: "at://did:plc:fresh/site.standard.publication/3a", Kind: "standardfeed", Verification: verifiedOutcome},
		AuthoredPublication{Key: "at://did:plc:fresh/site.standard.publication/3b", Kind: "standardfeed", Verification: "mismatch"},
	)

	crawler := &fakeAuthoredCrawler{byDID: map[string][]AuthoredPublication{
		"did:plc:cold": {{Key: "https://cold.example/feed", Kind: "rss", Verification: verifiedOutcome}},
	}}
	cc := newCachedAuthoredCrawlerUnderTest(dbs, crawler, time.Hour, now)

	got := cc.FetchAuthoredPublicationsBatch(context.Background(), []string{"did:plc:fresh", "did:plc:cold"})

	want := map[string][]string{
		"did:plc:fresh": {"at://did:plc:fresh/site.standard.publication/3a"},
		"did:plc:cold":  {"https://cold.example/feed"},
	}
	gotKeys := map[string][]string{}
	for did, pubs := range got {
		gotKeys[did] = authoredKeys(pubs)
	}
	if !reflect.DeepEqual(gotKeys, want) {
		t.Errorf("keys = %v, want %v (an unverified cached row must never leak)", gotKeys, want)
	}
	if crawled := crawler.crawledDIDs(); !reflect.DeepEqual(crawled, []string{"did:plc:cold"}) {
		t.Errorf("crawled = %v, want only the never-crawled did", crawled)
	}
}
