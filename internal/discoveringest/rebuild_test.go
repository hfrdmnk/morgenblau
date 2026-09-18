package discoveringest

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/bluesky-social/indigo/atproto/identity"
	"github.com/bluesky-social/indigo/atproto/syntax"
	"github.com/bluesky-social/jetstream"

	"morgenblau/internal/database"
	"morgenblau/internal/database/db"
	"morgenblau/internal/discovercrawl"
	"morgenblau/internal/discoverrank"
	"morgenblau/internal/lexicon"
	"morgenblau/internal/standardfeed"
)

const rebuildSchema = `
CREATE TABLE tap_records (
    did         TEXT NOT NULL,
    collection  TEXT NOT NULL,
    rkey        TEXT NOT NULL,
    cid         TEXT NOT NULL,
    record      TEXT NOT NULL,
    indexed_at  TEXT NOT NULL,
    PRIMARY KEY (did, collection, rkey)
);
CREATE TABLE tap_dirty_repos (
    did        TEXT PRIMARY KEY,
    marked_seq INTEGER NOT NULL
);
CREATE TABLE tap_repo_states (
    did        TEXT PRIMARY KEY,
    handle     TEXT NOT NULL,
    is_active  INTEGER NOT NULL,
    status     TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
CREATE TABLE discover_ingest_cursor (
    id         INTEGER PRIMARY KEY CHECK (id = 1),
    seq        INTEGER NOT NULL,
    updated_at TEXT NOT NULL
);
CREATE TABLE discover_trending_signals (
    repo_did    TEXT NOT NULL,
    source_key  TEXT NOT NULL,
    kind        TEXT NOT NULL,
    title       TEXT,
    site_url    TEXT,
    signal_kind TEXT NOT NULL,
    signal_at   TEXT,
    fetched_at  TEXT NOT NULL,
    PRIMARY KEY (repo_did, source_key)
);
CREATE TABLE discover_trending_follows (
    repo_did    TEXT NOT NULL,
    subject_did TEXT NOT NULL,
    fetched_at  TEXT NOT NULL,
    PRIMARY KEY (repo_did, subject_did)
);
CREATE TABLE discover_trending_source_counts (
    source_key     TEXT PRIMARY KEY,
    distinct_repos INTEGER NOT NULL
);
CREATE TABLE discover_trending_follow_counts (
    subject_did    TEXT PRIMARY KEY,
    distinct_repos INTEGER NOT NULL
);
`

func openRebuildTestDB(t *testing.T) *database.DB {
	t.Helper()
	t.Setenv("DB_PATH", filepath.Join(t.TempDir(), "test.db"))
	dbs, err := database.Open()
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { dbs.Close() })
	if _, err := dbs.Writer.ExecContext(context.Background(), rebuildSchema); err != nil {
		t.Fatalf("schema: %v", err)
	}
	return dbs
}

func seedMirror(t *testing.T, dbs *database.DB, did, collection, rkey, record string) {
	t.Helper()
	if err := db.New(dbs.Writer).UpsertTapRecord(context.Background(), db.UpsertTapRecordParams{
		Did: did, Collection: collection, Rkey: rkey, Cid: testCID, Record: record, IndexedAt: "2026-07-10T00:00:00Z",
	}); err != nil {
		t.Fatalf("UpsertTapRecord: %v", err)
	}
}

func markDirty(t *testing.T, dbs *database.DB, did string, markedSeq int64) {
	t.Helper()
	if err := db.New(dbs.Writer).MarkTapRepoDirty(context.Background(), db.MarkTapRepoDirtyParams{Did: did, MarkedSeq: markedSeq}); err != nil {
		t.Fatalf("MarkTapRepoDirty: %v", err)
	}
}

func seedRepoState(t *testing.T, dbs *database.DB, did, handle string, active bool, status string) {
	t.Helper()
	var isActive int64
	if active {
		isActive = 1
	}
	if err := db.New(dbs.Writer).UpsertTapRepoState(context.Background(), db.UpsertTapRepoStateParams{
		Did: did, Handle: handle, IsActive: isActive, Status: status, UpdatedAt: "2026-07-10T00:00:00Z",
	}); err != nil {
		t.Fatalf("UpsertTapRepoState: %v", err)
	}
}

// noEntries is the Tier-2 provenance resolver; the fixtures all carry their own feedUrl, so nothing has to be looked up.
type noEntries struct{}

func (noEntries) GetFeedURLByGuid(context.Context, string) (string, error) {
	return "", errors.New("not found")
}

func (noEntries) GetFeedURLByItemURL(context.Context, string) (string, error) {
	return "", errors.New("not found")
}

// hookEntries re-dirties a repo the moment the rebuild consults Tier-2, which happens before the write transaction opens.
type hookEntries struct {
	once    sync.Once
	hook    func()
	feedURL string
}

func (h *hookEntries) GetFeedURLByGuid(context.Context, string) (string, error) {
	h.once.Do(h.hook)
	if h.feedURL != "" {
		return h.feedURL, nil
	}
	return "", errors.New("not found")
}

func (h *hookEntries) GetFeedURLByItemURL(context.Context, string) (string, error) {
	h.once.Do(h.hook)
	if h.feedURL != "" {
		return h.feedURL, nil
	}
	return "", errors.New("not found")
}

// stubDirectory stands in for the SSRF-guarded identity directory.
type stubDirectory struct {
	handle syntax.Handle
	err    error
	calls  int
}

func (f *stubDirectory) LookupDID(_ context.Context, did syntax.DID) (*identity.Identity, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	return &identity.Identity{DID: did, Handle: f.handle}, nil
}

// stubDecoder records what the worker handed it, standing in for the decode paths already covered in internal/discovercrawl.
type stubDecoder struct {
	subs       []discovercrawl.Subscription
	pubs       []discovercrawl.AuthoredPublication
	gotHandle  syntax.Handle
	gotDID     syntax.DID
	authoredIn int
}

func (s *stubDecoder) DecodeSubscriptions(context.Context, map[string][]discovercrawl.RecordEntry) []discovercrawl.Subscription {
	return s.subs
}

func (s *stubDecoder) DecodeAuthoredPublications(_ context.Context, byCollection map[string][]discovercrawl.RecordEntry, did syntax.DID, handle syntax.Handle) ([]discovercrawl.AuthoredPublication, error) {
	s.authoredIn = len(byCollection[standardfeed.CollectionPublication])
	s.gotDID = did
	s.gotHandle = handle
	return s.pubs, nil
}

func newTestWorker(t *testing.T, dbs *database.DB, decoder RecordDecoder, entries EntryResolver, resolver Resolver) *RebuildWorker {
	t.Helper()
	w := NewRebuildWorker(db.New(dbs.Reader), decoder, resolver, entries).WithTxRunner(dbs.Writer)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if err := w.Shutdown(ctx); err != nil {
			t.Errorf("Shutdown: %v", err)
		}
	})
	return w
}

func signalsByKey(t *testing.T, dbs *database.DB) map[string]db.DiscoverTrendingSignal {
	t.Helper()
	rows, err := db.New(dbs.Reader).ListDiscoverTrendingSignals(context.Background())
	if err != nil {
		t.Fatalf("ListDiscoverTrendingSignals: %v", err)
	}
	out := map[string]db.DiscoverTrendingSignal{}
	for _, r := range rows {
		out[r.SourceKey] = r
	}
	return out
}

func TestRebuildWorker_RebuildsMirroredRecordsIntoSignalsAndFollows(t *testing.T) {
	dbs := openRebuildTestDB(t)
	seedMirror(t, dbs, testDID, lexicon.Subscription, "3sub",
		`{"source":{"$type":"blue.morgen.feed.subscription#rssFeed","feedUrl":"https://a.example/feed","siteUrl":"https://a.example"},"title":"Example Publication","createdAt":"2026-07-01T00:00:00Z"}`)
	seedMirror(t, dbs, testDID, lexicon.Save, "3sav",
		`{"itemUrl":"https://b.example/post","feedUrl":"https://b.example/feed","createdAt":"2026-07-02T00:00:00Z"}`)
	seedMirror(t, dbs, testDID, lexicon.Share, "3sha",
		`{"itemUrl":"https://c.example/post","feedUrl":"https://c.example/feed","createdAt":"2026-07-03T00:00:00Z"}`)
	seedMirror(t, dbs, testDID, lexicon.Follow, "3fol", fmt.Sprintf(`{"subject":%q,"createdAt":"2026-07-04T00:00:00Z"}`, subjectDID))
	markDirty(t, dbs, testDID, 10)

	w := newTestWorker(t, dbs, discovercrawl.NewClient(nil, nil, nil, nil, nil), noEntries{}, &stubDirectory{})
	w.drain(context.Background())

	got := signalsByKey(t, dbs)
	if len(got) != 3 {
		t.Fatalf("signals = %+v, want one per source", got)
	}
	if s := got["https://a.example/feed"]; s.SignalKind != "subscribe" || s.Title == nil || *s.Title != "Example Publication" {
		t.Errorf("subscription signal = %+v", s)
	}
	// The signal timestamp comes from the record's own createdAt, never from when the stream witnessed it.
	if s := got["https://a.example/feed"]; s.SignalAt == nil || *s.SignalAt != "2026-07-01T00:00:00Z" {
		t.Errorf("subscription signal_at = %v, want the record's createdAt", s.SignalAt)
	}
	if s := got["https://b.example/feed"]; s.SignalKind != "save" {
		t.Errorf("save signal = %+v", s)
	}
	if s := got["https://c.example/feed"]; s.SignalKind != "share" {
		t.Errorf("share signal = %+v", s)
	}

	follows, err := db.New(dbs.Reader).ListDiscoverTrendingFollows(context.Background())
	if err != nil {
		t.Fatalf("ListDiscoverTrendingFollows: %v", err)
	}
	if len(follows) != 1 || follows[0].SubjectDid != subjectDID {
		t.Fatalf("follows = %+v, want one row for %s", follows, subjectDID)
	}

	dirty, err := db.New(dbs.Reader).ListTapDirtyRepos(context.Background(), 10)
	if err != nil {
		t.Fatalf("ListTapDirtyRepos: %v", err)
	}
	if len(dirty) != 0 {
		t.Errorf("dirty = %+v, want the consumed mark cleared", dirty)
	}
}

func TestRebuildWorker_ReplacesRatherThanAccumulates(t *testing.T) {
	dbs := openRebuildTestDB(t)
	seedMirror(t, dbs, testDID, lexicon.Subscription, "3sub",
		`{"source":{"$type":"blue.morgen.feed.subscription#rssFeed","feedUrl":"https://old.example/feed"},"createdAt":"2026-07-01T00:00:00Z"}`)
	markDirty(t, dbs, testDID, 10)

	w := newTestWorker(t, dbs, discovercrawl.NewClient(nil, nil, nil, nil, nil), noEntries{}, &stubDirectory{})
	w.drain(context.Background())

	if err := db.New(dbs.Writer).DeleteTapRecord(context.Background(), db.DeleteTapRecordParams{
		Did: testDID, Collection: lexicon.Subscription, Rkey: "3sub",
	}); err != nil {
		t.Fatalf("DeleteTapRecord: %v", err)
	}
	seedMirror(t, dbs, testDID, lexicon.Subscription, "3new",
		`{"source":{"$type":"blue.morgen.feed.subscription#rssFeed","feedUrl":"https://new.example/feed"},"createdAt":"2026-07-11T00:00:00Z"}`)
	markDirty(t, dbs, testDID, 11)
	w.drain(context.Background())

	got := signalsByKey(t, dbs)
	if len(got) != 1 {
		t.Fatalf("signals = %+v, want only the current subscription", got)
	}
	if _, ok := got["https://new.example/feed"]; !ok {
		t.Errorf("signals = %+v, want the new source key", got)
	}
}

func TestRebuildWorker_RepoReDirtiedMidRebuildStaysQueued(t *testing.T) {
	dbs := openRebuildTestDB(t)
	seedMirror(t, dbs, testDID, lexicon.Save, "3sav", `{"itemUrl":"https://b.example/post","createdAt":"2026-07-02T00:00:00Z"}`)
	markDirty(t, dbs, testDID, 10)

	consumer := newConsumer(Config{}, requestCursorReader{}, nil).WithTxRunner(dbs.Writer)
	entries := &hookEntries{
		feedURL: "https://old.example/feed",
		hook: func() {
			err := consumer.foldBatch(context.Background(), sourceBatch{
				cursor: 11,
				events: []jetstream.Event{commitEvent(11, "3later", map[string]any{
					"$type": "blue.morgen.feed.save", "itemUrl": "https://later.example/post", "feedUrl": "https://later.example/feed",
				})},
			})
			if err != nil {
				t.Fatalf("fold later event: %v", err)
			}
		},
	}
	w := newTestWorker(t, dbs, discovercrawl.NewClient(nil, nil, nil, nil, nil), entries, &stubDirectory{})
	w.drain(context.Background())
	got := signalsByKey(t, dbs)
	if _, ok := got["https://old.example/feed"]; !ok {
		t.Fatalf("signals = %+v, want the old snapshot committed", got)
	}
	if _, ok := got["https://later.example/feed"]; ok {
		t.Fatalf("signals = %+v, later mirror event belongs to the next rebuild", got)
	}

	dirty, err := db.New(dbs.Reader).ListTapDirtyRepos(context.Background(), 10)
	if err != nil {
		t.Fatalf("ListTapDirtyRepos: %v", err)
	}
	if len(dirty) != 1 || dirty[0].MarkedSeq != 11 {
		t.Fatalf("dirty = %+v, want the newer mark to survive the rebuild", dirty)
	}
}

func TestDirtyGenerationCannotMoveBackwardAndExactGenerationClears(t *testing.T) {
	dbs := openRebuildTestDB(t)
	markDirty(t, dbs, testDID, 11)
	markDirty(t, dbs, testDID, 10)

	q := db.New(dbs.Writer)
	rows, err := q.ListTapDirtyRepos(context.Background(), 10)
	if err != nil {
		t.Fatalf("ListTapDirtyRepos: %v", err)
	}
	if len(rows) != 1 || rows[0].MarkedSeq != 11 {
		t.Fatalf("dirty = %+v, want generation 11", rows)
	}
	if err := q.DeleteTapDirtyRepo(context.Background(), db.DeleteTapDirtyRepoParams{Did: testDID, MarkedSeq: 10}); err != nil {
		t.Fatalf("DeleteTapDirtyRepo stale: %v", err)
	}
	rows, err = q.ListTapDirtyRepos(context.Background(), 10)
	if err != nil || len(rows) != 1 {
		t.Fatalf("stale cleanup removed row: rows=%+v err=%v", rows, err)
	}
	if err := q.DeleteTapDirtyRepo(context.Background(), db.DeleteTapDirtyRepoParams{Did: testDID, MarkedSeq: 11}); err != nil {
		t.Fatalf("DeleteTapDirtyRepo exact: %v", err)
	}
	rows, err = q.ListTapDirtyRepos(context.Background(), 10)
	if err != nil || len(rows) != 0 {
		t.Fatalf("exact cleanup did not remove row: rows=%+v err=%v", rows, err)
	}
}

func TestRebuildWorker_RefreshesTrendingCountsSoBarReadsSeeRows(t *testing.T) {
	dbs := openRebuildTestDB(t)
	for _, repo := range []string{testDID, otherDID, thirdDID} {
		seedMirror(t, dbs, repo, lexicon.Subscription, "3sub",
			`{"source":{"$type":"blue.morgen.feed.subscription#rssFeed","feedUrl":"https://shared.example/feed"},"createdAt":"2026-07-01T00:00:00Z"}`)
		seedMirror(t, dbs, repo, lexicon.Follow, "3fol", fmt.Sprintf(`{"subject":%q}`, subjectDID))
		markDirty(t, dbs, repo, 10)
	}

	w := newTestWorker(t, dbs, discovercrawl.NewClient(nil, nil, nil, nil, nil), noEntries{}, &stubDirectory{})
	w.drain(context.Background())

	q := db.New(dbs.Reader)
	signalRows, err := q.ListDiscoverTrendingSignalsAboveBar(context.Background(), discoverrank.MinDistinctRepos)
	if err != nil {
		t.Fatalf("ListDiscoverTrendingSignalsAboveBar: %v", err)
	}
	if len(signalRows) != 3 {
		t.Errorf("above-bar signal rows = %d, want 3 (counts refresh must make the bar read live)", len(signalRows))
	}
	followRows, err := q.ListDiscoverTrendingFollowsAboveBar(context.Background(), discoverrank.MinDistinctRepos)
	if err != nil {
		t.Fatalf("ListDiscoverTrendingFollowsAboveBar: %v", err)
	}
	if len(followRows) != 3 {
		t.Errorf("above-bar follow rows = %d, want 3", len(followRows))
	}
}

func TestRebuildWorker_InvalidatesCachesOncePerProductiveDrain(t *testing.T) {
	dbs := openRebuildTestDB(t)
	for _, repo := range []string{testDID, otherDID} {
		seedMirror(t, dbs, repo, lexicon.Subscription, "3sub",
			`{"source":{"$type":"blue.morgen.feed.subscription#rssFeed","feedUrl":"https://shared.example/feed"},"createdAt":"2026-07-01T00:00:00Z"}`)
		markDirty(t, dbs, repo, 10)
	}

	calls := 0
	w := newTestWorker(t, dbs, discovercrawl.NewClient(nil, nil, nil, nil, nil), noEntries{}, &stubDirectory{})
	w.WithInvalidator(func() { calls++ })

	w.drain(context.Background())
	if calls != 1 {
		t.Fatalf("invalidations = %d after a two-repo drain, want 1", calls)
	}
	w.drain(context.Background())
	if calls != 1 {
		t.Errorf("invalidations = %d after an empty drain, want it unchanged", calls)
	}
}

func TestRebuildWorker_FailedRepoStaysDirtyAndOthersStillRebuild(t *testing.T) {
	dbs := openRebuildTestDB(t)
	for _, repo := range []string{testDID, otherDID} {
		seedMirror(t, dbs, repo, lexicon.Subscription, "3sub",
			`{"source":{"$type":"blue.morgen.feed.subscription#rssFeed","feedUrl":"https://shared.example/feed"},"createdAt":"2026-07-01T00:00:00Z"}`)
		markDirty(t, dbs, repo, 10)
	}

	w := newTestWorker(t, dbs, discovercrawl.NewClient(nil, nil, nil, nil, nil), noEntries{}, &stubDirectory{})
	commit := w.runTx
	w.runTx = func(ctx context.Context, fn func(rebuildWriter) error) error {
		return commit(ctx, func(x rebuildWriter) error {
			return fn(&failingWriter{rebuildWriter: x, failFor: testDID})
		})
	}
	w.drain(context.Background())

	dirty, err := db.New(dbs.Reader).ListTapDirtyRepos(context.Background(), 10)
	if err != nil {
		t.Fatalf("ListTapDirtyRepos: %v", err)
	}
	if len(dirty) != 1 || dirty[0].Did != testDID {
		t.Fatalf("dirty = %+v, want only the failed repo left queued", dirty)
	}
	rows, err := db.New(dbs.Reader).ListDiscoverTrendingSignals(context.Background())
	if err != nil {
		t.Fatalf("ListDiscoverTrendingSignals: %v", err)
	}
	if len(rows) != 1 || rows[0].RepoDid != otherDID {
		t.Errorf("signals = %+v, want only the healthy repo's row", rows)
	}
}

// failingWriter fails the signal insert for one repo, leaving its dirty mark in place.
type failingWriter struct {
	rebuildWriter
	failFor string
}

func (f *failingWriter) InsertDiscoverTrendingSignal(ctx context.Context, arg db.InsertDiscoverTrendingSignalParams) error {
	if arg.RepoDid == f.failFor {
		return errors.New("insert refused")
	}
	return f.rebuildWriter.InsertDiscoverTrendingSignal(ctx, arg)
}

func TestRebuildWorker_ResolvesTheRepoHandleOnlyForPublicationBearingRepos(t *testing.T) {
	dbs := openRebuildTestDB(t)
	seedMirror(t, dbs, testDID, standardfeed.CollectionPublication, "3pub", `{"name":"Example Zine","url":"https://zine.example"}`)
	seedMirror(t, dbs, otherDID, lexicon.Save, "3sav", `{"itemUrl":"https://b.example/post","feedUrl":"https://b.example/feed"}`)
	markDirty(t, dbs, testDID, 10)
	markDirty(t, dbs, otherDID, 11)

	resolver := &stubDirectory{handle: syntax.Handle("reader.example")}
	decoder := &stubDecoder{}
	w := newTestWorker(t, dbs, decoder, noEntries{}, resolver)
	w.drain(context.Background())

	if resolver.calls != 1 {
		t.Errorf("identity lookups = %d, want 1 (only the repo with publication rows needs a handle)", resolver.calls)
	}
	if decoder.gotHandle != syntax.Handle("reader.example") || decoder.gotDID.String() != testDID {
		t.Errorf("decoder got did=%q handle=%q", decoder.gotDID, decoder.gotHandle)
	}
	if decoder.authoredIn != 1 {
		t.Errorf("publication rows handed to the decoder = %d, want 1", decoder.authoredIn)
	}
}

// Without a handle the well-known authority check can only match the DID form, so the repo stays queued rather than being written short a signal.
func TestRebuildWorker_IdentityFailureLeavesPublicationRepoDirty(t *testing.T) {
	dbs := openRebuildTestDB(t)
	seedMirror(t, dbs, testDID, standardfeed.CollectionPublication, "3pub", `{"name":"Example Zine","url":"https://zine.example"}`)
	markDirty(t, dbs, testDID, 10)

	w := newTestWorker(t, dbs, &stubDecoder{}, noEntries{}, &stubDirectory{err: errors.New("directory down")})
	w.drain(context.Background())

	dirty, err := db.New(dbs.Reader).ListTapDirtyRepos(context.Background(), 10)
	if err != nil {
		t.Fatalf("ListTapDirtyRepos: %v", err)
	}
	if len(dirty) != 1 {
		t.Errorf("dirty = %+v, want the repo left queued for retry", dirty)
	}
}

func TestRebuildWorker_InactiveRepoClearsAggregatesWithoutPurgingMirrorOrResolvingIdentity(t *testing.T) {
	dbs := openRebuildTestDB(t)
	seedMirror(t, dbs, testDID, lexicon.Subscription, "3sub",
		`{"source":{"$type":"blue.morgen.feed.subscription#rssFeed","feedUrl":"https://zine.example/feed"},"createdAt":"2026-07-01T00:00:00Z"}`)
	seedRepoState(t, dbs, testDID, "reader.example", false, "suspended")
	markDirty(t, dbs, testDID, 10)
	if err := db.New(dbs.Writer).InsertDiscoverTrendingSignal(context.Background(), db.InsertDiscoverTrendingSignalParams{
		RepoDid: testDID, SourceKey: "https://zine.example/feed",
		Kind: "rss", SignalKind: "subscribe", FetchedAt: "2026-07-09T00:00:00Z",
	}); err != nil {
		t.Fatalf("seed signal: %v", err)
	}

	resolver := &stubDirectory{err: errors.New("must not resolve inactive repo")}
	decoder := &stubDecoder{subs: []discovercrawl.Subscription{{
		Key: "https://zine.example/feed", Kind: "rss", CreatedAt: "2026-07-01T00:00:00Z",
	}}}
	w := newTestWorker(t, dbs, decoder, noEntries{}, resolver)
	w.drain(context.Background())

	if got := signalsByKey(t, dbs); len(got) != 0 {
		t.Fatalf("signals = %+v, want inactive repo removed from discovery", got)
	}
	rows, err := db.New(dbs.Reader).ListTapRecordsForRepo(context.Background(), testDID)
	if err != nil {
		t.Fatalf("ListTapRecordsForRepo: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("mirror rows = %d, want retained for reactivation", len(rows))
	}
	if resolver.calls != 0 {
		t.Fatalf("identity lookups = %d, want 0 for inactive repo", resolver.calls)
	}

	seedRepoState(t, dbs, testDID, "reader.example", true, "active")
	markDirty(t, dbs, testDID, 11)
	w.drain(context.Background())
	if got := signalsByKey(t, dbs); len(got) != 1 {
		t.Fatalf("signals after reactivation = %+v, want rebuilt retained subscription", got)
	}
}

func TestRebuildWorker_ActiveRepoUsesMirroredHandleWithoutIdentityLookup(t *testing.T) {
	dbs := openRebuildTestDB(t)
	seedMirror(t, dbs, testDID, standardfeed.CollectionPublication, "3pub", `{"name":"Example Zine","url":"https://zine.example"}`)
	seedRepoState(t, dbs, testDID, "new-handle.example", true, "active")
	markDirty(t, dbs, testDID, 10)

	decoder := &stubDecoder{}
	resolver := &stubDirectory{err: errors.New("the mirrored handle should avoid a lookup")}
	w := newTestWorker(t, dbs, decoder, noEntries{}, resolver)
	w.drain(context.Background())

	if decoder.gotHandle != syntax.Handle("new-handle.example") {
		t.Fatalf("decoder handle = %q, want the mirrored identity handle", decoder.gotHandle)
	}
	if resolver.calls != 0 {
		t.Fatalf("identity lookups = %d, want 0", resolver.calls)
	}
}

func TestRebuildWorker_InvalidMirroredHandleFallsBackToIdentityLookup(t *testing.T) {
	dbs := openRebuildTestDB(t)
	seedMirror(t, dbs, testDID, standardfeed.CollectionPublication, "3pub", `{"name":"Example Zine","url":"https://zine.example"}`)
	seedRepoState(t, dbs, testDID, "not a handle", true, "active")
	markDirty(t, dbs, testDID, 10)

	decoder := &stubDecoder{}
	resolver := &stubDirectory{handle: syntax.Handle("resolved.example")}
	w := newTestWorker(t, dbs, decoder, noEntries{}, resolver)
	w.drain(context.Background())

	if decoder.gotHandle != syntax.Handle("resolved.example") {
		t.Fatalf("decoder handle = %q, want resolved fallback", decoder.gotHandle)
	}
	if resolver.calls != 1 {
		t.Fatalf("identity lookups = %d, want 1", resolver.calls)
	}
}

type mutableWellKnown struct {
	value string
	err   error
}

func (f *mutableWellKnown) FetchWellKnown(context.Context, string) (string, error) {
	return f.value, f.err
}

func TestRebuildWorker_ProbeFailurePreservesVerifiedSignalAndDirtyMarkUntilRetry(t *testing.T) {
	dbs := openRebuildTestDB(t)
	uri := "at://" + testDID + "/" + standardfeed.CollectionPublication + "/3pub"
	seedMirror(t, dbs, testDID, standardfeed.CollectionPublication, "3pub", `{"name":"Example Zine","url":"https://zine.example"}`)
	markDirty(t, dbs, testDID, 10)
	if err := db.New(dbs.Writer).InsertDiscoverTrendingSignal(context.Background(), db.InsertDiscoverTrendingSignalParams{
		RepoDid: testDID, SourceKey: uri, Kind: "standardfeed", SignalKind: "author", FetchedAt: "2026-07-09T00:00:00Z",
	}); err != nil {
		t.Fatalf("seed signal: %v", err)
	}

	probe := &mutableWellKnown{err: errors.New("site unavailable")}
	decoder := discovercrawl.NewClient(nil, nil, nil, probe, nil)
	w := newTestWorker(t, dbs, decoder, noEntries{}, &stubDirectory{handle: syntax.Handle("reader.example")})
	w.drain(context.Background())

	if got := signalsByKey(t, dbs); len(got) != 1 {
		t.Fatalf("signals after probe failure = %+v, want previous verified signal", got)
	}
	dirty, err := db.New(dbs.Reader).ListTapDirtyRepos(context.Background(), 10)
	if err != nil {
		t.Fatalf("ListTapDirtyRepos: %v", err)
	}
	if len(dirty) != 1 {
		t.Fatalf("dirty after probe failure = %+v, want queued retry", dirty)
	}

	probe.err = nil
	probe.value = uri
	w.drain(context.Background())
	if got := signalsByKey(t, dbs); len(got) != 1 {
		t.Fatalf("signals after successful retry = %+v, want rebuilt verified signal", got)
	}
	dirty, err = db.New(dbs.Reader).ListTapDirtyRepos(context.Background(), 10)
	if err != nil {
		t.Fatalf("ListTapDirtyRepos after retry: %v", err)
	}
	if len(dirty) != 0 {
		t.Fatalf("dirty after successful retry = %+v, want cleared", dirty)
	}
}

func TestRebuildWorker_TickerDrainsWithoutAnExplicitCall(t *testing.T) {
	dbs := openRebuildTestDB(t)
	seedMirror(t, dbs, testDID, lexicon.Subscription, "3sub",
		`{"source":{"$type":"blue.morgen.feed.subscription#rssFeed","feedUrl":"https://a.example/feed"},"createdAt":"2026-07-01T00:00:00Z"}`)
	markDirty(t, dbs, testDID, 10)

	w := newTestWorker(t, dbs, discovercrawl.NewClient(nil, nil, nil, nil, nil), noEntries{}, &stubDirectory{})
	w.interval = 5 * time.Millisecond
	w.Start()

	deadline := time.Now().Add(3 * time.Second)
	for {
		if len(signalsByKey(t, dbs)) > 0 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("ticker never drained the dirty repo")
		}
		time.Sleep(2 * time.Millisecond)
	}
}
