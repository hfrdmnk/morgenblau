-- name: UpsertUserSave :exec
INSERT INTO user_saves (
    did, rkey, at_uri, item_url, feed_url, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (did, rkey) DO UPDATE SET
    at_uri     = excluded.at_uri,
    item_url   = excluded.item_url,
    feed_url   = excluded.feed_url,
    updated_at = excluded.updated_at;

-- name: GetUserSave :one
SELECT did, rkey, at_uri, item_url, feed_url, created_at, updated_at
FROM user_saves WHERE did = ? AND rkey = ?;

-- name: GetUserSaveByItemURL :one
SELECT did, rkey, at_uri, item_url, feed_url, created_at, updated_at
FROM user_saves WHERE did = ? AND item_url = ?;

-- name: DeleteUserSave :exec
DELETE FROM user_saves WHERE did = ? AND rkey = ?;
