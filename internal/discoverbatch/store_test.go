package discoverbatch

import (
	"context"
	"path/filepath"
	"testing"

	"morgenblau/internal/database"
	"morgenblau/internal/database/db"
	"morgenblau/internal/discovercrawl"
)

const trendingSignalsSchema = `
CREATE TABLE discover_trending_signals (
    repo_did    TEXT NOT NULL,
    source_key  TEXT NOT NULL,
    kind        TEXT NOT NULL,
    title       TEXT,
    site_url    TEXT,
    signal_kind TEXT NOT NULL,
    signal_at   TEXT,
    fetched_at  TEXT NOT NULL,
    PRIMARY KEY (repo_did, source_key)
);
CREATE TABLE discover_trending_follows (
    repo_did    TEXT NOT NULL,
    subject_did TEXT NOT NULL,
    fetched_at  TEXT NOT NULL,
    PRIMARY KEY (repo_did, subject_did)
);
`

func openTrendingTestDB(t *testing.T) *database.DB {
	t.Helper()
	t.Setenv("DB_PATH", filepath.Join(t.TempDir(), "test.db"))
	dbs, err := database.Open()
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { dbs.Close() })
	if _, err := dbs.Writer.ExecContext(context.Background(), trendingSignalsSchema); err != nil {
		t.Fatalf("schema: %v", err)
	}
	return dbs
}

func TestBatch_Run_WritesToRealSQLite(t *testing.T) {
	dbs := openTrendingTestDB(t)

	crawler := newFakeRepoCrawler()
	crawler.subs["did:plc:alice"] = []discovercrawl.Subscription{
		{Key: "https://a.example/feed", Kind: "rss", Title: "A", SiteURL: "https://a.example", CreatedAt: "2026-07-01T00:00:00Z"},
	}

	b, _ := newTestBatch(t, []string{"did:plc:alice"}, map[string]string{"did:plc:alice": "https://pds-a.example"}, crawler)
	b.WithTxRunner(dbs.Writer)

	if _, err := b.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}

	rows, err := db.New(dbs.Reader).ListDiscoverTrendingSignals(context.Background())
	if err != nil {
		t.Fatalf("ListDiscoverTrendingSignals: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows = %+v, want 1", rows)
	}
	if rows[0].RepoDid != "did:plc:alice" || rows[0].SourceKey != "https://a.example/feed" {
		t.Errorf("row = %+v", rows[0])
	}
	if rows[0].SignalKind != "subscribe" {
		t.Errorf("SignalKind = %q, want subscribe", rows[0].SignalKind)
	}
}

func TestBatch_Run_SameDayRerunReplacesInRealSQLite(t *testing.T) {
	dbs := openTrendingTestDB(t)

	crawler := newFakeRepoCrawler()
	crawler.subs["did:plc:alice"] = []discovercrawl.Subscription{{Key: "https://old.example/feed", Kind: "rss"}}

	b, _ := newTestBatch(t, []string{"did:plc:alice"}, map[string]string{"did:plc:alice": "https://pds-a.example"}, crawler)
	b.WithTxRunner(dbs.Writer)

	if _, err := b.Run(context.Background()); err != nil {
		t.Fatalf("first Run: %v", err)
	}

	crawler.subs["did:plc:alice"] = []discovercrawl.Subscription{{Key: "https://new.example/feed", Kind: "rss"}}
	if _, err := b.Run(context.Background()); err != nil {
		t.Fatalf("second Run: %v", err)
	}

	rows, err := db.New(dbs.Reader).ListDiscoverTrendingSignals(context.Background())
	if err != nil {
		t.Fatalf("ListDiscoverTrendingSignals: %v", err)
	}
	if len(rows) != 1 || rows[0].SourceKey != "https://new.example/feed" {
		t.Fatalf("rows = %+v, want only the new source key (diff/replace, not accumulate)", rows)
	}
}

func TestBatch_Run_MultipleReposWriteIndependentRowsInRealSQLite(t *testing.T) {
	dbs := openTrendingTestDB(t)

	crawler := newFakeRepoCrawler()
	crawler.subs["did:plc:alice"] = []discovercrawl.Subscription{{Key: "https://shared.example/feed", Kind: "rss"}}
	crawler.subs["did:plc:bob"] = []discovercrawl.Subscription{{Key: "https://shared.example/feed", Kind: "rss"}}

	b, _ := newTestBatch(t, []string{"did:plc:alice", "did:plc:bob"}, map[string]string{
		"did:plc:alice": "https://pds-a.example",
		"did:plc:bob":   "https://pds-b.example",
	}, crawler)
	b.WithTxRunner(dbs.Writer)

	if _, err := b.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}

	rows, err := db.New(dbs.Reader).ListDiscoverTrendingSignals(context.Background())
	if err != nil {
		t.Fatalf("ListDiscoverTrendingSignals: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("rows = %+v, want 2 (one per repo, same source key)", rows)
	}
}

func TestBatch_Run_FollowPass_WritesToRealSQLite(t *testing.T) {
	dbs := openTrendingTestDB(t)

	relay := fakeRelayByCollection(t, map[string][]string{
		"blue.morgen.graph.follow": {"did:plc:alice"},
	})
	crawler := newFakeRepoCrawler()
	crawler.follows["did:plc:alice"] = []discovercrawl.ReaderNetworkFollow{{DID: "did:plc:bob"}}

	b := New(relay.URL, relay.Client(), &fakeBatchResolver{hostByDID: map[string]string{
		"did:plc:alice": "https://pds-a.example",
	}}, crawler, fakeEntries{})
	b.collections = nil
	b.followCollections = []string{"blue.morgen.graph.follow"}
	b.WithTxRunner(dbs.Writer)

	if _, err := b.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}

	rows, err := db.New(dbs.Reader).ListDiscoverTrendingFollows(context.Background())
	if err != nil {
		t.Fatalf("ListDiscoverTrendingFollows: %v", err)
	}
	if len(rows) != 1 || rows[0].RepoDid != "did:plc:alice" || rows[0].SubjectDid != "did:plc:bob" {
		t.Fatalf("rows = %+v, want one alice->bob row", rows)
	}
}

func TestBatch_Run_FollowPass_SameDayRerunReplacesInRealSQLite(t *testing.T) {
	dbs := openTrendingTestDB(t)

	relay := fakeRelayByCollection(t, map[string][]string{
		"blue.morgen.graph.follow": {"did:plc:alice"},
	})
	crawler := newFakeRepoCrawler()
	crawler.follows["did:plc:alice"] = []discovercrawl.ReaderNetworkFollow{{DID: "did:plc:bob"}}

	b := New(relay.URL, relay.Client(), &fakeBatchResolver{hostByDID: map[string]string{
		"did:plc:alice": "https://pds-a.example",
	}}, crawler, fakeEntries{})
	b.collections = nil
	b.followCollections = []string{"blue.morgen.graph.follow"}
	b.WithTxRunner(dbs.Writer)

	if _, err := b.Run(context.Background()); err != nil {
		t.Fatalf("first Run: %v", err)
	}

	crawler.follows["did:plc:alice"] = []discovercrawl.ReaderNetworkFollow{{DID: "did:plc:carol"}}
	if _, err := b.Run(context.Background()); err != nil {
		t.Fatalf("second Run: %v", err)
	}

	rows, err := db.New(dbs.Reader).ListDiscoverTrendingFollows(context.Background())
	if err != nil {
		t.Fatalf("ListDiscoverTrendingFollows: %v", err)
	}
	if len(rows) != 1 || rows[0].SubjectDid != "did:plc:carol" {
		t.Fatalf("rows = %+v, want only the new follow (diff/replace, not accumulate)", rows)
	}
}
