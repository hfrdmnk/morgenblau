-- name: CreateWebSource :execrows
INSERT INTO web_sources (
    id, listing_key, listing_url, site_url, title, recipe, health, health_reason,
    needs_anchor, setup_cost, created_by_did, created_at, updated_at, setup_at, next_poll_at
) VALUES (
    sqlc.arg(id), sqlc.arg(listing_key), sqlc.arg(listing_url), sqlc.arg(site_url), sqlc.arg(title),
    sqlc.arg(recipe), sqlc.arg(health), sqlc.arg(health_reason), sqlc.arg(needs_anchor),
    sqlc.arg(setup_cost), sqlc.arg(created_by_did), sqlc.arg(created_at), sqlc.arg(created_at), sqlc.arg(created_at),
    sqlc.narg(next_poll_at)
)
ON CONFLICT (listing_key) DO NOTHING;

-- name: GetWebSource :one
SELECT * FROM web_sources WHERE id = ?;

-- name: GetWebSourceByListingKey :one
SELECT * FROM web_sources WHERE listing_key = ?;

-- name: ReplaceWebSourceSetup :exec
UPDATE web_sources SET
    listing_url = sqlc.arg(listing_url),
    site_url = sqlc.arg(site_url),
    title = sqlc.arg(title),
    recipe = sqlc.arg(recipe),
    health = sqlc.arg(health),
    health_reason = sqlc.arg(health_reason),
    needs_anchor = sqlc.arg(needs_anchor),
    setup_cost = sqlc.arg(setup_cost),
    updated_at = sqlc.arg(updated_at),
    setup_at = sqlc.arg(updated_at),
    next_poll_at = sqlc.narg(next_poll_at),
    last_polled_at = NULL,
    etag = NULL,
    last_modified = NULL,
    link_set_hash = NULL,
    consecutive_failures = 0
WHERE id = sqlc.arg(id);

-- name: UpdateWebSourcePoll :exec
UPDATE web_sources SET
    recipe = sqlc.arg(recipe),
    health = sqlc.arg(health),
    health_reason = sqlc.arg(health_reason),
    last_polled_at = sqlc.arg(last_polled_at),
    next_poll_at = sqlc.arg(next_poll_at),
    etag = sqlc.narg(etag),
    last_modified = sqlc.narg(last_modified),
    link_set_hash = sqlc.narg(link_set_hash),
    consecutive_failures = sqlc.arg(consecutive_failures),
    updated_at = sqlc.arg(updated_at)
WHERE id = sqlc.arg(id);

-- name: ListDueWebSources :many
-- A web source counts as followed once its feed URL is in the shared feeds catalog, the same set the global refresher fetches.
SELECT ws.* FROM web_sources ws
WHERE ws.next_poll_at IS NOT NULL
  AND ws.next_poll_at <= sqlc.arg(now)
  AND EXISTS (SELECT 1 FROM feeds f WHERE f.feed_url = sqlc.arg(feed_url_prefix) || ws.id)
ORDER BY ws.next_poll_at, ws.id
LIMIT sqlc.arg(limit);

-- name: DeleteWebSourceItems :exec
DELETE FROM web_source_items WHERE source_id = ?;

-- name: InsertWebSourceItem :exec
INSERT INTO web_source_items (source_id, url, title, published_at, excerpt, first_seen_at, history, position)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (source_id, url) DO NOTHING;

-- name: ListWebSourceItemURLs :many
SELECT url FROM web_source_items WHERE source_id = ?;

-- name: GetOldestWebSourceItemDate :one
SELECT published_at FROM web_source_items
WHERE source_id = ? AND published_at IS NOT NULL
ORDER BY published_at ASC
LIMIT 1;

-- name: CountWebSourceItems :one
SELECT COUNT(*) FROM web_source_items WHERE source_id = ?;

-- name: ListWebSourceFeedItems :many
-- Newest first: an item's own date, else the time a later poll first saw it; undated history keeps page order at the end.
SELECT url, title, published_at, excerpt, first_seen_at, history, position
FROM web_source_items
WHERE source_id = sqlc.arg(source_id)
ORDER BY COALESCE(published_at, CASE WHEN history = 0 THEN first_seen_at END) IS NULL,
         COALESCE(published_at, CASE WHEN history = 0 THEN first_seen_at END) DESC,
         first_seen_at DESC,
         position ASC
LIMIT sqlc.arg(limit);

-- name: DeleteWebSourcePolls :exec
DELETE FROM web_source_polls WHERE source_id = ?;

-- name: InsertWebSourcePoll :exec
INSERT INTO web_source_polls (source_id, polled_at, item_count, urls, titles, url_template, date_frac, title_shape)
VALUES (?, ?, ?, ?, ?, ?, ?, ?);

-- name: ListWebSourcePolls :many
SELECT * FROM web_source_polls
WHERE source_id = sqlc.arg(source_id)
ORDER BY id DESC
LIMIT sqlc.arg(limit);

-- name: PruneWebSourcePolls :exec
DELETE FROM web_source_polls
WHERE web_source_polls.source_id = sqlc.arg(source_id)
  AND web_source_polls.id NOT IN (
    SELECT kept.id FROM web_source_polls kept WHERE kept.source_id = sqlc.arg(source_id) ORDER BY kept.id DESC LIMIT sqlc.arg(keep)
  );
