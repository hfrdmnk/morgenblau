-- name: UpsertUserSave :exec
INSERT INTO user_saves (
    did, rkey, at_uri, item_url, feed_url, tags, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (did, rkey) DO UPDATE SET
    at_uri     = excluded.at_uri,
    item_url   = excluded.item_url,
    feed_url   = excluded.feed_url,
    tags       = excluded.tags,
    updated_at = excluded.updated_at;

-- name: GetUserSave :one
SELECT did, rkey, at_uri, item_url, feed_url, created_at, updated_at, tags
FROM user_saves WHERE did = ? AND rkey = ?;

-- name: GetUserSaveByItemURL :one
SELECT did, rkey, at_uri, item_url, feed_url, created_at, updated_at, tags
FROM user_saves WHERE did = ? AND item_url = ?;

-- name: ListUserSaves :many
-- Newest first. The entry join resolves title/slug/target, body and source for
-- saves whose entry is still cached; url is not unique in feed_entries, so the
-- newest matching entry is picked by id rather than joined on url directly,
-- which would fan one save out into a row per feed carrying the same link.
SELECT
    s.did, s.rkey, s.at_uri, s.item_url, s.feed_url, s.created_at, s.updated_at, s.tags,
    e.title AS entry_title,
    e.entry_slug AS entry_slug,
    e.url AS entry_url,
    e.feed_url AS entry_feed_url,
    e.content_html AS entry_content_html,
    e.extracted_body AS entry_extracted_body,
    e.content_type AS entry_content_type,
    f.title AS catalog_title,
    f.site_url AS feed_site_url,
    f.icon_url AS feed_icon_url,
    us.title AS subscription_title,
    us.rkey AS subscription_rkey
FROM user_saves s
LEFT JOIN feed_entries e ON e.id = (
    SELECT id FROM feed_entries WHERE url = s.item_url ORDER BY published_at DESC LIMIT 1
)
LEFT JOIN feeds f ON f.feed_url = e.feed_url
LEFT JOIN user_subscriptions us ON us.did = s.did AND us.feed_url = e.feed_url
WHERE s.did = ?
ORDER BY s.created_at DESC, s.rkey DESC;

-- name: ListUserSavesForSync :many
-- Snapshot of a user's local save index, used by sync_user to diff against the
-- PDS and reconcile inserts/deletes.
SELECT did, rkey, at_uri, item_url, feed_url, created_at, updated_at, tags FROM user_saves WHERE did = ?;

-- name: DeleteUserSave :exec
DELETE FROM user_saves WHERE did = ? AND rkey = ?;

-- name: ListUserSaveTags :many
SELECT tags FROM user_saves WHERE did = ? AND tags IS NOT NULL;
