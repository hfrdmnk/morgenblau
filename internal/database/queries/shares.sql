-- name: UpsertUserShare :exec
INSERT INTO user_shares (
    did, rkey, at_uri, kind, item_url, document, comment, feed_url, sidecar_rkey, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (did, rkey) DO UPDATE SET
    at_uri       = excluded.at_uri,
    kind         = excluded.kind,
    item_url     = excluded.item_url,
    document     = excluded.document,
    comment      = excluded.comment,
    feed_url     = excluded.feed_url,
    sidecar_rkey = excluded.sidecar_rkey,
    updated_at   = excluded.updated_at;

-- name: GetUserShare :one
SELECT did, rkey, at_uri, kind, item_url, document, comment, feed_url, sidecar_rkey, created_at, updated_at
FROM user_shares WHERE did = ? AND rkey = ?;

-- name: GetUserShareByItemURL :one
-- Dedupe probe for rss shares; standardfeed shares dedupe by document instead.
SELECT did, rkey, at_uri, kind, item_url, document, comment, feed_url, sidecar_rkey, created_at, updated_at
FROM user_shares WHERE did = ? AND item_url = ? AND kind = 'rss';

-- name: GetUserShareByDocument :one
SELECT did, rkey, at_uri, kind, item_url, document, comment, feed_url, sidecar_rkey, created_at, updated_at
FROM user_shares WHERE did = ? AND document = ?;

-- name: ListUserShares :many
-- Newest first. The entry join resolves title/slug for document shares whose
-- entry is still cached; deleted entries fall back to item_url display.
SELECT
    s.did, s.rkey, s.at_uri, s.kind, s.item_url, s.document, s.comment,
    s.feed_url, s.sidecar_rkey, s.created_at, s.updated_at,
    e.title AS entry_title,
    e.entry_slug AS entry_slug
FROM user_shares s
LEFT JOIN feed_entries e ON e.guid = s.document
WHERE s.did = ?
ORDER BY s.created_at DESC, s.rkey DESC;

-- name: ListUserSharesForSync :many
-- Snapshot of a user's local share index, used by sync_user to diff against
-- the PDS and reconcile inserts/deletes.
SELECT did, rkey, at_uri, kind, item_url, document, sidecar_rkey, created_at FROM user_shares WHERE did = ?;

-- name: DeleteUserShare :exec
DELETE FROM user_shares WHERE did = ? AND rkey = ?;
