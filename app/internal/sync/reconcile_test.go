package sync

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

type coreRow struct {
	rkey      string
	updatedAt string
}

type coreHarness struct {
	local        []coreRow
	baseline     []coreRow
	desiredKeys  []string
	updatedGuard bool
	changedGuard bool
	snapshotErr  error
	upsertFail   map[string]error
	deleteFail   map[string]error

	ops      []string
	store    SyncStore
	sawStore bool
}

func (h *coreHarness) pass() reconcilePass[coreRow] {
	p := reconcilePass[coreRow]{
		collection: "testcollection",
		snapshotAt: guardSnapshotAt,
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
	p.baseline = h.baseline
	if p.baseline == nil {
		p.baseline = h.local
	}
	if h.updatedGuard {
		p.updatedAtOf = func(r coreRow) string { return r.updatedAt }
	}
	if h.changedGuard {
		p.changedSinceSnapshot = func(current, baseline coreRow) bool {
			return current.updatedAt != baseline.updatedAt
		}
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

type txSpy struct {
	beginErr  error
	commitErr error
	inner     error
	opened    bool
}

func (t *txSpy) run(ctx context.Context, fn func(SyncStore) error) error {
	if t.beginErr != nil {
		return t.beginErr
	}
	t.opened = true
	t.inner = fn(nil)
	if t.inner != nil {
		return t.inner
	}
	return t.commitErr
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

	if _, err := reconcileCollection(context.Background(), tx.run, h.pass()); err != nil {
		t.Fatal(err)
	}
	assertOpSet(t, h.ops, "upsert:b", "upsert:c", "delete:a")
}

func TestReconcileCollection_EmptyDesiredDeletesEveryLocalRow(t *testing.T) {
	h := &coreHarness{local: []coreRow{{rkey: "a"}, {rkey: "b"}}}
	tx := &txSpy{}

	if _, err := reconcileCollection(context.Background(), tx.run, h.pass()); err != nil {
		t.Fatal(err)
	}
	assertOpSet(t, h.ops, "delete:a", "delete:b")
}

func TestReconcileCollection_UpsertsRunInDesiredOrder(t *testing.T) {
	h := &coreHarness{desiredKeys: []string{"c", "a", "b"}}
	tx := &txSpy{}

	if _, err := reconcileCollection(context.Background(), tx.run, h.pass()); err != nil {
		t.Fatal(err)
	}
	assertOps(t, h.ops, "upsert:c", "upsert:a", "upsert:b")
}

func TestReconcileCollection_SparesRowInsertedDuringListing(t *testing.T) {
	h := &coreHarness{local: []coreRow{{rkey: "a"}, {rkey: "b"}}, baseline: []coreRow{{rkey: "b"}}}
	tx := &txSpy{}

	if _, err := reconcileCollection(context.Background(), tx.run, h.pass()); err != nil {
		t.Fatal(err)
	}
	assertOps(t, h.ops, "delete:b")
}

func TestReconcileCollection_DoesNotResurrectRowDeletedDuringListing(t *testing.T) {
	h := &coreHarness{local: []coreRow{}, baseline: []coreRow{{rkey: "a"}}, desiredKeys: []string{"a", "c"}}
	tx := &txSpy{}

	if _, err := reconcileCollection(context.Background(), tx.run, h.pass()); err != nil {
		t.Fatal(err)
	}
	assertOps(t, h.ops, "upsert:c")
}

func TestReconcileCollection_SparesLocalSubscriptionUpdatedAfterSnapshot(t *testing.T) {
	h := &coreHarness{
		updatedGuard: true,
		local:        []coreRow{{rkey: "a", updatedAt: "2026-07-20T12:00:01Z"}},
		baseline:     []coreRow{{rkey: "a", updatedAt: "2026-07-20T11:59:59Z"}},
		desiredKeys:  []string{"a"},
	}
	tx := &txSpy{}

	if _, err := reconcileCollection(context.Background(), tx.run, h.pass()); err != nil {
		t.Fatal(err)
	}
	assertOps(t, h.ops)
}

func TestReconcileCollection_SparesLocalSubscriptionUpdatedInSnapshotSecond(t *testing.T) {
	h := &coreHarness{
		changedGuard: true,
		local:        []coreRow{{rkey: "a", updatedAt: guardSnapshotAt.Format(time.RFC3339)}},
		baseline:     []coreRow{{rkey: "a", updatedAt: "2026-07-20T11:59:59Z"}},
		desiredKeys:  []string{"a"},
	}
	tx := &txSpy{}

	if _, err := reconcileCollection(context.Background(), tx.run, h.pass()); err != nil {
		t.Fatal(err)
	}
	assertOps(t, h.ops)
}

func TestReconcileCollection_PerStatementErrorsFailThePass(t *testing.T) {
	h := &coreHarness{
		local:       []coreRow{{rkey: "a"}, {rkey: "b"}},
		desiredKeys: []string{"c", "d"},
		upsertFail:  map[string]error{"c": errors.New("upsert boom")},
		deleteFail:  map[string]error{"a": errors.New("delete boom")},
	}
	tx := &txSpy{}

	if _, err := reconcileCollection(context.Background(), tx.run, h.pass()); err == nil {
		t.Fatal("reconcile returned nil despite a failed local write")
	}
	assertOps(t, h.ops, "delete:a")
	if tx.inner == nil {
		t.Error("tx closure returned nil; the batch would commit despite the failed write")
	}
}

func TestReconcileCollection_SnapshotFailureRollsBackBeforeAnyWrite(t *testing.T) {
	boom := errors.New("snapshot boom")
	h := &coreHarness{snapshotErr: boom, local: []coreRow{{rkey: "a"}}, desiredKeys: []string{"c"}}
	tx := &txSpy{}

	if _, err := reconcileCollection(context.Background(), tx.run, h.pass()); !errors.Is(err, boom) {
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

	if _, err := reconcileCollection(context.Background(), tx.run, h.pass()); !errors.Is(err, boom) {
		t.Fatalf("err = %v, want %v", err, boom)
	}
	assertOps(t, h.ops)
	if tx.opened {
		t.Error("the pass ran despite the tx failing to open")
	}
}

func TestReconcileCollection_CommitProofOnlyAfterTheTransactionCommits(t *testing.T) {
	boom := errors.New("tx boom")
	cases := []struct {
		name string
		h    *coreHarness
		tx   *txSpy
	}{
		{"begin fails", &coreHarness{desiredKeys: []string{"c"}}, &txSpy{beginErr: boom}},
		{"a statement fails", &coreHarness{desiredKeys: []string{"c"}, upsertFail: map[string]error{"c": boom}}, &txSpy{}},
		{"commit fails after a clean closure", &coreHarness{desiredKeys: []string{"c"}}, &txSpy{commitErr: boom}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			proof, err := reconcileCollection(context.Background(), tc.tx.run, tc.h.pass())
			if !errors.Is(err, boom) {
				t.Fatalf("err = %v, want %v", err, boom)
			}
			if proof != nil {
				t.Fatal("reconcileCollection minted a commit proof for a transaction that did not commit")
			}
		})
	}

	proof, err := reconcileCollection(context.Background(), (&txSpy{}).run, (&coreHarness{desiredKeys: []string{"c"}}).pass())
	if err != nil || proof == nil {
		t.Fatalf("committed pass: proof = %v, err = %v, want a proof", proof, err)
	}
}

func TestReconcileCollection_StaleRowsLeaveBeforeDesiredRowsUpsert(t *testing.T) {
	h := &coreHarness{
		local:       []coreRow{{rkey: "old"}},
		desiredKeys: []string{"new"},
	}
	tx := &txSpy{}

	if _, err := reconcileCollection(context.Background(), tx.run, h.pass()); err != nil {
		t.Fatal(err)
	}
	if len(h.ops) != 2 || h.ops[0] != "delete:old" || h.ops[1] != "upsert:new" {
		t.Fatalf("ops = %v, want [delete:old upsert:new]: a record recreated under a new rkey keeps its other unique keys, such as UNIQUE(did, feed_url), so its stale row must leave first", h.ops)
	}
}

func TestReconcileCollection_StaleDeleteFailureRollsBack(t *testing.T) {
	boom := errors.New("delete boom")
	h := &coreHarness{
		local:       []coreRow{{rkey: "old"}},
		desiredKeys: []string{"new"},
		deleteFail:  map[string]error{"old": boom},
	}
	tx := &txSpy{}

	_, err := reconcileCollection(context.Background(), tx.run, h.pass())
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

func TestReconcileCollection_DesiredWriteFailureRollsBackStaleDelete(t *testing.T) {
	boom := errors.New("upsert boom")
	h := &coreHarness{
		local:       []coreRow{{rkey: "old"}},
		desiredKeys: []string{"new"},
		upsertFail:  map[string]error{"new": boom},
	}
	tx := &txSpy{}

	_, err := reconcileCollection(context.Background(), tx.run, h.pass())
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

func TestReconcileCollection_ReadsOnlyTheStoreTheTxHandsIt(t *testing.T) {
	h := &coreHarness{local: []coreRow{{rkey: "a"}}, desiredKeys: []string{"c"}}
	tx := &txSpy{}

	if _, err := reconcileCollection(context.Background(), tx.run, h.pass()); err != nil {
		t.Fatal(err)
	}
	if !h.sawStore || h.store != nil {
		t.Errorf("snapshot got store %v (called = %v), want the tx's own: a store outside the tx blocks on the sole writer connection", h.store, h.sawStore)
	}
}
