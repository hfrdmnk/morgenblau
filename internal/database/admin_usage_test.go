package database

import (
	"context"
	"path/filepath"
	"testing"

	"morgenblau/internal/database/db"
)

func TestAdminUsageCountsEachReaderBySourceType(t *testing.T) {
	path := filepath.Join(t.TempDir(), "usage.db")
	runGoose(t, path, "up")
	q := db.New(openMigrationDB(t, path))
	ctx := context.Background()
	const prefix, legacy = "https://app.example.com/feed/", "https://app.example.com/web/"
	const now = "2026-10-14T08:00:00Z"
	subscribe := func(did, rkey, feedURL, kind string) {
		t.Helper()
		if err := q.UpsertFeed(ctx, db.UpsertFeedParams{FeedUrl: feedURL, Kind: kind, CreatedAt: now, UpdatedAt: now}); err != nil {
			t.Fatal(err)
		}
		if err := q.UpsertUserSubscription(ctx, db.UpsertUserSubscriptionParams{
			Did: did, Rkey: rkey, FeedUrl: feedURL, Kind: kind,
			AtUri: "at://" + did + "/blue.morgen.feed.subscription/" + rkey, CreatedAt: now, UpdatedAt: now,
		}); err != nil {
			t.Fatal(err)
		}
	}
	subscribe("did:plc:alpha", "a1", "https://blog.example.com/feed.xml", "rss")
	subscribe("did:plc:alpha", "a2", prefix+"01generated", "rss")
	subscribe("did:plc:alpha", "a3", "at://did:plc:publication/site.standard.publication/self", "standard")
	subscribe("did:plc:alpha", "a4", legacy+"01legacy", "rss")
	// Another deployment's generated feed is an ordinary feed to this one.
	subscribe("did:plc:beta", "b1", "https://other.example.com/feed/01generated", "rss")

	got := map[string][2]int64{}
	rows, err := q.CountSubscriptionsByReader(ctx, db.CountSubscriptionsByReaderParams{WebFeedPrefix: prefix, LegacyWebFeedPrefix: legacy})
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range rows {
		got[row.Did] = [2]int64{row.TotalCount, row.GeneratedCount}
	}
	want := map[string][2]int64{"did:plc:alpha": {4, 2}, "did:plc:beta": {1, 0}}
	if len(got) != len(want) || got["did:plc:alpha"] != want["did:plc:alpha"] || got["did:plc:beta"] != want["did:plc:beta"] {
		t.Fatalf("counts = %v, want %v as {total, generated}", got, want)
	}
	rows, err = q.CountSubscriptionsByReader(ctx, db.CountSubscriptionsByReaderParams{})
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range rows {
		if row.GeneratedCount != 0 {
			t.Fatalf("%s generated = %d with no generated-feed prefix, want 0", row.Did, row.GeneratedCount)
		}
	}

	for _, s := range []struct{ did, sid, at string }{
		{"did:plc:alpha", "s1", "2026-10-10T08:00:00Z"},
		{"did:plc:alpha", "s2", "2026-10-13T08:00:00Z"},
		{"did:plc:gamma", "s3", "2026-10-01T08:00:00Z"},
	} {
		if err := q.PutSession(ctx, db.PutSessionParams{Did: s.did, SessionID: s.sid, Data: []byte("sealed"), UpdatedAt: s.at}); err != nil {
			t.Fatal(err)
		}
	}
	activity, err := q.ListAccountsLastActive(ctx)
	if err != nil {
		t.Fatal(err)
	}
	last := map[string]string{}
	for _, row := range activity {
		last[row.Did] = row.LastActiveAt
	}
	if len(last) != 2 || last["did:plc:alpha"] != "2026-10-13T08:00:00Z" || last["did:plc:gamma"] != "2026-10-01T08:00:00Z" {
		t.Fatalf("last active = %v, want each account's latest session", last)
	}

	for _, did := range []string{"did:plc:alpha", "did:plc:beta"} {
		if err := q.CreateNewsletterAddress(ctx, db.CreateNewsletterAddressParams{Did: did, LocalPart: "local-" + did[8:], CreatedAt: now}); err != nil {
			t.Fatal(err)
		}
	}
	for _, id := range []string{"n1", "n2", "n3"} {
		if err := q.CreateManualNewsletterSource(ctx, db.CreateManualNewsletterSourceParams{ID: id, Did: "did:plc:alpha", SourceKey: id, Title: "Example Newsletter", CreatedAt: now}); err != nil {
			t.Fatal(err)
		}
	}
	if err := q.SetNewsletterSourceStatus(ctx, db.SetNewsletterSourceStatusParams{Did: "did:plc:alpha", ID: "n3", Status: "stopped", UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	newsletters, err := q.CountActiveNewsletterSourcesByOwner(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(newsletters) != 1 || newsletters[0].Did != "did:plc:alpha" || newsletters[0].Active != 2 {
		t.Fatalf("newsletter counts = %+v, want only alpha's two active sources", newsletters)
	}
}
