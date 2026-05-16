-- name: UpsertFeedEntry :exec
INSERT INTO feed_entries (feed_url, guid, entry_slug, url, title, content_html, content_type, published_at, fetched_at, metadata)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (feed_url, guid) DO UPDATE SET
    url          = excluded.url,
    title        = excluded.title,
    content_html = excluded.content_html,
    content_type = excluded.content_type,
    published_at = excluded.published_at,
    fetched_at   = excluded.fetched_at,
    metadata     = excluded.metadata;

-- name: GetFeedEntryBySlug :one
SELECT id, feed_url, guid, entry_slug, url, title, content_html, content_type, published_at, fetched_at, metadata, extracted_body
FROM feed_entries WHERE entry_slug = ?;

-- name: UpdateFeedEntryExtractedBody :exec
UPDATE feed_entries SET extracted_body = ? WHERE id = ?;

-- name: ListDigestForUser :many
SELECT
    e.id, e.feed_url, e.guid, e.entry_slug, e.url, e.title, e.content_html, e.content_type,
    e.published_at, e.fetched_at, e.metadata, e.extracted_body,
    f.title AS feed_title, f.site_url AS feed_site_url
FROM feed_entries e
JOIN feeds f ON f.feed_url = e.feed_url
JOIN user_subscriptions us ON us.feed_url = e.feed_url
WHERE us.did = ?
  AND e.published_at >= ?
  AND e.published_at < ?
ORDER BY e.published_at DESC, e.id DESC;

-- name: ListAllEntriesForUser :many
SELECT
    e.id, e.feed_url, e.guid, e.entry_slug, e.url, e.title, e.content_html, e.content_type,
    e.published_at, e.fetched_at, e.metadata, e.extracted_body,
    f.title AS feed_title, f.site_url AS feed_site_url
FROM feed_entries e
JOIN feeds f ON f.feed_url = e.feed_url
JOIN user_subscriptions us ON us.feed_url = e.feed_url
WHERE us.did = ?
ORDER BY e.published_at DESC, e.id DESC;
