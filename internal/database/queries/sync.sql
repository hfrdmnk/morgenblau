-- name: ListUserSubscriptionsForSync :many
SELECT did, rkey, at_uri, feed_url, kind, sidecar_rkey, title, is_primary, tags, created_at, updated_at
FROM user_subscriptions
WHERE did = ?;

-- name: ListFeedURLsForUser :many
SELECT feed_url FROM user_subscriptions WHERE did = ?;

-- name: ListFollowedFeedURLs :many
-- A feed nobody follows only feeds a cache nobody reads; the sweep's orphaned_at lags by up to a sweep, so it is not the test.
SELECT DISTINCT feed_url FROM user_subscriptions ORDER BY feed_url;
