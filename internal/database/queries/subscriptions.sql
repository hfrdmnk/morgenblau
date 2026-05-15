-- name: UpsertFeed :exec
INSERT INTO feeds (feed_url, title, site_url, created_at, updated_at)
VALUES (?, ?, ?, ?, ?)
ON CONFLICT (feed_url) DO UPDATE SET
    title    = COALESCE(NULLIF(excluded.title, ''), feeds.title),
    site_url = COALESCE(NULLIF(excluded.site_url, ''), feeds.site_url),
    updated_at = excluded.updated_at;

-- name: GetFeed :one
SELECT feed_url, title, site_url, etag, last_modified, last_fetched_at, created_at, updated_at
FROM feeds WHERE feed_url = ?;

-- name: UpdateFeedFetchState :exec
UPDATE feeds
SET etag = ?, last_modified = ?, last_fetched_at = ?, updated_at = ?
WHERE feed_url = ?;

-- name: UpsertUserSubscription :exec
INSERT INTO user_subscriptions (
    did, rkey, at_uri, feed_url, title, custom_title, custom_icon_url, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (did, rkey) DO UPDATE SET
    at_uri          = excluded.at_uri,
    feed_url        = excluded.feed_url,
    title           = excluded.title,
    custom_title    = excluded.custom_title,
    custom_icon_url = excluded.custom_icon_url,
    updated_at      = excluded.updated_at;

-- name: GetUserSubscription :one
SELECT did, rkey, at_uri, feed_url, title, custom_title, custom_icon_url, created_at, updated_at
FROM user_subscriptions WHERE did = ? AND rkey = ?;

-- name: GetUserSubscriptionByFeedURL :one
SELECT did, rkey, at_uri, feed_url, title, custom_title, custom_icon_url, created_at, updated_at
FROM user_subscriptions WHERE did = ? AND feed_url = ?;

-- name: ListUserSubscriptions :many
SELECT did, rkey, at_uri, feed_url, title, custom_title, custom_icon_url, created_at, updated_at
FROM user_subscriptions WHERE did = ?
ORDER BY COALESCE(NULLIF(custom_title, ''), title, feed_url) COLLATE NOCASE ASC;

-- name: DeleteUserSubscription :exec
DELETE FROM user_subscriptions WHERE did = ? AND rkey = ?;
