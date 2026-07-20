-- name: UpsertUserFollow :exec
INSERT INTO user_follows (
    did, rkey, at_uri, subject_did, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?)
ON CONFLICT (did, rkey) DO UPDATE SET
    at_uri      = excluded.at_uri,
    subject_did = excluded.subject_did,
    updated_at  = excluded.updated_at;

-- name: GetUserFollow :one
SELECT did, rkey, at_uri, subject_did, created_at, updated_at
FROM user_follows WHERE did = ? AND rkey = ?;

-- name: GetUserFollowBySubjectDID :one
SELECT did, rkey, at_uri, subject_did, created_at, updated_at
FROM user_follows
WHERE did = ? AND subject_did = ? AND subject_did <> did;

-- name: ListUserFollows :many
SELECT did, rkey, at_uri, subject_did, created_at, updated_at
FROM user_follows
WHERE did = ? AND subject_did <> did
ORDER BY created_at DESC;

-- name: ListUserFollowsForSync :many
-- Snapshot of a user's local follow index, used by sync_user to diff against
-- the PDS and reconcile inserts/deletes.
SELECT did, rkey, at_uri, subject_did FROM user_follows WHERE did = ?;

-- name: DeleteUserFollow :exec
DELETE FROM user_follows WHERE did = ? AND rkey = ?;
