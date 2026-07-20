package sync

import (
	"context"
	"testing"

	"morgenblau/internal/database/db"
	"morgenblau/internal/jobs"
)

func TestSyncUser_ReconcileSaves_InsertsAndDeletes(t *testing.T) {
	store := newFakeStore()
	// A local save the PDS no longer has → should be deleted.
	store.saves["did:plc:alice"] = map[string]db.ListUserSavesForSyncRow{
		"goneA": {Did: "did:plc:alice", Rkey: "goneA", AtUri: "at://x/s/goneA", ItemUrl: "https://item/old"},
	}
	feed := "https://feed/new"
	lister := &fakeLister{saves: []PDSSave{
		{URI: "at://x/s/newB", Rkey: "newB", ItemURL: "https://item/new", FeedURL: feed, CreatedAt: "2026-06-01T00:00:00Z"},
	}}
	eng := NewEngine(jobs.New(), store, lister, &countingFetcher{}, nil, nil)

	if err := eng.reconcileSaves(context.Background(), mustDID("did:plc:alice"), newSession("did:plc:alice")); err != nil {
		t.Fatal(err)
	}

	if store.saveUpserts != 1 {
		t.Errorf("saveUpserts = %d, want 1", store.saveUpserts)
	}
	if got, ok := store.saves["did:plc:alice"]["newB"]; !ok {
		t.Error("remote save newB was not inserted locally")
	} else if got.ItemUrl != "https://item/new" || got.FeedUrl == nil || *got.FeedUrl != feed {
		t.Errorf("inserted save = %+v, want item/new + feed/new", got)
	}
	if len(store.saveDeletes) != 1 || store.saveDeletes[0] != "goneA" {
		t.Errorf("saveDeletes = %v, want [goneA]", store.saveDeletes)
	}
	if _, ok := store.saves["did:plc:alice"]["goneA"]; ok {
		t.Error("stale local save goneA was not deleted")
	}
}

// TestSyncUser_ReconcileSaves_EmptyCreatedAtFallsBackToNow guards a save record without createdAt getting a non-empty fallback timestamp.
func TestSyncUser_ReconcileSaves_EmptyCreatedAtFallsBackToNow(t *testing.T) {
	store := newFakeStore()
	lister := &fakeLister{saves: []PDSSave{
		{URI: "at://x/s/noDate", Rkey: "noDate", ItemURL: "https://item/x", CreatedAt: ""},
	}}
	eng := NewEngine(jobs.New(), store, lister, &countingFetcher{}, nil, nil)
	if err := eng.reconcileSaves(context.Background(), mustDID("did:plc:alice"), newSession("did:plc:alice")); err != nil {
		t.Fatal(err)
	}
	got := store.saveUpsertParams["noDate"]
	if got.CreatedAt == "" {
		t.Error("CreatedAt is empty; want the now-fallback timestamp")
	}
}
