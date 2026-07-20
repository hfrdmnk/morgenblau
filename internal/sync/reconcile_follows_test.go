package sync

import (
	"context"
	"errors"
	"testing"

	"morgenblau/internal/database/db"
	"morgenblau/internal/jobs"
)

func TestReconcileFollows_InsertsAndDeletes(t *testing.T) {
	store := newFakeStore()
	// A local follow the PDS no longer has → should be deleted.
	store.follows["did:plc:alice"] = map[string]db.ListUserFollowsForSyncRow{
		"goneA": {Did: "did:plc:alice", Rkey: "goneA", AtUri: "at://x/f/goneA", SubjectDid: "did:plc:old"},
	}
	lister := &fakeLister{follows: []PDSFollow{
		{URI: "at://x/f/newB", Rkey: "newB", SubjectDID: "did:plc:bob", CreatedAt: "2026-07-01T00:00:00Z"},
	}}
	eng := NewEngine(jobs.New(), store, lister, &countingFetcher{}, nil, nil)

	if err := eng.reconcileFollows(context.Background(), mustDID("did:plc:alice"), newSession("did:plc:alice")); err != nil {
		t.Fatal(err)
	}

	if store.followUpserts != 1 {
		t.Errorf("followUpserts = %d, want 1", store.followUpserts)
	}
	got, ok := store.follows["did:plc:alice"]["newB"]
	if !ok {
		t.Fatal("remote follow newB was not inserted locally")
	}
	if got.SubjectDid != "did:plc:bob" {
		t.Errorf("SubjectDid = %q, want did:plc:bob", got.SubjectDid)
	}
	if len(store.followDeletes) != 1 || store.followDeletes[0] != "goneA" {
		t.Errorf("followDeletes = %v, want [goneA]", store.followDeletes)
	}
	if _, ok := store.follows["did:plc:alice"]["goneA"]; ok {
		t.Error("stale local follow goneA was not deleted")
	}
}

func TestReconcileFollows_EmptyCreatedAtFallsBackToNow(t *testing.T) {
	store := newFakeStore()
	lister := &fakeLister{follows: []PDSFollow{
		{URI: "at://x/f/noDate", Rkey: "noDate", SubjectDID: "did:plc:bob", CreatedAt: ""},
	}}
	eng := NewEngine(jobs.New(), store, lister, &countingFetcher{}, nil, nil)
	if err := eng.reconcileFollows(context.Background(), mustDID("did:plc:alice"), newSession("did:plc:alice")); err != nil {
		t.Fatal(err)
	}
	got := store.followUpsertParams["noDate"]
	if got.CreatedAt == "" {
		t.Error("CreatedAt is empty; want the now-fallback timestamp")
	}
}

// Two devices can each write a follow record for the same subject before syncing; reconcile must converge to one row (the canonical, oldest rkey) and stay converged across repeats.
func TestReconcileFollows_DuplicateSubjectRecords_ConvergesOnCanonical(t *testing.T) {
	store := newFakeStore()
	lister := &fakeLister{follows: []PDSFollow{
		{URI: "at://x/f/3jzzzzzzzzz", Rkey: "3jzzzzzzzzz", SubjectDID: "did:plc:bob", CreatedAt: "2026-07-02T00:00:00Z"},
		{URI: "at://x/f/3jaaaaaaaaa", Rkey: "3jaaaaaaaaa", SubjectDID: "did:plc:bob", CreatedAt: "2026-07-01T00:00:00Z"},
	}}
	eng := NewEngine(jobs.New(), store, lister, &countingFetcher{}, nil, nil)

	if err := eng.reconcileFollows(context.Background(), mustDID("did:plc:alice"), newSession("did:plc:alice")); err != nil {
		t.Fatal(err)
	}
	rows := store.follows["did:plc:alice"]
	if len(rows) != 1 {
		t.Fatalf("local follows = %d rows, want 1: %+v", len(rows), rows)
	}
	if _, ok := rows["3jaaaaaaaaa"]; !ok {
		t.Fatalf("expected canonical (smallest) rkey 3jaaaaaaaaa to survive, got %+v", rows)
	}

	// Repeat reconcile: must stay converged, no duplicate upserts, no deletes.
	if err := eng.reconcileFollows(context.Background(), mustDID("did:plc:alice"), newSession("did:plc:alice")); err != nil {
		t.Fatal(err)
	}
	rows = store.follows["did:plc:alice"]
	if len(rows) != 1 {
		t.Fatalf("after repeat reconcile: local follows = %d rows, want 1: %+v", len(rows), rows)
	}
	if len(store.followDeletes) != 0 {
		t.Errorf("followDeletes = %v, want none (canonical row never changed)", store.followDeletes)
	}
}

func TestReconcileFollows_RemovesSelfFollowFromPDSAndIndex(t *testing.T) {
	const did = "did:plc:reader"
	store := newFakeStore()
	store.follows[did] = map[string]db.ListUserFollowsForSyncRow{
		"3self": {Did: did, Rkey: "3self", AtUri: "at://" + did + "/blue.morgen.graph.follow/3self", SubjectDid: did},
	}
	lister := &fakeLister{follows: []PDSFollow{
		{URI: "at://" + did + "/blue.morgen.graph.follow/3self", Rkey: "3self", SubjectDID: did},
		{URI: "at://" + did + "/blue.morgen.graph.follow/3other", Rkey: "3other", SubjectDID: "did:plc:other"},
	}}
	pds := &fakeRecordWriter{}
	eng := NewEngine(jobs.New(), store, lister, &countingFetcher{}, nil, pds)

	if err := eng.reconcileFollows(context.Background(), mustDID(did), newSession(did)); err != nil {
		t.Fatal(err)
	}

	if len(pds.deleted) != 1 || pds.deleted[0] != "blue.morgen.graph.follow/3self" {
		t.Errorf("PDS deletes = %v, want self-follow tombstoned", pds.deleted)
	}
	if _, ok := store.follows[did]["3self"]; ok {
		t.Error("self-follow remains in the local index")
	}
	if got, ok := store.follows[did]["3other"]; !ok || got.SubjectDid != "did:plc:other" {
		t.Errorf("normal follow = %+v, present = %v", got, ok)
	}
}

// The previously-canonical record disappears and two other duplicates take its place; reconcile must delete the stale row and converge on the new (smallest surviving) canonical rkey in one pass.
func TestReconcileFollows_CanonicalRecordDisappears_SurvivorTakesOver(t *testing.T) {
	store := newFakeStore()
	store.follows["did:plc:alice"] = map[string]db.ListUserFollowsForSyncRow{
		"3jaaaaaaaaa": {Did: "did:plc:alice", Rkey: "3jaaaaaaaaa", AtUri: "at://x/f/3jaaaaaaaaa", SubjectDid: "did:plc:bob"},
	}
	lister := &fakeLister{follows: []PDSFollow{
		{URI: "at://x/f/3jzzzzzzzzz", Rkey: "3jzzzzzzzzz", SubjectDID: "did:plc:bob", CreatedAt: "2026-07-03T00:00:00Z"},
		{URI: "at://x/f/3jbbbbbbbbb", Rkey: "3jbbbbbbbbb", SubjectDID: "did:plc:bob", CreatedAt: "2026-07-02T00:00:00Z"},
	}}
	eng := NewEngine(jobs.New(), store, lister, &countingFetcher{}, nil, nil)

	if err := eng.reconcileFollows(context.Background(), mustDID("did:plc:alice"), newSession("did:plc:alice")); err != nil {
		t.Fatal(err)
	}

	rows := store.follows["did:plc:alice"]
	if len(rows) != 1 {
		t.Fatalf("local follows = %d rows, want 1: %+v", len(rows), rows)
	}
	if _, ok := rows["3jbbbbbbbbb"]; !ok {
		t.Errorf("expected new canonical (smallest surviving) rkey 3jbbbbbbbbb, got %+v", rows)
	}
	if _, ok := rows["3jaaaaaaaaa"]; ok {
		t.Errorf("stale local row 3jaaaaaaaaa should have been deleted, got %+v", rows)
	}
	if len(store.followDeletes) != 1 || store.followDeletes[0] != "3jaaaaaaaaa" {
		t.Errorf("followDeletes = %v, want [3jaaaaaaaaa]", store.followDeletes)
	}
}

func TestReconcileFollows_ListFailure_NoMutation(t *testing.T) {
	store := newFakeStore()
	store.follows["did:plc:alice"] = map[string]db.ListUserFollowsForSyncRow{
		"3local": {Did: "did:plc:alice", Rkey: "3local", SubjectDid: "did:plc:bob"},
	}
	lister := &fakeLister{followsErr: errors.New("pds down")}
	eng := NewEngine(jobs.New(), store, lister, &countingFetcher{}, nil, nil)

	if err := eng.reconcileFollows(context.Background(), mustDID("did:plc:alice"), newSession("did:plc:alice")); err == nil {
		t.Fatal("expected error")
	}
	if store.followUpserts != 0 || len(store.followDeletes) != 0 {
		t.Errorf("mutations despite list failure: upserts=%d deletes=%v", store.followUpserts, store.followDeletes)
	}
}
