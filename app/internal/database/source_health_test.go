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
		FeedUrl: url, ConsecutiveFailures: 1, LoneFailures: 1, ChannelGoneAt: &now, NextFetchAt: &next, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	params := db.ListUserSourcesWithStatsParams{Did: "did:plc:example", Now: now}
	rows, err := q.ListUserSourcesWithStats(ctx, params)
	if err != nil || len(rows) != 1 {
		t.Fatalf("list: %v, %v", rows, err)
	}
	if rows[0].ConsecutiveFailures != 1 || rows[0].LoneFailures != 1 || rows[0].ChannelGoneAt == nil || rows[0].NextFetchAt == nil || *rows[0].NextFetchAt != next || rows[0].LastFetchedAt != nil {
		t.Fatalf("first failure lost: %+v", rows[0])
	}
	detail, err := q.GetUserSourceWithStats(ctx, db.GetUserSourceWithStatsParams{Did: "did:plc:example", Rkey: "example", Now: now})
	if err != nil {
		t.Fatal(err)
	}
	if detail.ConsecutiveFailures != 1 || detail.LoneFailures != 1 || detail.ChannelGoneAt == nil || detail.NextFetchAt == nil || *detail.NextFetchAt != next || detail.LastFetchedAt != nil {
		t.Fatalf("detail lost the first failure: %+v", detail)
	}
	if err := q.UpdateFeedFetchState(ctx, db.UpdateFeedFetchStateParams{FeedUrl: url, LastFetchedAt: &next, UpdatedAt: next}); err != nil {
		t.Fatal(err)
	}
	rows, err = q.ListUserSourcesWithStats(ctx, params)
	if err != nil || len(rows) != 1 {
		t.Fatalf("recovered list: %v, %v", rows, err)
	}
	if rows[0].ConsecutiveFailures != 0 || rows[0].LoneFailures != 0 || rows[0].ChannelGoneAt != nil || rows[0].NextFetchAt != nil || rows[0].LastFetchedAt == nil || *rows[0].LastFetchedAt != next {
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

func TestCountYouTubeFeedFailuresCountsLiveYouTubeFeedsOnly(t *testing.T) {
	path := filepath.Join(t.TempDir(), "youtube.db")
	runGoose(t, path, "up")
	q := db.New(openMigrationDB(t, path))
	ctx := context.Background()
	now := "2026-11-15T08:00:00Z"
	next := "2026-11-15T08:05:00Z"
	gone := now
	feeds := []struct {
		url    string
		failed bool
		gone   *string
	}{
		{"https://www.youtube.com/feeds/videos.xml?channel_id=UCaaaaaaaaaaaaaaaaaaaaaa", true, nil},
		{"https://www.youtube.com/feeds/videos.xml?playlist_id=UULFbbbbbbbbbbbbbbbbbbbbbb", true, nil},
		{"https://youtube.com/feeds/videos.xml?channel_id=UCcccccccccccccccccccccc", true, nil},
		{"https://www.youtube.com/feeds/videos.xml?channel_id=UCdddddddddddddddddddddd", false, nil},
		{"https://www.youtube.com/feeds/videos.xml?channel_id=UCorphanorphanorphanorph", true, nil},
		{"https://www.youtube.com/feeds/videos.xml?channel_id=UCgonegonegonegonegonego", true, &gone},
		{"https://publication.example.com/feed.xml", true, nil},
		{"https://publication.example.com/www.youtube.com/feeds/videos.xml", true, nil},
	}
	for _, f := range feeds {
		if err := q.UpsertFeed(ctx, db.UpsertFeedParams{FeedUrl: f.url, CreatedAt: now, UpdatedAt: now}); err != nil {
			t.Fatal(err)
		}
		if f.failed {
			if err := q.UpdateFeedFetchFailure(ctx, db.UpdateFeedFetchFailureParams{FeedUrl: f.url, ConsecutiveFailures: 2, ChannelGoneAt: f.gone, NextFetchAt: &next, UpdatedAt: now}); err != nil {
				t.Fatal(err)
			}
		}
	}
	if _, err := openMigrationDB(t, path).ExecContext(ctx, `UPDATE feeds SET orphaned_at = ? WHERE feed_url LIKE '%orphan%'`, now); err != nil {
		t.Fatal(err)
	}
	if _, err := openMigrationDB(t, path).ExecContext(ctx, `INSERT INTO feeds (feed_url, created_at, updated_at, consecutive_failures, last_fetched_at) VALUES
		('https://www.youtube.com/feeds/videos.xml?channel_id=UCdeaddeaddeaddeaddeadde', '2026-11-01T08:00:00Z', ?1, 300, NULL),
		('https://www.youtube.com/feeds/videos.xml?channel_id=UCrecentrecentrecentrece', '2026-11-01T08:00:00Z', ?1, 2, '2026-11-14T08:00:00Z')`, now); err != nil {
		t.Fatal(err)
	}
	got, err := q.CountYouTubeFeedFailures(ctx, "2026-11-13T08:00:00Z")
	if err != nil {
		t.Fatal(err)
	}
	if got.Total != 5 || got.Failing != 4 {
		t.Fatalf("got total %d failing %d, want 5 and 4: counted the orphan, the gone channel, the feed failing for two weeks, a non-YouTube feed, or missed the bare host", got.Total, got.Failing)
	}
}
