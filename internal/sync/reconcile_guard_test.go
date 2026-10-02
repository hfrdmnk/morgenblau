package sync

import (
	"context"
	"go/ast"
	"testing"
	"time"

	"morgenblau/internal/database/db"
	"morgenblau/internal/jobs"
)

const guardDID = "did:plc:alice"

var guardSnapshotAt = time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)

func TestUpdatedAfterSnapshot(t *testing.T) {
	cases := []struct {
		name      string
		updatedAt string
		want      bool
	}{
		{"after snapshot", "2026-07-20T12:00:01Z", true},
		{"same second as snapshot", "2026-07-20T12:00:00Z", false},
		{"before snapshot", "2026-07-20T11:59:59Z", false},
		{"offset resolving after snapshot", "2026-07-20T14:00:05+02:00", true},
		{"invalid", "not-a-timestamp", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := updatedAfterSnapshot(tc.updatedAt, guardSnapshotAt); got != tc.want {
				t.Errorf("updatedAfterSnapshot(%q, %s) = %v, want %v", tc.updatedAt, guardSnapshotAt.Format(time.RFC3339), got, tc.want)
			}
		})
	}
}

func TestReconcile_StaleSubscriptionListingPreservesSameSecondMirrorUpdate(t *testing.T) {
	for _, staleListing := range []struct {
		name string
		rows []PDSSubscription
	}{
		{name: "remote record missing", rows: nil},
		{name: "remote metadata is stale", rows: []PDSSubscription{{URI: "at://" + guardDID + "/blue.morgen.feed.subscription/3sub", Kind: "rss", Rkey: "3sub", FeedURL: "https://example.com/feed", Title: "Old"}}},
	} {
		t.Run(staleListing.name, func(t *testing.T) {
			store := newFakeStore()
			store.rows[guardDID] = map[string]db.UserSubscription{
				"3sub": {Did: guardDID, Rkey: "3sub", AtUri: "at://" + guardDID + "/blue.morgen.feed.subscription/3sub", FeedUrl: "https://example.com/feed", Kind: "rss", Title: strPtr("Old"), UpdatedAt: "2026-07-20T11:59:59Z"},
			}
			lister := &fakeLister{subs: staleListing.rows}
			lister.beforeSubs = func() {
				store.mu.Lock()
				defer store.mu.Unlock()
				row := store.rows[guardDID]["3sub"]
				row.Title = strPtr("New local title")
				row.UpdatedAt = guardSnapshotAt.Format(time.RFC3339)
				store.rows[guardDID]["3sub"] = row
			}
			eng := NewEngine(jobs.New(), store, lister, &countingFetcher{}, nil, nil)
			eng.now = func() time.Time { return guardSnapshotAt }
			baseline, err := store.ListUserSubscriptionsForSync(context.Background(), guardDID)
			if err != nil {
				t.Fatal(err)
			}

			if _, _, err := eng.reconcileTier1(context.Background(), mustDID(guardDID), newSession(guardDID), baseline, guardSnapshotAt, func(string) {}); err != nil {
				t.Fatal(err)
			}
			row := store.rows[guardDID]["3sub"]
			if row.Title == nil || *row.Title != "New local title" {
				t.Fatalf("local title = %v, want the concurrent mirror value", row.Title)
			}
			if len(store.deletes) != 0 {
				t.Errorf("deletes = %v, want none", store.deletes)
			}
			if staleListing.rows != nil && store.upserts != 0 {
				t.Errorf("reconcile upserts = %d, want none for the stale rkey", store.upserts)
			}
		})
	}
}

// Drives runDualTrack so the baseline both subscription passes guard against is the one the engine itself took before listing.
func TestSyncUser_StaleSubscriptionListingPreservesMirrorWritesInBothPasses(t *testing.T) {
	store := newFakeStore()
	store.rows[guardDID] = map[string]db.UserSubscription{
		"3sub": {Did: guardDID, Rkey: "3sub", AtUri: "at://" + guardDID + "/blue.morgen.feed.subscription/3sub", FeedUrl: "https://example.com/feed", Kind: "rss", Title: strPtr("Old"), UpdatedAt: "2026-07-20T11:59:59Z"},
		"3std": {Did: guardDID, Rkey: "3std", AtUri: "at://" + guardDID + "/site.standard.graph.subscription/3std", FeedUrl: pubA, Kind: "standardfeed", Title: strPtr("Old"), UpdatedAt: "2026-07-20T11:59:59Z"},
	}
	lister := &fakeLister{}
	lister.beforeSubs = func() {
		store.mu.Lock()
		defer store.mu.Unlock()
		for rkey, row := range store.rows[guardDID] {
			row.Title = strPtr("New local title")
			row.UpdatedAt = guardSnapshotAt.Format(time.RFC3339)
			store.rows[guardDID][rkey] = row
		}
	}
	eng := NewEngine(jobs.New(), store, lister, &countingFetcher{}, nil, nil)
	eng.now = func() time.Time { return guardSnapshotAt }

	if _, err := eng.runDualTrack(context.Background(), mustDID(guardDID), newSession(guardDID)); err != nil {
		t.Fatal(err)
	}
	for _, rkey := range []string{"3sub", "3std"} {
		row, ok := store.rows[guardDID][rkey]
		if !ok || row.Title == nil || *row.Title != "New local title" {
			t.Errorf("%s = %+v (present %v), want the concurrent mirror write", rkey, row, ok)
		}
	}
	if len(store.deletes) != 0 {
		t.Errorf("deletes = %v, want none", store.deletes)
	}
}

func TestReconcile_StaleSaveListingPreservesSameSecondMirrorInsert(t *testing.T) {
	store := newFakeStore()
	lister := &fakeLister{}
	lister.beforeSaves = func() {
		store.mu.Lock()
		defer store.mu.Unlock()
		store.saves[guardDID] = map[string]db.UserSave{
			"3save": {Did: guardDID, Rkey: "3save", AtUri: "at://" + guardDID + "/blue.morgen.feed.save/3save", ItemUrl: "https://example.com/post", CreatedAt: "2026-07-20T12:00:00Z", UpdatedAt: guardSnapshotAt.Format(time.RFC3339)},
		}
	}
	eng := NewEngine(jobs.New(), store, lister, &countingFetcher{}, nil, nil)
	eng.now = func() time.Time { return guardSnapshotAt }

	if _, err := eng.reconcileSaves(context.Background(), mustDID(guardDID), newSession(guardDID)); err != nil {
		t.Fatal(err)
	}
	if _, ok := store.saves[guardDID]["3save"]; !ok {
		t.Fatal("same-second local mirror insert was deleted by the stale listing")
	}
	if len(store.saveDeletes) != 0 {
		t.Errorf("save deletes = %v, want none", store.saveDeletes)
	}
}

// Omitting any of these weakens the guard for one collection without failing that collection's other tests.
var listingGuardFields = []string{"snapshotAt", "baseline", "changedSinceSnapshot", "updatedAtOf"}

func TestEveryReconcilePassCarriesTheListingGuard(t *testing.T) {
	passes := 0
	fset, files := parseGoSources(t, ".")
	for _, file := range files {
		ast.Inspect(file, func(n ast.Node) bool {
			lit, ok := n.(*ast.CompositeLit)
			if !ok || !isReconcilePassType(lit.Type) {
				return true
			}
			passes++
			set := map[string]bool{}
			for _, elt := range lit.Elts {
				kv, ok := elt.(*ast.KeyValueExpr)
				if !ok {
					continue
				}
				key, _ := kv.Key.(*ast.Ident)
				value, isIdent := kv.Value.(*ast.Ident)
				if key != nil && !(isIdent && value.Name == "nil") {
					set[key.Name] = true
				}
			}
			for _, field := range listingGuardFields {
				if !set[field] {
					t.Errorf("%s: reconcile pass omits %s", fset.Position(lit.Pos()), field)
				}
			}
			return true
		})
	}
	if passes == 0 {
		t.Error("found no reconcilePass literals")
	}
}

func isReconcilePassType(expr ast.Expr) bool {
	if index, ok := expr.(*ast.IndexExpr); ok {
		expr = index.X
	}
	ident, ok := expr.(*ast.Ident)
	return ok && ident.Name == "reconcilePass"
}

func strPtr(s string) *string { return &s }
