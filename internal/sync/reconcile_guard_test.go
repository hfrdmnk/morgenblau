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

// reconcileGuardCase asserts one pass wires createdAtOf; the guard's own semantics belong to
// TestCreatedAfterSnapshot and TestReconcileCollection_GuardSparesRowsNewerThanTheSnapshot.
// The two subscription passes are absent: their snapshot precedes the PDS listing, so an in-flight row is never a delete candidate; they also select no created_at to guard on.
type reconcileGuardCase struct {
	name    string
	seed    func(s *fakeStore, createdAt string)
	run     func(e *Engine) error
	deletes func(s *fakeStore) []string
}

var reconcileGuardCases = []reconcileGuardCase{
	{
		name: "saves",
		seed: func(s *fakeStore, createdAt string) {
			s.saves[guardDID] = map[string]db.ListUserSavesForSyncRow{
				"3inflight": {Did: guardDID, Rkey: "3inflight", AtUri: "at://" + guardDID + "/blue.morgen.feed.save/3inflight", ItemUrl: "https://example.test/post", CreatedAt: createdAt},
			}
		},
		run: func(e *Engine) error {
			return e.reconcileSaves(context.Background(), mustDID(guardDID), newSession(guardDID))
		},
		deletes: func(s *fakeStore) []string { return s.saveDeletes },
	},
	{
		name: "shares",
		seed: func(s *fakeStore, createdAt string) {
			s.shares[guardDID] = map[string]db.ListUserSharesForSyncRow{
				"3inflight": {Did: guardDID, Rkey: "3inflight", AtUri: "at://" + guardDID + "/blue.morgen.feed.share/3inflight", Kind: "rss", ItemUrl: ptr("https://example.test/post"), CreatedAt: createdAt},
			}
		},
		run: func(e *Engine) error {
			return e.reconcileShares(context.Background(), mustDID(guardDID), newSession(guardDID))
		},
		deletes: func(s *fakeStore) []string { return s.shareDeletes },
	},
	{
		name: "follows",
		seed: func(s *fakeStore, createdAt string) {
			s.follows[guardDID] = map[string]db.ListUserFollowsForSyncRow{
				"3inflight": {Did: guardDID, Rkey: "3inflight", AtUri: "at://" + guardDID + "/blue.morgen.graph.follow/3inflight", SubjectDid: "did:plc:bob", CreatedAt: createdAt},
			}
		},
		run: func(e *Engine) error {
			return e.reconcileFollows(context.Background(), mustDID(guardDID), newSession(guardDID))
		},
		deletes: func(s *fakeStore) []string { return s.followDeletes },
	},
}

// A row written in-app after the PDS listing was taken is absent from that listing without having been deleted remotely; reconcile must not mistake it for a remote delete.
func TestReconcile_RowCreatedAfterSnapshot_SurvivesDeletePass(t *testing.T) {
	rows := []struct {
		name      string
		createdAt string
		wantGone  bool
	}{
		{"created after the snapshot", "2026-07-20T12:00:01Z", false},
		{"created before the snapshot", "2026-07-20T11:00:00Z", true},
	}
	for _, rc := range reconcileGuardCases {
		t.Run(rc.name, func(t *testing.T) {
			for _, row := range rows {
				t.Run(row.name, func(t *testing.T) {
					store := newFakeStore()
					rc.seed(store, row.createdAt)
					// Empty lister: the local row is absent from the PDS listing either way, only created_at decides.
					eng := NewEngine(jobs.New(), store, &fakeLister{}, &countingFetcher{}, nil, nil)
					eng.now = func() time.Time { return guardSnapshotAt }

					if err := rc.run(eng); err != nil {
						t.Fatal(err)
					}

					deletes := rc.deletes(store)
					if row.wantGone {
						if len(deletes) != 1 || deletes[0] != "3inflight" {
							t.Errorf("deletes = %v, want [3inflight]", deletes)
						}
						return
					}
					if len(deletes) != 0 {
						t.Errorf("deletes = %v, want none: the row was created after the snapshot", deletes)
					}
				})
			}
		})
	}
}
