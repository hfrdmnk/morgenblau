package discoveringest

import (
	"context"
	"database/sql"
	"errors"
	"iter"
	"math"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/bluesky-social/jetstream"

	"morgenblau/internal/database/db"
)

type requestCursorReader struct {
	row db.GetDiscoverIngestCursorRow
	err error
}

func (r requestCursorReader) GetDiscoverIngestCursor(context.Context) (db.GetDiscoverIngestCursorRow, error) {
	return r.row, r.err
}

type blockingEventSource struct {
	closed chan struct{}
	once   sync.Once
}

func newBlockingEventSource() *blockingEventSource {
	return &blockingEventSource{closed: make(chan struct{})}
}

func (s *blockingEventSource) Events(ctx context.Context) iter.Seq2[sourceBatch, error] {
	return func(yield func(sourceBatch, error) bool) {
		<-ctx.Done()
	}
}

func (s *blockingEventSource) Close() error {
	s.once.Do(func() { close(s.closed) })
	return nil
}

func TestConsumerBuildsReplayAndResumeRequests(t *testing.T) {
	tests := []struct {
		name      string
		reader    CursorReader
		wantAfter *uint64
		wantLive  *uint64
	}{
		{name: "fresh replay", reader: requestCursorReader{err: sql.ErrNoRows}, wantAfter: uint64Ptr(0)},
		{name: "saved live cursor", reader: requestCursorReader{row: db.GetDiscoverIngestCursorRow{Seq: 42}}, wantLive: uint64Ptr(42)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			requests := make(chan sourceRequest, 1)
			source := newBlockingEventSource()
			consumer := newConsumer(Config{URL: "wss://jetstream.example", APIKey: "secret"}, tt.reader, func(req sourceRequest) (eventSource, error) {
				requests <- req
				return source, nil
			})
			consumer.Start()
			select {
			case req := <-requests:
				if req.Host != "wss://jetstream.example" || req.APIKey != "secret" {
					t.Fatalf("request = %+v", req)
				}
				if !slices.Equal(req.Collections, Collections) {
					t.Fatalf("collections = %v, want %v", req.Collections, Collections)
				}
				if !equalUint64Ptr(req.AfterSeq, tt.wantAfter) || !equalUint64Ptr(req.LiveCursor, tt.wantLive) {
					t.Fatalf("after/live = %v/%v, want %v/%v", req.AfterSeq, req.LiveCursor, tt.wantAfter, tt.wantLive)
				}
			case <-time.After(time.Second):
				t.Fatal("source factory was not called")
			}
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			if err := consumer.Shutdown(ctx); err != nil {
				t.Fatalf("Shutdown: %v", err)
			}
			select {
			case <-source.closed:
			default:
				t.Fatal("source was not closed")
			}
		})
	}
}

func TestFoldBatchRejectsCursorOutsideSQLiteRange(t *testing.T) {
	c := newConsumer(Config{}, requestCursorReader{}, nil)
	called := false
	c.runTx = func(context.Context, func(MirrorStore) error) error {
		called = true
		return nil
	}
	err := c.foldBatch(context.Background(), sourceBatch{cursor: uint64(math.MaxInt64) + 1})
	if err == nil {
		t.Fatal("foldBatch returned nil error")
	}
	if called {
		t.Fatal("transaction opened for an unrepresentable cursor")
	}
}

func TestFoldBatchRollsBackWritesAndCursorTogether(t *testing.T) {
	boom := errors.New("write failed")
	store := newAtomicTestStore()
	store.failRkey = "bad"
	store.failErr = boom
	c := newConsumer(Config{}, requestCursorReader{}, nil)
	c.runTx = store.runTx

	err := c.foldBatch(context.Background(), sourceBatch{
		cursor: 12,
		events: []jetstream.Event{
			commitEvent(10, "good", map[string]any{"value": "kept only on commit"}),
			commitEvent(11, "bad", map[string]any{"value": "boom"}),
		},
	})
	if !errors.Is(err, boom) {
		t.Fatalf("error = %v, want %v", err, boom)
	}
	if len(store.records) != 0 || store.cursor != 0 {
		t.Fatalf("records/cursor = %v/%d, want rolled back", store.records, store.cursor)
	}
}

func TestFoldBatchAppliesInOrderAndCommitsCursor(t *testing.T) {
	store := newAtomicTestStore()
	c := newConsumer(Config{}, requestCursorReader{}, nil)
	c.runTx = store.runTx
	err := c.foldBatch(context.Background(), sourceBatch{
		cursor: 12,
		events: []jetstream.Event{
			commitEvent(10, "same", map[string]any{"value": "first"}),
			commitEvent(11, "same", map[string]any{"value": "second"}),
		},
	})
	if err != nil {
		t.Fatalf("foldBatch: %v", err)
	}
	if got := store.records[recordKey(testDID, testCollection, "same")]; got != `{"value":"second"}` {
		t.Fatalf("record = %q", got)
	}
	if store.cursor != 12 {
		t.Fatalf("cursor = %d, want 12", store.cursor)
	}
}

func TestConsumerContinuesAfterRecoverableSourceErrorWithoutResubscribing(t *testing.T) {
	store := newAtomicTestStore()
	source := &scriptedEventSource{items: []sourceItem{
		{err: errors.New("temporary stream error")},
		{batch: sourceBatch{cursor: 10, events: []jetstream.Event{commitEvent(10, "saved", map[string]any{"value": "ok"})}}},
	}}
	factoryCalls := 0
	c := newConsumer(Config{}, requestCursorReader{err: sql.ErrNoRows}, func(sourceRequest) (eventSource, error) {
		factoryCalls++
		return source, nil
	})
	c.runTx = store.runTx
	c.run()
	if factoryCalls != 1 {
		t.Fatalf("factory calls = %d, want one official iterator", factoryCalls)
	}
	if store.cursor != 10 {
		t.Fatalf("cursor = %d, want 10", store.cursor)
	}
}

func TestConsumerStopsAfterFailedBatchSoLaterCursorCannotLeapfrog(t *testing.T) {
	store := newAtomicTestStore()
	store.failRkey = "bad"
	store.failErr = errors.New("write failed")
	source := &scriptedEventSource{items: []sourceItem{
		{batch: sourceBatch{cursor: 11, events: []jetstream.Event{commitEvent(10, "good", map[string]any{"value": "rolled back"}), commitEvent(11, "bad", map[string]any{"value": "boom"})}}},
		{batch: sourceBatch{cursor: 12, events: []jetstream.Event{commitEvent(12, "later", map[string]any{"value": "must not apply"})}}},
	}}
	c := newConsumer(Config{}, requestCursorReader{err: sql.ErrNoRows}, func(sourceRequest) (eventSource, error) { return source, nil })
	c.runTx = store.runTx
	c.run()
	if store.cursor != 0 || len(store.records) != 0 {
		t.Fatalf("records/cursor = %v/%d, failed batch was crossed", store.records, store.cursor)
	}
}

func TestSyncPurgesOnlyMirroredRepoBeforeReplacementCommits(t *testing.T) {
	store := newAtomicTestStore()
	store.records[recordKey(testDID, testCollection, "stale")] = `{"value":"stale"}`
	c := newConsumer(Config{}, requestCursorReader{}, nil)
	c.runTx = store.runTx
	err := c.foldBatch(context.Background(), sourceBatch{cursor: 21, events: []jetstream.Event{
		{DID: otherDID, Seq: 19, Kind: jetstream.KindSync, Sync: &jetstream.Sync{DID: otherDID}},
		{DID: testDID, Seq: 20, Kind: jetstream.KindSync, Sync: &jetstream.Sync{DID: testDID}},
		commitEvent(21, "replacement", map[string]any{"value": "fresh"}),
	}})
	if err != nil {
		t.Fatalf("foldBatch: %v", err)
	}
	if _, ok := store.records[recordKey(testDID, testCollection, "stale")]; ok {
		t.Fatal("stale record survived sync marker")
	}
	if got := store.records[recordKey(testDID, testCollection, "replacement")]; got != `{"value":"fresh"}` {
		t.Fatalf("replacement = %q", got)
	}
	if store.dirty[testDID] != 21 {
		t.Fatalf("dirty generation = %d, want 21", store.dirty[testDID])
	}
	if _, ok := store.dirty[otherDID]; ok {
		t.Fatal("unmirrored network-wide sync marker minted dirty state")
	}
}

func TestIdentityAndAccountChangesUseTheirOuterSequence(t *testing.T) {
	store := newAtomicTestStore()
	store.records[recordKey(testDID, testCollection, "kept")] = `{"value":"kept"}`
	c := newConsumer(Config{}, requestCursorReader{}, nil)
	c.runTx = store.runTx
	err := c.foldBatch(context.Background(), sourceBatch{cursor: 31, events: []jetstream.Event{
		{DID: testDID, Seq: 30, Kind: jetstream.KindIdentity, Identity: &jetstream.Identity{DID: testDID, Handle: "reader.example"}},
		{DID: testDID, Seq: 31, Kind: jetstream.KindAccount, Account: &jetstream.Account{DID: testDID, Active: false, Status: "suspended"}},
	}})
	if err != nil {
		t.Fatalf("foldBatch: %v", err)
	}
	if store.handles[testDID] != "reader.example" || store.active[testDID] {
		t.Fatalf("handle/active = %q/%t", store.handles[testDID], store.active[testDID])
	}
	if _, ok := store.records[recordKey(testDID, testCollection, "kept")]; !ok {
		t.Fatal("reversible account state purged the mirror")
	}
	if store.dirty[testDID] != 31 {
		t.Fatalf("dirty generation = %d, want 31", store.dirty[testDID])
	}

	err = c.foldBatch(context.Background(), sourceBatch{cursor: 32, events: []jetstream.Event{
		{DID: testDID, Seq: 32, Kind: jetstream.KindAccount, Account: &jetstream.Account{DID: testDID, Active: false, Status: statusDeleted}},
	}})
	if err != nil {
		t.Fatalf("fold deleted account: %v", err)
	}
	if _, ok := store.records[recordKey(testDID, testCollection, "kept")]; ok {
		t.Fatal("deleted account retained mirror records")
	}
	if store.dirty[testDID] != 32 {
		t.Fatalf("dirty generation = %d, want 32", store.dirty[testDID])
	}
}

func TestOfficialAdapterAddsAPIKeyOptionOnlyWhenConfigured(t *testing.T) {
	for _, tt := range []struct {
		name     string
		apiKey   string
		wantOpts int
	}{{name: "empty", wantOpts: 3}, {name: "configured", apiKey: "secret", wantOpts: 4}} {
		t.Run(tt.name, func(t *testing.T) {
			zero := uint64(0)
			gotHost := ""
			gotOpts := 0
			source, err := subscribeOfficialSource(sourceRequest{
				Host: "wss://jetstream.example", Collections: Collections, AfterSeq: &zero, APIKey: tt.apiKey,
			}, func(host string, opts ...jetstream.Option) (upstreamClient, error) {
				gotHost, gotOpts = host, len(opts)
				return emptyUpstreamClient{}, nil
			})
			if err != nil {
				t.Fatalf("subscribeOfficialSource: %v", err)
			}
			_ = source.Close()
			if gotHost != "https://jetstream.example" || gotOpts != tt.wantOpts {
				t.Fatalf("host/options = %q/%d, want normalized HTTPS host/%d", gotHost, gotOpts, tt.wantOpts)
			}
		})
	}
}

func TestOfficialAdapterNormalizesOnlyWebSocketSchemes(t *testing.T) {
	for _, tt := range []struct {
		input string
		want  string
	}{
		{input: "ws://jetstream.example:8080/path?mode=test", want: "http://jetstream.example:8080/path?mode=test"},
		{input: "wss://jetstream.example/path?mode=test", want: "https://jetstream.example/path?mode=test"},
		{input: "http://jetstream.example/path", want: "http://jetstream.example/path"},
		{input: "https://jetstream.example/path", want: "https://jetstream.example/path"},
		{input: "jetstream.example:8080", want: "jetstream.example:8080"},
	} {
		t.Run(tt.input, func(t *testing.T) {
			got := ""
			source, err := subscribeOfficialSource(sourceRequest{Host: tt.input}, func(host string, _ ...jetstream.Option) (upstreamClient, error) {
				got = host
				return emptyUpstreamClient{}, nil
			})
			if err != nil {
				t.Fatalf("subscribeOfficialSource: %v", err)
			}
			_ = source.Close()
			if got != tt.want {
				t.Fatalf("host = %q, want %q", got, tt.want)
			}
		})
	}
}

func commitEvent(seq uint64, rkey string, record map[string]any) jetstream.Event {
	return jetstream.Event{
		DID:  testDID,
		Seq:  seq,
		Kind: jetstream.KindCommit,
		Commit: &jetstream.Commit{
			Operation:  jetstream.OpCreate,
			Collection: testCollection,
			Rkey:       rkey,
			CID:        testCID,
			Record:     record,
		},
	}
}

func uint64Ptr(v uint64) *uint64 { return &v }

func equalUint64Ptr(a, b *uint64) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

type sourceItem struct {
	batch sourceBatch
	err   error
}

type scriptedEventSource struct {
	items  []sourceItem
	closed bool
}

func (s *scriptedEventSource) Events(context.Context) iter.Seq2[sourceBatch, error] {
	return func(yield func(sourceBatch, error) bool) {
		for _, item := range s.items {
			if !yield(item.batch, item.err) {
				return
			}
		}
	}
}

func (s *scriptedEventSource) Close() error {
	s.closed = true
	return nil
}

type emptyUpstreamClient struct{}

func (emptyUpstreamClient) Events(context.Context) iter.Seq2[*jetstream.Batch, error] {
	return func(func(*jetstream.Batch, error) bool) {}
}

func (emptyUpstreamClient) Close() error { return nil }

func recordKey(did, collection, rkey string) string {
	return did + "|" + collection + "|" + rkey
}

type atomicTestStore struct {
	records  map[string]string
	dirty    map[string]int64
	handles  map[string]string
	active   map[string]bool
	cursor   int64
	failRkey string
	failErr  error
}

func newAtomicTestStore() *atomicTestStore {
	return &atomicTestStore{
		records: map[string]string{}, dirty: map[string]int64{}, handles: map[string]string{}, active: map[string]bool{},
	}
}

func (s *atomicTestStore) clone() *atomicTestStore {
	out := newAtomicTestStore()
	out.cursor = s.cursor
	out.failRkey = s.failRkey
	out.failErr = s.failErr
	for k, v := range s.records {
		out.records[k] = v
	}
	for k, v := range s.dirty {
		out.dirty[k] = v
	}
	for k, v := range s.handles {
		out.handles[k] = v
	}
	for k, v := range s.active {
		out.active[k] = v
	}
	return out
}

func (s *atomicTestStore) runTx(ctx context.Context, fn func(MirrorStore) error) error {
	tx := s.clone()
	if err := fn(tx); err != nil {
		return err
	}
	*s = *tx
	return nil
}

func (s *atomicTestStore) UpsertTapRecord(_ context.Context, arg db.UpsertTapRecordParams) error {
	if arg.Rkey == s.failRkey {
		return s.failErr
	}
	s.records[recordKey(arg.Did, arg.Collection, arg.Rkey)] = arg.Record
	return nil
}

func (s *atomicTestStore) DeleteTapRecord(_ context.Context, arg db.DeleteTapRecordParams) error {
	delete(s.records, recordKey(arg.Did, arg.Collection, arg.Rkey))
	return nil
}

func (s *atomicTestStore) DeleteTapRecordsForRepo(_ context.Context, did string) error {
	for key := range s.records {
		if len(key) > len(did) && key[:len(did)+1] == did+"|" {
			delete(s.records, key)
		}
	}
	return nil
}

func (s *atomicTestStore) UpsertTapRepoHandle(_ context.Context, arg db.UpsertTapRepoHandleParams) error {
	s.handles[arg.Did] = arg.Handle
	return nil
}

func (s *atomicTestStore) UpsertTapRepoAccount(_ context.Context, arg db.UpsertTapRepoAccountParams) error {
	s.active[arg.Did] = arg.IsActive != 0
	return nil
}

func (s *atomicTestStore) MarkTapRepoDirty(_ context.Context, arg db.MarkTapRepoDirtyParams) error {
	if arg.MarkedSeq > s.dirty[arg.Did] {
		s.dirty[arg.Did] = arg.MarkedSeq
	}
	return nil
}

func (s *atomicTestStore) TapRepoIsMirrored(_ context.Context, did string) (bool, error) {
	if _, ok := s.handles[did]; ok {
		return true, nil
	}
	for key := range s.records {
		if len(key) > len(did) && key[:len(did)+1] == did+"|" {
			return true, nil
		}
	}
	return false, nil
}

func (s *atomicTestStore) UpsertDiscoverIngestCursor(_ context.Context, arg db.UpsertDiscoverIngestCursorParams) error {
	s.cursor = arg.Seq
	return nil
}
