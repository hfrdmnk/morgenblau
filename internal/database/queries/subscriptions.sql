-- name: UpsertFeed :exec
INSERT INTO feeds (feed_url, site_url, created_at, updated_at)
VALUES (?, ?, ?, ?)
ON CONFLICT (feed_url) DO UPDATE SET
    site_url = COALESCE(NULLIF(excluded.site_url, ''), feeds.site_url),
    updated_at = excluded.updated_at;

-- name: GetFeed :one
SELECT feed_url, site_url, etag, last_modified, last_fetched_at, icon_url, icon_fetched_at, created_at, updated_at
FROM feeds WHERE feed_url = ?;

-- name: GetFeedIconURL :one
-- Returns the stored icon URL for a feed. Drives the favicon-proxy SSRF guard:
-- the proxy only streams URLs the sync pipeline has already vetted.
SELECT icon_url FROM feeds WHERE feed_url = ?;

-- name: SetFeedIconURL :exec
UPDATE feeds
SET icon_url = ?, icon_fetched_at = ?, updated_at = ?
WHERE feed_url = ?;

-- name: UpdateFeedFetchState :exec
UPDATE feeds
SET etag = ?, last_modified = ?, last_fetched_at = ?, updated_at = ?
WHERE feed_url = ?;

-- name: UpsertUserSubscription :exec
INSERT INTO user_subscriptions (
    did, rkey, at_uri, feed_url, title, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (did, rkey) DO UPDATE SET
    at_uri     = excluded.at_uri,
    feed_url   = excluded.feed_url,
    title      = excluded.title,
    updated_at = excluded.updated_at;

-- name: GetUserSubscription :one
SELECT did, rkey, at_uri, feed_url, title, created_at, updated_at
FROM user_subscriptions WHERE did = ? AND rkey = ?;

-- name: GetUserSubscriptionByFeedURL :one
SELECT did, rkey, at_uri, feed_url, title, created_at, updated_at
FROM user_subscriptions WHERE did = ? AND feed_url = ?;

-- name: ListUserSubscriptions :many
SELECT did, rkey, at_uri, feed_url, title, created_at, updated_at
FROM user_subscriptions WHERE did = ?
ORDER BY COALESCE(NULLIF(title, ''), feed_url) COLLATE NOCASE ASC;

-- name: ListUserSourcesWithStats :many
-- One row per subscription with feed metadata and windowed entry stats. The
-- four window cutoffs (7d, 28d, 56d, 84d as ISO timestamps) and "now" are
-- passed in by the handler so all rows share a single clock.
SELECT
    us.did, us.rkey, us.at_uri, us.feed_url, us.title,
    us.created_at, us.updated_at,
    f.site_url, f.icon_url,
    COALESCE((SELECT MAX(published_at) FROM feed_entries fe WHERE fe.feed_url = us.feed_url), '') AS last_published_at,
    COALESCE((SELECT MIN(published_at) FROM feed_entries fe WHERE fe.feed_url = us.feed_url), '') AS first_published_at,
    (SELECT COUNT(*) FROM feed_entries fe WHERE fe.feed_url = us.feed_url AND fe.published_at >= sqlc.arg(cutoff_7d) AND fe.published_at < sqlc.arg(now)) AS count_7d,
    (SELECT COUNT(*) FROM feed_entries fe WHERE fe.feed_url = us.feed_url AND fe.published_at >= sqlc.arg(cutoff_28d) AND fe.published_at < sqlc.arg(now)) AS count_28d,
    (SELECT COUNT(*) FROM feed_entries fe WHERE fe.feed_url = us.feed_url AND fe.published_at >= sqlc.arg(cutoff_56d) AND fe.published_at < sqlc.arg(now)) AS count_56d,
    (SELECT COUNT(*) FROM feed_entries fe WHERE fe.feed_url = us.feed_url AND fe.published_at >= sqlc.arg(cutoff_84d) AND fe.published_at < sqlc.arg(now)) AS count_84d
FROM user_subscriptions us
LEFT JOIN feeds f ON f.feed_url = us.feed_url
WHERE us.did = sqlc.arg(did)
ORDER BY COALESCE(NULLIF(us.title, ''), us.feed_url) COLLATE NOCASE ASC;

-- name: GetUserSourceWithStats :one
-- Single-source counterpart to ListUserSourcesWithStats, keyed by (did, rkey).
-- Carries the same windowed counts plus total_entries and saved_by_you so the
-- source detail page can render its stat row in one query.
SELECT
    us.did, us.rkey, us.at_uri, us.feed_url, us.title,
    us.created_at, us.updated_at,
    f.site_url, f.icon_url,
    COALESCE((SELECT MAX(published_at) FROM feed_entries fe WHERE fe.feed_url = us.feed_url), '') AS last_published_at,
    COALESCE((SELECT MIN(published_at) FROM feed_entries fe WHERE fe.feed_url = us.feed_url), '') AS first_published_at,
    (SELECT COUNT(*) FROM feed_entries fe WHERE fe.feed_url = us.feed_url AND fe.published_at >= sqlc.arg(cutoff_7d) AND fe.published_at < sqlc.arg(now)) AS count_7d,
    (SELECT COUNT(*) FROM feed_entries fe WHERE fe.feed_url = us.feed_url AND fe.published_at >= sqlc.arg(cutoff_28d) AND fe.published_at < sqlc.arg(now)) AS count_28d,
    (SELECT COUNT(*) FROM feed_entries fe WHERE fe.feed_url = us.feed_url AND fe.published_at >= sqlc.arg(cutoff_56d) AND fe.published_at < sqlc.arg(now)) AS count_56d,
    (SELECT COUNT(*) FROM feed_entries fe WHERE fe.feed_url = us.feed_url AND fe.published_at >= sqlc.arg(cutoff_84d) AND fe.published_at < sqlc.arg(now)) AS count_84d,
    (SELECT COUNT(*) FROM feed_entries fe WHERE fe.feed_url = us.feed_url) AS total_entries,
    (SELECT COUNT(*) FROM user_saves s WHERE s.did = us.did AND s.feed_url = us.feed_url) AS saved_by_you
FROM user_subscriptions us
LEFT JOIN feeds f ON f.feed_url = us.feed_url
WHERE us.did = sqlc.arg(did) AND us.rkey = sqlc.arg(rkey);

-- name: DeleteUserSubscription :exec
DELETE FROM user_subscriptions WHERE did = ? AND rkey = ?;
