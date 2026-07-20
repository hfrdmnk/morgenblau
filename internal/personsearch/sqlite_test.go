package personsearch

import (
	"context"
	"path/filepath"
	"testing"

	"morgenblau/internal/database"
	"morgenblau/internal/database/db"
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
`

func openPresenceTestDB(t *testing.T) *database.DB {
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

func insertSignal(t *testing.T, q *db.Queries, repoDID, sourceKey, kind, title, signalKind string) {
	t.Helper()
	if err := q.InsertDiscoverTrendingSignal(context.Background(), db.InsertDiscoverTrendingSignalParams{
		RepoDid:    repoDID,
		SourceKey:  sourceKey,
		Kind:       kind,
		Title:      &title,
		SignalKind: signalKind,
		FetchedAt:  "2026-07-09T00:00:00Z",
	}); err != nil {
		t.Fatalf("InsertDiscoverTrendingSignal: %v", err)
	}
}

func TestSQLitePresenceReader_Presence_SaveOnlyDIDDoesNotBadge(t *testing.T) {
	dbs := openPresenceTestDB(t)
	q := db.New(dbs.Writer)

	insertSignal(t, q, didAlice, "https://a.example/feed", "rss", "A Feed", "subscribe")
	insertSignal(t, q, didBob, "https://b.example/feed", "rss", "B Feed", "save")
	// didCarol has no rows at all.

	r := NewSQLitePresenceReader(db.New(dbs.Reader))
	present, err := r.Presence(context.Background(), []string{didAlice, didBob, didCarol})
	if err != nil {
		t.Fatalf("Presence: %v", err)
	}

	if !present[didAlice] {
		t.Errorf("didAlice present = %v, want true (has a subscribe signal)", present[didAlice])
	}
	if present[didBob] {
		t.Errorf("didBob present = %v, want false (save-only must not badge)", present[didBob])
	}
	if present[didCarol] {
		t.Errorf("didCarol present = %v, want false (no signals at all)", present[didCarol])
	}
}

func TestSQLitePresenceReader_TasteHints_SubscribeAuthorOnlyCapped(t *testing.T) {
	dbs := openPresenceTestDB(t)
	q := db.New(dbs.Writer)

	insertSignal(t, q, didAlice, "https://a1.example/feed", "rss", "Feed One", "subscribe")
	insertSignal(t, q, didAlice, "https://a2.example/feed", "rss", "Feed Two", "author")
	insertSignal(t, q, didAlice, "https://a3.example/feed", "rss", "Feed Three", "share")

	r := NewSQLitePresenceReader(db.New(dbs.Reader))
	hints, err := r.TasteHints(context.Background(), didAlice, 1)
	if err != nil {
		t.Fatalf("TasteHints: %v", err)
	}

	if len(hints) != 1 {
		t.Fatalf("hints = %v, want exactly 1 (capped, share signal excluded)", hints)
	}
	if hints[0] == "Feed Three" {
		t.Errorf("hints = %v, share-kind title must never appear", hints)
	}
}
