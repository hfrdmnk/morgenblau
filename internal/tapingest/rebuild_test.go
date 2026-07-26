package tapingest

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
    did       TEXT PRIMARY KEY,
    marked_at TEXT NOT NULL
);
CREATE TABLE tap_repo_states (
    did        TEXT PRIMARY KEY,
    handle     TEXT NOT NULL,
    is_active  INTEGER NOT NULL,
    status     TEXT NOT NULL,
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

const (
	repoA     = "did:plc:aaaaaaaaaaaaaaaaaaaaaaaa"
	repoB     = "did:plc:bbbbbbbbbbbbbbbbbbbbbbbb"
	repoC     = "did:plc:cccccccccccccccccccccccc"
	subjectID = "did:plc:dddddddddddddddddddddddd"
)

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
	q := db.New(dbs.Writer)
	if err := q.UpsertTapRecord(context.Background(), db.UpsertTapRecordParams{
		Did: did, Collection: collection, Rkey: rkey, Cid: testCID, Record: record, IndexedAt: "2026-07-10T00:00:00Z",
	}); err != nil {
		t.Fatalf("UpsertTapRecord: %v", err)
	}
}

func markDirty(t *testing.T, dbs *database.DB, did, markedAt string) {
	t.Helper()
	if err := db.New(dbs.Writer).MarkTapRepoDirty(context.Background(), db.MarkTapRepoDirtyParams{Did: did, MarkedAt: markedAt}); err != nil {
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

func (noEntries) GetFeedURLByGuid(ctx context.Context, guid string) (string, error) {
	return "", errors.New("not found")
}

func (noEntries) GetFeedURLByItemURL(ctx context.Context, url string) (string, error) {
	return "", errors.New("not found")
}

// hookEntries re-dirties a repo the moment the rebuild consults Tier-2, which happens before the write transaction opens.
type hookEntries struct {
	once sync.Once
	hook func()
}

func (h *hookEntries) GetFeedURLByGuid(ctx context.Context, guid string) (string, error) {
	h.once.Do(h.hook)
	return "", errors.New("not found")
}

func (h *hookEntries) GetFeedURLByItemURL(ctx context.Context, url string) (string, error) {
	h.once.Do(h.hook)
	return "", errors.New("not found")
}

type fakeResolver struct {
	handle syntax.Handle
	err    error
	calls  int
}

func (f *fakeResolver) LookupDID(ctx context.Context, did syntax.DID) (*identity.Identity, error) {
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

func (s *stubDecoder) DecodeSubscriptions(ctx context.Context, byCollection map[string][]discovercrawl.RecordEntry) []discovercrawl.Subscription {
	return s.subs
}

func (s *stubDecoder) DecodeAuthoredPublications(ctx context.Context, byCollection map[string][]discovercrawl.RecordEntry, did syntax.DID, handle syntax.Handle) ([]discovercrawl.AuthoredPublication, error) {
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
	seedMirror(t, dbs, repoA, lexicon.Subscription, "3sub",
		`{"source":{"$type":"blue.morgen.feed.subscription#rssFeed","feedUrl":"https://a.example/feed","siteUrl":"https://a.example"},"title":"Example Publication","createdAt":"2026-07-01T00:00:00Z"}`)
	seedMirror(t, dbs, repoA, lexicon.Save, "3sav",
		`{"itemUrl":"https://b.example/post","feedUrl":"https://b.example/feed","createdAt":"2026-07-02T00:00:00Z"}`)
	seedMirror(t, dbs, repoA, lexicon.Share, "3sha",
		`{"itemUrl":"https://c.example/post","feedUrl":"https://c.example/feed","createdAt":"2026-07-03T00:00:00Z"}`)
	seedMirror(t, dbs, repoA, lexicon.Follow, "3fol", fmt.Sprintf(`{"subject":%q,"createdAt":"2026-07-04T00:00:00Z"}`, subjectID))
	markDirty(t, dbs, repoA, "2026-07-10T00:00:00Z")

	w := newTestWorker(t, dbs, discovercrawl.NewClient(nil, nil, nil, nil, nil), noEntries{}, &fakeResolver{})
	w.drain(context.Background())

	got := signalsByKey(t, dbs)
	if len(got) != 3 {
		t.Fatalf("signals = %+v, want one per source", got)
	}
	if s := got["https://a.example/feed"]; s.SignalKind != "subscribe" || s.Title == nil || *s.Title != "Example Publication" {
		t.Errorf("subscription signal = %+v", s)
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
	if len(follows) != 1 || follows[0].SubjectDid != subjectID {
		t.Fatalf("follows = %+v, want one row for %s", follows, subjectID)
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
	seedMirror(t, dbs, repoA, lexicon.Subscription, "3sub",
		`{"source":{"$type":"blue.morgen.feed.subscription#rssFeed","feedUrl":"https://old.example/feed"},"createdAt":"2026-07-01T00:00:00Z"}`)
	markDirty(t, dbs, repoA, "2026-07-10T00:00:00Z")

	w := newTestWorker(t, dbs, discovercrawl.NewClient(nil, nil, nil, nil, nil), noEntries{}, &fakeResolver{})
	w.drain(context.Background())

	if err := db.New(dbs.Writer).DeleteTapRecord(context.Background(), db.DeleteTapRecordParams{
		Did: repoA, Collection: lexicon.Subscription, Rkey: "3sub",
	}); err != nil {
		t.Fatalf("DeleteTapRecord: %v", err)
	}
	seedMirror(t, dbs, repoA, lexicon.Subscription, "3new",
		`{"source":{"$type":"blue.morgen.feed.subscription#rssFeed","feedUrl":"https://new.example/feed"},"createdAt":"2026-07-11T00:00:00Z"}`)
	markDirty(t, dbs, repoA, "2026-07-11T00:00:00Z")
	w.drain(context.Background())

	got := signalsByKey(t, dbs)
	if len(got) != 1 {
		t.Fatalf("signals = %+v, want only the current subscription", got)
	}
	if _, ok := got["https://new.example/feed"]; !ok {
		t.Errorf("signals = %+v, want the new source key", got)
	}
}

// The dirty-mark delete is guarded by marked_at, so a change that lands while a rebuild is in flight keeps the repo queued instead of being silently dropped.
func TestRebuildWorker_RepoReDirtiedMidRebuildStaysQueued(t *testing.T) {
	dbs := openRebuildTestDB(t)
	seedMirror(t, dbs, repoA, lexicon.Save, "3sav", `{"itemUrl":"https://b.example/post","createdAt":"2026-07-02T00:00:00Z"}`)
	markDirty(t, dbs, repoA, "2026-07-10T00:00:00Z")

	entries := &hookEntries{hook: func() { markDirty(t, dbs, repoA, "2026-07-10T00:00:05Z") }}
	w := newTestWorker(t, dbs, discovercrawl.NewClient(nil, nil, nil, nil, nil), entries, &fakeResolver{})
	w.drain(context.Background())

	dirty, err := db.New(dbs.Reader).ListTapDirtyRepos(context.Background(), 10)
	if err != nil {
		t.Fatalf("ListTapDirtyRepos: %v", err)
	}
	if len(dirty) != 1 || dirty[0].MarkedAt != "2026-07-10T00:00:05Z" {
		t.Fatalf("dirty = %+v, want the newer mark to survive the rebuild", dirty)
	}
}

func TestRebuildWorker_RefreshesTrendingCountsSoBarReadsSeeRows(t *testing.T) {
	dbs := openRebuildTestDB(t)
	for _, repo := range []string{repoA, repoB, repoC} {
		seedMirror(t, dbs, repo, lexicon.Subscription, "3sub",
			`{"source":{"$type":"blue.morgen.feed.subscription#rssFeed","feedUrl":"https://shared.example/feed"},"createdAt":"2026-07-01T00:00:00Z"}`)
		seedMirror(t, dbs, repo, lexicon.Follow, "3fol", fmt.Sprintf(`{"subject":%q}`, subjectID))
		markDirty(t, dbs, repo, "2026-07-10T00:00:00Z")
	}

	w := newTestWorker(t, dbs, discovercrawl.NewClient(nil, nil, nil, nil, nil), noEntries{}, &fakeResolver{})
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
	for _, repo := range []string{repoA, repoB} {
		seedMirror(t, dbs, repo, lexicon.Subscription, "3sub",
			`{"source":{"$type":"blue.morgen.feed.subscription#rssFeed","feedUrl":"https://shared.example/feed"},"createdAt":"2026-07-01T00:00:00Z"}`)
		markDirty(t, dbs, repo, "2026-07-10T00:00:00Z")
	}

	calls := 0
	w := newTestWorker(t, dbs, discovercrawl.NewClient(nil, nil, nil, nil, nil), noEntries{}, &fakeResolver{})
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
	for _, repo := range []string{repoA, repoB} {
		seedMirror(t, dbs, repo, lexicon.Subscription, "3sub",
			`{"source":{"$type":"blue.morgen.feed.subscription#rssFeed","feedUrl":"https://shared.example/feed"},"createdAt":"2026-07-01T00:00:00Z"}`)
		markDirty(t, dbs, repo, "2026-07-10T00:00:00Z")
	}

	w := newTestWorker(t, dbs, discovercrawl.NewClient(nil, nil, nil, nil, nil), noEntries{}, &fakeResolver{})
	commit := w.runTx
	w.runTx = func(ctx context.Context, fn func(RebuildWriter) error) error {
		return commit(ctx, func(x RebuildWriter) error {
			return fn(&failingWriter{RebuildWriter: x, failFor: repoA})
		})
	}
	w.drain(context.Background())

	dirty, err := db.New(dbs.Reader).ListTapDirtyRepos(context.Background(), 10)
	if err != nil {
		t.Fatalf("ListTapDirtyRepos: %v", err)
	}
	if len(dirty) != 1 || dirty[0].Did != repoA {
		t.Fatalf("dirty = %+v, want only the failed repo left queued", dirty)
	}
	rows, err := db.New(dbs.Reader).ListDiscoverTrendingSignals(context.Background())
	if err != nil {
		t.Fatalf("ListDiscoverTrendingSignals: %v", err)
	}
	if len(rows) != 1 || rows[0].RepoDid != repoB {
		t.Errorf("signals = %+v, want only the healthy repo's row", rows)
	}
}

// failingWriter fails the signal insert for one repo, leaving its dirty mark in place.
type failingWriter struct {
	RebuildWriter
	failFor string
}

func (f *failingWriter) InsertDiscoverTrendingSignal(ctx context.Context, arg db.InsertDiscoverTrendingSignalParams) error {
	if arg.RepoDid == f.failFor {
		return errors.New("insert refused")
	}
	return f.RebuildWriter.InsertDiscoverTrendingSignal(ctx, arg)
}

func TestRebuildWorker_ResolvesTheRepoHandleOnlyForPublicationBearingRepos(t *testing.T) {
	dbs := openRebuildTestDB(t)
	seedMirror(t, dbs, repoA, standardfeed.CollectionPublication, "3pub", `{"name":"Example Zine","url":"https://zine.example"}`)
	seedMirror(t, dbs, repoB, lexicon.Save, "3sav", `{"itemUrl":"https://b.example/post","feedUrl":"https://b.example/feed"}`)
	markDirty(t, dbs, repoA, "2026-07-10T00:00:00Z")
	markDirty(t, dbs, repoB, "2026-07-10T00:00:01Z")

	resolver := &fakeResolver{handle: syntax.Handle("reader.example")}
	decoder := &stubDecoder{}
	w := newTestWorker(t, dbs, decoder, noEntries{}, resolver)
	w.drain(context.Background())

	if resolver.calls != 1 {
		t.Errorf("identity lookups = %d, want 1 (only the repo with publication rows needs a handle)", resolver.calls)
	}
	if decoder.gotHandle != syntax.Handle("reader.example") || decoder.gotDID.String() != repoA {
		t.Errorf("decoder got did=%q handle=%q", decoder.gotDID, decoder.gotHandle)
	}
	if decoder.authoredIn != 1 {
		t.Errorf("publication rows handed to the decoder = %d, want 1", decoder.authoredIn)
	}
}

// Without a handle the well-known authority check can only match the DID form, so the repo stays queued rather than being written short a signal.
func TestRebuildWorker_IdentityFailureLeavesPublicationRepoDirty(t *testing.T) {
	dbs := openRebuildTestDB(t)
	seedMirror(t, dbs, repoA, standardfeed.CollectionPublication, "3pub", `{"name":"Example Zine","url":"https://zine.example"}`)
	markDirty(t, dbs, repoA, "2026-07-10T00:00:00Z")

	w := newTestWorker(t, dbs, &stubDecoder{}, noEntries{}, &fakeResolver{err: errors.New("directory down")})
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
	seedMirror(t, dbs, repoA, lexicon.Subscription, "3sub",
		`{"source":{"$type":"blue.morgen.feed.subscription#rssFeed","feedUrl":"https://zine.example/feed"},"createdAt":"2026-07-01T00:00:00Z"}`)
	seedRepoState(t, dbs, repoA, "reader.example", false, "suspended")
	markDirty(t, dbs, repoA, "2026-07-10T00:00:00Z")
	if err := db.New(dbs.Writer).InsertDiscoverTrendingSignal(context.Background(), db.InsertDiscoverTrendingSignalParams{
		RepoDid: repoA, SourceKey: "https://zine.example/feed",
		Kind: "rss", SignalKind: "subscribe", FetchedAt: "2026-07-09T00:00:00Z",
	}); err != nil {
		t.Fatalf("seed signal: %v", err)
	}

	resolver := &fakeResolver{err: errors.New("must not resolve inactive repo")}
	decoder := &stubDecoder{subs: []discovercrawl.Subscription{{
		Key: "https://zine.example/feed", Kind: "rss", CreatedAt: "2026-07-01T00:00:00Z",
	}}}
	w := newTestWorker(t, dbs, decoder, noEntries{}, resolver)
	w.drain(context.Background())

	if got := signalsByKey(t, dbs); len(got) != 0 {
		t.Fatalf("signals = %+v, want inactive repo removed from discovery", got)
	}
	rows, err := db.New(dbs.Reader).ListTapRecordsForRepo(context.Background(), repoA)
	if err != nil {
		t.Fatalf("ListTapRecordsForRepo: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("mirror rows = %d, want retained for reactivation", len(rows))
	}
	if resolver.calls != 0 {
		t.Fatalf("identity lookups = %d, want 0 for inactive repo", resolver.calls)
	}

	seedRepoState(t, dbs, repoA, "reader.example", true, "active")
	markDirty(t, dbs, repoA, "2026-07-10T00:00:01Z")
	w.drain(context.Background())
	if got := signalsByKey(t, dbs); len(got) != 1 {
		t.Fatalf("signals after reactivation = %+v, want rebuilt retained subscription", got)
	}
}

func TestRebuildWorker_ActiveRepoUsesTapHandleWithoutIdentityLookup(t *testing.T) {
	dbs := openRebuildTestDB(t)
	seedMirror(t, dbs, repoA, standardfeed.CollectionPublication, "3pub", `{"name":"Example Zine","url":"https://zine.example"}`)
	seedRepoState(t, dbs, repoA, "new-handle.example", true, "active")
	markDirty(t, dbs, repoA, "2026-07-10T00:00:00Z")

	decoder := &stubDecoder{}
	resolver := &fakeResolver{err: errors.New("stored Tap handle should avoid lookup")}
	w := newTestWorker(t, dbs, decoder, noEntries{}, resolver)
	w.drain(context.Background())

	if decoder.gotHandle != syntax.Handle("new-handle.example") {
		t.Fatalf("decoder handle = %q, want Tap identity handle", decoder.gotHandle)
	}
	if resolver.calls != 0 {
		t.Fatalf("identity lookups = %d, want 0", resolver.calls)
	}
}

func TestRebuildWorker_InvalidTapHandleFallsBackToIdentityLookup(t *testing.T) {
	dbs := openRebuildTestDB(t)
	seedMirror(t, dbs, repoA, standardfeed.CollectionPublication, "3pub", `{"name":"Example Zine","url":"https://zine.example"}`)
	seedRepoState(t, dbs, repoA, "not a handle", true, "active")
	markDirty(t, dbs, repoA, "2026-07-10T00:00:00Z")

	decoder := &stubDecoder{}
	resolver := &fakeResolver{handle: syntax.Handle("resolved.example")}
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

func (f *mutableWellKnown) FetchWellKnown(ctx context.Context, siteURL string) (string, error) {
	return f.value, f.err
}

func TestRebuildWorker_ProbeFailurePreservesVerifiedSignalAndDirtyMarkUntilRetry(t *testing.T) {
	dbs := openRebuildTestDB(t)
	uri := "at://" + repoA + "/" + standardfeed.CollectionPublication + "/3pub"
	seedMirror(t, dbs, repoA, standardfeed.CollectionPublication, "3pub", `{"name":"Example Zine","url":"https://zine.example"}`)
	markDirty(t, dbs, repoA, "2026-07-10T00:00:00Z")
	if err := db.New(dbs.Writer).InsertDiscoverTrendingSignal(context.Background(), db.InsertDiscoverTrendingSignalParams{
		RepoDid: repoA, SourceKey: uri, Kind: "standardfeed", SignalKind: "author", FetchedAt: "2026-07-09T00:00:00Z",
	}); err != nil {
		t.Fatalf("seed signal: %v", err)
	}

	probe := &mutableWellKnown{err: errors.New("site unavailable")}
	decoder := discovercrawl.NewClient(nil, nil, nil, probe, nil)
	w := newTestWorker(t, dbs, decoder, noEntries{}, &fakeResolver{handle: syntax.Handle("reader.example")})
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
	seedMirror(t, dbs, repoA, lexicon.Subscription, "3sub",
		`{"source":{"$type":"blue.morgen.feed.subscription#rssFeed","feedUrl":"https://a.example/feed"},"createdAt":"2026-07-01T00:00:00Z"}`)
	markDirty(t, dbs, repoA, "2026-07-10T00:00:00Z")

	w := newTestWorker(t, dbs, discovercrawl.NewClient(nil, nil, nil, nil, nil), noEntries{}, &fakeResolver{})
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
