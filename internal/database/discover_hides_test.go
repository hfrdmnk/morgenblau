package database

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	"morgenblau/internal/database/db"
)

// discoverHidesSchema mirrors internal/database/migrations/*_discover_hides.sql; keep in sync.
const discoverHidesSchema = `
CREATE TABLE discover_hides (
    did          TEXT NOT NULL,
    target_kind  TEXT NOT NULL,
    target_key   TEXT NOT NULL,
    hidden_until TEXT NOT NULL,
    hide_count   INTEGER NOT NULL DEFAULT 1,
    created_at   TEXT NOT NULL,
    updated_at   TEXT NOT NULL,
    PRIMARY KEY (did, target_kind, target_key)
);
CREATE INDEX discover_hides_active_idx
    ON discover_hides (did, target_kind, hidden_until);
`

func openDiscoverHidesTestDB(t *testing.T) *DB {
	t.Helper()
	t.Setenv("DB_PATH", filepath.Join(t.TempDir(), "test.db"))
	dbs, err := Open()
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { dbs.Close() })
	if _, err := dbs.Writer.ExecContext(context.Background(), discoverHidesSchema); err != nil {
		t.Fatalf("schema: %v", err)
	}
	return dbs
}

func TestDiscoverHides_UpsertAndGet(t *testing.T) {
	dbs := openDiscoverHidesTestDB(t)
	ctx := context.Background()
	q := db.New(dbs.Writer)

	if err := q.UpsertDiscoverHide(ctx, db.UpsertDiscoverHideParams{
		Did:         "did:plc:alice",
		TargetKind:  "source",
		TargetKey:   "https://example.com/feed",
		HiddenUntil: "2026-08-08T00:00:00Z",
		HideCount:   1,
		CreatedAt:   "2026-07-09T00:00:00Z",
		UpdatedAt:   "2026-07-09T00:00:00Z",
	}); err != nil {
		t.Fatalf("UpsertDiscoverHide: %v", err)
	}

	got, err := q.GetDiscoverHide(ctx, db.GetDiscoverHideParams{
		Did: "did:plc:alice", TargetKind: "source", TargetKey: "https://example.com/feed",
	})
	if err != nil {
		t.Fatalf("GetDiscoverHide: %v", err)
	}
	if got.HideCount != 1 || got.HiddenUntil != "2026-08-08T00:00:00Z" {
		t.Errorf("got = %+v", got)
	}
}

func TestDiscoverHides_UpsertOnConflict_EscalatesInPlace(t *testing.T) {
	dbs := openDiscoverHidesTestDB(t)
	ctx := context.Background()
	q := db.New(dbs.Writer)

	seed := db.UpsertDiscoverHideParams{
		Did: "did:plc:alice", TargetKind: "source", TargetKey: "https://example.com/feed",
		HiddenUntil: "2026-08-08T00:00:00Z", HideCount: 1,
		CreatedAt: "2026-07-09T00:00:00Z", UpdatedAt: "2026-07-09T00:00:00Z",
	}
	if err := q.UpsertDiscoverHide(ctx, seed); err != nil {
		t.Fatalf("first upsert: %v", err)
	}

	seed.HiddenUntil = "2027-01-05T00:00:00Z" // +180d
	seed.HideCount = 2
	seed.UpdatedAt = "2026-08-10T00:00:00Z"
	if err := q.UpsertDiscoverHide(ctx, seed); err != nil {
		t.Fatalf("second upsert: %v", err)
	}

	got, err := q.GetDiscoverHide(ctx, db.GetDiscoverHideParams{
		Did: "did:plc:alice", TargetKind: "source", TargetKey: "https://example.com/feed",
	})
	if err != nil {
		t.Fatalf("GetDiscoverHide: %v", err)
	}
	if got.HideCount != 2 {
		t.Errorf("HideCount = %d, want 2 (escalated, not duplicated)", got.HideCount)
	}
	if got.HiddenUntil != "2027-01-05T00:00:00Z" {
		t.Errorf("HiddenUntil = %q, want escalated value", got.HiddenUntil)
	}
	if got.CreatedAt != "2026-07-09T00:00:00Z" {
		t.Errorf("CreatedAt = %q, want original creation time preserved", got.CreatedAt)
	}
}

func TestDiscoverHides_SourceAndPersonKindsAreIndependentRows(t *testing.T) {
	dbs := openDiscoverHidesTestDB(t)
	ctx := context.Background()
	q := db.New(dbs.Writer)

	// Same target_key across kinds would collide if target_kind weren't part of the primary key.
	if err := q.UpsertDiscoverHide(ctx, db.UpsertDiscoverHideParams{
		Did: "did:plc:alice", TargetKind: "source", TargetKey: "did:plc:x",
		HiddenUntil: "2026-08-08T00:00:00Z", HideCount: 1,
		CreatedAt: "2026-07-09T00:00:00Z", UpdatedAt: "2026-07-09T00:00:00Z",
	}); err != nil {
		t.Fatalf("seed source: %v", err)
	}
	if err := q.UpsertDiscoverHide(ctx, db.UpsertDiscoverHideParams{
		Did: "did:plc:alice", TargetKind: "person", TargetKey: "did:plc:x",
		HiddenUntil: "2026-08-01T00:00:00Z", HideCount: 1,
		CreatedAt: "2026-07-09T00:00:00Z", UpdatedAt: "2026-07-09T00:00:00Z",
	}); err != nil {
		t.Fatalf("seed person: %v", err)
	}

	source, err := q.GetDiscoverHide(ctx, db.GetDiscoverHideParams{Did: "did:plc:alice", TargetKind: "source", TargetKey: "did:plc:x"})
	if err != nil {
		t.Fatalf("get source: %v", err)
	}
	person, err := q.GetDiscoverHide(ctx, db.GetDiscoverHideParams{Did: "did:plc:alice", TargetKind: "person", TargetKey: "did:plc:x"})
	if err != nil {
		t.Fatalf("get person: %v", err)
	}
	if source.HiddenUntil == person.HiddenUntil {
		t.Errorf("expected independent rows, both had HiddenUntil = %q", source.HiddenUntil)
	}
	count, err := q.CountDiscoverHidesForUser(ctx, "did:plc:alice")
	if err != nil {
		t.Fatalf("CountDiscoverHidesForUser: %v", err)
	}
	if count != 2 {
		t.Errorf("count = %d, want both target kinds for the owner", count)
	}
}

func TestDiscoverHides_ListActiveDiscoverHides_ScopedToOwnerKindAndActive(t *testing.T) {
	dbs := openDiscoverHidesTestDB(t)
	ctx := context.Background()
	q := db.New(dbs.Writer)

	seedHide := func(did, kind, key, until string) {
		t.Helper()
		if err := q.UpsertDiscoverHide(ctx, db.UpsertDiscoverHideParams{
			Did: did, TargetKind: kind, TargetKey: key,
			HiddenUntil: until, HideCount: 1,
			CreatedAt: "2026-07-01T00:00:00Z", UpdatedAt: "2026-07-01T00:00:00Z",
		}); err != nil {
			t.Fatalf("seedHide(%s,%s,%s): %v", did, kind, key, err)
		}
	}
	seedHide("did:plc:alice", "source", "https://active.example/feed", "2026-08-08T00:00:00Z")  // active
	seedHide("did:plc:alice", "source", "https://expired.example/feed", "2026-01-01T00:00:00Z") // resurfaced
	seedHide("did:plc:alice", "person", "did:plc:someone", "2026-08-08T00:00:00Z")              // wrong kind
	seedHide("did:plc:bob", "source", "https://active.example/feed", "2026-08-08T00:00:00Z")    // wrong owner

	got, err := q.ListActiveDiscoverHides(ctx, db.ListActiveDiscoverHidesParams{
		Did: "did:plc:alice", TargetKind: "source", HiddenUntil: "2026-07-09T00:00:00Z",
	})
	if err != nil {
		t.Fatalf("ListActiveDiscoverHides: %v", err)
	}
	if len(got) != 1 || got[0] != "https://active.example/feed" {
		t.Fatalf("got = %v, want only the active source-kind hide for did:plc:alice", got)
	}
}

func TestDiscoverHides_GetMissing_ErrNoRows(t *testing.T) {
	dbs := openDiscoverHidesTestDB(t)
	ctx := context.Background()
	q := db.New(dbs.Writer)

	if _, err := q.GetDiscoverHide(ctx, db.GetDiscoverHideParams{
		Did: "did:plc:alice", TargetKind: "source", TargetKey: "https://nope.example/feed",
	}); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("err = %v, want sql.ErrNoRows", err)
	}
}
