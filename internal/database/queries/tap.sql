-- name: UpsertTapRecord :exec
INSERT INTO tap_records (did, collection, rkey, cid, record, indexed_at)
VALUES (?, ?, ?, ?, ?, ?)
ON CONFLICT (did, collection, rkey) DO UPDATE SET
    cid        = excluded.cid,
    record     = excluded.record,
    indexed_at = excluded.indexed_at;

-- name: DeleteTapRecord :exec
DELETE FROM tap_records WHERE did = ? AND collection = ? AND rkey = ?;

-- name: DeleteTapRecordsForRepo :exec
DELETE FROM tap_records WHERE did = ?;

-- name: ListTapRecordsForRepo :many
SELECT did, collection, rkey, cid, record, indexed_at
FROM tap_records WHERE did = ?;

-- name: MarkTapRepoDirty :exec
INSERT INTO tap_dirty_repos (did, marked_at) VALUES (?, ?)
ON CONFLICT (did) DO UPDATE SET marked_at = excluded.marked_at;

-- name: ListTapDirtyRepos :many
-- Oldest mark first so a backlog drains in arrival order. marked_at rides
-- along because DeleteTapDirtyRepo needs the value that was read.
SELECT did, marked_at FROM tap_dirty_repos ORDER BY marked_at, did LIMIT ?;

-- name: DeleteTapDirtyRepo :exec
-- The marked_at guard clears only the mark the rebuild actually consumed: a
-- repo re-dirtied while its rebuild was running keeps a newer row and gets
-- rebuilt again, instead of having that change silently dropped.
DELETE FROM tap_dirty_repos WHERE did = ? AND marked_at <= ?;

-- name: ListTapSeededDids :many
SELECT did FROM tap_seeder_state;

-- name: InsertTapSeededDid :exec
-- DO NOTHING keeps seeded_at at the first successful backfill, so a re-run
-- never rewrites the stamp it is meant to check.
INSERT INTO tap_seeder_state (did, seeded_at) VALUES (?, ?)
ON CONFLICT (did) DO NOTHING;

-- name: GetTapRepoState :one
SELECT did, handle, is_active, status, updated_at
FROM tap_repo_states
WHERE did = ?;

-- name: UpsertTapRepoState :exec
INSERT INTO tap_repo_states (did, handle, is_active, status, updated_at)
VALUES (?, ?, ?, ?, ?)
ON CONFLICT (did) DO UPDATE SET
    handle     = excluded.handle,
    is_active  = excluded.is_active,
    status     = excluded.status,
    updated_at = excluded.updated_at;
