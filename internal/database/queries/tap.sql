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
INSERT INTO tap_dirty_repos (did, marked_seq) VALUES (?, ?)
ON CONFLICT (did) DO UPDATE SET marked_seq = MAX(tap_dirty_repos.marked_seq, excluded.marked_seq);

-- name: ListTapDirtyRepos :many
SELECT did, marked_seq FROM tap_dirty_repos ORDER BY marked_seq, did LIMIT ?;

-- name: DeleteTapDirtyRepo :exec
DELETE FROM tap_dirty_repos WHERE did = ? AND marked_seq = ?;

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

-- name: UpsertTapRepoHandle :exec
-- Handle-only upsert. An identity event carries no hosting status, so an
-- existing row keeps whatever the account stream last said; a brand new row
-- defaults to active, which is what a repo emitting identity events is.
INSERT INTO tap_repo_states (did, handle, is_active, status, updated_at)
VALUES (?, ?, 1, '', ?)
ON CONFLICT (did) DO UPDATE SET
    handle     = excluded.handle,
    updated_at = excluded.updated_at;

-- name: UpsertTapRepoAccount :exec
-- Status-only upsert. An account event carries no handle, so an existing row
-- keeps the handle identity last resolved instead of blanking it.
INSERT INTO tap_repo_states (did, handle, is_active, status, updated_at)
VALUES (?, '', ?, ?, ?)
ON CONFLICT (did) DO UPDATE SET
    is_active  = excluded.is_active,
    status     = excluded.status,
    updated_at = excluded.updated_at;

-- name: TapRepoIsMirrored :one
-- Existence probe for the network-wide identity/account/sync stream. Those
-- markers reach every subscriber regardless of the collection filter, so a
-- marker for a repo we never mirrored must not mint state rows for it.
SELECT EXISTS (
    SELECT 1 FROM tap_records r WHERE r.did = sqlc.arg(did)
    UNION ALL
    SELECT 1 FROM tap_repo_states s WHERE s.did = sqlc.arg(did)
) AS mirrored;
