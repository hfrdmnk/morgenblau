package database

import (
	"context"
	"path/filepath"
	"testing"

	"morgenblau/internal/database/db"
)

const shareMetadataSchema = `
CREATE TABLE feeds (
    feed_url TEXT PRIMARY KEY
);
CREATE TABLE feed_entries (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    feed_url     TEXT NOT NULL,
    guid         TEXT NOT NULL,
    entry_slug   TEXT NOT NULL,
    url          TEXT NOT NULL,
    title        TEXT,
    content_html TEXT,
    content_type TEXT NOT NULL,
    published_at TEXT NOT NULL,
    fetched_at   TEXT NOT NULL,
    metadata     TEXT,
    extracted_body TEXT,
    record_cid   TEXT,
    UNIQUE (feed_url, guid),
    UNIQUE (entry_slug)
);
CREATE TABLE share_metadata_cache (
    target_key     TEXT PRIMARY KEY,
    title          TEXT,
    target_url     TEXT,
    fetched_at     TEXT,
    failure_count  INTEGER NOT NULL DEFAULT 0,
    next_retry_at  TEXT
);
`

func openShareMetadataTestDB(t *testing.T) *DB {
	t.Helper()
	t.Setenv("DB_PATH", filepath.Join(t.TempDir(), "test.db"))
	database, err := Open()
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if _, err := database.Writer.ExecContext(context.Background(), shareMetadataSchema); err != nil {
		t.Fatalf("schema: %v", err)
	}
	return database
}

func TestShareMetadataQueries_ResolveFeedEntriesByDocumentAndURL(t *testing.T) {
	database := openShareMetadataTestDB(t)
	ctx := context.Background()
	queries := db.New(database.Writer)
	title := "An entry"
	if _, err := database.Writer.ExecContext(ctx, `
		INSERT INTO feed_entries (
			feed_url, guid, entry_slug, url, title, content_type, published_at, fetched_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, "https://example.com/feed", "at://did:plc:pub/site.standard.document/3doc", "entry-slug",
		"https://example.com/post", title, "blogpost", "2026-07-01T00:00:00Z", "2026-07-01T00:00:00Z"); err != nil {
		t.Fatalf("insert entry: %v", err)
	}

	byDocument, err := queries.GetFeedEntryShareMetadataByDocument(ctx, "at://did:plc:pub/site.standard.document/3doc")
	if err != nil {
		t.Fatalf("GetFeedEntryShareMetadataByDocument: %v", err)
	}
	byURL, err := queries.GetFeedEntryShareMetadataByItemURL(ctx, "https://example.com/post")
	if err != nil {
		t.Fatalf("GetFeedEntryShareMetadataByItemURL: %v", err)
	}
	if byDocument.Title == nil || *byDocument.Title != title || byDocument.EntrySlug != "entry-slug" {
		t.Errorf("byDocument = %+v", byDocument)
	}
	if byURL.Title == nil || *byURL.Title != title || byURL.EntrySlug != "entry-slug" {
		t.Errorf("byURL = %+v", byURL)
	}
}

func TestShareMetadataQueries_FailurePreservesLastSuccess(t *testing.T) {
	database := openShareMetadataTestDB(t)
	ctx := context.Background()
	queries := db.New(database.Writer)
	title := "Last known title"
	targetURL := "https://example.com/post"
	fetchedAt := "2026-07-19T12:00:00Z"
	if err := queries.UpsertShareMetadataSuccess(ctx, db.UpsertShareMetadataSuccessParams{
		TargetKey: targetURL,
		Title:     &title,
		TargetUrl: &targetURL,
		FetchedAt: &fetchedAt,
	}); err != nil {
		t.Fatalf("UpsertShareMetadataSuccess: %v", err)
	}
	nextRetryAt := "2026-07-19T12:05:00Z"
	if err := queries.RecordShareMetadataFailure(ctx, db.RecordShareMetadataFailureParams{
		TargetKey: targetURL, FailureCount: 1, NextRetryAt: &nextRetryAt,
	}); err != nil {
		t.Fatalf("RecordShareMetadataFailure: %v", err)
	}

	got, err := queries.GetShareMetadataCache(ctx, targetURL)
	if err != nil {
		t.Fatalf("GetShareMetadataCache: %v", err)
	}
	if got.Title == nil || *got.Title != title || got.TargetUrl == nil || *got.TargetUrl != targetURL {
		t.Errorf("payload = %+v, want stale success preserved", got)
	}
	if got.FailureCount != 1 || got.NextRetryAt == nil || *got.NextRetryAt != nextRetryAt {
		t.Errorf("failure state = %+v", got)
	}
}
