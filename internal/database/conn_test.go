package database

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	"morgenblau/internal/database/db"
)

// Minimal feeds table, enough for UpsertFeed / GetFeed / ListAllFeedURLs.
const feedsSchema = `
CREATE TABLE feeds (
    feed_url         TEXT PRIMARY KEY,
    kind             TEXT NOT NULL DEFAULT 'rss',
    site_url         TEXT,
    title            TEXT,
    etag             TEXT,
    last_modified    TEXT,
    last_fetched_at  TEXT,
    icon_url         TEXT,
    icon_fetched_at  TEXT,
    created_at       TEXT NOT NULL,
    updated_at       TEXT NOT NULL,
    language         TEXT,
    consecutive_failures INTEGER NOT NULL DEFAULT 0,
    next_fetch_at    TEXT
);`

func openTestDB(t *testing.T) *DB {
	t.Helper()
	// A real file (not :memory:): the two pools must share one database.
	t.Setenv("DB_PATH", filepath.Join(t.TempDir(), "test.db"))
	dbs, err := Open()
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { dbs.Close() })
	if _, err := dbs.Writer.ExecContext(context.Background(), feedsSchema); err != nil {
		t.Fatalf("schema: %v", err)
	}
	return dbs
}

func TestWithTx_Commit(t *testing.T) {
	dbs := openTestDB(t)
	ctx := context.Background()
	if err := WithTx(ctx, dbs.Writer, func(q *db.Queries) error {
		return q.UpsertFeed(ctx, db.UpsertFeedParams{FeedUrl: "https://a/feed", CreatedAt: "t", UpdatedAt: "t"})
	}); err != nil {
		t.Fatalf("WithTx: %v", err)
	}
	if _, err := db.New(dbs.Writer).GetFeed(ctx, "https://a/feed"); err != nil {
		t.Errorf("committed row not found: %v", err)
	}
}

func TestWithTx_Rollback(t *testing.T) {
	dbs := openTestDB(t)
	ctx := context.Background()
	sentinel := errors.New("boom")
	err := WithTx(ctx, dbs.Writer, func(q *db.Queries) error {
		if err := q.UpsertFeed(ctx, db.UpsertFeedParams{FeedUrl: "https://b/feed", CreatedAt: "t", UpdatedAt: "t"}); err != nil {
			return err
		}
		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("WithTx error = %v, want sentinel", err)
	}
	if _, err := db.New(dbs.Writer).GetFeed(ctx, "https://b/feed"); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("row survived rollback: err=%v", err)
	}
}

func TestWithTx_ReaderSeesCommit(t *testing.T) {
	dbs := openTestDB(t)
	ctx := context.Background()
	if err := WithTx(ctx, dbs.Writer, func(q *db.Queries) error {
		return q.UpsertFeed(ctx, db.UpsertFeedParams{FeedUrl: "https://c/feed", CreatedAt: "t", UpdatedAt: "t"})
	}); err != nil {
		t.Fatal(err)
	}
	// A read begun after the commit must observe it, even across pools (WAL).
	urls, err := db.New(dbs.Reader).ListAllFeedURLs(ctx)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, u := range urls {
		if u == "https://c/feed" {
			found = true
		}
	}
	if !found {
		t.Errorf("reader pool didn't see committed row: %v", urls)
	}
}
