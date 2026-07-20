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

-- name: UpsertStandardfeedEntry :exec
-- Standardfeed counterpart to UpsertFeedEntry. Unlike the rss upsert it owns
-- extracted_body and record_cid on conflict: a CID change must reset the
-- cached readability body (path-ful docs pass NULL) or refresh the plaintext
-- fallback (path-less docs pass the new textContent).
INSERT INTO feed_entries (feed_url, guid, entry_slug, url, title, content_html, content_type, published_at, fetched_at, metadata, extracted_body, record_cid)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (feed_url, guid) DO UPDATE SET
    url            = excluded.url,
    title          = excluded.title,
    content_html   = excluded.content_html,
    content_type   = excluded.content_type,
    published_at   = excluded.published_at,
    fetched_at     = excluded.fetched_at,
    metadata       = excluded.metadata,
    extracted_body = excluded.extracted_body,
    record_cid     = excluded.record_cid;

-- name: ListFeedEntriesForDiff :many
SELECT guid, record_cid FROM feed_entries WHERE feed_url = ?;

-- name: GetFeedEntryURLByGuid :one
-- Reconcile backfills a comment-less share's item_url from its cached entry.
-- Standardfeed document guids (the document at-uri) are globally unique.
SELECT url FROM feed_entries WHERE guid = ? LIMIT 1;

-- name: GetFeedURLByGuid :one
-- Discover signal resolution (SPEC <discovery>): a reaction's document
-- at-uri (standardfeed provenance) maps straight to its source's feed_url
-- when the entry is cached.
SELECT feed_url FROM feed_entries WHERE guid = ? LIMIT 1;

-- name: GetFeedURLByItemURL :one
-- Discover signal resolution, Tier-2 fallback (SPEC <discovery>): a reaction
-- carrying only itemUrl resolves to its source via the cached entry's own
-- url match.
SELECT feed_url FROM feed_entries WHERE url = ? LIMIT 1;

-- name: DeleteFeedEntry :exec
DELETE FROM feed_entries WHERE feed_url = ? AND guid = ?;

-- name: GetFeedEntryBySlug :one
SELECT id, feed_url, guid, entry_slug, url, title, content_html, content_type, published_at, fetched_at, metadata, extracted_body, record_cid
FROM feed_entries WHERE entry_slug = ?;

-- name: UpdateFeedEntryExtractedBody :exec
UPDATE feed_entries SET extracted_body = ? WHERE id = ?;

-- name: ListDigestForUser :many
SELECT
    e.id, e.feed_url, e.guid, e.entry_slug, e.url, e.title, e.content_html, e.content_type,
    e.published_at, e.fetched_at, e.metadata, e.extracted_body,
    us.title AS feed_title,
    f.title AS catalog_title,
    f.site_url AS feed_site_url,
    f.icon_url AS feed_icon_url
FROM feed_entries e
JOIN feeds f ON f.feed_url = e.feed_url
JOIN user_subscriptions us ON us.feed_url = e.feed_url
WHERE us.did = ?
  AND e.published_at >= ?
  AND e.published_at < ?
ORDER BY e.published_at DESC, e.id DESC;

-- name: ListEntriesForSource :many
-- Entries from a single feed, newest first, bounded by limit. The join to
-- user_subscriptions doubles as an ownership filter; the handler still
-- distinguishes "not subscribed" from "no posts" via a separate lookup.
SELECT
    e.id, e.feed_url, e.guid, e.entry_slug, e.url, e.title, e.content_html, e.content_type,
    e.published_at, e.fetched_at, e.metadata, e.extracted_body,
    us.title AS feed_title,
    f.title AS catalog_title,
    f.site_url AS feed_site_url,
    f.icon_url AS feed_icon_url
FROM feed_entries e
JOIN feeds f ON f.feed_url = e.feed_url
JOIN user_subscriptions us ON us.feed_url = e.feed_url AND us.did = sqlc.arg(did)
WHERE e.feed_url = sqlc.arg(feed_url)
ORDER BY e.published_at DESC, e.id DESC
LIMIT sqlc.arg(limit);

-- name: ListAllEntriesForUser :many
SELECT
    e.id, e.feed_url, e.guid, e.entry_slug, e.url, e.title, e.content_html, e.content_type,
    e.published_at, e.fetched_at, e.metadata, e.extracted_body,
    us.title AS feed_title,
    f.title AS catalog_title,
    f.site_url AS feed_site_url,
    f.icon_url AS feed_icon_url
FROM feed_entries e
JOIN feeds f ON f.feed_url = e.feed_url
JOIN user_subscriptions us ON us.feed_url = e.feed_url
WHERE us.did = ?
ORDER BY e.published_at DESC, e.id DESC;
