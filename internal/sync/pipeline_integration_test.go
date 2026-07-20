package sync

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"time"

	"morgenblau/internal/database"
	"morgenblau/internal/database/db"
	"morgenblau/internal/fetcher"
	"morgenblau/internal/safehttp"
	"morgenblau/internal/standardfeed"
)

// pipelineSchemaSQL must stay in sync with internal/database/migrations/*_subscriptions_and_feeds.sql, *_feed_entries.sql, *_feeds_language.sql, and *_feed_fetch_backoff.sql.
const pipelineSchemaSQL = `
CREATE TABLE feeds (
    feed_url         TEXT PRIMARY KEY,
    kind             TEXT NOT NULL DEFAULT 'rss',
    site_url         TEXT,
    title            TEXT,
    etag             TEXT,
    last_modified    TEXT,
    last_fetched_at  TEXT,
    icon_url         TEXT,
    icon_fetched_at  TEXT,
    created_at       TEXT NOT NULL,
    updated_at       TEXT NOT NULL,
    language         TEXT,
    consecutive_failures INTEGER NOT NULL DEFAULT 0,
    next_fetch_at    TEXT
);
CREATE TABLE feed_entries (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    feed_url        TEXT NOT NULL,
    guid            TEXT NOT NULL,
    entry_slug      TEXT NOT NULL,
    url             TEXT NOT NULL,
    title           TEXT,
    content_html    TEXT,
    content_type    TEXT NOT NULL,
    published_at    TEXT NOT NULL,
    fetched_at      TEXT NOT NULL,
    metadata        TEXT,
    extracted_body  TEXT,
    record_cid      TEXT,
    UNIQUE (feed_url, guid),
    UNIQUE (entry_slug),
    FOREIGN KEY (feed_url) REFERENCES feeds(feed_url) ON DELETE CASCADE
);
`

// openPipelineTestDB uses a t.TempDir() file, not a :memory: DSN, which would give the reader and writer pools separate databases.
func openPipelineTestDB(t *testing.T) *database.DB {
	t.Helper()
	t.Setenv("DB_PATH", filepath.Join(t.TempDir(), "test.db"))
	dbs, err := database.Open()
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { dbs.Close() })
	if _, err := dbs.Writer.ExecContext(context.Background(), pipelineSchemaSQL); err != nil {
		t.Fatalf("schema: %v", err)
	}
	return dbs
}

// seedCatalogFeed inserts the feeds row FetchAndStore expects to exist; production callers always upsert it before a fetch is ever dispatched.
func seedCatalogFeed(t *testing.T, dbs *database.DB, feedURL string) {
	t.Helper()
	nowStr := time.Now().UTC().Format(time.RFC3339)
	if err := db.New(dbs.Writer).UpsertFeed(context.Background(), db.UpsertFeedParams{
		FeedUrl: feedURL, CreatedAt: nowStr, UpdatedAt: nowStr,
	}); err != nil {
		t.Fatalf("seedCatalogFeed: %v", err)
	}
}

func TestFeedPipeline_FetchAndStore_BackoffPersistsAcrossCallsThenRecovers(t *testing.T) {
	var mu sync.Mutex
	hits := 0
	feedBody := rssWithLink("https://site.example.com/", `<item>
<title>Recovered</title>
<link>https://site.example.com/recovered</link>
<guid>guid-recovered</guid>
</item>`)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		hits++
		n := hits
		mu.Unlock()
		if n == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = w.Write([]byte(feedBody))
	}))
	defer srv.Close()
	feedURL := srv.URL + "/feed.xml"

	dbs := openPipelineTestDB(t)
	ctx := context.Background()
	seedCatalogFeed(t, dbs, feedURL)

	now := time.Date(2026, 7, 9, 10, 0, 0, 0, time.UTC)
	p := NewFeedPipeline(fetcher.New(fetcher.WithSafeHTTPOptions(safehttp.WithAllowLoopback())), db.New(dbs.Writer)).
		WithFaviconDiscoverer(&fakeFaviconDiscoverer{}).
		WithTxRunner(dbs.Writer)
	p.now = func() time.Time { return now }

	if err := p.FetchAndStore(ctx, feedURL); err == nil {
		t.Fatal("first FetchAndStore: want error for the scripted 500")
	}
	readerQ := db.New(dbs.Reader)
	feed, err := readerQ.GetFeed(ctx, feedURL)
	if err != nil {
		t.Fatalf("GetFeed after failure: %v", err)
	}
	if feed.ConsecutiveFailures != 1 {
		t.Errorf("ConsecutiveFailures = %d, want 1", feed.ConsecutiveFailures)
	}
	wantNext := now.Add(5 * time.Minute).Format(time.RFC3339)
	if feed.NextFetchAt == nil || *feed.NextFetchAt != wantNext {
		t.Errorf("NextFetchAt = %v, want %s", feed.NextFetchAt, wantNext)
	}

	if err := p.FetchAndStore(ctx, feedURL); err != nil {
		t.Fatalf("second FetchAndStore (should skip, still in backoff): %v", err)
	}
	if hits != 1 {
		t.Errorf("HTTP hits after second call = %d, want 1 (skip made no request)", hits)
	}

	p.now = func() time.Time { return now.Add(10 * time.Minute) }
	if err := p.FetchAndStore(ctx, feedURL); err != nil {
		t.Fatalf("third FetchAndStore: %v", err)
	}
	if hits != 2 {
		t.Errorf("HTTP hits after third call = %d, want 2", hits)
	}
	feed, err = readerQ.GetFeed(ctx, feedURL)
	if err != nil {
		t.Fatalf("GetFeed after recovery: %v", err)
	}
	if feed.ConsecutiveFailures != 0 {
		t.Errorf("ConsecutiveFailures after recovery = %d, want 0", feed.ConsecutiveFailures)
	}
	if feed.NextFetchAt != nil {
		t.Errorf("NextFetchAt after recovery = %v, want nil", feed.NextFetchAt)
	}
	if _, err := readerQ.GetFeedEntryBySlug(ctx, EntrySlug(feedURL, "guid-recovered")); err != nil {
		t.Errorf("recovered entry missing: %v", err)
	}
}

func TestFeedPipeline_FetchAndStore_BatchCommitsAtomicallyAcrossPools(t *testing.T) {
	feedURL, closeServer := serveFeed(t, rssWithLink("https://site.example.com/", `<item>
<title>First</title>
<link>https://site.example.com/first</link>
<guid>guid-first</guid>
<description><![CDATA[<p>First body</p>]]></description>
</item>
<item>
<title>Second</title>
<link>https://site.example.com/second</link>
<guid>guid-second</guid>
<description><![CDATA[<p>Second body</p>]]></description>
</item>`))
	defer closeServer()

	dbs := openPipelineTestDB(t)
	ctx := context.Background()
	seedCatalogFeed(t, dbs, feedURL)
	fav := &fakeFaviconDiscoverer{icon: "https://site.example.com/favicon.ico"}
	p := NewFeedPipeline(fetcher.New(fetcher.WithSafeHTTPOptions(safehttp.WithAllowLoopback())), db.New(dbs.Writer)).
		WithFaviconDiscoverer(fav).
		WithTxRunner(dbs.Writer)
	p.now = func() time.Time { return time.Date(2026, 7, 9, 10, 0, 0, 0, time.UTC) }

	if err := p.FetchAndStore(ctx, feedURL); err != nil {
		t.Fatalf("FetchAndStore: %v", err)
	}

	// Every read below goes through the Reader pool, proving the writer-pool commit is visible cross-pool over WAL.
	readerQ := db.New(dbs.Reader)
	feed, err := readerQ.GetFeed(ctx, feedURL)
	if err != nil {
		t.Fatalf("GetFeed via reader: %v", err)
	}
	if feed.LastFetchedAt == nil {
		t.Errorf("fetch state not persisted: %+v", feed)
	}
	if feed.IconUrl == nil || *feed.IconUrl != fav.icon {
		t.Errorf("feed icon = %v, want %s", feed.IconUrl, fav.icon)
	}
	if _, err := readerQ.GetFeedEntryBySlug(ctx, EntrySlug(feedURL, "guid-first")); err != nil {
		t.Errorf("first entry missing: %v", err)
	}
	if _, err := readerQ.GetFeedEntryBySlug(ctx, EntrySlug(feedURL, "guid-second")); err != nil {
		t.Errorf("second entry missing: %v", err)
	}
}

func TestFeedPipeline_FetchAndStore_SkippedEntryDoesNotLoseSiblings(t *testing.T) {
	feedURL, closeServer := serveFeed(t, rssWithLink("https://site.example.com/", `<item>
<title>Good one</title>
<link>https://site.example.com/good-1</link>
<guid>guid-good-1</guid>
</item>
<item>
<title>No guid or link</title>
<description>chooseGUID returns empty for this one — must be skipped, not fatal</description>
</item>
<item>
<title>Good two</title>
<link>https://site.example.com/good-2</link>
<guid>guid-good-2</guid>
</item>`))
	defer closeServer()

	dbs := openPipelineTestDB(t)
	ctx := context.Background()
	seedCatalogFeed(t, dbs, feedURL)
	p := NewFeedPipeline(fetcher.New(fetcher.WithSafeHTTPOptions(safehttp.WithAllowLoopback())), db.New(dbs.Writer)).
		WithFaviconDiscoverer(&fakeFaviconDiscoverer{}).
		WithTxRunner(dbs.Writer)
	p.now = func() time.Time { return time.Date(2026, 7, 9, 10, 0, 0, 0, time.UTC) }

	if err := p.FetchAndStore(ctx, feedURL); err != nil {
		t.Fatalf("FetchAndStore: %v", err)
	}

	q := db.New(dbs.Writer)
	feed, err := q.GetFeed(ctx, feedURL)
	if err != nil {
		t.Fatalf("feed row missing after batch with a bad entry: %v", err)
	}
	if feed.LastFetchedAt == nil {
		t.Errorf("fetch state not persisted: %+v", feed)
	}
	entries, err := q.ListFeedEntriesForDiff(ctx, feedURL)
	if err != nil {
		t.Fatalf("ListFeedEntriesForDiff: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("entries = %d, want 2 good siblings, got %+v", len(entries), entries)
	}
}

// failingEntryQueries fails UpsertFeedEntry for one guid, driving the log-and-continue branch on a real per-entry write error (unlike the skipped-entry tests' pre-INSERT continue).
type failingEntryQueries struct {
	failGUID string
	upserted []string
}

func (q *failingEntryQueries) GetFeed(context.Context, string) (db.Feed, error) {
	return db.Feed{}, sql.ErrNoRows
}
func (q *failingEntryQueries) UpdateFeedFetchState(context.Context, db.UpdateFeedFetchStateParams) error {
	return nil
}
func (q *failingEntryQueries) UpdateFeedFetchFailure(context.Context, db.UpdateFeedFetchFailureParams) error {
	return nil
}
func (q *failingEntryQueries) UpsertFeed(context.Context, db.UpsertFeedParams) error { return nil }
func (q *failingEntryQueries) SetFeedIconURL(context.Context, db.SetFeedIconURLParams) error {
	return nil
}
func (q *failingEntryQueries) UpsertFeedEntry(_ context.Context, arg db.UpsertFeedEntryParams) error {
	if arg.Guid == q.failGUID {
		return errors.New("simulated per-entry constraint violation")
	}
	q.upserted = append(q.upserted, arg.Guid)
	return nil
}

func TestFeedPipeline_FetchAndStore_EntryUpsertErrorDoesNotFailBatch(t *testing.T) {
	feedURL, closeServer := serveFeed(t, rssWithLink("https://site.example.com/", `<item>
<title>Good one</title>
<link>https://site.example.com/good-1</link>
<guid>guid-good-1</guid>
</item>
<item>
<title>Doomed</title>
<link>https://site.example.com/bad</link>
<guid>guid-bad</guid>
</item>
<item>
<title>Good two</title>
<link>https://site.example.com/good-2</link>
<guid>guid-good-2</guid>
</item>`))
	defer closeServer()

	q := &failingEntryQueries{failGUID: "guid-bad"}
	p := NewFeedPipeline(fetcher.New(fetcher.WithSafeHTTPOptions(safehttp.WithAllowLoopback())), q).
		WithFaviconDiscoverer(&fakeFaviconDiscoverer{})
	p.now = func() time.Time { return time.Date(2026, 7, 9, 10, 0, 0, 0, time.UTC) }

	if err := p.FetchAndStore(context.Background(), feedURL); err != nil {
		t.Fatalf("FetchAndStore returned %v; a per-entry upsert error must be tolerated, not fail the batch", err)
	}
	if !slices.Contains(q.upserted, "guid-good-1") || !slices.Contains(q.upserted, "guid-good-2") {
		t.Errorf("siblings lost after a mid-batch entry error: upserted=%v", q.upserted)
	}
	if slices.Contains(q.upserted, "guid-bad") {
		t.Errorf("failing entry should not be recorded as upserted: %v", q.upserted)
	}
}

func TestStandardfeedPipeline_FetchAndStore_MalformedDocDoesNotLoseSiblings(t *testing.T) {
	dbs := openPipelineTestDB(t)
	ctx := context.Background()

	goodURI1 := testDocURI + "-a"
	goodURI2 := testDocURI + "-b"
	badURI := testDocURI + "-bad"
	src := &fakeStandardSource{
		pub: testPublication(),
		docs: []standardfeed.Document{
			{URI: goodURI1, CID: "cid-a", Site: testPubURI, Title: "Good A", Path: "/a", PublishedAt: "2026-07-01T08:00:00Z"},
			{URI: badURI, CID: "cid-bad", Site: testPubURI, Path: "/bad", PublishedAt: "2026-07-01T08:00:00Z"}, // no title
			{URI: goodURI2, CID: "cid-b", Site: testPubURI, Title: "Good B", Path: "/b", PublishedAt: "2026-07-01T08:00:00Z"},
		},
	}
	p := NewStandardfeedPipeline(src, db.New(dbs.Writer)).WithTxRunner(dbs.Writer)
	p.now = func() time.Time { return time.Date(2026, 7, 9, 10, 0, 0, 0, time.UTC) }

	if err := p.FetchAndStore(ctx, testPubURI); err != nil {
		t.Fatalf("FetchAndStore: %v", err)
	}

	q := db.New(dbs.Writer)
	feed, err := q.GetFeed(ctx, testPubURI)
	if err != nil {
		t.Fatalf("feed row missing after batch with a malformed doc: %v", err)
	}
	if feed.LastFetchedAt == nil {
		t.Errorf("fetch state not persisted: %+v", feed)
	}
	if _, err := q.GetFeedEntryBySlug(ctx, EntrySlug(testPubURI, goodURI1)); err != nil {
		t.Errorf("good sibling A missing: %v", err)
	}
	if _, err := q.GetFeedEntryBySlug(ctx, EntrySlug(testPubURI, goodURI2)); err != nil {
		t.Errorf("good sibling B missing: %v", err)
	}
	if _, err := q.GetFeedEntryBySlug(ctx, EntrySlug(testPubURI, badURI)); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("malformed doc should not have been persisted: err=%v", err)
	}
}

func TestStandardfeedPipeline_FetchAndStore_DeleteSweepAndMalformedProtection(t *testing.T) {
	dbs := openPipelineTestDB(t)
	ctx := context.Background()
	seedQ := db.New(dbs.Writer)

	goneURI := testDocURI + "-gone"
	keepURI := testDocURI
	seedTime := time.Date(2026, 7, 9, 9, 0, 0, 0, time.UTC).Format(time.RFC3339)

	if err := seedQ.UpsertFeed(ctx, db.UpsertFeedParams{FeedUrl: testPubURI, Kind: "standardfeed", CreatedAt: seedTime, UpdatedAt: seedTime}); err != nil {
		t.Fatalf("seed feed: %v", err)
	}
	goneCID, keepCID := "cid-gone", "cid-keep"
	if err := seedQ.UpsertStandardfeedEntry(ctx, db.UpsertStandardfeedEntryParams{
		FeedUrl: testPubURI, Guid: goneURI, EntrySlug: EntrySlug(testPubURI, goneURI),
		Url: "https://example.com/gone", Title: nilIfEmpty("Gone"), ContentType: "blogpost",
		PublishedAt: seedTime, FetchedAt: seedTime, RecordCid: &goneCID,
	}); err != nil {
		t.Fatalf("seed gone entry: %v", err)
	}
	if err := seedQ.UpsertStandardfeedEntry(ctx, db.UpsertStandardfeedEntryParams{
		FeedUrl: testPubURI, Guid: keepURI, EntrySlug: EntrySlug(testPubURI, keepURI),
		Url: "https://example.com/keep", Title: nilIfEmpty("Keep"), ContentType: "blogpost",
		PublishedAt: seedTime, FetchedAt: seedTime, RecordCid: &keepCID,
	}); err != nil {
		t.Fatalf("seed keep entry: %v", err)
	}

	// keepURI comes back malformed (no title) but must survive: it's marked present before the validity check runs.
	src := &fakeStandardSource{
		pub: testPublication(),
		docs: []standardfeed.Document{
			{URI: keepURI, CID: "cid-new", Site: testPubURI, Path: "/x", PublishedAt: seedTime},
		},
	}
	p := NewStandardfeedPipeline(src, db.New(dbs.Writer)).WithTxRunner(dbs.Writer)
	p.now = func() time.Time { return time.Date(2026, 7, 9, 10, 0, 0, 0, time.UTC) }

	if err := p.FetchAndStore(ctx, testPubURI); err != nil {
		t.Fatalf("FetchAndStore: %v", err)
	}

	readerQ := db.New(dbs.Reader)
	if _, err := readerQ.GetFeedEntryBySlug(ctx, EntrySlug(testPubURI, goneURI)); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("entry missing upstream should have been deleted: err=%v", err)
	}
	kept, err := readerQ.GetFeedEntryBySlug(ctx, EntrySlug(testPubURI, keepURI))
	if err != nil {
		t.Fatalf("malformed-but-present doc's cached entry was deleted: %v", err)
	}
	if kept.RecordCid == nil || *kept.RecordCid != keepCID {
		t.Errorf("cached entry should be untouched (still old cid), got %v", kept.RecordCid)
	}
}
