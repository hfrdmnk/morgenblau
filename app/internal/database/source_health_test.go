package database

import (
	"context"
	"path/filepath"
	"testing"

	"morgenblau/internal/database/db"
)

func TestSourceListReadsPersistentFetchHealthAndRecovery(t *testing.T) {
	path := filepath.Join(t.TempDir(), "health.db")
	runGoose(t, path, "up")
	q := db.New(openMigrationDB(t, path))
	ctx := context.Background()
	url := "https://publication.example.com/feed.xml"
	now := "2026-10-08T08:00:00Z"
	if err := q.UpsertFeed(ctx, db.UpsertFeedParams{FeedUrl: url, CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := q.UpsertUserSubscription(ctx, db.UpsertUserSubscriptionParams{
		Did: "did:plc:example", Rkey: "example", FeedUrl: url,
		AtUri: "at://did:plc:example/blue.morgen.feed.subscription/example", CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	next := "2026-10-08T08:05:00Z"
	if err := q.UpdateFeedFetchFailure(ctx, db.UpdateFeedFetchFailureParams{
		FeedUrl: url, ConsecutiveFailures: 1, NextFetchAt: &next, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	params := db.ListUserSourcesWithStatsParams{Did: "did:plc:example", Now: now}
	rows, err := q.ListUserSourcesWithStats(ctx, params)
	if err != nil || len(rows) != 1 {
		t.Fatalf("list: %v, %v", rows, err)
	}
	if rows[0].ConsecutiveFailures != 1 || rows[0].NextFetchAt == nil || *rows[0].NextFetchAt != next || rows[0].LastFetchedAt != nil {
		t.Fatalf("first failure lost: %+v", rows[0])
	}
	detail, err := q.GetUserSourceWithStats(ctx, db.GetUserSourceWithStatsParams{Did: "did:plc:example", Rkey: "example", Now: now})
	if err != nil {
		t.Fatal(err)
	}
	if detail.ConsecutiveFailures != 1 || detail.NextFetchAt == nil || *detail.NextFetchAt != next || detail.LastFetchedAt != nil {
		t.Fatalf("detail lost the first failure: %+v", detail)
	}
	if err := q.UpdateFeedFetchState(ctx, db.UpdateFeedFetchStateParams{FeedUrl: url, LastFetchedAt: &next, UpdatedAt: next}); err != nil {
		t.Fatal(err)
	}
	rows, err = q.ListUserSourcesWithStats(ctx, params)
	if err != nil || len(rows) != 1 {
		t.Fatalf("recovered list: %v, %v", rows, err)
	}
	if rows[0].ConsecutiveFailures != 0 || rows[0].NextFetchAt != nil || rows[0].LastFetchedAt == nil || *rows[0].LastFetchedAt != next {
		t.Fatalf("recovery lost: %+v", rows[0])
	}
	detail, err = q.GetUserSourceWithStats(ctx, db.GetUserSourceWithStatsParams{Did: "did:plc:example", Rkey: "example", Now: now})
	if err != nil {
		t.Fatal(err)
	}
	if detail.ConsecutiveFailures != 0 || detail.NextFetchAt != nil || detail.LastFetchedAt == nil || *detail.LastFetchedAt != next {
		t.Fatalf("detail lost the recovery: %+v", detail)
	}
	params.Did = "did:plc:other"
	rows, err = q.ListUserSourcesWithStats(ctx, params)
	if err != nil || len(rows) != 0 {
		t.Fatalf("other reader can see source health: %v, %v", rows, err)
	}
}
