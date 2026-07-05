package sync

import (
	"context"
	"errors"
	"testing"

	"morgenblau/internal/database/db"
	"morgenblau/internal/jobs"
)

const (
	docA = "at://did:plc:puba/site.standard.document/3da"
	docB = "at://did:plc:pubb/site.standard.document/3db"
)

func recommend(rkey, doc string) PDSRecommend {
	return PDSRecommend{
		URI:       "at://did:plc:alice/site.standard.graph.recommend/" + rkey,
		Rkey:      rkey,
		Document:  doc,
		CreatedAt: "2026-06-01T10:00:00Z",
	}
}

func shareSidecar(rkey, doc, comment string) PDSShare {
	return PDSShare{
		URI:       "at://did:plc:alice/blue.morgen.feed.share/" + rkey,
		Rkey:      rkey,
		ItemURL:   "https://blog.example.test/post",
		Document:  doc,
		FeedURL:   pubA,
		Comment:   comment,
		CreatedAt: "2026-06-01T10:00:00Z",
	}
}

func rssShare(rkey, itemURL string) PDSShare {
	return PDSShare{
		URI:       "at://did:plc:alice/blue.morgen.feed.share/" + rkey,
		Rkey:      rkey,
		ItemURL:   itemURL,
		CreatedAt: "2026-06-01T10:00:00Z",
	}
}

func TestReconcileShares_ForeignRecommendAdopted(t *testing.T) {
	store := newFakeStore()
	lister := &fakeLister{recommends: []PDSRecommend{recommend("3rec", docA)}}
	eng := NewEngine(jobs.New(), store, lister, &countingFetcher{}, nil, nil)

	if err := eng.reconcileShares(context.Background(), mustDID("did:plc:alice"), newSession("did:plc:alice")); err != nil {
		t.Fatal(err)
	}
	row, ok := store.shareUpsertParams["3rec"]
	if !ok {
		t.Fatalf("no upsert for adopted recommend: %+v", store.shareUpsertParams)
	}
	if row.Kind != "standardfeed" || row.Document == nil || *row.Document != docA {
		t.Errorf("row = %+v", row)
	}
	if row.SidecarRkey != nil || row.Comment != nil {
		t.Errorf("bare recommend must not carry sidecar fields: %+v", row)
	}
	if row.CreatedAt != "2026-06-01T10:00:00Z" {
		t.Errorf("createdAt = %q, want record createdAt", row.CreatedAt)
	}
}

func TestReconcileShares_SidecarMergesOntoRecommend(t *testing.T) {
	store := newFakeStore()
	lister := &fakeLister{
		recommends: []PDSRecommend{recommend("3rec", docA)},
		shares:     []PDSShare{shareSidecar("3sc", docA, "great read")},
	}
	eng := NewEngine(jobs.New(), store, lister, &countingFetcher{}, nil, nil)

	if err := eng.reconcileShares(context.Background(), mustDID("did:plc:alice"), newSession("did:plc:alice")); err != nil {
		t.Fatal(err)
	}
	row := store.shareUpsertParams["3rec"]
	if row.Comment == nil || *row.Comment != "great read" {
		t.Errorf("comment = %v", row.Comment)
	}
	if row.SidecarRkey == nil || *row.SidecarRkey != "3sc" {
		t.Errorf("sidecarRkey = %v", row.SidecarRkey)
	}
	if row.ItemUrl == nil || *row.ItemUrl != "https://blog.example.test/post" {
		t.Errorf("itemUrl = %v", row.ItemUrl)
	}
	// The sidecar itself never becomes its own row.
	if _, ok := store.shareUpsertParams["3sc"]; ok {
		t.Errorf("sidecar upserted as its own share row")
	}
}

func TestReconcileShares_DuplicateSidecarNewestWinsOlderDeleted(t *testing.T) {
	// Two sidecars for one document (a sync/PATCH race): the newest edit must
	// win and the older duplicate must be deleted from the PDS.
	store := newFakeStore()
	writer := &fakeRecordWriter{}
	lister := &fakeLister{
		recommends: []PDSRecommend{recommend("3rec", docA)},
		shares:     []PDSShare{shareSidecar("3aaa", docA, "old"), shareSidecar("3zzz", docA, "new")},
	}
	eng := NewEngine(jobs.New(), store, lister, &countingFetcher{}, nil, writer)

	if err := eng.reconcileShares(context.Background(), mustDID("did:plc:alice"), newSession("did:plc:alice")); err != nil {
		t.Fatal(err)
	}
	row := store.shareUpsertParams["3rec"]
	if row.Comment == nil || *row.Comment != "new" {
		t.Errorf("comment = %v, want the newest sidecar's", row.Comment)
	}
	if row.SidecarRkey == nil || *row.SidecarRkey != "3zzz" {
		t.Errorf("sidecarRkey = %v, want newest 3zzz", row.SidecarRkey)
	}
	if len(writer.deleted) != 1 || writer.deleted[0] != "blue.morgen.feed.share/3aaa" {
		t.Errorf("PDS deletes = %v, want the older duplicate 3aaa", writer.deleted)
	}
}

func TestReconcileShares_OrphanSidecarDeletedFromPDS(t *testing.T) {
	store := newFakeStore()
	writer := &fakeRecordWriter{}
	lister := &fakeLister{shares: []PDSShare{shareSidecar("3sc", docA, "stale")}}
	eng := NewEngine(jobs.New(), store, lister, &countingFetcher{}, nil, writer)

	if err := eng.reconcileShares(context.Background(), mustDID("did:plc:alice"), newSession("did:plc:alice")); err != nil {
		t.Fatal(err)
	}
	if len(writer.deleted) != 1 || writer.deleted[0] != "blue.morgen.feed.share/3sc" {
		t.Errorf("PDS deletes = %v", writer.deleted)
	}
	if len(store.shareUpsertParams) != 0 {
		t.Errorf("orphan sidecar produced rows: %+v", store.shareUpsertParams)
	}
}

func TestReconcileShares_RemoteDeleteRemovesLocalRow(t *testing.T) {
	store := newFakeStore()
	store.shares["did:plc:alice"] = map[string]db.ListUserSharesForSyncRow{
		"3gone": {Did: "did:plc:alice", Rkey: "3gone", Kind: "standardfeed", Document: ptr(docA)},
	}
	lister := &fakeLister{}
	eng := NewEngine(jobs.New(), store, lister, &countingFetcher{}, nil, nil)

	if err := eng.reconcileShares(context.Background(), mustDID("did:plc:alice"), newSession("did:plc:alice")); err != nil {
		t.Fatal(err)
	}
	if len(store.shareDeletes) != 1 || store.shareDeletes[0] != "3gone" {
		t.Errorf("deletes = %v", store.shareDeletes)
	}
}

func TestReconcileShares_DuplicateRecommendsCollapseToMinRkey(t *testing.T) {
	store := newFakeStore()
	lister := &fakeLister{recommends: []PDSRecommend{
		recommend("3zzz", docA),
		recommend("3aaa", docA),
	}}
	eng := NewEngine(jobs.New(), store, lister, &countingFetcher{}, nil, nil)

	if err := eng.reconcileShares(context.Background(), mustDID("did:plc:alice"), newSession("did:plc:alice")); err != nil {
		t.Fatal(err)
	}
	if len(store.shareUpsertParams) != 1 {
		t.Fatalf("upserts = %+v, want just the canonical", store.shareUpsertParams)
	}
	if _, ok := store.shareUpsertParams["3aaa"]; !ok {
		t.Errorf("canonical should be min rkey 3aaa: %+v", store.shareUpsertParams)
	}
}

func TestReconcileShares_RSSSharesPassThrough(t *testing.T) {
	store := newFakeStore()
	lister := &fakeLister{shares: []PDSShare{rssShare("3rss", "https://example.test/post")}}
	eng := NewEngine(jobs.New(), store, lister, &countingFetcher{}, nil, nil)

	if err := eng.reconcileShares(context.Background(), mustDID("did:plc:alice"), newSession("did:plc:alice")); err != nil {
		t.Fatal(err)
	}
	row, ok := store.shareUpsertParams["3rss"]
	if !ok {
		t.Fatalf("rss share not upserted: %+v", store.shareUpsertParams)
	}
	if row.Kind != "rss" || row.Document != nil {
		t.Errorf("row = %+v", row)
	}
	if row.ItemUrl == nil || *row.ItemUrl != "https://example.test/post" {
		t.Errorf("itemUrl = %v", row.ItemUrl)
	}
}

func TestReconcileShares_ListFailureAbortsBeforeAnyMutation(t *testing.T) {
	cases := map[string]*fakeLister{
		"shares list fails":     {sharesErr: errors.New("pds down"), recommends: []PDSRecommend{recommend("3rec", docA)}},
		"recommends list fails": {recommendsErr: errors.New("pds down"), shares: []PDSShare{rssShare("3rss", "https://x/post")}},
	}
	for name, lister := range cases {
		t.Run(name, func(t *testing.T) {
			store := newFakeStore()
			store.shares["did:plc:alice"] = map[string]db.ListUserSharesForSyncRow{
				"3local": {Did: "did:plc:alice", Rkey: "3local", Kind: "rss", ItemUrl: ptr("https://x/old")},
			}
			eng := NewEngine(jobs.New(), store, lister, &countingFetcher{}, nil, nil)

			if err := eng.reconcileShares(context.Background(), mustDID("did:plc:alice"), newSession("did:plc:alice")); err == nil {
				t.Fatal("expected error")
			}
			if len(store.shareDeletes) != 0 || len(store.shareUpsertParams) != 0 {
				t.Errorf("mutations despite list failure: deletes=%v upserts=%+v", store.shareDeletes, store.shareUpsertParams)
			}
		})
	}
}

func TestReconcileShares_RekeyedCanonicalDeletesBeforeUpsert(t *testing.T) {
	// The canonical recommend for docA changed rkey (delete + recreate in
	// another app). The stale local row holds (did, document) — the partial
	// unique index forces its delete BEFORE the new upsert.
	store := newFakeStore()
	store.shares["did:plc:alice"] = map[string]db.ListUserSharesForSyncRow{
		"3old": {Did: "did:plc:alice", Rkey: "3old", Kind: "standardfeed", Document: ptr(docA)},
	}
	lister := &fakeLister{recommends: []PDSRecommend{recommend("3new", docA)}}
	eng := NewEngine(jobs.New(), store, lister, &countingFetcher{}, nil, nil)

	if err := eng.reconcileShares(context.Background(), mustDID("did:plc:alice"), newSession("did:plc:alice")); err != nil {
		t.Fatal(err)
	}
	if len(store.shareDeletes) != 1 || store.shareDeletes[0] != "3old" {
		t.Errorf("deletes = %v, want [3old]", store.shareDeletes)
	}
	if _, ok := store.shareUpsertParams["3new"]; !ok {
		t.Errorf("rekeyed canonical not upserted: %+v", store.shareUpsertParams)
	}
	store.assertDeleteBeforeUpsert(t, "3old", "3new")
}

func TestReconcileShares_BareRecommendBackfillsItemURL(t *testing.T) {
	// A recommend with no comment sidecar: reconcile fills item_url from the
	// cached entry so the display fallback survives the entry's later deletion.
	store := newFakeStore()
	store.entryURLs = map[string]string{docA: "https://pub.example/post"}
	lister := &fakeLister{recommends: []PDSRecommend{recommend("3rec", docA)}}
	eng := NewEngine(jobs.New(), store, lister, &countingFetcher{}, nil, nil)

	if err := eng.reconcileShares(context.Background(), mustDID("did:plc:alice"), newSession("did:plc:alice")); err != nil {
		t.Fatal(err)
	}
	got, ok := store.shareUpsertParams["3rec"]
	if !ok {
		t.Fatalf("recommend not upserted: %+v", store.shareUpsertParams)
	}
	if got.ItemUrl == nil || *got.ItemUrl != "https://pub.example/post" {
		t.Errorf("item_url = %v, want backfill from cached entry", got.ItemUrl)
	}
}

func ptr[T any](v T) *T { return &v }

// assertDeleteBeforeUpsert fails unless the delete of delRkey precedes the
// upsert of upRkey in the ordered op-log — the ordering real SQLite's unique
// constraints demand within one reconcile pass.
func (s *fakeStore) assertDeleteBeforeUpsert(t *testing.T, delRkey, upRkey string) {
	t.Helper()
	di, ui := -1, -1
	for i, op := range s.ops {
		if di == -1 && op == "delete:"+delRkey {
			di = i
		}
		if ui == -1 && op == "upsert:"+upRkey {
			ui = i
		}
	}
	if di == -1 {
		t.Fatalf("no delete:%s in ops %v", delRkey, s.ops)
	}
	if ui == -1 {
		t.Fatalf("no upsert:%s in ops %v", upRkey, s.ops)
	}
	if di > ui {
		t.Errorf("delete:%s (idx %d) ran after upsert:%s (idx %d); ops=%v", delRkey, di, upRkey, ui, s.ops)
	}
}
