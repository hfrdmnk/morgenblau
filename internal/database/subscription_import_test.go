package database

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	"morgenblau/internal/database/db"
)

func TestImportMirrorRejectsSupersededBaseline(t *testing.T) {
	path := filepath.Join(t.TempDir(), "import.db")
	runGoose(t, path, "up")
	t.Setenv("DB_PATH", path)
	dbs, err := Open()
	if err != nil {
		t.Fatal(err)
	}
	defer dbs.Close()
	index, ok := any(dbs).(interface {
		SnapshotImportedSubscription(context.Context, string, string) (*db.UserSubscription, error)
		MirrorImportedSubscription(context.Context, *db.UserSubscription, db.UpsertFeedParams, db.UpsertUserSubscriptionParams) error
	})
	if !ok {
		t.Fatal("missing guarded import mirror")
	}
	ctx := context.Background()
	q := db.New(dbs.Writer)
	for _, scenario := range []string{"unchanged", "updated", "deleted", "inserted"} {
		t.Run(scenario, func(t *testing.T) {
			url := "https://" + scenario + ".example.com/feed"
			feed := db.UpsertFeedParams{FeedUrl: url, CreatedAt: "2026-01-01T00:00:00Z", UpdatedAt: "2026-01-01T00:00:00Z"}
			first, newer := `["First"]`, `["First","Second"]`
			row := db.UpsertUserSubscriptionParams{Did: "did:plc:example", Rkey: scenario, AtUri: "at://did:plc:example/blue.morgen.feed.subscription/" + scenario, FeedUrl: url, Tags: &first, CreatedAt: feed.CreatedAt, UpdatedAt: feed.UpdatedAt}
			if err := q.UpsertFeed(ctx, feed); err != nil {
				t.Fatal(err)
			}
			if scenario != "inserted" {
				if err := q.UpsertUserSubscription(ctx, row); err != nil {
					t.Fatal(err)
				}
			}
			baseline, err := index.SnapshotImportedSubscription(ctx, row.Did, url)
			if err != nil {
				t.Fatal(err)
			}
			if scenario == "deleted" {
				if err := q.DeleteUserSubscription(ctx, db.DeleteUserSubscriptionParams{Did: row.Did, Rkey: row.Rkey}); err != nil {
					t.Fatal(err)
				}
			} else if scenario != "unchanged" {
				current := row
				current.Tags = &newer
				if err := q.UpsertUserSubscription(ctx, current); err != nil {
					t.Fatal(err)
				}
			}
			incomingSite := "https://stale.example.com"
			feed.SiteUrl = &incomingSite
			err = index.MirrorImportedSubscription(ctx, baseline, feed, row)
			if scenario == "unchanged" {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			if err == nil {
				t.Fatal("superseded baseline was accepted")
			}
			got, err := q.GetUserSubscriptionByFeedURL(ctx, db.GetUserSubscriptionByFeedURLParams{Did: row.Did, FeedUrl: url})
			if scenario == "deleted" {
				if !errors.Is(err, sql.ErrNoRows) {
					t.Fatalf("deleted row resurrected: %+v %v", got, err)
				}
			} else if err != nil || got.Tags == nil || *got.Tags != newer {
				t.Fatalf("newer row lost: %+v %v", got, err)
			}
			gotFeed, err := q.GetFeed(ctx, url)
			if err != nil || gotFeed.SiteUrl != nil {
				t.Fatalf("feed updated despite failed guard: %+v %v", gotFeed, err)
			}
		})
	}
}
