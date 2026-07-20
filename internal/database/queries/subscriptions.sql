-- name: UpsertFeed :exec
-- kind defaults to 'rss' via NULLIF so pre-standardfeed callers passing the
-- zero value keep working; it is never changed on conflict. title is the
-- cached publication name; COALESCE keeps rss callers (nil) from clobbering it.
-- language is the pipeline's freshly-detected value (discoverlang); COALESCE
-- keeps an inconclusive detection on this fetch from erasing a previously
-- known language (SPEC <discovery>: detection runs on entry content already
-- fetched, no dedicated network call).
-- All params are named (not positional): mixing sqlc.arg() with bare ? makes
-- sqlc emit non-contiguous placeholder numbers that modernc.org/sqlite can't
-- bind ("missing argument with index N").
INSERT INTO feeds (feed_url, kind, site_url, title, language, created_at, updated_at)
VALUES (
    sqlc.arg(feed_url),
    COALESCE(NULLIF(sqlc.arg(kind), ''), 'rss'),
    sqlc.arg(site_url),
    sqlc.arg(title),
    sqlc.arg(language),
    sqlc.arg(created_at),
    sqlc.arg(updated_at)
)
ON CONFLICT (feed_url) DO UPDATE SET
    site_url = COALESCE(NULLIF(excluded.site_url, ''), feeds.site_url),
    title = COALESCE(excluded.title, feeds.title),
    language = COALESCE(excluded.language, feeds.language),
    updated_at = excluded.updated_at;

-- name: GetFeed :one
-- Column order matches the table's physical layout so sqlc reuses the Feed
-- model instead of minting a one-off row type (see
-- ListDiscoverCrawlSubscriptions for the same convention).
SELECT feed_url, kind, site_url, title, language, etag, last_modified, last_fetched_at, icon_url, icon_fetched_at, created_at, updated_at, consecutive_failures, next_fetch_at
FROM feeds WHERE feed_url = ?;

-- name: ListFeedLanguages :many
-- Discover trending's language-filter lookup (SPEC <discovery> "Global/
-- Trending ranking"): every Tier-2 source with a known detected language, in
-- one query rather than one per candidate. Tier-2 only holds feeds a
-- Morgenblau user actually subscribes to, so this table is small relative to
-- the network-wide trending aggregate.
SELECT feed_url, language FROM feeds WHERE language IS NOT NULL;

-- name: GetFeedIconURL :one
-- Returns the stored icon URL for a feed. Drives the favicon-proxy SSRF guard:
-- the proxy only streams URLs the sync pipeline has already vetted.
SELECT icon_url FROM feeds WHERE feed_url = ?;

-- name: SetFeedIconURL :exec
UPDATE feeds
SET icon_url = ?, icon_fetched_at = ?, updated_at = ?
WHERE feed_url = ?;

-- name: UpdateFeedFetchState :exec
-- SPEC <feed-sources> failure handling: success resets the backoff counter and clears the skip stamp.
UPDATE feeds
SET etag = ?, last_modified = ?, last_fetched_at = ?, updated_at = ?, consecutive_failures = 0, next_fetch_at = NULL
WHERE feed_url = ?;

-- name: UpdateFeedFetchFailure :exec
-- SPEC <feed-sources> failure handling; success (UpdateFeedFetchState) resets both.
UPDATE feeds
SET consecutive_failures = ?, next_fetch_at = ?, updated_at = ?
WHERE feed_url = ?;

-- name: UpsertUserSubscription :exec
-- Fully named params (see UpsertFeed): avoids the mixed named/positional
-- numbering that breaks binding under modernc.org/sqlite.
INSERT INTO user_subscriptions (
    did, rkey, at_uri, feed_url, kind, sidecar_rkey, title, is_primary, tags, created_at, updated_at
) VALUES (
    sqlc.arg(did),
    sqlc.arg(rkey),
    sqlc.arg(at_uri),
    sqlc.arg(feed_url),
    COALESCE(NULLIF(sqlc.arg(kind), ''), 'rss'),
    sqlc.arg(sidecar_rkey),
    sqlc.arg(title),
    sqlc.arg(is_primary),
    sqlc.arg(tags),
    sqlc.arg(created_at),
    sqlc.arg(updated_at)
)
ON CONFLICT (did, rkey) DO UPDATE SET
    at_uri       = excluded.at_uri,
    feed_url     = excluded.feed_url,
    sidecar_rkey = excluded.sidecar_rkey,
    title        = excluded.title,
    is_primary   = excluded.is_primary,
    tags         = excluded.tags,
    updated_at   = excluded.updated_at;

-- name: GetUserSubscription :one
SELECT did, rkey, at_uri, feed_url, kind, sidecar_rkey, title, is_primary, tags, created_at, updated_at
FROM user_subscriptions WHERE did = ? AND rkey = ?;

-- name: GetUserSubscriptionByFeedURL :one
SELECT did, rkey, at_uri, feed_url, kind, sidecar_rkey, title, is_primary, tags, created_at, updated_at
FROM user_subscriptions WHERE did = ? AND feed_url = ?;

-- name: ListUserSubscriptions :many
SELECT did, rkey, at_uri, feed_url, kind, sidecar_rkey, title, is_primary, tags, created_at, updated_at
FROM user_subscriptions WHERE did = ?
ORDER BY COALESCE(NULLIF(title, ''), feed_url) COLLATE NOCASE ASC;

-- name: ListUserSubscriptionTags :many
-- Ordered by rkey (a TID, so creation order) to make "first-seen casing wins"
-- deterministic across a tag case-collision.
SELECT tags FROM user_subscriptions WHERE did = ? AND tags IS NOT NULL AND tags != ''
ORDER BY rkey;

-- name: ListUserSourcesWithStats :many
-- One row per subscription with feed metadata and windowed entry stats. The
-- four window cutoffs (7d, 28d, 56d, 84d as ISO timestamps) and "now" are
-- passed in by the handler so all rows share a single clock.
SELECT
    us.did, us.rkey, us.at_uri, us.feed_url, us.kind, us.sidecar_rkey, us.title,
    us.is_primary, us.tags,
    us.created_at, us.updated_at,
    f.site_url, f.icon_url,
    f.title AS catalog_title,
    f.last_fetched_at,
    COALESCE(f.consecutive_failures, 0) AS consecutive_failures,
    COALESCE((SELECT MAX(published_at) FROM feed_entries fe WHERE fe.feed_url = us.feed_url), '') AS last_published_at,
    COALESCE((SELECT MIN(published_at) FROM feed_entries fe WHERE fe.feed_url = us.feed_url), '') AS first_published_at,
    (SELECT COUNT(*) FROM feed_entries fe WHERE fe.feed_url = us.feed_url AND fe.published_at >= sqlc.arg(cutoff_7d) AND fe.published_at < sqlc.arg(now)) AS count_7d,
    (SELECT COUNT(*) FROM feed_entries fe WHERE fe.feed_url = us.feed_url AND fe.published_at >= sqlc.arg(cutoff_28d) AND fe.published_at < sqlc.arg(now)) AS count_28d,
    (SELECT COUNT(*) FROM feed_entries fe WHERE fe.feed_url = us.feed_url AND fe.published_at >= sqlc.arg(cutoff_56d) AND fe.published_at < sqlc.arg(now)) AS count_56d,
    (SELECT COUNT(*) FROM feed_entries fe WHERE fe.feed_url = us.feed_url AND fe.published_at >= sqlc.arg(cutoff_84d) AND fe.published_at < sqlc.arg(now)) AS count_84d
FROM user_subscriptions us
LEFT JOIN feeds f ON f.feed_url = us.feed_url
WHERE us.did = sqlc.arg(did)
ORDER BY COALESCE(NULLIF(us.title, ''), f.title, us.feed_url) COLLATE NOCASE ASC;

-- name: GetUserSourceWithStats :one
-- Single-source counterpart to ListUserSourcesWithStats, keyed by (did, rkey).
-- Carries the same windowed counts plus total_entries and saved_by_you so the
-- source detail page can render its stat row in one query.
SELECT
    us.did, us.rkey, us.at_uri, us.feed_url, us.kind, us.sidecar_rkey, us.title,
    us.is_primary, us.tags,
    us.created_at, us.updated_at,
    f.site_url, f.icon_url,
    f.title AS catalog_title,
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

-- name: ListUserSubscriptionsWithSiteURL :many
-- Sibling-guard read: every subscription with its catalog site_url so the
-- resolve handler can flag candidates that point at the same site under the
-- other kind (rss vs standardfeed).
SELECT us.rkey, us.feed_url, us.kind, us.title, f.site_url, f.title AS catalog_title
FROM user_subscriptions us
LEFT JOIN feeds f ON f.feed_url = us.feed_url
WHERE us.did = ?;

-- name: DeleteUserSubscription :exec
DELETE FROM user_subscriptions WHERE did = ? AND rkey = ?;
