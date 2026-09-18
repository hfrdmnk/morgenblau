package sync

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// coreRow stands in for any collection's snapshot row, so these tests exercise the diff and not a schema.
type coreRow struct {
	rkey      string
	createdAt string
}

// coreHarness builds a reconcilePass over coreRow and logs every statement the core issues, in order.
type coreHarness struct {
	local       []coreRow
	desiredKeys []string
	guarded     bool
	deleteFirst bool
	snapshotErr error
	upsertFail  map[string]error
	deleteFail  map[string]error

	ops []string
	// store is what the tx handed the pass closures; the core must never reach past them to a store of its own.
	store    SyncStore
	sawStore bool
}

func (h *coreHarness) pass() reconcilePass[coreRow] {
	p := reconcilePass[coreRow]{
		collection:  "testcollection",
		snapshotAt:  guardSnapshotAt,
		deleteFirst: h.deleteFirst,
		snapshot: func(_ context.Context, q SyncStore) ([]coreRow, error) {
			h.store, h.sawStore = q, true
			if h.snapshotErr != nil {
				return nil, h.snapshotErr
			}
			return h.local, nil
		},
		rkeyOf: func(r coreRow) string { return r.rkey },
		deleteRow: func(_ context.Context, _ SyncStore, rkey string) error {
			h.ops = append(h.ops, "delete:"+rkey)
			return h.deleteFail[rkey]
		},
	}
	if h.guarded {
		p.createdAtOf = func(r coreRow) string { return r.createdAt }
	}
	for _, k := range h.desiredKeys {
		p.desired = append(p.desired, desiredRow{
			rkey: k,
			write: func(_ context.Context, _ SyncStore) error {
				h.ops = append(h.ops, "upsert:"+k)
				return h.upsertFail[k]
			},
		})
	}
	return p
}

// txSpy stands in for Engine.runTx: beginErr fails the whole batch, inner records what the pass closure returned.
type txSpy struct {
	beginErr error
	inner    error
	opened   bool
}

func (t *txSpy) run(ctx context.Context, fn func(SyncStore) error) error {
	if t.beginErr != nil {
		return t.beginErr
	}
	t.opened = true
	t.inner = fn(nil)
	return t.inner
}

func assertOps(t *testing.T, got []string, want ...string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("ops = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("ops = %v, want %v", got, want)
		}
	}
}

func assertOpSet(t *testing.T, got []string, want ...string) {
	t.Helper()
	seen := make(map[string]int, len(got))
	for _, op := range got {
		seen[op]++
	}
	if len(seen) != len(got) {
		t.Fatalf("duplicate ops: %v", got)
	}
	for _, w := range want {
		if seen[w] == 0 {
			t.Fatalf("ops = %v, missing %s", got, w)
		}
		delete(seen, w)
	}
	if len(seen) != 0 {
		t.Fatalf("ops = %v, want exactly %v", got, want)
	}
}

func TestReconcileCollection_DeletesStaleAndUpsertsDesired(t *testing.T) {
	h := &coreHarness{
		local:       []coreRow{{rkey: "a"}, {rkey: "b"}},
		desiredKeys: []string{"b", "c"},
	}
	tx := &txSpy{}

	if err := reconcileCollection(context.Background(), tx.run, h.pass()); err != nil {
		t.Fatal(err)
	}
	assertOpSet(t, h.ops, "upsert:b", "upsert:c", "delete:a")
}

func TestReconcileCollection_EmptyDesiredDeletesEveryLocalRow(t *testing.T) {
	h := &coreHarness{local: []coreRow{{rkey: "a"}, {rkey: "b"}}}
	tx := &txSpy{}

	if err := reconcileCollection(context.Background(), tx.run, h.pass()); err != nil {
		t.Fatal(err)
	}
	assertOpSet(t, h.ops, "delete:a", "delete:b")
}

func TestReconcileCollection_UpsertsRunInDesiredOrder(t *testing.T) {
	h := &coreHarness{desiredKeys: []string{"c", "a", "b"}}
	tx := &txSpy{}

	if err := reconcileCollection(context.Background(), tx.run, h.pass()); err != nil {
		t.Fatal(err)
	}
	assertOps(t, h.ops, "upsert:c", "upsert:a", "upsert:b")
}

// A row written in-app after the PDS listing was taken is absent from that listing without having been deleted remotely.
func TestReconcileCollection_GuardSparesRowsNewerThanTheSnapshot(t *testing.T) {
	cases := []struct {
		name      string
		createdAt string
		wantGone  bool
	}{
		{"created after the snapshot", "2026-07-20T12:00:01Z", false},
		{"created before the snapshot", "2026-07-20T11:00:00Z", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := &coreHarness{guarded: true, local: []coreRow{{rkey: "a", createdAt: tc.createdAt}}}
			tx := &txSpy{}

			if err := reconcileCollection(context.Background(), tx.run, h.pass()); err != nil {
				t.Fatal(err)
			}
			if tc.wantGone {
				assertOps(t, h.ops, "delete:a")
				return
			}
			assertOps(t, h.ops)
		})
	}
}

// A pass whose snapshot query carries no created_at leaves createdAtOf nil and deletes on absence alone.
func TestReconcileCollection_WithoutCreatedAtOf_DeletesUnguarded(t *testing.T) {
	h := &coreHarness{local: []coreRow{{rkey: "a", createdAt: "2026-07-20T12:00:01Z"}}}
	tx := &txSpy{}

	if err := reconcileCollection(context.Background(), tx.run, h.pass()); err != nil {
		t.Fatal(err)
	}
	assertOps(t, h.ops, "delete:a")
}

func TestReconcileCollection_PerStatementErrorsAreTolerated(t *testing.T) {
	h := &coreHarness{
		local:       []coreRow{{rkey: "a"}, {rkey: "b"}},
		desiredKeys: []string{"c", "d"},
		upsertFail:  map[string]error{"c": errors.New("upsert boom")},
		deleteFail:  map[string]error{"a": errors.New("delete boom")},
	}
	tx := &txSpy{}

	if err := reconcileCollection(context.Background(), tx.run, h.pass()); err != nil {
		t.Fatalf("a failing statement must not fail the pass: %v", err)
	}
	assertOpSet(t, h.ops, "upsert:c", "upsert:d", "delete:a", "delete:b")
	if tx.inner != nil {
		t.Errorf("tx closure returned %v, want nil so the batch commits", tx.inner)
	}
}

func TestReconcileCollection_SnapshotFailureRollsBackBeforeAnyWrite(t *testing.T) {
	boom := errors.New("snapshot boom")
	h := &coreHarness{snapshotErr: boom, local: []coreRow{{rkey: "a"}}, desiredKeys: []string{"c"}}
	tx := &txSpy{}

	if err := reconcileCollection(context.Background(), tx.run, h.pass()); !errors.Is(err, boom) {
		t.Fatalf("err = %v, want %v", err, boom)
	}
	assertOps(t, h.ops)
	if tx.inner == nil {
		t.Error("tx closure returned nil; the batch would commit despite the failed read")
	}
}

func TestReconcileCollection_TxFailurePropagatesWithNoWrites(t *testing.T) {
	boom := errors.New("begin boom")
	h := &coreHarness{local: []coreRow{{rkey: "a"}}, desiredKeys: []string{"c"}}
	tx := &txSpy{beginErr: boom}

	if err := reconcileCollection(context.Background(), tx.run, h.pass()); !errors.Is(err, boom) {
		t.Fatalf("err = %v, want %v", err, boom)
	}
	assertOps(t, h.ops)
	if tx.opened {
		t.Error("the pass ran despite the tx failing to open")
	}
}

// Where a non-rkey unique index exists, a rekeyed record's stale row must vacate before its replacement upserts.
func TestReconcileCollection_DeleteFirstOrdersTheWholePass(t *testing.T) {
	h := &coreHarness{
		deleteFirst: true,
		local:       []coreRow{{rkey: "old"}},
		desiredKeys: []string{"new"},
	}
	tx := &txSpy{}

	if err := reconcileCollection(context.Background(), tx.run, h.pass()); err != nil {
		t.Fatal(err)
	}
	assertOps(t, h.ops, "delete:old", "upsert:new")
}

func TestReconcileCollection_DeleteFirstDeleteFailureRollsBack(t *testing.T) {
	boom := errors.New("delete boom")
	h := &coreHarness{
		deleteFirst: true,
		local:       []coreRow{{rkey: "old"}},
		desiredKeys: []string{"new"},
		deleteFail:  map[string]error{"old": boom},
	}
	tx := &txSpy{}

	err := reconcileCollection(context.Background(), tx.run, h.pass())
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, want wrapped %v", err, boom)
	}
	for _, context := range []string{"testcollection", "delete", "old"} {
		if !strings.Contains(err.Error(), context) {
			t.Errorf("err = %q, want %q context", err, context)
		}
	}
	assertOps(t, h.ops, "delete:old")
	if tx.inner == nil {
		t.Error("tx closure returned nil; the stale delete would commit")
	}
}

func TestReconcileCollection_DeleteFirstDesiredWriteFailureRollsBackStaleDelete(t *testing.T) {
	boom := errors.New("upsert boom")
	h := &coreHarness{
		deleteFirst: true,
		local:       []coreRow{{rkey: "old"}},
		desiredKeys: []string{"new"},
		upsertFail:  map[string]error{"new": boom},
	}
	tx := &txSpy{}

	err := reconcileCollection(context.Background(), tx.run, h.pass())
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, want wrapped %v", err, boom)
	}
	for _, context := range []string{"testcollection", "upsert", "new"} {
		if !strings.Contains(err.Error(), context) {
			t.Errorf("err = %q, want %q context", err, context)
		}
	}
	assertOps(t, h.ops, "delete:old", "upsert:new")
	if tx.inner == nil {
		t.Error("tx closure returned nil; the stale delete would commit")
	}
}

func TestReconcileCollection_WithoutDeleteFirstUpsertsLead(t *testing.T) {
	h := &coreHarness{
		local:       []coreRow{{rkey: "old"}},
		desiredKeys: []string{"new"},
	}
	tx := &txSpy{}

	if err := reconcileCollection(context.Background(), tx.run, h.pass()); err != nil {
		t.Fatal(err)
	}
	assertOps(t, h.ops, "upsert:new", "delete:old")
}

// Every statement goes through the pass's own closures, so the core can never reach a non-tx store and block on the sole writer connection.
func TestReconcileCollection_ReadsOnlyTheStoreTheTxHandsIt(t *testing.T) {
	h := &coreHarness{local: []coreRow{{rkey: "a"}}, desiredKeys: []string{"c"}}
	tx := &txSpy{}

	if err := reconcileCollection(context.Background(), tx.run, h.pass()); err != nil {
		t.Fatal(err)
	}
	if !h.sawStore || h.store != nil {
		t.Errorf("snapshot got store %v (called = %v), want the tx's own", h.store, h.sawStore)
	}
}
