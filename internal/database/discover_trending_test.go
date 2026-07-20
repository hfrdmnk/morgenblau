package database

import (
	"context"
	"path/filepath"
	"sort"
	"testing"

	"morgenblau/internal/database/db"
)

// discoverTrendingSchema mirrors internal/database/migrations/*_discover_trending_signals.sql and *_discover_trending_follows.sql; keep in sync.
const discoverTrendingSchema = `
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
CREATE INDEX discover_trending_signals_source_idx
    ON discover_trending_signals (source_key);

CREATE TABLE discover_trending_follows (
    repo_did    TEXT NOT NULL,
    subject_did TEXT NOT NULL,
    fetched_at  TEXT NOT NULL,
    PRIMARY KEY (repo_did, subject_did)
);
CREATE INDEX discover_trending_follows_subject_idx
    ON discover_trending_follows (subject_did);
`

func openDiscoverTrendingTestDB(t *testing.T) *DB {
	t.Helper()
	t.Setenv("DB_PATH", filepath.Join(t.TempDir(), "test.db"))
	dbs, err := Open()
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { dbs.Close() })
	if _, err := dbs.Writer.ExecContext(context.Background(), discoverTrendingSchema); err != nil {
		t.Fatalf("schema: %v", err)
	}
	return dbs
}

func insertTrendingSignal(t *testing.T, q *db.Queries, repoDID, sourceKey string) {
	t.Helper()
	if err := q.InsertDiscoverTrendingSignal(context.Background(), db.InsertDiscoverTrendingSignalParams{
		RepoDid:    repoDID,
		SourceKey:  sourceKey,
		Kind:       "rss",
		SignalKind: "subscribe",
		FetchedAt:  "2026-07-09T00:00:00Z",
	}); err != nil {
		t.Fatalf("InsertDiscoverTrendingSignal(%s, %s): %v", repoDID, sourceKey, err)
	}
}

// insertTrendingSignalKind inserts a signal under a caller-chosen signal_kind, for save-privacy tests that need something other than insertTrendingSignal's fixed "subscribe".
func insertTrendingSignalKind(t *testing.T, q *db.Queries, repoDID, sourceKey, signalKind string) {
	t.Helper()
	if err := q.InsertDiscoverTrendingSignal(context.Background(), db.InsertDiscoverTrendingSignalParams{
		RepoDid:    repoDID,
		SourceKey:  sourceKey,
		Kind:       "rss",
		SignalKind: signalKind,
		FetchedAt:  "2026-07-09T00:00:00Z",
	}); err != nil {
		t.Fatalf("InsertDiscoverTrendingSignal(%s, %s, %s): %v", repoDID, sourceKey, signalKind, err)
	}
}

func insertTrendingFollow(t *testing.T, q *db.Queries, repoDID, subjectDID string) {
	t.Helper()
	if err := q.InsertDiscoverTrendingFollow(context.Background(), db.InsertDiscoverTrendingFollowParams{
		RepoDid:    repoDID,
		SubjectDid: subjectDID,
		FetchedAt:  "2026-07-09T00:00:00Z",
	}); err != nil {
		t.Fatalf("InsertDiscoverTrendingFollow(%s, %s): %v", repoDID, subjectDID, err)
	}
}

func sourceKeys(rows []db.DiscoverTrendingSignal) []string {
	seen := map[string]struct{}{}
	for _, r := range rows {
		seen[r.SourceKey] = struct{}{}
	}
	out := make([]string, 0, len(seen))
	for k := range seen {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func subjectDIDs(rows []db.DiscoverTrendingFollow) []string {
	seen := map[string]struct{}{}
	for _, r := range rows {
		seen[r.SubjectDid] = struct{}{}
	}
	out := make([]string, 0, len(seen))
	for k := range seen {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func TestListDiscoverTrendingSignalsAboveBar_TwoDistinctReposExcludedThreeIncluded(t *testing.T) {
	dbs := openDiscoverTrendingTestDB(t)
	ctx := context.Background()
	q := db.New(dbs.Writer)

	insertTrendingSignal(t, q, "did:plc:repo1", "https://two.example/feed")
	insertTrendingSignal(t, q, "did:plc:repo2", "https://two.example/feed")

	insertTrendingSignal(t, q, "did:plc:repo1", "https://three.example/feed")
	insertTrendingSignal(t, q, "did:plc:repo2", "https://three.example/feed")
	insertTrendingSignal(t, q, "did:plc:repo3", "https://three.example/feed")

	rows, err := q.ListDiscoverTrendingSignalsAboveBar(ctx, 3)
	if err != nil {
		t.Fatalf("ListDiscoverTrendingSignalsAboveBar: %v", err)
	}
	got := sourceKeys(rows)
	if len(got) != 1 || got[0] != "https://three.example/feed" {
		t.Fatalf("got source keys = %v, want only the 3-distinct-repo source", got)
	}
}

func TestListDiscoverTrendingSignalsAboveBar_DuplicateRepoDIDRowsCountOnce(t *testing.T) {
	dbs := openDiscoverTrendingTestDB(t)
	ctx := context.Background()
	q := db.New(dbs.Writer)

	// Simulates the daily batch's delete-then-reinsert idempotency for repo1; the replace must not inflate the distinct-repo count.
	insertTrendingSignal(t, q, "did:plc:repo1", "https://two.example/feed")
	insertTrendingSignal(t, q, "did:plc:repo1", "https://three.example/feed")
	if err := q.DeleteDiscoverTrendingSignalsForRepo(ctx, "did:plc:repo1"); err != nil {
		t.Fatalf("DeleteDiscoverTrendingSignalsForRepo: %v", err)
	}
	insertTrendingSignal(t, q, "did:plc:repo1", "https://two.example/feed")
	insertTrendingSignal(t, q, "did:plc:repo1", "https://three.example/feed")

	insertTrendingSignal(t, q, "did:plc:repo2", "https://two.example/feed")

	insertTrendingSignal(t, q, "did:plc:repo2", "https://three.example/feed")
	insertTrendingSignal(t, q, "did:plc:repo3", "https://three.example/feed")

	rows, err := q.ListDiscoverTrendingSignalsAboveBar(ctx, 3)
	if err != nil {
		t.Fatalf("ListDiscoverTrendingSignalsAboveBar: %v", err)
	}
	got := sourceKeys(rows)
	if len(got) != 1 || got[0] != "https://three.example/feed" {
		t.Fatalf("got source keys = %v, want only the 3-distinct-repo source (repo1's replayed rows must count once)", got)
	}
}

func TestListDiscoverTrendingFollowsAboveBar_TwoDistinctReposExcludedThreeIncluded(t *testing.T) {
	dbs := openDiscoverTrendingTestDB(t)
	ctx := context.Background()
	q := db.New(dbs.Writer)

	insertTrendingFollow(t, q, "did:plc:repo1", "did:plc:two-followers")
	insertTrendingFollow(t, q, "did:plc:repo2", "did:plc:two-followers")

	insertTrendingFollow(t, q, "did:plc:repo1", "did:plc:three-followers")
	insertTrendingFollow(t, q, "did:plc:repo2", "did:plc:three-followers")
	insertTrendingFollow(t, q, "did:plc:repo3", "did:plc:three-followers")

	rows, err := q.ListDiscoverTrendingFollowsAboveBar(ctx, 3)
	if err != nil {
		t.Fatalf("ListDiscoverTrendingFollowsAboveBar: %v", err)
	}
	got := subjectDIDs(rows)
	if len(got) != 1 || got[0] != "did:plc:three-followers" {
		t.Fatalf("got subject DIDs = %v, want only the 3-distinct-follower subject", got)
	}
}

func TestListDiscoverTrendingFollowsAboveBar_DuplicateRepoDIDRowsCountOnce(t *testing.T) {
	dbs := openDiscoverTrendingTestDB(t)
	ctx := context.Background()
	q := db.New(dbs.Writer)

	insertTrendingFollow(t, q, "did:plc:repo1", "did:plc:two-followers")
	insertTrendingFollow(t, q, "did:plc:repo1", "did:plc:three-followers")
	if err := q.DeleteDiscoverTrendingFollowsForRepo(ctx, "did:plc:repo1"); err != nil {
		t.Fatalf("DeleteDiscoverTrendingFollowsForRepo: %v", err)
	}
	insertTrendingFollow(t, q, "did:plc:repo1", "did:plc:two-followers")
	insertTrendingFollow(t, q, "did:plc:repo1", "did:plc:three-followers")

	insertTrendingFollow(t, q, "did:plc:repo2", "did:plc:two-followers")

	insertTrendingFollow(t, q, "did:plc:repo2", "did:plc:three-followers")
	insertTrendingFollow(t, q, "did:plc:repo3", "did:plc:three-followers")

	rows, err := q.ListDiscoverTrendingFollowsAboveBar(ctx, 3)
	if err != nil {
		t.Fatalf("ListDiscoverTrendingFollowsAboveBar: %v", err)
	}
	got := subjectDIDs(rows)
	if len(got) != 1 || got[0] != "did:plc:three-followers" {
		t.Fatalf("got subject DIDs = %v, want only the 3-distinct-follower subject (repo1's replayed rows must count once)", got)
	}
}

func TestListDiscoverTrendingSignalsForEligibleSubjects_OnlyReturnsSignalsForBarPassingSubjects(t *testing.T) {
	dbs := openDiscoverTrendingTestDB(t)
	ctx := context.Background()
	q := db.New(dbs.Writer)

	// did:plc:eligible clears the follower bar (3 distinct repos) and has its own signal.
	insertTrendingFollow(t, q, "did:plc:repo1", "did:plc:eligible")
	insertTrendingFollow(t, q, "did:plc:repo2", "did:plc:eligible")
	insertTrendingFollow(t, q, "did:plc:repo3", "did:plc:eligible")
	insertTrendingSignal(t, q, "did:plc:eligible", "https://eligible-owns.example/feed")

	// did:plc:below-bar has 2 distinct followers (below the bar); its signals must not leak into the bounded read.
	insertTrendingFollow(t, q, "did:plc:repo1", "did:plc:below-bar")
	insertTrendingFollow(t, q, "did:plc:repo2", "did:plc:below-bar")
	insertTrendingSignal(t, q, "did:plc:below-bar", "https://below-bar-owns.example/feed")

	rows, err := q.ListDiscoverTrendingSignalsForEligibleSubjects(ctx, 3)
	if err != nil {
		t.Fatalf("ListDiscoverTrendingSignalsForEligibleSubjects: %v", err)
	}
	if len(rows) != 1 || rows[0].RepoDid != "did:plc:eligible" {
		t.Fatalf("got rows = %+v, want only did:plc:eligible's own signal", rows)
	}
}

func TestListDiscoverTrendingSignalsForEligibleSubjects_SaveOnlySignalNeverGrantsEligibility(t *testing.T) {
	// SPEC <discovery> People "Eligibility": saves don't confer eligibility.
	dbs := openDiscoverTrendingTestDB(t)
	ctx := context.Background()
	q := db.New(dbs.Writer)

	// did:plc:save-only clears the follower bar (3 distinct repos) but its only own-DID signal is a save.
	insertTrendingFollow(t, q, "did:plc:repo1", "did:plc:save-only")
	insertTrendingFollow(t, q, "did:plc:repo2", "did:plc:save-only")
	insertTrendingFollow(t, q, "did:plc:repo3", "did:plc:save-only")
	insertTrendingSignalKind(t, q, "did:plc:save-only", "https://save-only-owns.example/feed", "save")

	rows, err := q.ListDiscoverTrendingSignalsForEligibleSubjects(ctx, 3)
	if err != nil {
		t.Fatalf("ListDiscoverTrendingSignalsForEligibleSubjects: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("got rows = %+v, want none (a save-only signal must never mark a bar-passing subject eligible)", rows)
	}
}
