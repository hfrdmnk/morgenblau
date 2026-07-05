package sync

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/bluesky-social/indigo/atproto/auth/oauth"
	"github.com/bluesky-social/indigo/atproto/syntax"

	"morgenblau/internal/atprepo"
	"morgenblau/internal/jobs"
)

// fakeRecordWriter records DeleteRecord calls — the reconcile write exception.
type fakeRecordWriter struct {
	mu      sync.Mutex
	deleted []string // "<collection>/<rkey>"
}

func (f *fakeRecordWriter) CreateRecord(context.Context, *oauth.ClientSession, syntax.NSID, map[string]any) (*atprepo.RecordRef, error) {
	return nil, errors.New("reconcile must never create records")
}

func (f *fakeRecordWriter) PutRecord(context.Context, *oauth.ClientSession, syntax.NSID, string, map[string]any) (*atprepo.RecordRef, error) {
	return nil, errors.New("reconcile must never put records")
}

func (f *fakeRecordWriter) DeleteRecord(_ context.Context, _ *oauth.ClientSession, collection syntax.NSID, rkey string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.deleted = append(f.deleted, collection.String()+"/"+rkey)
	return nil
}

const (
	pubA = "at://did:plc:puba/site.standard.publication/3pa"
	pubB = "at://did:plc:pubb/site.standard.publication/3pb"
)

func stdSub(rkey, pub string) PDSStandardSubscription {
	return PDSStandardSubscription{
		URI:         "at://did:plc:alice/site.standard.graph.subscription/" + rkey,
		Rkey:        rkey,
		Publication: pub,
	}
}

func sidecar(rkey, pub, title string) PDSSubscription {
	return PDSSubscription{
		URI:         "at://did:plc:alice/blue.morgen.feed.subscription/" + rkey,
		Rkey:        rkey,
		Kind:        "standardfeed",
		Publication: pub,
		Title:       title,
	}
}

func runStandardReconcile(t *testing.T, store *fakeStore, lister *fakeLister, pds atprepo.Writer) []string {
	t.Helper()
	eng := NewEngine(jobs.New(), store, lister, &countingFetcher{}, nil, pds)
	var added []string
	var mu sync.Mutex
	snapshot, err := store.ListUserSubscriptionsForSync(context.Background(), "did:plc:alice")
	if err != nil {
		t.Fatal(err)
	}
	err = eng.reconcileTier1(context.Background(), mustDID("did:plc:alice"), newSession("did:plc:alice"), snapshot, func(url string) {
		mu.Lock()
		added = append(added, url)
		mu.Unlock()
	})
	if err != nil {
		t.Fatal(err)
	}
	return added
}

// Foreign adoption: a standard record created in another app becomes a local
// source with kind=standardfeed, a Tier-2 row keyed by the publication, and
// an onAdded trigger for the Phase-2 document fetch.
func TestStandardReconcile_AdoptsForeignSubscription(t *testing.T) {
	store := newFakeStore()
	lister := &fakeLister{standardSubs: []PDSStandardSubscription{stdSub("3s1", pubA)}}

	added := runStandardReconcile(t, store, lister, nil)

	if len(added) != 1 || added[0] != pubA {
		t.Fatalf("onAdded = %v, want [%s]", added, pubA)
	}
	up, ok := store.upsertParams["3s1"]
	if !ok {
		t.Fatalf("no Tier-1 upsert for 3s1: %+v", store.upsertParams)
	}
	if up.Kind != "standardfeed" || up.FeedUrl != pubA || up.SidecarRkey != nil || up.Title != nil {
		t.Fatalf("Tier-1 upsert: %+v", up)
	}
	var feedKind any
	for _, fp := range store.feedParams {
		if fp.FeedUrl == pubA {
			feedKind = fp.Kind
		}
	}
	if feedKind != "standardfeed" {
		t.Fatalf("Tier-2 kind = %v, want standardfeed", feedKind)
	}
}

func TestStandardReconcile_SidecarJoinAppliesMetadata(t *testing.T) {
	store := newFakeStore()
	lister := &fakeLister{
		subs: []PDSSubscription{{
			URI: "at://did:plc:alice/blue.morgen.feed.subscription/3sc", Rkey: "3sc",
			Kind: "standardfeed", Publication: pubA,
			Title: "My Journal", Primary: true, Tags: []string{"tech"},
		}},
		standardSubs: []PDSStandardSubscription{stdSub("3s1", pubA)},
	}

	runStandardReconcile(t, store, lister, nil)

	up := store.upsertParams["3s1"]
	if up.Title == nil || *up.Title != "My Journal" {
		t.Fatalf("title = %v, want My Journal", up.Title)
	}
	if up.IsPrimary != 1 {
		t.Fatalf("primary = %d, want 1", up.IsPrimary)
	}
	if up.Tags == nil || *up.Tags != `["tech"]` {
		t.Fatalf("tags = %v", up.Tags)
	}
	if up.SidecarRkey == nil || *up.SidecarRkey != "3sc" {
		t.Fatalf("sidecarRkey = %v, want 3sc", up.SidecarRkey)
	}
}

// Duplicate standard records for one publication collapse to the smallest
// rkey; when the canonical rkey changes, delete-before-upsert keeps
// UNIQUE(did, feed_url) satisfied within a single pass.
func TestStandardReconcile_DuplicatesCollapseAndRekeySurvives(t *testing.T) {
	store := newFakeStore()
	// Local row exists under rkey 3zz (previous canonical).
	lister := &fakeLister{standardSubs: []PDSStandardSubscription{stdSub("3zz", pubA)}}
	runStandardReconcile(t, store, lister, nil)
	if _, ok := store.upsertParams["3zz"]; !ok {
		t.Fatalf("setup: no row for 3zz")
	}

	// A duplicate with a smaller rkey appears → canonical rekeys to 3aa.
	lister = &fakeLister{standardSubs: []PDSStandardSubscription{stdSub("3zz", pubA), stdSub("3aa", pubA)}}
	runStandardReconcile(t, store, lister, nil)

	if len(store.deletes) != 1 || store.deletes[0] != "3zz" {
		t.Fatalf("deletes = %v, want [3zz]", store.deletes)
	}
	if _, ok := store.upsertParams["3aa"]; !ok {
		t.Fatalf("no row for new canonical 3aa: %+v", store.upsertParams)
	}
	rows, _ := store.ListUserSubscriptionsForSync(context.Background(), "did:plc:alice")
	if len(rows) != 1 || rows[0].Rkey != "3aa" {
		t.Fatalf("rows = %+v, want single 3aa", rows)
	}
}

func TestStandardReconcile_RemoteDeleteRemovesLocalOnly(t *testing.T) {
	store := newFakeStore()
	lister := &fakeLister{
		subs:         []PDSSubscription{{URI: "at://x/a/r1", Kind: "rss", Rkey: "r1", FeedURL: "https://feed/a"}},
		standardSubs: []PDSStandardSubscription{stdSub("3s1", pubA)},
	}
	runStandardReconcile(t, store, lister, nil)

	// Publication unsubscribed elsewhere; rss subscription unchanged.
	lister.standardSubs = nil
	runStandardReconcile(t, store, lister, nil)

	rows, _ := store.ListUserSubscriptionsForSync(context.Background(), "did:plc:alice")
	if len(rows) != 1 || rows[0].Kind != "rss" {
		t.Fatalf("rows = %+v, want only the rss row", rows)
	}
}

func TestStandardReconcile_OrphanedSidecarDeletedFromPDS(t *testing.T) {
	store := newFakeStore()
	pds := &fakeRecordWriter{}
	lister := &fakeLister{
		subs: []PDSSubscription{sidecar("3sc", pubA, "Old Title")},
		// No standard record for pubA → the sidecar is orphaned.
		standardSubs: []PDSStandardSubscription{stdSub("3s2", pubB)},
	}

	runStandardReconcile(t, store, lister, pds)

	if len(pds.deleted) != 1 || pds.deleted[0] != "blue.morgen.feed.subscription/3sc" {
		t.Fatalf("PDS deletes = %v, want the orphaned sidecar", pds.deleted)
	}
	// The orphaned sidecar must not have produced a local row.
	rows, _ := store.ListUserSubscriptionsForSync(context.Background(), "did:plc:alice")
	if len(rows) != 1 || rows[0].FeedUrl != pubB {
		t.Fatalf("rows = %+v, want only pubB", rows)
	}
}

func TestStandardReconcile_LiveSidecarNotDeleted(t *testing.T) {
	store := newFakeStore()
	pds := &fakeRecordWriter{}
	lister := &fakeLister{
		subs:         []PDSSubscription{sidecar("3sc", pubA, "Title")},
		standardSubs: []PDSStandardSubscription{stdSub("3s1", pubA)},
	}
	runStandardReconcile(t, store, lister, pds)
	if len(pds.deleted) != 0 {
		t.Fatalf("PDS deletes = %v, want none", pds.deleted)
	}
}

// A failed standard listing aborts before ANY mutation: transient PDS errors
// must not wipe local publication sources (or rss rows).
func TestStandardReconcile_ListErrorAbortsBeforeDeletes(t *testing.T) {
	store := newFakeStore()
	lister := &fakeLister{
		subs:         []PDSSubscription{{URI: "at://x/a/r1", Kind: "rss", Rkey: "r1", FeedURL: "https://feed/a"}},
		standardSubs: []PDSStandardSubscription{stdSub("3s1", pubA)},
	}
	runStandardReconcile(t, store, lister, nil)

	eng := NewEngine(jobs.New(), store, lister, &countingFetcher{}, nil, nil)
	snapshot, _ := store.ListUserSubscriptionsForSync(context.Background(), "did:plc:alice")

	lister.standardErr = errors.New("pds down")
	err := eng.reconcileTier1(context.Background(), mustDID("did:plc:alice"), newSession("did:plc:alice"), snapshot, func(string) {})
	if err == nil {
		t.Fatal("expected error")
	}
	if len(store.deletes) != 0 {
		t.Fatalf("deletes = %v, want none on list error", store.deletes)
	}

	lister.standardErr = nil
	lister.subsErr = errors.New("pds down")
	err = eng.reconcileTier1(context.Background(), mustDID("did:plc:alice"), newSession("did:plc:alice"), snapshot, func(string) {})
	if err == nil {
		t.Fatal("expected error")
	}
	if len(store.deletes) != 0 {
		t.Fatalf("deletes = %v, want none on list error", store.deletes)
	}
}

// The rss pass must not touch standardfeed rows and vice versa.
func TestStandardReconcile_KindsAreIsolated(t *testing.T) {
	store := newFakeStore()
	lister := &fakeLister{
		subs:         []PDSSubscription{{URI: "at://x/a/r1", Kind: "rss", Rkey: "r1", FeedURL: "https://feed/a"}},
		standardSubs: []PDSStandardSubscription{stdSub("3s1", pubA)},
	}
	runStandardReconcile(t, store, lister, nil)

	// Remove the rss record remotely; the standardfeed row must survive.
	lister.subs = nil
	runStandardReconcile(t, store, lister, nil)

	rows, _ := store.ListUserSubscriptionsForSync(context.Background(), "did:plc:alice")
	if len(rows) != 1 || rows[0].Kind != "standardfeed" {
		t.Fatalf("rows = %+v, want only the standardfeed row", rows)
	}
}
