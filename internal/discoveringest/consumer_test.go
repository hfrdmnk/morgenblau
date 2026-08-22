package discoveringest

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"

	"morgenblau/internal/backoff"
	"morgenblau/internal/database/db"
)

const testCollection = "blue.morgen.feed.save"

// trace is the shared ordered log the fakes write to, so a test can assert both that something happened and in what order.
type trace struct {
	mu      sync.Mutex
	entries []string
}

func (tr *trace) add(entry string) {
	tr.mu.Lock()
	defer tr.mu.Unlock()
	tr.entries = append(tr.entries, entry)
}

func (tr *trace) snapshot() []string {
	tr.mu.Lock()
	defer tr.mu.Unlock()
	return slices.Clone(tr.entries)
}

func (tr *trace) await(t *testing.T, want ...string) []string {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		got := tr.snapshot()
		missing := false
		for _, w := range want {
			if !slices.Contains(got, w) {
				missing = true
				break
			}
		}
		if !missing {
			return got
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %v; trace = %v", want, got)
		}
		time.Sleep(2 * time.Millisecond)
	}
}

func (tr *trace) refute(t *testing.T, unwanted string) {
	t.Helper()
	if slices.Contains(tr.snapshot(), unwanted) {
		t.Fatalf("%q happened; trace = %v", unwanted, tr.snapshot())
	}
}

func indexOf(t *testing.T, entries []string, want string) int {
	t.Helper()
	i := slices.Index(entries, want)
	if i < 0 {
		t.Fatalf("%q missing from trace %v", want, entries)
	}
	return i
}

// fakeStore is the consumer's narrow write surface backed by maps, so a test asserts converged state rather than a call log.
type fakeStore struct {
	rec *trace

	// cursors is the reader-pool view the same database would serve back, so a redial resumes from what the writer committed.
	cursors *fakeCursors

	mu      sync.Mutex
	records map[string]string
	handles map[string]string
	active  map[string]bool
	status  map[string]string
	dirty   map[string]string
	cursor  *db.UpsertDiscoverIngestCursorParams
	err     error
}

func newFakeStore(rec *trace) *fakeStore {
	return &fakeStore{
		rec:     rec,
		records: map[string]string{},
		handles: map[string]string{},
		active:  map[string]bool{},
		status:  map[string]string{},
		dirty:   map[string]string{},
	}
}

func recordKey(did, collection, rkey string) string {
	return did + "|" + collection + "|" + rkey
}

func (f *fakeStore) UpsertTapRecord(_ context.Context, arg db.UpsertTapRecordParams) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return f.err
	}
	f.records[recordKey(arg.Did, arg.Collection, arg.Rkey)] = arg.Record
	f.rec.add("upsert:" + arg.Rkey)
	return nil
}

func (f *fakeStore) DeleteTapRecord(_ context.Context, arg db.DeleteTapRecordParams) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return f.err
	}
	delete(f.records, recordKey(arg.Did, arg.Collection, arg.Rkey))
	f.rec.add("delete:" + arg.Rkey)
	return nil
}

func (f *fakeStore) DeleteTapRecordsForRepo(_ context.Context, did string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return f.err
	}
	for k := range f.records {
		if len(k) > len(did) && k[:len(did)+1] == did+"|" {
			delete(f.records, k)
		}
	}
	f.rec.add("purge:" + did)
	return nil
}

func (f *fakeStore) UpsertTapRepoHandle(_ context.Context, arg db.UpsertTapRepoHandleParams) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return f.err
	}
	if _, seen := f.active[arg.Did]; !seen {
		f.active[arg.Did] = true
		f.status[arg.Did] = ""
	}
	f.handles[arg.Did] = arg.Handle
	f.rec.add("handle:" + arg.Did)
	return nil
}

func (f *fakeStore) UpsertTapRepoAccount(_ context.Context, arg db.UpsertTapRepoAccountParams) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return f.err
	}
	if _, seen := f.handles[arg.Did]; !seen {
		f.handles[arg.Did] = ""
	}
	f.active[arg.Did] = arg.IsActive != 0
	f.status[arg.Did] = arg.Status
	f.rec.add("account:" + arg.Did)
	return nil
}

func (f *fakeStore) MarkTapRepoDirty(_ context.Context, arg db.MarkTapRepoDirtyParams) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return f.err
	}
	f.dirty[arg.Did] = arg.MarkedAt
	f.rec.add("dirty:" + arg.Did)
	return nil
}

func (f *fakeStore) TapRepoIsMirrored(_ context.Context, did string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.handles[did]; ok {
		return true, nil
	}
	for k := range f.records {
		if len(k) > len(did) && k[:len(did)+1] == did+"|" {
			return true, nil
		}
	}
	return false, nil
}

func (f *fakeStore) UpsertDiscoverIngestCursor(_ context.Context, arg db.UpsertDiscoverIngestCursorParams) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return f.err
	}
	f.cursor = &arg
	if f.cursors != nil {
		f.cursors.set(db.GetDiscoverIngestCursorRow{
			Seq:                 arg.Seq,
			BootstrapTipSeq:     arg.BootstrapTipSeq,
			BootstrapThroughSeq: arg.BootstrapThroughSeq,
			UpdatedAt:           arg.UpdatedAt,
		})
	}
	f.rec.add(fmt.Sprintf("cursor:%d", arg.Seq))
	return nil
}

func (f *fakeStore) recordAt(did, collection, rkey string) (string, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	v, ok := f.records[recordKey(did, collection, rkey)]
	return v, ok
}

func (f *fakeStore) recordCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.records)
}

func (f *fakeStore) seed(did, collection, rkey, record string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.records[recordKey(did, collection, rkey)] = record
}

func (f *fakeStore) snapshotState(did string) (handle string, active bool, status string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.handles[did], f.active[did], f.status[did]
}

func (f *fakeStore) cursorRow() *db.UpsertDiscoverIngestCursorParams {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.cursor
}

// fakeCursors is the reader-pool side of the stream position.
type fakeCursors struct {
	mu  sync.Mutex
	row db.GetDiscoverIngestCursorRow
	has bool
}

func (f *fakeCursors) GetDiscoverIngestCursor(context.Context) (db.GetDiscoverIngestCursorRow, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.has {
		return db.GetDiscoverIngestCursorRow{}, sql.ErrNoRows
	}
	return f.row, nil
}

func (f *fakeCursors) set(row db.GetDiscoverIngestCursorRow) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.row = row
	f.has = true
}

// fakeFetcher stands in for the listRecords re-crawl a divergence marker triggers.
type fakeFetcher struct {
	rec     *trace
	mu      sync.Mutex
	records []MirrorRecord
	err     error
	calls   []string
}

func (f *fakeFetcher) FetchRepoRecords(_ context.Context, did string) ([]MirrorRecord, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, did)
	f.rec.add("refetch:" + did)
	return f.records, f.err
}

// jetstreamServer speaks the v2 subscribe wire: it records the query it was dialed with and writes a per-connection script of frames.
type jetstreamServer struct {
	*httptest.Server
	rec     *trace
	scripts [][]string
	// dropAfterScript closes the connection once its script is written, so a test can watch the consumer redial.
	dropAfterScript bool

	// archive serves the Replay endpoints; nil means an empty sealed archive, which makes bootstrap a no-op.
	archive *fakeArchive

	mu      sync.Mutex
	conns   int
	queries []url.Values
}

func newJetstreamServer(t *testing.T, rec *trace, scripts ...[]string) *jetstreamServer {
	t.Helper()
	s := &jetstreamServer{rec: rec, scripts: scripts}
	s.Server = httptest.NewServer(http.HandlerFunc(s.serve))
	t.Cleanup(s.Close)
	return s
}

func (s *jetstreamServer) serve(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case planPath, blockPath, segmentPath:
		s.archive.serve(w, r)
		return
	}
	if r.URL.Path != subscribePath {
		http.NotFound(w, r)
		return
	}
	s.mu.Lock()
	n := s.conns
	s.conns++
	s.queries = append(s.queries, r.URL.Query())
	s.mu.Unlock()
	s.rec.add(fmt.Sprintf("connect:%d", n))

	conn, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	defer conn.CloseNow()

	ctx := r.Context()
	if n < len(s.scripts) {
		for _, msg := range s.scripts[n] {
			if err := conn.Write(ctx, websocket.MessageText, []byte(msg)); err != nil {
				return
			}
		}
	}
	if s.dropAfterScript {
		return
	}
	<-ctx.Done()
}

func (s *jetstreamServer) query(t *testing.T, n int) url.Values {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	if n >= len(s.queries) {
		t.Fatalf("connection %d never happened; %d so far", n, len(s.queries))
	}
	return s.queries[n]
}

func commitFrame(seq int, action, rkey, record string) string {
	body := fmt.Sprintf(`{"$type":"network.bsky.jetstream.subscribeEvents#commit","seq":%d,"did":%q,"time":"2026-08-01T10:00:00.000000Z","rev":"3rrrrrrrrrrr2","operation":%q,"collection":%q,"rkey":%q,"cid":%q`,
		seq, testDID, action, testCollection, rkey, testCID)
	if record != "" {
		body += `,"record":` + record
	}
	return `{"$type":"message","payload":` + body + `}}`
}

func identityFrame(seq int, did, handle string) string {
	return fmt.Sprintf(`{"$type":"message","payload":{"$type":"network.bsky.jetstream.subscribeEvents#identity","seq":%d,"did":%q,"time":"2026-08-01T10:00:00.000000Z","identity":{"did":%q,"handle":%q}}}`,
		seq, did, did, handle)
}

func accountFrame(seq int, did string, active bool, status string) string {
	return fmt.Sprintf(`{"$type":"message","payload":{"$type":"network.bsky.jetstream.subscribeEvents#account","seq":%d,"did":%q,"time":"2026-08-01T10:00:00.000000Z","account":{"did":%q,"active":%t,"status":%q}}}`,
		seq, did, did, active, status)
}

func syncFrame(seq int, did string) string {
	return fmt.Sprintf(`{"$type":"message","payload":{"$type":"network.bsky.jetstream.subscribeEvents#sync","seq":%d,"did":%q,"time":"2026-08-01T10:00:00.000000Z","sync":{"did":%q,"rev":"3rrrrrrrrrrr2"}}}`,
		seq, did, did)
}

type harness struct {
	consumer *Consumer
	store    *fakeStore
	cursors  *fakeCursors
	fetcher  *fakeFetcher
	rec      *trace
}

func startConsumer(t *testing.T, srv *jetstreamServer, opts ...func(*Consumer)) *harness {
	t.Helper()
	rec := &trace{}
	return startConsumerWith(t, srv, rec, newFakeStore(rec), &fakeCursors{}, &fakeFetcher{rec: rec}, opts...)
}

func startConsumerWith(t *testing.T, srv *jetstreamServer, rec *trace, store *fakeStore, cursors *fakeCursors, fetcher *fakeFetcher, opts ...func(*Consumer)) *harness {
	t.Helper()
	store.cursors = cursors
	c := NewConsumer(Config{URL: srv.URL}, cursors, fetcher)
	c.httpClient = srv.Client()
	c.policy = backoff.Policy{Steps: []time.Duration{time.Millisecond}}
	c.pingInterval = 50 * time.Millisecond
	c.cursorFlushEvents = 1
	c.runTx = func(ctx context.Context, fn func(MirrorStore) error) error {
		if err := fn(store); err != nil {
			return err
		}
		rec.add("commit")
		return nil
	}
	for _, opt := range opts {
		opt(c)
	}
	c.Start()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := c.Shutdown(ctx); err != nil {
			t.Errorf("Shutdown: %v", err)
		}
	})
	return &harness{consumer: c, store: store, cursors: cursors, fetcher: fetcher, rec: rec}
}

func TestConsumer_SubscribesToEveryReaderNetworkCollection(t *testing.T) {
	rec := &trace{}
	srv := newJetstreamServer(t, rec)
	h := startConsumerWith(t, srv, rec, newFakeStore(rec), &fakeCursors{}, &fakeFetcher{rec: rec})
	_ = h
	rec.await(t, "connect:0")

	got := srv.query(t, 0)["collections"]
	if len(got) != len(Collections) {
		t.Fatalf("collections = %v, want %d entries", got, len(Collections))
	}
	for _, want := range Collections {
		if !slices.Contains(got, want) {
			t.Errorf("collections is missing %q", want)
		}
	}
	// kinds is deliberately absent: a collection filter must not drop the account and sync markers that purge a repo.
	if kinds := srv.query(t, 0)["kinds"]; len(kinds) != 0 {
		t.Errorf("kinds = %v, want none", kinds)
	}
}

func TestConsumer_CommitMirrorsRecordAndMarksRepoDirty(t *testing.T) {
	rec := &trace{}
	srv := newJetstreamServer(t, rec, []string{
		commitFrame(101, actionCreate, "3aaaaaaaaaaa2", `{"$type":"blue.morgen.feed.save","itemUrl":"https://news.example.com/a"}`),
	})
	h := startConsumerWith(t, srv, rec, newFakeStore(rec), &fakeCursors{}, &fakeFetcher{rec: rec})

	entries := rec.await(t, "upsert:3aaaaaaaaaaa2", "dirty:"+testDID, "commit")
	if indexOf(t, entries, "dirty:"+testDID) < indexOf(t, entries, "upsert:3aaaaaaaaaaa2") {
		t.Errorf("dirty mark landed before the mirror write: %v", entries)
	}
	got, ok := h.store.recordAt(testDID, testCollection, "3aaaaaaaaaaa2")
	if !ok {
		t.Fatal("record was not mirrored")
	}
	if got != `{"$type":"blue.morgen.feed.save","itemUrl":"https://news.example.com/a"}` {
		t.Errorf("record = %s", got)
	}
}

func TestConsumer_DeleteRemovesTheMirroredRecord(t *testing.T) {
	rec := &trace{}
	store := newFakeStore(rec)
	store.seed(testDID, testCollection, "3aaaaaaaaaaa2", `{"$type":"blue.morgen.feed.save"}`)
	srv := newJetstreamServer(t, rec, []string{commitFrame(102, actionDelete, "3aaaaaaaaaaa2", "")})
	startConsumerWith(t, srv, rec, store, &fakeCursors{}, &fakeFetcher{rec: rec})

	rec.await(t, "delete:3aaaaaaaaaaa2", "dirty:"+testDID)
	if _, ok := store.recordAt(testDID, testCollection, "3aaaaaaaaaaa2"); ok {
		t.Error("record survived the delete")
	}
}

func TestConsumer_IdentityUpdatesTheHandleWithoutTouchingHostingStatus(t *testing.T) {
	rec := &trace{}
	store := newFakeStore(rec)
	store.seed(testDID, testCollection, "3aaaaaaaaaaa2", `{"$type":"blue.morgen.feed.save"}`)
	srv := newJetstreamServer(t, rec, []string{
		accountFrame(200, testDID, false, "suspended"),
		identityFrame(201, testDID, "reader.example"),
	})
	startConsumerWith(t, srv, rec, store, &fakeCursors{}, &fakeFetcher{rec: rec})

	rec.await(t, "account:"+testDID, "handle:"+testDID)
	handle, active, status := store.snapshotState(testDID)
	if handle != "reader.example" {
		t.Errorf("handle = %q", handle)
	}
	if active {
		t.Error("identity reactivated a suspended repo")
	}
	if status != "suspended" {
		t.Errorf("status = %q, want suspended", status)
	}
}

func TestConsumer_AccountDeletionPurgesTheRepo(t *testing.T) {
	rec := &trace{}
	store := newFakeStore(rec)
	store.seed(testDID, testCollection, "3aaaaaaaaaaa2", `{"$type":"blue.morgen.feed.save"}`)
	srv := newJetstreamServer(t, rec, []string{accountFrame(300, testDID, false, "deleted")})
	startConsumerWith(t, srv, rec, store, &fakeCursors{}, &fakeFetcher{rec: rec})

	rec.await(t, "purge:"+testDID, "dirty:"+testDID)
	if store.recordCount() != 0 {
		t.Errorf("records survived the account deletion: %d", store.recordCount())
	}
}

func TestConsumer_AccountDeactivationRetainsRecordsForReactivation(t *testing.T) {
	rec := &trace{}
	store := newFakeStore(rec)
	store.seed(testDID, testCollection, "3aaaaaaaaaaa2", `{"$type":"blue.morgen.feed.save"}`)
	srv := newJetstreamServer(t, rec, []string{
		accountFrame(400, testDID, false, "takendown"),
		accountFrame(401, testDID, true, "active"),
	})
	startConsumerWith(t, srv, rec, store, &fakeCursors{}, &fakeFetcher{rec: rec})

	rec.await(t, "cursor:401")
	rec.refute(t, "purge:"+testDID)
	if store.recordCount() != 1 {
		t.Errorf("records = %d, want the retained one", store.recordCount())
	}
	_, active, status := store.snapshotState(testDID)
	if !active || status != "active" {
		t.Errorf("active = %v, status = %q, want reactivated", active, status)
	}
}

// Markers arrive network-wide because the collection filter never drops them, so a repo we do not mirror must not gain state rows.
func TestConsumer_MarkerForAnUnmirroredRepoIsIgnored(t *testing.T) {
	rec := &trace{}
	store := newFakeStore(rec)
	srv := newJetstreamServer(t, rec, []string{
		accountFrame(500, otherDID, false, "deleted"),
		commitFrame(501, actionCreate, "3aaaaaaaaaaa2", `{"$type":"blue.morgen.feed.save","itemUrl":"https://news.example.com/a"}`),
	})
	startConsumerWith(t, srv, rec, store, &fakeCursors{}, &fakeFetcher{rec: rec})

	rec.await(t, "upsert:3aaaaaaaaaaa2")
	rec.refute(t, "account:"+otherDID)
	rec.refute(t, "purge:"+otherDID)
	rec.refute(t, "dirty:"+otherDID)
}

func TestConsumer_SyncRederivesTheRepoFromThePDS(t *testing.T) {
	rec := &trace{}
	store := newFakeStore(rec)
	store.seed(testDID, testCollection, "3staleaaaaaa2", `{"$type":"blue.morgen.feed.save"}`)
	fetcher := &fakeFetcher{rec: rec, records: []MirrorRecord{
		{Collection: testCollection, Rkey: "3freshaaaaaa2", CID: testCID, Record: `{"$type":"blue.morgen.feed.save","itemUrl":"https://news.example.com/b"}`},
	}}
	srv := newJetstreamServer(t, rec, []string{syncFrame(600, testDID)})
	startConsumerWith(t, srv, rec, store, &fakeCursors{}, fetcher)

	entries := rec.await(t, "refetch:"+testDID, "purge:"+testDID, "upsert:3freshaaaaaa2", "dirty:"+testDID)
	// Network I/O must finish before the transaction opens.
	if indexOf(t, entries, "refetch:"+testDID) > indexOf(t, entries, "purge:"+testDID) {
		t.Errorf("re-crawl ran inside the transaction: %v", entries)
	}
	if _, ok := store.recordAt(testDID, testCollection, "3staleaaaaaa2"); ok {
		t.Error("diverged record survived the re-derive")
	}
	if _, ok := store.recordAt(testDID, testCollection, "3freshaaaaaa2"); !ok {
		t.Error("replacement record was not mirrored")
	}
}

func TestConsumer_PersistsTheCursor(t *testing.T) {
	rec := &trace{}
	srv := newJetstreamServer(t, rec, []string{
		commitFrame(700, actionCreate, "3aaaaaaaaaaa2", `{"$type":"blue.morgen.feed.save"}`),
	})
	h := startConsumerWith(t, srv, rec, newFakeStore(rec), &fakeCursors{}, &fakeFetcher{rec: rec})

	rec.await(t, "cursor:700")
	row := h.store.cursorRow()
	if row == nil || row.Seq != 700 {
		t.Fatalf("cursor row = %+v", row)
	}
	if row.BootstrapTipSeq != nil || row.BootstrapThroughSeq != nil {
		t.Error("a live cursor write left bootstrap columns set")
	}
}

func TestConsumer_ResumesFromThePersistedCursor(t *testing.T) {
	rec := &trace{}
	cursors := &fakeCursors{}
	cursors.set(db.GetDiscoverIngestCursorRow{Seq: 9001, UpdatedAt: "2026-08-01T00:00:00Z"})
	srv := newJetstreamServer(t, rec)
	startConsumerWith(t, srv, rec, newFakeStore(rec), cursors, &fakeFetcher{rec: rec})

	rec.await(t, "connect:0")
	if got := srv.query(t, 0).Get("cursor"); got != "9001" {
		t.Errorf("cursor = %q, want 9001", got)
	}
}

// A #info frame is seq-less; treating its zero seq as a position would rewind the stream to the archive floor.
func TestConsumer_InfoFrameDoesNotRewindTheCursor(t *testing.T) {
	rec := &trace{}
	srv := newJetstreamServer(t, rec, []string{
		commitFrame(800, actionCreate, "3aaaaaaaaaaa2", `{"$type":"blue.morgen.feed.save"}`),
		`{"$type":"message","payload":{"$type":"network.bsky.jetstream.subscribeEvents#info","name":"OutdatedCursor","message":"resumed"}}`,
		commitFrame(801, actionCreate, "3bbbbbbbbbbb2", `{"$type":"blue.morgen.feed.save"}`),
	})
	h := startConsumerWith(t, srv, rec, newFakeStore(rec), &fakeCursors{}, &fakeFetcher{rec: rec})

	rec.await(t, "cursor:801")
	rec.refute(t, "cursor:0")
	if row := h.store.cursorRow(); row.Seq != 801 {
		t.Errorf("cursor = %d, want 801", row.Seq)
	}
}

func TestConsumer_ReconnectsFromTheLastCursorAfterTheStreamDrops(t *testing.T) {
	rec := &trace{}
	srv := newJetstreamServer(t, rec,
		[]string{commitFrame(900, actionCreate, "3aaaaaaaaaaa2", `{"$type":"blue.morgen.feed.save"}`)},
		[]string{commitFrame(901, actionCreate, "3bbbbbbbbbbb2", `{"$type":"blue.morgen.feed.save"}`)},
	)
	srv.dropAfterScript = true
	// The reader pool sees what the writer pool committed, so the redial resumes from the persisted seq.
	startConsumerWith(t, srv, rec, newFakeStore(rec), &fakeCursors{}, &fakeFetcher{rec: rec})

	rec.await(t, "connect:1", "upsert:3bbbbbbbbbbb2")
	if got := srv.query(t, 1).Get("cursor"); got != "900" {
		t.Errorf("redial cursor = %q, want 900", got)
	}
}

func TestConsumer_TerminalErrorFrameEndsTheSession(t *testing.T) {
	rec := &trace{}
	srv := newJetstreamServer(t, rec,
		[]string{`{"$type":"error","error":"ConsumerTooSlow","message":"too far behind"}`},
		[]string{commitFrame(1000, actionCreate, "3aaaaaaaaaaa2", `{"$type":"blue.morgen.feed.save"}`)},
	)
	startConsumerWith(t, srv, rec, newFakeStore(rec), &fakeCursors{}, &fakeFetcher{rec: rec})

	rec.await(t, "connect:1", "upsert:3aaaaaaaaaaa2")
}

func TestConsumer_ShutdownDrainsTheSession(t *testing.T) {
	rec := &trace{}
	srv := newJetstreamServer(t, rec)
	c := NewConsumer(Config{URL: srv.URL}, &fakeCursors{}, &fakeFetcher{rec: rec})
	c.httpClient = srv.Client()
	c.policy = backoff.Policy{Steps: []time.Duration{time.Millisecond}}
	c.runTx = func(ctx context.Context, fn func(MirrorStore) error) error { return fn(newFakeStore(rec)) }
	c.Start()
	rec.await(t, "connect:0")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := c.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
}
