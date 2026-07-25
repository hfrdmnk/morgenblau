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

-- name: ListUserSaves :many
-- Newest first. The entry join resolves title/slug/target for saves whose entry
-- is still cached; url is not unique in feed_entries, so the newest matching
-- entry is picked by id rather than joined on url directly, which would fan one
-- save out into a row per feed carrying the same link.
SELECT
    s.did, s.rkey, s.at_uri, s.item_url, s.feed_url, s.created_at, s.updated_at,
    e.title AS entry_title,
    e.entry_slug AS entry_slug,
    e.url AS entry_url,
    e.feed_url AS entry_feed_url
FROM user_saves s
LEFT JOIN feed_entries e ON e.id = (
    SELECT id FROM feed_entries WHERE url = s.item_url ORDER BY published_at DESC LIMIT 1
)
WHERE s.did = ?
ORDER BY s.created_at DESC, s.rkey DESC;

-- name: ListUserSavesForSync :many
-- Snapshot of a user's local save index, used by sync_user to diff against the
-- PDS and reconcile inserts/deletes.
SELECT did, rkey, at_uri, item_url, feed_url, created_at FROM user_saves WHERE did = ?;

-- name: DeleteUserSave :exec
DELETE FROM user_saves WHERE did = ? AND rkey = ?;
