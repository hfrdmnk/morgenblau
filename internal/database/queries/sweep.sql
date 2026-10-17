-- name: ClaimFollowedFeeds :exec
UPDATE feeds SET orphaned_at = NULL
WHERE orphaned_at IS NOT NULL
  AND EXISTS (SELECT 1 FROM user_subscriptions us WHERE us.feed_url = feeds.feed_url);

-- name: MarkOrphanedFeeds :exec
UPDATE feeds SET orphaned_at = sqlc.arg(now)
WHERE orphaned_at IS NULL
  AND NOT EXISTS (SELECT 1 FROM user_subscriptions us WHERE us.feed_url = feeds.feed_url);

-- name: DeleteOrphanedFeeds :execrows
-- A saved entry keeps its feed: the Library reads a save's title and reader link from the cached entry.
DELETE FROM feeds
WHERE orphaned_at <= sqlc.arg(cutoff)
  AND NOT EXISTS (SELECT 1 FROM user_subscriptions us WHERE us.feed_url = feeds.feed_url)
  AND NOT EXISTS (
      SELECT 1 FROM feed_entries fe JOIN user_saves s ON s.item_url = fe.url
      WHERE fe.feed_url = feeds.feed_url
  );

-- name: DeleteUnfollowedWebSources :execrows
-- Runs after DeleteOrphanedFeeds, so a source whose feed was swept goes in the same pass. A feed on any origin counts:
-- readers keep the URL they subscribed under when the server's public origin changes, and ids carry no GLOB metacharacters.
-- The paths are websource.FeedPaths: /feed/ and its legacy alias /web/.
DELETE FROM web_sources
WHERE offered_at <= sqlc.arg(cutoff)
  AND (served_at IS NULL OR served_at <= sqlc.arg(cutoff))
  AND NOT EXISTS (
      SELECT 1 FROM feeds f
      WHERE f.feed_url GLOB '*/feed/' || web_sources.id OR f.feed_url GLOB '*/web/' || web_sources.id
  );
