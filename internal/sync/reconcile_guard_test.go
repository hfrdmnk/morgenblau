package sync

import (
	"context"
	"testing"
	"time"

	"morgenblau/internal/database/db"
	"morgenblau/internal/jobs"
)

const guardDID = "did:plc:alice"

var guardSnapshotAt = time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)

func TestCreatedAfterSnapshot(t *testing.T) {
	cases := []struct {
		name      string
		createdAt string
		want      bool
	}{
		{"one second after the snapshot", "2026-07-20T12:00:01Z", true},
		{"one second before the snapshot", "2026-07-20T11:59:59Z", false},
		{"exactly at the snapshot", "2026-07-20T12:00:00Z", false},
		{"fractional seconds after the snapshot", "2026-07-20T12:00:00.250Z", true},
		{"numeric offset resolving after the snapshot", "2026-07-20T14:00:05+02:00", true},
		{"numeric offset resolving before the snapshot", "2026-07-20T13:00:00+02:00", false},
		{"empty", "", false},
		{"garbage", "not-a-timestamp", false},
		{"date only", "2026-07-21", false},
		{"sqlite space separator", "2026-07-20 12:00:01", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := createdAfterSnapshot(tc.createdAt, guardSnapshotAt); got != tc.want {
				t.Errorf("createdAfterSnapshot(%q, %s) = %v, want %v", tc.createdAt, guardSnapshotAt.Format(time.RFC3339), got, tc.want)
			}
		})
	}
}

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

			if err := eng.reconcileTier1(context.Background(), mustDID(guardDID), newSession(guardDID), baseline, guardSnapshotAt, func(string) {}); err != nil {
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

	if err := eng.reconcileSaves(context.Background(), mustDID(guardDID), newSession(guardDID)); err != nil {
		t.Fatal(err)
	}
	if _, ok := store.saves[guardDID]["3save"]; !ok {
		t.Fatal("same-second local mirror insert was deleted by the stale listing")
	}
	if len(store.saveDeletes) != 0 {
		t.Errorf("save deletes = %v, want none", store.saveDeletes)
	}
}

func strPtr(s string) *string { return &s }
