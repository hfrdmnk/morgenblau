-- name: ListPublicFollowingSources :many
SELECT us.kind, us.feed_url, us.title, us.is_primary, us.tags, f.site_url, f.title AS catalog_title
FROM user_subscriptions us
LEFT JOIN feeds f ON f.feed_url = us.feed_url
WHERE us.did = ?
ORDER BY COALESCE(NULLIF(us.title, ''), f.title, us.feed_url) COLLATE NOCASE ASC;
