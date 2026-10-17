package database

import (
	"context"
	"path/filepath"
	"testing"

	"morgenblau/internal/database/db"
)

func TestSourceListCarriesGeneratedFeedHealthOnlyForOwnWebFeeds(t *testing.T) {
	path := filepath.Join(t.TempDir(), "web-health.db")
	runGoose(t, path, "up")
	q := db.New(openMigrationDB(t, path))
	ctx := context.Background()
	did, now := "did:plc:example", "2026-10-10T12:00:00Z"
	const prefix, legacy = "https://app.example.com/feed/", "https://app.example.com/web/"
	if err := q.CreateWebSource(ctx, db.CreateWebSourceParams{
		ID: "generated", ListingKey: "news.example.com/", ListingUrl: "https://news.example.com/", SiteUrl: "https://news.example.com/",
		Title: "Example Publication", Recipe: "{}", Health: "needs_attention", HealthReason: "Morgenblau isn't sure which links are posts",
		SetupCost: "{}", CreatedByDid: "did:plc:creator", CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	feeds := map[string]string{
		"generated": prefix + "generated",
		"legacy":    legacy + "generated",
		"plain":     "https://news.example.com/feed.xml",
		// Same id under another origin is somebody else's feed, not one of ours.
		"foreign": "https://other.example.com/web/generated",
	}
	for rkey, url := range feeds {
		if err := q.UpsertFeed(ctx, db.UpsertFeedParams{FeedUrl: url, CreatedAt: now, UpdatedAt: now}); err != nil {
			t.Fatal(err)
		}
		if err := q.UpsertUserSubscription(ctx, db.UpsertUserSubscriptionParams{
			Did: did, Rkey: rkey, FeedUrl: url,
			AtUri: "at://did:plc:example/blue.morgen.feed.subscription/" + rkey, CreatedAt: now, UpdatedAt: now,
		}); err != nil {
			t.Fatal(err)
		}
	}

	rows, err := q.ListUserSourcesWithStats(ctx, db.ListUserSourcesWithStatsParams{Did: did, Now: now, WebFeedPrefix: prefix, LegacyWebFeedPrefix: legacy})
	if err != nil || len(rows) != len(feeds) {
		t.Fatalf("list: %v, %v", rows, err)
	}
	for _, row := range rows {
		if row.Rkey == "generated" || row.Rkey == "legacy" {
			if row.WebHealth == nil || *row.WebHealth != "needs_attention" || row.WebHealthReason == nil || *row.WebHealthReason != "Morgenblau isn't sure which links are posts" {
				t.Errorf("generated feed lost its health: %+v", row)
			}
			continue
		}
		if row.WebHealth != nil || row.WebHealthReason != nil {
			t.Errorf("%s is not a generated feed but carries health: %+v", row.Rkey, row)
		}
	}

	for rkey := range feeds {
		detail, err := q.GetUserSourceWithStats(ctx, db.GetUserSourceWithStatsParams{Did: did, Rkey: rkey, Now: now, WebFeedPrefix: prefix, LegacyWebFeedPrefix: legacy})
		if err != nil {
			t.Fatal(err)
		}
		if generated := detail.WebHealth != nil && *detail.WebHealth == "needs_attention" && detail.WebHealthReason != nil; generated != (rkey == "generated" || rkey == "legacy") {
			t.Errorf("detail health for %s: %+v", rkey, detail)
		}
	}
	detail, err := q.GetUserSourceWithStats(ctx, db.GetUserSourceWithStatsParams{Did: did, Rkey: "generated", Now: now})
	if err != nil || detail.WebHealth != nil {
		t.Errorf("without a web feed prefix the detail is not generated: %+v, %v", detail, err)
	}

	rows, err = q.ListUserSourcesWithStats(ctx, db.ListUserSourcesWithStatsParams{Did: did, Now: now})
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range rows {
		if row.WebHealth != nil {
			t.Errorf("without a web feed prefix nothing is generated, got %s on %s", *row.WebHealth, row.Rkey)
		}
	}
}
