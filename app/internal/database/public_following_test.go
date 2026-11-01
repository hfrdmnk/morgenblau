package database

import (
	"context"
	"path/filepath"
	"testing"

	"morgenblau/internal/database/db"
)

func TestPublicFollowingOptInIsPerReaderAndListsOnlyTheirSources(t *testing.T) {
	path := filepath.Join(t.TempDir(), "public-following.db")
	runGoose(t, path, "up")
	q := db.New(openMigrationDB(t, path))
	ctx := context.Background()
	now := "2026-10-31T08:00:00Z"
	reader, other := "did:plc:reader", "did:plc:other"

	feed := "https://blog.example.com/feed.xml"
	publication := "at://did:plc:publisher/site.standard.publication/3pub"
	site := "https://publication.example.com/"
	catalog := "Example Publication"
	for _, f := range []db.UpsertFeedParams{
		{FeedUrl: feed, CreatedAt: now, UpdatedAt: now},
		{FeedUrl: publication, Kind: "standardfeed", SiteUrl: &site, Title: &catalog, CreatedAt: now, UpdatedAt: now},
	} {
		if err := q.UpsertFeed(ctx, f); err != nil {
			t.Fatal(err)
		}
	}
	own := "Example Blog"
	for _, s := range []db.UpsertUserSubscriptionParams{
		{Did: reader, Rkey: "a", AtUri: "at://" + reader + "/blue.morgen.feed.subscription/a", FeedUrl: feed, Title: &own, CreatedAt: now, UpdatedAt: now},
		{Did: reader, Rkey: "b", AtUri: "at://" + reader + "/site.standard.graph.subscription/b", FeedUrl: publication, Kind: "standardfeed", IsPrimary: 1, CreatedAt: now, UpdatedAt: now},
		{Did: other, Rkey: "c", AtUri: "at://" + other + "/blue.morgen.feed.subscription/c", FeedUrl: feed, CreatedAt: now, UpdatedAt: now},
	} {
		if err := q.UpsertUserSubscription(ctx, s); err != nil {
			t.Fatal(err)
		}
	}

	enabled := func(did string) bool {
		t.Helper()
		on, err := q.PublicFollowingEnabled(ctx, did)
		if err != nil {
			t.Fatal(err)
		}
		return on
	}
	if enabled(reader) {
		t.Fatal("public following must be off by default")
	}
	for range 2 {
		if err := q.SetPublicFollowing(ctx, db.SetPublicFollowingParams{Did: reader, PublicFollowing: 1, UpdatedAt: now}); err != nil {
			t.Fatalf("setting must be an upsert: %v", err)
		}
	}
	if !enabled(reader) || enabled(other) {
		t.Fatalf("opt-in leaked across readers: reader=%v other=%v", enabled(reader), enabled(other))
	}

	rows, err := q.ListPublicFollowingSources(ctx, reader)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("rows = %+v, want the reader's two sources only", rows)
	}
	byURL := map[string]db.ListPublicFollowingSourcesRow{}
	for _, row := range rows {
		byURL[row.FeedUrl] = row
	}
	if got := byURL[feed]; got.Kind != "rss" || got.Title == nil || *got.Title != own {
		t.Errorf("rss row = %+v", got)
	}
	if got := byURL[publication]; got.Kind != "standardfeed" || got.IsPrimary != 1 || got.SiteUrl == nil || *got.SiteUrl != site || got.CatalogTitle == nil || *got.CatalogTitle != catalog {
		t.Errorf("standardfeed row = %+v", got)
	}

	if err := q.SetPublicFollowing(ctx, db.SetPublicFollowingParams{Did: reader, PublicFollowing: 0, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if enabled(reader) {
		t.Fatal("disable left the list public")
	}
	if _, err := openMigrationDB(t, path).Exec(`INSERT INTO user_settings (did, public_following, updated_at) VALUES ('did:plc:x', 2, ?)`, now); err == nil {
		t.Error("public_following accepted a value other than 0 or 1")
	}
}
