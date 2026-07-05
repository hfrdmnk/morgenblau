-- name: ListUserSubscriptionsForSync :many
SELECT did, rkey, at_uri, feed_url, kind, sidecar_rkey, title
FROM user_subscriptions
WHERE did = ?;

-- name: ListFeedURLsForUser :many
SELECT feed_url FROM user_subscriptions WHERE did = ?;

-- name: ListAllFeedURLs :many
SELECT feed_url FROM feeds ORDER BY feed_url;
