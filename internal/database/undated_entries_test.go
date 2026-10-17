package database

import (
	"context"
	"path/filepath"
	"testing"

	"morgenblau/internal/database/db"
)

// Generated web feeds store setup history with an empty published_at; it must reach no digest day and not hide the feed's real dates.
func TestUndatedEntriesStayOutOfTheDigestAndSourceStats(t *testing.T) {
	path := filepath.Join(t.TempDir(), "undated.db")
	runGoose(t, path, "up")
	q := db.New(openMigrationDB(t, path))
	ctx := context.Background()
	did, url, now := "did:plc:example", "https://app.example.com/feed/example", "2026-10-10T12:00:00Z"
	if err := q.UpsertFeed(ctx, db.UpsertFeedParams{FeedUrl: url, CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := q.UpsertUserSubscription(ctx, db.UpsertUserSubscriptionParams{
		Did: did, Rkey: "example", FeedUrl: url,
		AtUri: "at://did:plc:example/blue.morgen.feed.subscription/example", CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	for _, e := range []struct {
		guid, published string
		keepUndated     int64
	}{
		{"https://news.example.com/history", "", 1},
		{"https://news.example.com/dated", "2026-10-01T08:00:00Z", 1},
		{"https://news.example.com/later", "2026-10-10T11:00:00Z", 1},
	} {
		if err := q.UpsertFeedEntry(ctx, db.UpsertFeedEntryParams{
			FeedUrl: url, Guid: e.guid, EntrySlug: e.guid, Url: e.guid, ContentType: "article",
			PublishedAt: e.published, KeepUndated: e.keepUndated, FetchedAt: now,
		}); err != nil {
			t.Fatal(err)
		}
	}

	digest, err := q.ListDigestForUser(ctx, db.ListDigestForUserParams{Did: did, PublishedAt: "0000-01-01T00:00:00Z", PublishedAt_2: "9999-01-01T00:00:00Z"})
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range digest {
		if row.PublishedAt == "" {
			t.Errorf("digest includes undated entry %s", row.Url)
		}
	}
	if len(digest) != 2 {
		t.Errorf("digest has %d entries, want the 2 dated ones", len(digest))
	}

	rows, err := q.ListUserSourcesWithStats(ctx, db.ListUserSourcesWithStatsParams{Did: did, Now: now})
	if err != nil || len(rows) != 1 {
		t.Fatalf("list: %v, %v", rows, err)
	}
	if rows[0].FirstPublishedAt != "2026-10-01T08:00:00Z" {
		t.Errorf("first published = %v, want the oldest real date", rows[0].FirstPublishedAt)
	}
	one, err := q.GetUserSourceWithStats(ctx, db.GetUserSourceWithStatsParams{Did: did, Rkey: "example", Now: now})
	if err != nil {
		t.Fatal(err)
	}
	if one.FirstPublishedAt != "2026-10-01T08:00:00Z" || one.TotalEntries != 3 {
		t.Errorf("source detail first published = %v, total = %d", one.FirstPublishedAt, one.TotalEntries)
	}
}
