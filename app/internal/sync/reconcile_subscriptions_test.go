package sync

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/bluesky-social/indigo/atproto/syntax"
	"morgenblau/internal/session"

	"morgenblau/internal/atprepo"
	"morgenblau/internal/database/db"
	"morgenblau/internal/jobs"
)

type fakeRecordWriter struct {
	mu       sync.Mutex
	attempts []string
	deleted  []string
	failures map[string]int
}

func (f *fakeRecordWriter) CreateRecord(context.Context, *session.Session, syntax.NSID, map[string]any) (*atprepo.RecordRef, error) {
	return nil, errors.New("reconcile must never create records")
}

func (f *fakeRecordWriter) PutRecord(context.Context, *session.Session, syntax.NSID, string, map[string]any) (*atprepo.RecordRef, error) {
	return nil, errors.New("reconcile must never put records")
}

func (f *fakeRecordWriter) DeleteRecord(_ context.Context, _ *session.Session, collection syntax.NSID, rkey string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.attempts = append(f.attempts, collection.String()+"/"+rkey)
	if f.failures[rkey] > 0 {
		f.failures[rkey]--
		return errors.New("temporary PDS failure")
	}
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
	snapshotAt := eng.now().UTC()
	snapshot, err := store.ListUserSubscriptionsForSync(context.Background(), "did:plc:alice")
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = eng.reconcileTier1(context.Background(), mustDID("did:plc:alice"), newSession("did:plc:alice"), snapshot, snapshotAt, func(url string) {
		mu.Lock()
		added = append(added, url)
		mu.Unlock()
	})
	if err != nil {
		t.Fatal(err)
	}
	return added
}

func TestStandardReconcile_AdoptsForeignSubscription(t *testing.T) {
	store := newFakeStore()
	lister := &fakeLister{standardSubs: []PDSStandardSubscription{stdSub("3s1", pubA)}}

	added := runStandardReconcile(t, store, lister, nil)

	if len(added) != 1 || added[0] != pubA {
		t.Fatalf("onAdded = %v, want [%s] so the newly adopted publication gets its Phase-2 fetch", added, pubA)
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

func TestStandardReconcile_DuplicatesCollapseAndRekeySurvives(t *testing.T) {
	store := newFakeStore()
	lister := &fakeLister{standardSubs: []PDSStandardSubscription{stdSub("3zz", pubA)}}
	runStandardReconcile(t, store, lister, nil)
	if _, ok := store.upsertParams["3zz"]; !ok {
		t.Fatalf("setup: no row for 3zz")
	}

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
	store.assertDeleteBeforeUpsert(t, "3zz", "3aa")
}

func TestRSSReconcile_RekeyedSubscriptionDeletesBeforeUpsert(t *testing.T) {
	const feedURL = "https://feed.example.com/a"
	store := newFakeStore()
	store.rows["did:plc:alice"] = map[string]db.UserSubscription{
		"3old": {Did: "did:plc:alice", Rkey: "3old", AtUri: "at://did:plc:alice/blue.morgen.feed.subscription/3old", FeedUrl: feedURL, Kind: "rss"},
	}
	lister := &fakeLister{subs: []PDSSubscription{
		{URI: "at://did:plc:alice/blue.morgen.feed.subscription/3new", Rkey: "3new", Kind: "rss", FeedURL: feedURL},
	}}

	runStandardReconcile(t, store, lister, nil)

	if len(store.deletes) != 1 || store.deletes[0] != "3old" {
		t.Fatalf("deletes = %v, want [3old]", store.deletes)
	}
	if _, ok := store.upsertParams["3new"]; !ok {
		t.Fatalf("rekeyed subscription not upserted: %+v", store.upsertParams)
	}
	rows, _ := store.ListUserSubscriptionsForSync(context.Background(), "did:plc:alice")
	if len(rows) != 1 || rows[0].Rkey != "3new" {
		t.Fatalf("rows = %+v, want a single row under 3new", rows)
	}
	store.assertDeleteBeforeUpsert(t, "3old", "3new")
}

func TestRSSReconcile_DuplicateFeedRecordsCollapseToCanonical(t *testing.T) {
	const feedURL = "https://feed.example.com/a"
	store := newFakeStore()
	lister := &fakeLister{subs: []PDSSubscription{
		{URI: "at://did:plc:alice/blue.morgen.feed.subscription/3zzz", Rkey: "3zzz", Kind: "rss", FeedURL: feedURL, Title: "Newer duplicate"},
		{URI: "at://did:plc:alice/blue.morgen.feed.subscription/3aaa", Rkey: "3aaa", Kind: "rss", FeedURL: feedURL, Title: "Canonical"},
	}}

	runStandardReconcile(t, store, lister, nil)

	rows, _ := store.ListUserSubscriptionsForSync(context.Background(), "did:plc:alice")
	if len(rows) != 1 || rows[0].Rkey != "3aaa" {
		t.Fatalf("rows = %+v, want one canonical row under 3aaa", rows)
	}
	if rows[0].Title == nil || *rows[0].Title != "Canonical" {
		t.Errorf("title = %v, want canonical record metadata", rows[0].Title)
	}
	if store.upserts != 1 {
		t.Errorf("upserts = %d, want 1", store.upserts)
	}
}

func TestStandardReconcile_RemoteDeleteRemovesLocalOnly(t *testing.T) {
	store := newFakeStore()
	lister := &fakeLister{
		subs:         []PDSSubscription{{URI: "at://x/a/r1", Kind: "rss", Rkey: "r1", FeedURL: "https://feed/a"}},
		standardSubs: []PDSStandardSubscription{stdSub("3s1", pubA)},
	}
	runStandardReconcile(t, store, lister, nil)

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
		subs:         []PDSSubscription{sidecar("3sc", pubA, "Old Title")},
		standardSubs: []PDSStandardSubscription{stdSub("3s2", pubB)},
	}

	runStandardReconcile(t, store, lister, pds)

	if len(pds.deleted) != 1 || pds.deleted[0] != "blue.morgen.feed.subscription/3sc" {
		t.Fatalf("PDS deletes = %v, want the orphaned sidecar", pds.deleted)
	}
	rows, _ := store.ListUserSubscriptionsForSync(context.Background(), "did:plc:alice")
	if len(rows) != 1 || rows[0].FeedUrl != pubB {
		t.Fatalf("rows = %+v, want only pubB", rows)
	}
}

func TestStandardReconcile_SidecarCleanupFailureIsRetried(t *testing.T) {
	store := newFakeStore()
	pds := &fakeRecordWriter{failures: map[string]int{"3sc": 1}}
	lister := &fakeLister{subs: []PDSSubscription{sidecar("3sc", pubA, "Orphan")}}

	runStandardReconcile(t, store, lister, pds)
	if len(pds.deleted) != 0 || len(pds.attempts) != 1 {
		t.Fatalf("after first pass, attempts=%v deleted=%v", pds.attempts, pds.deleted)
	}

	runStandardReconcile(t, store, lister, pds)
	if len(pds.attempts) != 2 || len(pds.deleted) != 1 {
		t.Fatalf("after retry, attempts=%v deleted=%v", pds.attempts, pds.deleted)
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

func TestStandardReconcile_DuplicateSidecarNewestWinsOlderDeleted(t *testing.T) {
	store := newFakeStore()
	pds := &fakeRecordWriter{}
	lister := &fakeLister{
		subs:         []PDSSubscription{sidecar("3aaa", pubA, "Old Title"), sidecar("3zzz", pubA, "New Title")},
		standardSubs: []PDSStandardSubscription{stdSub("3s1", pubA)},
	}
	runStandardReconcile(t, store, lister, pds)

	if len(pds.deleted) != 1 || pds.deleted[0] != "blue.morgen.feed.subscription/3aaa" {
		t.Fatalf("PDS deletes = %v, want the older duplicate sidecar", pds.deleted)
	}
	rows, _ := store.ListUserSubscriptionsForSync(context.Background(), "did:plc:alice")
	if len(rows) != 1 {
		t.Fatalf("rows = %+v, want one", rows)
	}
	if rows[0].Title == nil || *rows[0].Title != "New Title" {
		t.Errorf("title = %v, want the newest sidecar's, so the latest edit survives a sync/PATCH race duplicate", rows[0].Title)
	}
	if rows[0].SidecarRkey == nil || *rows[0].SidecarRkey != "3zzz" {
		t.Errorf("sidecarRkey = %v, want newest 3zzz", rows[0].SidecarRkey)
	}
}

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
	_, _, err := eng.reconcileTier1(context.Background(), mustDID("did:plc:alice"), newSession("did:plc:alice"), snapshot, eng.now().UTC(), func(string) {})
	if err == nil {
		t.Fatal("expected error")
	}
	if len(store.deletes) != 0 {
		t.Fatalf("deletes = %v, want none on list error: both listings come before any write, so a transient PDS error cannot wipe local sources", store.deletes)
	}

	lister.standardErr = nil
	lister.subsErr = errors.New("pds down")
	_, _, err = eng.reconcileTier1(context.Background(), mustDID("did:plc:alice"), newSession("did:plc:alice"), snapshot, eng.now().UTC(), func(string) {})
	if err == nil {
		t.Fatal("expected error")
	}
	if len(store.deletes) != 0 {
		t.Fatalf("deletes = %v, want none on list error: both listings come before any write, so a transient PDS error cannot wipe local sources", store.deletes)
	}
}

func TestStandardReconcile_KindsAreIsolated(t *testing.T) {
	store := newFakeStore()
	lister := &fakeLister{
		subs:         []PDSSubscription{{URI: "at://x/a/r1", Kind: "rss", Rkey: "r1", FeedURL: "https://feed/a"}},
		standardSubs: []PDSStandardSubscription{stdSub("3s1", pubA)},
	}
	runStandardReconcile(t, store, lister, nil)

	lister.subs = nil
	runStandardReconcile(t, store, lister, nil)

	rows, _ := store.ListUserSubscriptionsForSync(context.Background(), "did:plc:alice")
	if len(rows) != 1 || rows[0].Kind != "standardfeed" {
		t.Fatalf("rows = %+v, want only the standardfeed row: the rss pass must never delete rows the standardfeed pass owns", rows)
	}
}
