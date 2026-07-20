package sync

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"morgenblau/internal/database/db"
	"morgenblau/internal/standardfeed"
)

type fakeStandardSource struct {
	pub     *standardfeed.Publication
	pubErr  error
	docs    []standardfeed.Document
	listErr error
}

func (f *fakeStandardSource) GetPublication(context.Context, string) (*standardfeed.Publication, error) {
	return f.pub, f.pubErr
}

func (f *fakeStandardSource) ListDocuments(context.Context, string) ([]standardfeed.Document, error) {
	return f.docs, f.listErr
}

type fakeStdQueries struct {
	feedUpserts  []db.UpsertFeedParams
	iconSets     []db.SetFeedIconURLParams
	stateUpdates []db.UpdateFeedFetchStateParams
	diffRows     []db.ListFeedEntriesForDiffRow
	diffErr      error
	entryUpserts []db.UpsertStandardfeedEntryParams
	deletes      []db.DeleteFeedEntryParams
}

func (f *fakeStdQueries) UpsertFeed(_ context.Context, arg db.UpsertFeedParams) error {
	f.feedUpserts = append(f.feedUpserts, arg)
	return nil
}

func (f *fakeStdQueries) SetFeedIconURL(_ context.Context, arg db.SetFeedIconURLParams) error {
	f.iconSets = append(f.iconSets, arg)
	return nil
}

func (f *fakeStdQueries) UpdateFeedFetchState(_ context.Context, arg db.UpdateFeedFetchStateParams) error {
	f.stateUpdates = append(f.stateUpdates, arg)
	return nil
}

func (f *fakeStdQueries) ListFeedEntriesForDiff(context.Context, string) ([]db.ListFeedEntriesForDiffRow, error) {
	return f.diffRows, f.diffErr
}

func (f *fakeStdQueries) UpsertStandardfeedEntry(_ context.Context, arg db.UpsertStandardfeedEntryParams) error {
	f.entryUpserts = append(f.entryUpserts, arg)
	return nil
}

func (f *fakeStdQueries) DeleteFeedEntry(_ context.Context, arg db.DeleteFeedEntryParams) error {
	f.deletes = append(f.deletes, arg)
	return nil
}

const (
	testPubURI = "at://did:plc:pub123/site.standard.publication/3abc"
	testDocURI = "at://did:plc:pub123/site.standard.document/3doc"
)

func testPublication() *standardfeed.Publication {
	return &standardfeed.Publication{
		URI:     testPubURI,
		DID:     "did:plc:pub123",
		Name:    "Example Journal",
		URL:     "https://example.com",
		IconURL: "https://pds.example.com/xrpc/com.atproto.sync.getBlob?cid=x",
	}
}

func newStdPipeline(src StandardfeedSource, q stdPipelineQueries) *StandardfeedPipeline {
	p := NewStandardfeedPipeline(src, q)
	p.now = func() time.Time { return time.Date(2026, 7, 4, 12, 0, 0, 0, time.UTC) }
	return p
}

func TestStandardPipeline_InsertsNewDocument(t *testing.T) {
	src := &fakeStandardSource{
		pub: testPublication(),
		docs: []standardfeed.Document{{
			URI: testDocURI, CID: "cid1", Site: testPubURI,
			Title: "Hello", Path: "/hello", Description: "an excerpt",
			TextContent: "plain text", PublishedAt: "2026-07-01T08:00:00.500Z",
		}},
	}
	q := &fakeStdQueries{}
	if err := newStdPipeline(src, q).FetchAndStore(context.Background(), testPubURI); err != nil {
		t.Fatalf("FetchAndStore: %v", err)
	}

	if len(q.feedUpserts) != 1 {
		t.Fatalf("feed upserts: %+v", q.feedUpserts)
	}
	fu := q.feedUpserts[0]
	if fu.FeedUrl != testPubURI || fu.Kind != "standardfeed" || fu.Title == nil || *fu.Title != "Example Journal" || fu.SiteUrl == nil || *fu.SiteUrl != "https://example.com" {
		t.Fatalf("feed upsert: %+v", fu)
	}
	if len(q.iconSets) != 1 || q.iconSets[0].IconUrl == nil || *q.iconSets[0].IconUrl != src.pub.IconURL {
		t.Fatalf("icon sets: %+v", q.iconSets)
	}
	if len(q.stateUpdates) != 1 {
		t.Fatalf("state updates: %+v", q.stateUpdates)
	}

	if len(q.entryUpserts) != 1 {
		t.Fatalf("entry upserts: %+v", q.entryUpserts)
	}
	e := q.entryUpserts[0]
	if e.FeedUrl != testPubURI || e.Guid != testDocURI || e.Url != "https://example.com/hello" {
		t.Fatalf("entry identity: %+v", e)
	}
	if e.EntrySlug != EntrySlug(testPubURI, testDocURI) {
		t.Fatalf("entry slug: %q", e.EntrySlug)
	}
	if e.Title == nil || *e.Title != "Hello" || e.ContentType != "blogpost" {
		t.Fatalf("entry title/type: %+v", e)
	}
	if e.ContentHtml == nil || *e.ContentHtml != "an excerpt" {
		t.Fatalf("summary should come from description: %+v", e.ContentHtml)
	}
	if e.PublishedAt != "2026-07-01T08:00:00Z" {
		t.Fatalf("published_at not normalized: %q", e.PublishedAt)
	}
	if e.ExtractedBody != nil {
		t.Fatalf("path-ful doc must not prefill extracted_body: %+v", e.ExtractedBody)
	}
	if e.RecordCid == nil || *e.RecordCid != "cid1" {
		t.Fatalf("record cid: %+v", e.RecordCid)
	}
	if len(q.deletes) != 0 {
		t.Fatalf("unexpected deletes: %+v", q.deletes)
	}
}

func TestStandardPipeline_DetectsLanguageFromDocumentContent(t *testing.T) {
	src := &fakeStandardSource{
		pub: testPublication(),
		docs: []standardfeed.Document{{
			URI: testDocURI, CID: "cid1", Site: testPubURI,
			Title: "Article", Path: "/a",
			Description: "Le rapide renard brun saute par-dessus le chien paresseux pendant que le soleil se couche lentement derriere les collines lointaines.",
			PublishedAt: "2026-07-01T08:00:00Z",
		}},
	}
	q := &fakeStdQueries{}
	if err := newStdPipeline(src, q).FetchAndStore(context.Background(), testPubURI); err != nil {
		t.Fatalf("FetchAndStore: %v", err)
	}
	if len(q.feedUpserts) != 1 {
		t.Fatalf("feed upserts: %+v", q.feedUpserts)
	}
	if lang := q.feedUpserts[0].Language; lang == nil || *lang != "fr" {
		t.Errorf("Language = %v, want fr (detected from document content)", lang)
	}
}

func TestStandardPipeline_UnknownLanguageWhenContentTooShort(t *testing.T) {
	src := &fakeStandardSource{
		pub: testPublication(),
		docs: []standardfeed.Document{{
			URI: testDocURI, CID: "cid1", Site: testPubURI,
			Title: "Hi", Path: "/a", Description: "Hi", PublishedAt: "2026-07-01T08:00:00Z",
		}},
	}
	q := &fakeStdQueries{}
	if err := newStdPipeline(src, q).FetchAndStore(context.Background(), testPubURI); err != nil {
		t.Fatalf("FetchAndStore: %v", err)
	}
	if lang := q.feedUpserts[0].Language; lang != nil {
		t.Errorf("Language = %v, want nil (standardfeed documents carry no language tag hint)", lang)
	}
}

func TestStandardPipeline_UnchangedCIDWritesNothing(t *testing.T) {
	cid := "cid1"
	src := &fakeStandardSource{
		pub: testPublication(),
		docs: []standardfeed.Document{{
			URI: testDocURI, CID: cid, Site: testPubURI,
			Title: "Hello", Path: "/hello", PublishedAt: "2026-07-01T08:00:00Z",
		}},
	}
	q := &fakeStdQueries{diffRows: []db.ListFeedEntriesForDiffRow{{Guid: testDocURI, RecordCid: &cid}}}
	if err := newStdPipeline(src, q).FetchAndStore(context.Background(), testPubURI); err != nil {
		t.Fatalf("FetchAndStore: %v", err)
	}
	if len(q.entryUpserts) != 0 || len(q.deletes) != 0 {
		t.Fatalf("unchanged CID must not write: upserts=%+v deletes=%+v", q.entryUpserts, q.deletes)
	}
}

func TestStandardPipeline_ChangedCIDUpdates(t *testing.T) {
	oldCID := "cid1"
	src := &fakeStandardSource{
		pub: testPublication(),
		docs: []standardfeed.Document{{
			URI: testDocURI, CID: "cid2", Site: testPubURI,
			Title: "Hello v2", Path: "/hello", PublishedAt: "2026-07-01T08:00:00Z",
		}},
	}
	q := &fakeStdQueries{diffRows: []db.ListFeedEntriesForDiffRow{{Guid: testDocURI, RecordCid: &oldCID}}}
	if err := newStdPipeline(src, q).FetchAndStore(context.Background(), testPubURI); err != nil {
		t.Fatalf("FetchAndStore: %v", err)
	}
	if len(q.entryUpserts) != 1 {
		t.Fatalf("entry upserts: %+v", q.entryUpserts)
	}
	e := q.entryUpserts[0]
	if e.RecordCid == nil || *e.RecordCid != "cid2" {
		t.Fatalf("record cid: %+v", e.RecordCid)
	}
	if e.ExtractedBody != nil {
		t.Fatalf("CID change on path-ful doc must reset extracted_body to NULL, got %+v", e.ExtractedBody)
	}
}

func TestStandardPipeline_PathlessDocument(t *testing.T) {
	src := &fakeStandardSource{
		pub: testPublication(),
		docs: []standardfeed.Document{{
			URI: testDocURI, CID: "cid1", Site: testPubURI,
			Title: "Loose", TextContent: "first para\n\nsecond <para>", PublishedAt: "2026-07-01T08:00:00Z",
		}},
	}
	q := &fakeStdQueries{}
	if err := newStdPipeline(src, q).FetchAndStore(context.Background(), testPubURI); err != nil {
		t.Fatalf("FetchAndStore: %v", err)
	}
	e := q.entryUpserts[0]
	if e.Url != "" {
		t.Fatalf("path-less doc must have empty url, got %q", e.Url)
	}
	if e.ExtractedBody == nil {
		t.Fatal("path-less doc must prefill extracted_body")
	}
	if want := "<p>first para</p><p>second &lt;para&gt;</p>"; *e.ExtractedBody != want {
		t.Fatalf("extracted_body = %q, want %q", *e.ExtractedBody, want)
	}
	if e.ContentHtml == nil || !strings.Contains(*e.ContentHtml, "first para") {
		t.Fatalf("summary should fall back to textContent: %+v", e.ContentHtml)
	}
}

func TestStandardPipeline_MissingDocumentHardDeletes(t *testing.T) {
	keepCID := "cid1"
	goneCID := "cid9"
	goneURI := "at://did:plc:pub123/site.standard.document/3gone"
	src := &fakeStandardSource{
		pub: testPublication(),
		docs: []standardfeed.Document{{
			URI: testDocURI, CID: keepCID, Site: testPubURI,
			Title: "Hello", Path: "/hello", PublishedAt: "2026-07-01T08:00:00Z",
		}},
	}
	q := &fakeStdQueries{diffRows: []db.ListFeedEntriesForDiffRow{
		{Guid: testDocURI, RecordCid: &keepCID},
		{Guid: goneURI, RecordCid: &goneCID},
	}}
	if err := newStdPipeline(src, q).FetchAndStore(context.Background(), testPubURI); err != nil {
		t.Fatalf("FetchAndStore: %v", err)
	}
	if len(q.deletes) != 1 || q.deletes[0].Guid != goneURI || q.deletes[0].FeedUrl != testPubURI {
		t.Fatalf("deletes: %+v", q.deletes)
	}
}

func TestStandardPipeline_ResolveFailureNeverDeletes(t *testing.T) {
	cid := "cid1"
	q := &fakeStdQueries{diffRows: []db.ListFeedEntriesForDiffRow{{Guid: testDocURI, RecordCid: &cid}}}

	src := &fakeStandardSource{pubErr: errors.New("pds down")}
	if err := newStdPipeline(src, q).FetchAndStore(context.Background(), testPubURI); err == nil {
		t.Fatal("expected error from publication resolve failure")
	}
	if len(q.deletes) != 0 || len(q.entryUpserts) != 0 || len(q.feedUpserts) != 0 {
		t.Fatalf("resolve failure must not mutate: %+v", q)
	}

	src = &fakeStandardSource{pub: testPublication(), listErr: errors.New("timeout")}
	if err := newStdPipeline(src, q).FetchAndStore(context.Background(), testPubURI); err == nil {
		t.Fatal("expected error from list failure")
	}
	if len(q.deletes) != 0 || len(q.entryUpserts) != 0 {
		t.Fatalf("list failure must not touch entries: deletes=%+v upserts=%+v", q.deletes, q.entryUpserts)
	}
}

func TestStandardPipeline_SkipsMalformedDocuments(t *testing.T) {
	src := &fakeStandardSource{
		pub: testPublication(),
		docs: []standardfeed.Document{
			{URI: testDocURI, CID: "c1", Site: testPubURI, Path: "/x", PublishedAt: "2026-07-01T08:00:00Z"}, // no title
			{URI: testDocURI + "2", CID: "c2", Site: testPubURI, Title: "ok"},                               // no publishedAt
		},
	}
	q := &fakeStdQueries{}
	if err := newStdPipeline(src, q).FetchAndStore(context.Background(), testPubURI); err != nil {
		t.Fatalf("FetchAndStore: %v", err)
	}
	if len(q.entryUpserts) != 0 {
		t.Fatalf("malformed docs must be skipped: %+v", q.entryUpserts)
	}
}

func TestStandardPipeline_MalformedDocKeepsCachedEntry(t *testing.T) {
	// A doc that comes back malformed but still exists upstream must not be swept as a delete.
	cid := "c1"
	src := &fakeStandardSource{
		pub: testPublication(),
		docs: []standardfeed.Document{
			{URI: testDocURI, CID: "c1", Site: testPubURI, Path: "/x", PublishedAt: "2026-07-01T08:00:00Z"}, // no title
		},
	}
	q := &fakeStdQueries{
		diffRows: []db.ListFeedEntriesForDiffRow{{Guid: testDocURI, RecordCid: &cid}},
	}
	if err := newStdPipeline(src, q).FetchAndStore(context.Background(), testPubURI); err != nil {
		t.Fatalf("FetchAndStore: %v", err)
	}
	if len(q.deletes) != 0 {
		t.Fatalf("malformed-but-present doc must not be deleted: %+v", q.deletes)
	}
	if len(q.entryUpserts) != 0 {
		t.Fatalf("malformed doc must not be upserted: %+v", q.entryUpserts)
	}
}

func TestSourceRouter_RoutesByKeyScheme(t *testing.T) {
	rss := &recordingFetcher{}
	std := &recordingFetcher{}
	router := NewSourceRouter(rss, std)

	if err := router.FetchAndStore(context.Background(), "https://example.com/feed.xml"); err != nil {
		t.Fatalf("rss route: %v", err)
	}
	if err := router.FetchAndStore(context.Background(), testPubURI); err != nil {
		t.Fatalf("standard route: %v", err)
	}
	if len(rss.seen) != 1 || rss.seen[0] != "https://example.com/feed.xml" {
		t.Fatalf("rss fetcher got %v", rss.seen)
	}
	if len(std.seen) != 1 || std.seen[0] != testPubURI {
		t.Fatalf("standard fetcher got %v", std.seen)
	}
}
