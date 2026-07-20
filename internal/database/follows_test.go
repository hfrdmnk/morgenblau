package database

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	"morgenblau/internal/database/db"
)

// followsSchema mirrors internal/database/migrations/*_user_follows.sql; keep in sync.
const followsSchema = `
CREATE TABLE user_follows (
    did         TEXT NOT NULL,
    rkey        TEXT NOT NULL,
    at_uri      TEXT NOT NULL,
    subject_did TEXT NOT NULL,
    created_at  TEXT NOT NULL,
    updated_at  TEXT NOT NULL,
    PRIMARY KEY (did, rkey)
);
CREATE UNIQUE INDEX user_follows_did_subject_did_idx
    ON user_follows (did, subject_did);
`

func openFollowsTestDB(t *testing.T) *DB {
	t.Helper()
	t.Setenv("DB_PATH", filepath.Join(t.TempDir(), "test.db"))
	dbs, err := Open()
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { dbs.Close() })
	if _, err := dbs.Writer.ExecContext(context.Background(), followsSchema); err != nil {
		t.Fatalf("schema: %v", err)
	}
	return dbs
}

func TestUserFollows_UpsertAndGet(t *testing.T) {
	dbs := openFollowsTestDB(t)
	ctx := context.Background()
	q := db.New(dbs.Writer)

	if err := q.UpsertUserFollow(ctx, db.UpsertUserFollowParams{
		Did:        "did:plc:alice",
		Rkey:       "3fa",
		AtUri:      "at://did:plc:alice/blue.morgen.graph.follow/3fa",
		SubjectDid: "did:plc:bob",
		CreatedAt:  "2026-07-01T00:00:00Z",
		UpdatedAt:  "2026-07-01T00:00:00Z",
	}); err != nil {
		t.Fatalf("UpsertUserFollow: %v", err)
	}

	got, err := q.GetUserFollow(ctx, db.GetUserFollowParams{Did: "did:plc:alice", Rkey: "3fa"})
	if err != nil {
		t.Fatalf("GetUserFollow: %v", err)
	}
	if got.SubjectDid != "did:plc:bob" {
		t.Errorf("SubjectDid = %q, want did:plc:bob", got.SubjectDid)
	}
}

func TestUserFollows_UpsertOnConflict_UpdatesInPlace(t *testing.T) {
	dbs := openFollowsTestDB(t)
	ctx := context.Background()
	q := db.New(dbs.Writer)

	seed := db.UpsertUserFollowParams{
		Did: "did:plc:alice", Rkey: "3fa", AtUri: "at://a/1", SubjectDid: "did:plc:bob",
		CreatedAt: "2026-07-01T00:00:00Z", UpdatedAt: "2026-07-01T00:00:00Z",
	}
	if err := q.UpsertUserFollow(ctx, seed); err != nil {
		t.Fatalf("first upsert: %v", err)
	}
	seed.AtUri = "at://a/1-updated"
	seed.UpdatedAt = "2026-07-02T00:00:00Z"
	if err := q.UpsertUserFollow(ctx, seed); err != nil {
		t.Fatalf("second upsert: %v", err)
	}

	got, err := q.GetUserFollow(ctx, db.GetUserFollowParams{Did: "did:plc:alice", Rkey: "3fa"})
	if err != nil {
		t.Fatalf("GetUserFollow: %v", err)
	}
	if got.AtUri != "at://a/1-updated" {
		t.Errorf("AtUri = %q, want updated value (re-run overwrote, not duplicated)", got.AtUri)
	}
}

func TestUserFollows_GetBySubjectDID(t *testing.T) {
	dbs := openFollowsTestDB(t)
	ctx := context.Background()
	q := db.New(dbs.Writer)

	if err := q.UpsertUserFollow(ctx, db.UpsertUserFollowParams{
		Did: "did:plc:alice", Rkey: "3fa", AtUri: "at://a/1", SubjectDid: "did:plc:bob",
		CreatedAt: "2026-07-01T00:00:00Z", UpdatedAt: "2026-07-01T00:00:00Z",
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	got, err := q.GetUserFollowBySubjectDID(ctx, db.GetUserFollowBySubjectDIDParams{
		Did: "did:plc:alice", SubjectDid: "did:plc:bob",
	})
	if err != nil {
		t.Fatalf("GetUserFollowBySubjectDID: %v", err)
	}
	if got.Rkey != "3fa" {
		t.Errorf("Rkey = %q, want 3fa", got.Rkey)
	}

	if _, err := q.GetUserFollowBySubjectDID(ctx, db.GetUserFollowBySubjectDIDParams{
		Did: "did:plc:alice", SubjectDid: "did:plc:nobody",
	}); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("err = %v, want sql.ErrNoRows for an unfollowed subject", err)
	}
}

func TestUserFollows_SelfFollowHiddenFromReads(t *testing.T) {
	dbs := openFollowsTestDB(t)
	ctx := context.Background()
	q := db.New(dbs.Writer)

	if err := q.UpsertUserFollow(ctx, db.UpsertUserFollowParams{
		Did:        "did:plc:reader",
		Rkey:       "3self",
		AtUri:      "at://did:plc:reader/blue.morgen.graph.follow/3self",
		SubjectDid: "did:plc:reader",
		CreatedAt:  "2026-07-01T00:00:00Z",
		UpdatedAt:  "2026-07-01T00:00:00Z",
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	got, err := q.ListUserFollows(ctx, "did:plc:reader")
	if err != nil {
		t.Fatalf("ListUserFollows: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("ListUserFollows = %+v, want self-follow hidden", got)
	}

	if _, err := q.GetUserFollowBySubjectDID(ctx, db.GetUserFollowBySubjectDIDParams{
		Did:        "did:plc:reader",
		SubjectDid: "did:plc:reader",
	}); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("GetUserFollowBySubjectDID err = %v, want sql.ErrNoRows for self-follow", err)
	}
}

// The unique index on (did, subject_did) is the DB-level backstop for the idempotent-follow contract: it must reject a second row for the same subject even if the API's dedupe probe races.
func TestUserFollows_UniqueSubjectPerUser_RejectsDuplicate(t *testing.T) {
	dbs := openFollowsTestDB(t)
	ctx := context.Background()
	q := db.New(dbs.Writer)

	if err := q.UpsertUserFollow(ctx, db.UpsertUserFollowParams{
		Did: "did:plc:alice", Rkey: "3fa", AtUri: "at://a/1", SubjectDid: "did:plc:bob",
		CreatedAt: "2026-07-01T00:00:00Z", UpdatedAt: "2026-07-01T00:00:00Z",
	}); err != nil {
		t.Fatalf("first insert: %v", err)
	}

	err := q.UpsertUserFollow(ctx, db.UpsertUserFollowParams{
		Did: "did:plc:alice", Rkey: "3fb", AtUri: "at://a/2", SubjectDid: "did:plc:bob",
		CreatedAt: "2026-07-02T00:00:00Z", UpdatedAt: "2026-07-02T00:00:00Z",
	})
	if err == nil {
		t.Fatal("expected a UNIQUE constraint violation for a second rkey following the same subject")
	}
}

func TestUserFollows_ListScopedToOwner_NewestFirst(t *testing.T) {
	dbs := openFollowsTestDB(t)
	ctx := context.Background()
	q := db.New(dbs.Writer)

	seedFollow := func(did, rkey, subject, createdAt string) {
		t.Helper()
		if err := q.UpsertUserFollow(ctx, db.UpsertUserFollowParams{
			Did: did, Rkey: rkey, AtUri: "at://" + did + "/f/" + rkey, SubjectDid: subject,
			CreatedAt: createdAt, UpdatedAt: createdAt,
		}); err != nil {
			t.Fatalf("seedFollow(%s): %v", rkey, err)
		}
	}
	seedFollow("did:plc:alice", "3fa", "did:plc:bob", "2026-07-01T00:00:00Z")
	seedFollow("did:plc:alice", "3fb", "did:plc:carol", "2026-07-03T00:00:00Z")
	seedFollow("did:plc:bob", "3fc", "did:plc:dave", "2026-07-02T00:00:00Z") // another user's row

	got, err := q.ListUserFollows(ctx, "did:plc:alice")
	if err != nil {
		t.Fatalf("ListUserFollows: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2 (bob's row must not leak)", len(got))
	}
	if got[0].SubjectDid != "did:plc:carol" || got[1].SubjectDid != "did:plc:bob" {
		t.Errorf("order = [%s, %s], want newest first [carol, bob]", got[0].SubjectDid, got[1].SubjectDid)
	}
}

func TestUserFollows_Delete_FreesSubjectForRefollow(t *testing.T) {
	dbs := openFollowsTestDB(t)
	ctx := context.Background()
	q := db.New(dbs.Writer)

	if err := q.UpsertUserFollow(ctx, db.UpsertUserFollowParams{
		Did: "did:plc:alice", Rkey: "3fa", AtUri: "at://a/1", SubjectDid: "did:plc:bob",
		CreatedAt: "2026-07-01T00:00:00Z", UpdatedAt: "2026-07-01T00:00:00Z",
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := q.DeleteUserFollow(ctx, db.DeleteUserFollowParams{Did: "did:plc:alice", Rkey: "3fa"}); err != nil {
		t.Fatalf("DeleteUserFollow: %v", err)
	}
	if _, err := q.GetUserFollow(ctx, db.GetUserFollowParams{Did: "did:plc:alice", Rkey: "3fa"}); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("err = %v, want sql.ErrNoRows after delete", err)
	}

	// Re-following under a new rkey must succeed now that the delete freed the unique index slot.
	if err := q.UpsertUserFollow(ctx, db.UpsertUserFollowParams{
		Did: "did:plc:alice", Rkey: "3fz", AtUri: "at://a/2", SubjectDid: "did:plc:bob",
		CreatedAt: "2026-07-05T00:00:00Z", UpdatedAt: "2026-07-05T00:00:00Z",
	}); err != nil {
		t.Errorf("re-follow after unfollow: %v", err)
	}
}
