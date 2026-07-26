package tapingest

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"

	"morgenblau/internal/backoff"
	"morgenblau/internal/database/db"
)

const (
	testDID        = "did:plc:aaaaaaaaaaaaaaaaaaaaaaaa"
	testCID        = "bafyreiaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	testCollection = "blue.morgen.feed.save"
)

// trace is the shared ordered log both the fake store and the fake tap write to, so a test can assert that a commit landed before its ack.
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

// await polls until every wanted entry is present, so a test never races the consumer's goroutines.
func (tr *trace) await(t *testing.T, want ...string) []string {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
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

func indexOf(t *testing.T, entries []string, want string) int {
	t.Helper()
	i := slices.Index(entries, want)
	if i < 0 {
		t.Fatalf("%q missing from trace %v", want, entries)
	}
	return i
}

// fakeTapStore is the consumer's narrow write surface, recording every call in call order.
type fakeTapStore struct {
	rec *trace
	err error

	mu          sync.Mutex
	upserts     []db.UpsertTapRecordParams
	deletes     []db.DeleteTapRecordParams
	repoDeletes []string
	repoStates  []db.UpsertTapRepoStateParams
	dirty       []db.MarkTapRepoDirtyParams
}

func (f *fakeTapStore) UpsertTapRecord(ctx context.Context, arg db.UpsertTapRecordParams) error {
	f.mu.Lock()
	f.upserts = append(f.upserts, arg)
	f.mu.Unlock()
	f.rec.add("upsert:" + arg.Rkey)
	return f.err
}

func (f *fakeTapStore) DeleteTapRecord(ctx context.Context, arg db.DeleteTapRecordParams) error {
	f.mu.Lock()
	f.deletes = append(f.deletes, arg)
	f.mu.Unlock()
	f.rec.add("delete:" + arg.Rkey)
	return f.err
}

func (f *fakeTapStore) DeleteTapRecordsForRepo(ctx context.Context, did string) error {
	f.mu.Lock()
	f.repoDeletes = append(f.repoDeletes, did)
	f.mu.Unlock()
	f.rec.add("delete-repo:" + did)
	return f.err
}

func (f *fakeTapStore) UpsertTapRepoState(ctx context.Context, arg db.UpsertTapRepoStateParams) error {
	f.mu.Lock()
	f.repoStates = append(f.repoStates, arg)
	f.mu.Unlock()
	f.rec.add("state:" + arg.Did)
	return f.err
}

func (f *fakeTapStore) MarkTapRepoDirty(ctx context.Context, arg db.MarkTapRepoDirtyParams) error {
	f.mu.Lock()
	f.dirty = append(f.dirty, arg)
	f.mu.Unlock()
	f.rec.add("dirty:" + arg.Did)
	return f.err
}

func (f *fakeTapStore) counts() (upserts, deletes, dirty int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.upserts), len(f.deletes), len(f.dirty)
}

// tapServer speaks tap's /channel protocol: it writes a per-connection script of envelopes, then records the acks the consumer writes back.
type tapServer struct {
	*httptest.Server
	rec     *trace
	scripts [][]string
	// dropFirst closes connection 0 the moment its script is written, without acking, so a test can watch the consumer redial.
	dropFirst bool

	mu    sync.Mutex
	conns int
}

func newTapServer(t *testing.T, rec *trace, scripts ...[]string) *tapServer {
	t.Helper()
	ts := &tapServer{rec: rec, scripts: scripts}
	ts.Server = httptest.NewServer(http.HandlerFunc(ts.serve))
	t.Cleanup(ts.Close)
	return ts
}

func (s *tapServer) serve(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/channel" {
		http.NotFound(w, r)
		return
	}
	conn, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	defer conn.CloseNow()

	s.mu.Lock()
	n := s.conns
	s.conns++
	s.mu.Unlock()
	s.rec.add(fmt.Sprintf("connect:%d", n))

	ctx := r.Context()
	if n < len(s.scripts) {
		for _, msg := range s.scripts[n] {
			if err := conn.Write(ctx, websocket.MessageText, []byte(msg)); err != nil {
				return
			}
		}
	}
	if n == 0 && s.dropFirst {
		return
	}
	for {
		_, data, err := conn.Read(ctx)
		if err != nil {
			return
		}
		var a struct {
			Type string `json:"type"`
			ID   uint64 `json:"id"`
		}
		if json.Unmarshal(data, &a) != nil {
			continue
		}
		s.rec.add(fmt.Sprintf("%s:%d", a.Type, a.ID))
	}
}

// startConsumer wires a consumer to srv with a fake commit boundary that logs into the shared trace.
func startConsumer(t *testing.T, srv *tapServer, store *fakeTapStore) *Consumer {
	t.Helper()
	c := NewConsumer(srv.URL)
	c.httpClient = srv.Client()
	c.policy = backoff.Policy{Steps: []time.Duration{time.Millisecond}}
	c.pingInterval = 50 * time.Millisecond
	c.runTx = func(ctx context.Context, fn func(TapStore) error) error {
		if err := fn(store); err != nil {
			return err
		}
		store.rec.add("commit")
		return nil
	}
	c.Start()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if err := c.Shutdown(ctx); err != nil {
			t.Errorf("Shutdown: %v", err)
		}
	})
	return c
}

func recordEnvelope(id int, action, rkey, record string) string {
	body := fmt.Sprintf(`{"live":true,"did":%q,"rev":"3rev","collection":%q,"rkey":%q,"action":%q,"cid":%q`,
		testDID, testCollection, rkey, action, testCID)
	if record != "" {
		body += `,"record":` + record
	}
	return fmt.Sprintf(`{"id":%d,"type":"record","record":%s}}`, id, body)
}

func identityEnvelope(id int, handle string, active bool, status string) string {
	return fmt.Sprintf(`{"id":%d,"type":"identity","identity":{"did":%q,"handle":%q,"is_active":%t,"status":%q}}`,
		id, testDID, handle, active, status)
}

func TestConsumer_AcksOnlyAfterTheMirrorWriteCommits(t *testing.T) {
	rec := &trace{}
	store := &fakeTapStore{rec: rec}
	srv := newTapServer(t, rec, []string{
		recordEnvelope(1, "create", "3aaa", `{"$type":"blue.morgen.feed.save","itemUrl":"https://a.example/post"}`),
	})
	startConsumer(t, srv, store)

	entries := rec.await(t, "commit", "ack:1")
	if indexOf(t, entries, "commit") > indexOf(t, entries, "ack:1") {
		t.Errorf("ack landed before commit: %v", entries)
	}
}

func TestConsumer_CreateMirrorsRecordAndMarksRepoDirty(t *testing.T) {
	rec := &trace{}
	store := &fakeTapStore{rec: rec}
	srv := newTapServer(t, rec, []string{
		recordEnvelope(1, "create", "3aaa", `{"$type":"blue.morgen.feed.save", "itemUrl": "https://a.example/post"}`),
	})
	startConsumer(t, srv, store)
	rec.await(t, "ack:1")

	store.mu.Lock()
	defer store.mu.Unlock()
	if len(store.upserts) != 1 {
		t.Fatalf("upserts = %+v, want 1", store.upserts)
	}
	got := store.upserts[0]
	if got.Did != testDID || got.Collection != testCollection || got.Rkey != "3aaa" || got.Cid != testCID {
		t.Errorf("upsert identity = %+v", got)
	}
	want := `{"$type":"blue.morgen.feed.save","itemUrl":"https://a.example/post"}`
	if got.Record != want {
		t.Errorf("Record = %q, want compacted %q", got.Record, want)
	}
	if len(store.dirty) != 1 || store.dirty[0].Did != testDID {
		t.Errorf("dirty = %+v, want one mark for %s", store.dirty, testDID)
	}
}

func TestConsumer_DeleteDropsMirrorRowAndMarksRepoDirty(t *testing.T) {
	rec := &trace{}
	store := &fakeTapStore{rec: rec}
	srv := newTapServer(t, rec, []string{recordEnvelope(1, "delete", "3aaa", "")})
	startConsumer(t, srv, store)
	rec.await(t, "ack:1")

	upserts, deletes, dirty := store.counts()
	if upserts != 0 || deletes != 1 || dirty != 1 {
		t.Fatalf("upserts=%d deletes=%d dirty=%d, want 0/1/1", upserts, deletes, dirty)
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.deletes[0].Rkey != "3aaa" || store.deletes[0].Collection != testCollection {
		t.Errorf("delete = %+v", store.deletes[0])
	}
}

// A tap-side decode failure delivers a record event with no body; the mirror keeps whatever it had, but the repo still has to be rebuilt.
func TestConsumer_CreateWithoutRecordBodyMarksDirtyWithoutUpsert(t *testing.T) {
	rec := &trace{}
	store := &fakeTapStore{rec: rec}
	srv := newTapServer(t, rec, []string{recordEnvelope(1, "create", "3aaa", "")})
	startConsumer(t, srv, store)
	rec.await(t, "ack:1")

	upserts, deletes, dirty := store.counts()
	if upserts != 0 || deletes != 0 || dirty != 1 {
		t.Fatalf("upserts=%d deletes=%d dirty=%d, want 0/0/1", upserts, deletes, dirty)
	}
}

func TestConsumer_ActiveIdentityEventPersistsStateAndMarksDirtyBeforeAck(t *testing.T) {
	rec := &trace{}
	store := &fakeTapStore{rec: rec}
	srv := newTapServer(t, rec, []string{
		identityEnvelope(1, "reader.example", true, "active"),
	})
	startConsumer(t, srv, store)
	entries := rec.await(t, "ack:1")

	store.mu.Lock()
	defer store.mu.Unlock()
	if len(store.repoStates) != 1 {
		t.Fatalf("repo states = %+v, want one", store.repoStates)
	}
	state := store.repoStates[0]
	if state.Did != testDID || state.Handle != "reader.example" || state.IsActive != 1 || state.Status != "active" {
		t.Errorf("repo state = %+v", state)
	}
	if len(store.dirty) != 1 || len(store.repoDeletes) != 0 {
		t.Errorf("dirty=%d repo deletes=%d, want 1/0", len(store.dirty), len(store.repoDeletes))
	}
	if indexOf(t, entries, "commit") > indexOf(t, entries, "ack:1") {
		t.Errorf("identity ack landed before commit: %v", entries)
	}
}

func TestConsumer_InactiveIdentityEventsRetainMirrorAndMarkDirty(t *testing.T) {
	for _, status := range []string{"deactivated", "suspended", "takendown"} {
		t.Run(status, func(t *testing.T) {
			rec := &trace{}
			store := &fakeTapStore{rec: rec}
			srv := newTapServer(t, rec, []string{identityEnvelope(1, "reader.example", false, status)})
			startConsumer(t, srv, store)
			rec.await(t, "ack:1")

			store.mu.Lock()
			defer store.mu.Unlock()
			if len(store.repoStates) != 1 || store.repoStates[0].IsActive != 0 || store.repoStates[0].Status != status {
				t.Errorf("repo states = %+v", store.repoStates)
			}
			if len(store.dirty) != 1 || len(store.repoDeletes) != 0 {
				t.Errorf("dirty=%d repo deletes=%d, want 1/0", len(store.dirty), len(store.repoDeletes))
			}
		})
	}
}

func TestConsumer_DeletedIdentityEventPurgesMirrorAndMarksDirty(t *testing.T) {
	rec := &trace{}
	store := &fakeTapStore{rec: rec}
	srv := newTapServer(t, rec, []string{identityEnvelope(1, "reader.example", false, "deleted")})
	startConsumer(t, srv, store)
	rec.await(t, "ack:1")

	store.mu.Lock()
	defer store.mu.Unlock()
	if len(store.repoStates) != 1 || store.repoStates[0].Status != "deleted" {
		t.Errorf("repo states = %+v", store.repoStates)
	}
	if len(store.repoDeletes) != 1 || store.repoDeletes[0] != testDID || len(store.dirty) != 1 {
		t.Errorf("repo deletes=%v dirty=%v", store.repoDeletes, store.dirty)
	}
}

func TestConsumer_DeletedIdentityPurgesSQLiteThenRebuildClearsAggregates(t *testing.T) {
	dbs := openRebuildTestDB(t)
	seedMirror(t, dbs, testDID, testCollection, "3aaa", `{"itemUrl":"https://a.example/post"}`)
	if err := db.New(dbs.Writer).InsertDiscoverTrendingSignal(context.Background(), db.InsertDiscoverTrendingSignalParams{
		RepoDid: testDID, SourceKey: "https://a.example/feed", Kind: "rss", SignalKind: "save", FetchedAt: "2026-07-09T00:00:00Z",
	}); err != nil {
		t.Fatalf("seed signal: %v", err)
	}

	rec := &trace{}
	srv := newTapServer(t, rec, []string{identityEnvelope(1, "reader.example", false, "deleted")})
	c := NewConsumer(srv.URL).WithTxRunner(dbs.Writer)
	c.httpClient = srv.Client()
	c.policy = backoff.Policy{Steps: []time.Duration{time.Millisecond}}
	c.pingInterval = 50 * time.Millisecond
	c.Start()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if err := c.Shutdown(ctx); err != nil {
			t.Errorf("Shutdown: %v", err)
		}
	})
	rec.await(t, "ack:1")

	q := db.New(dbs.Reader)
	rows, err := q.ListTapRecordsForRepo(context.Background(), testDID)
	if err != nil {
		t.Fatalf("ListTapRecordsForRepo: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("mirror rows = %d, want purged", len(rows))
	}
	state, err := q.GetTapRepoState(context.Background(), testDID)
	if err != nil {
		t.Fatalf("GetTapRepoState: %v", err)
	}
	if state.IsActive != 0 || state.Status != "deleted" {
		t.Fatalf("repo state = %+v", state)
	}

	w := newTestWorker(t, dbs, &stubDecoder{}, noEntries{}, &fakeResolver{})
	w.drain(context.Background())
	if got := signalsByKey(t, dbs); len(got) != 0 {
		t.Fatalf("signals = %+v, want deleted repo removed from discovery", got)
	}
}

func TestConsumer_UnusableIdentityEventIsAckedAndSkipped(t *testing.T) {
	rec := &trace{}
	store := &fakeTapStore{rec: rec}
	srv := newTapServer(t, rec, []string{`{"id":1,"type":"identity","identity":{"status":"active","is_active":true}}`})
	startConsumer(t, srv, store)
	rec.await(t, "ack:1")

	store.mu.Lock()
	defer store.mu.Unlock()
	if len(store.repoStates) != 0 || len(store.dirty) != 0 {
		t.Fatalf("repo states=%v dirty=%v, want no writes", store.repoStates, store.dirty)
	}
}

func TestConsumer_IdentityStoreFailureLeavesTheEventUnacked(t *testing.T) {
	rec := &trace{}
	store := &fakeTapStore{rec: rec, err: fmt.Errorf("disk on fire")}
	srv := newTapServer(t, rec, []string{identityEnvelope(1, "reader.example", true, "active")})
	startConsumer(t, srv, store)
	rec.await(t, "state:"+testDID)

	time.Sleep(50 * time.Millisecond)
	if entries := rec.snapshot(); slices.Contains(entries, "ack:1") {
		t.Errorf("acked a failed identity write: %v", entries)
	}
}

func TestConsumer_UnknownEnvelopeTypeIsAckedAndSkipped(t *testing.T) {
	rec := &trace{}
	store := &fakeTapStore{rec: rec}
	srv := newTapServer(t, rec, []string{`{"id":1,"type":"somethingNew"}`})
	startConsumer(t, srv, store)
	rec.await(t, "ack:1")

	if upserts, deletes, dirty := store.counts(); upserts+deletes+dirty != 0 {
		t.Errorf("store touched: upserts=%d deletes=%d dirty=%d", upserts, deletes, dirty)
	}
}

// Redelivery cannot make a malformed envelope parse, so it is acked rather than left to stall the DID's stream forever.
func TestConsumer_MalformedEnvelopeIsAckedAndSkipped(t *testing.T) {
	rec := &trace{}
	store := &fakeTapStore{rec: rec}
	srv := newTapServer(t, rec, []string{
		`{"id":1,"type":"record","record":{"did":42,"collection":true}}`,
		`{ this is not json at all`,
		recordEnvelope(3, "create", "3aaa", `{"itemUrl":"https://a.example/post"}`),
	})
	startConsumer(t, srv, store)
	entries := rec.await(t, "ack:1", "ack:3")

	if slices.Contains(entries, "ack:2") {
		t.Errorf("acked an id it could not read: %v", entries)
	}
	if upserts, _, _ := store.counts(); upserts != 1 {
		t.Errorf("upserts = %d, want only the well-formed event's", upserts)
	}
}

// A failed mirror write stays unacked: tap redelivers after ~60s and UpsertTapRecord is idempotent.
func TestConsumer_StoreFailureLeavesTheEventUnacked(t *testing.T) {
	rec := &trace{}
	store := &fakeTapStore{rec: rec, err: fmt.Errorf("disk on fire")}
	srv := newTapServer(t, rec, []string{recordEnvelope(1, "create", "3aaa", `{"itemUrl":"https://a.example/post"}`)})
	startConsumer(t, srv, store)
	rec.await(t, "upsert:3aaa")

	time.Sleep(50 * time.Millisecond)
	if entries := rec.snapshot(); slices.Contains(entries, "ack:1") {
		t.Errorf("acked a failed write: %v", entries)
	}
}

func TestConsumer_RedialsAfterTheConnectionDrops(t *testing.T) {
	rec := &trace{}
	store := &fakeTapStore{rec: rec}
	srv := newTapServer(t, rec,
		[]string{recordEnvelope(1, "create", "3aaa", `{"itemUrl":"https://a.example/post"}`)},
		[]string{recordEnvelope(2, "create", "3bbb", `{"itemUrl":"https://b.example/post"}`)},
	)
	srv.dropFirst = true
	startConsumer(t, srv, store)

	rec.await(t, "connect:0", "connect:1", "ack:2")
	if upserts, _, _ := store.counts(); upserts < 2 {
		t.Errorf("upserts = %d, want both events mirrored across the reconnect", upserts)
	}
}

// A session that ends without delivering anything is a failure, so the next dial waits out the backoff step rather than hot-looping against a sick tap.
func TestConsumer_PacesRedialsWhenASessionDeliversNothing(t *testing.T) {
	const step = 200 * time.Millisecond
	rec := &trace{}
	store := &fakeTapStore{rec: rec}
	srv := newTapServer(t, rec, nil, []string{recordEnvelope(1, "create", "3aaa", `{"itemUrl":"https://a.example/post"}`)})
	srv.dropFirst = true

	c := NewConsumer(srv.URL)
	c.httpClient = srv.Client()
	c.policy = backoff.Policy{Steps: []time.Duration{step}}
	c.runTx = func(ctx context.Context, fn func(TapStore) error) error { return fn(store) }
	start := time.Now()
	c.Start()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if err := c.Shutdown(ctx); err != nil {
			t.Errorf("Shutdown: %v", err)
		}
	})

	rec.await(t, "connect:1")
	if elapsed := time.Since(start); elapsed < step {
		t.Errorf("redialed after %v, want at least the %v backoff step", elapsed, step)
	}
}

func TestChannelURL_DerivesWebsocketEndpointFromTapBaseURL(t *testing.T) {
	cases := map[string]string{
		"http://localhost:2480":       "ws://localhost:2480/channel",
		"https://tap.example":         "wss://tap.example/channel",
		"ws://localhost:2480":         "ws://localhost:2480/channel",
		"http://localhost:2480/":      "ws://localhost:2480/channel",
		"http://localhost:2480/other": "ws://localhost:2480/channel",
	}
	for in, want := range cases {
		if got := channelURL(in); got != want {
			t.Errorf("channelURL(%q) = %q, want %q", in, got, want)
		}
	}
}
