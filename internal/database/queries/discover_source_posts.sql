-- name: GetDiscoverSourcePostsState :one
SELECT source_key, fetched_at, favicon_url, failure_count, next_retry_at, favicon_failure_count, favicon_next_retry_at FROM discover_source_posts_state WHERE source_key = ?;

-- name: ListDiscoverSourcePosts :many
SELECT source_key, position, title, published_at, url, post_key
FROM discover_source_posts WHERE source_key = ? ORDER BY position ASC;

-- name: DeleteDiscoverSourcePosts :exec
DELETE FROM discover_source_posts WHERE source_key = ?;

-- name: InsertDiscoverSourcePost :exec
INSERT INTO discover_source_posts (source_key, position, title, published_at, url, post_key)
VALUES (?, ?, ?, ?, ?, ?);

-- name: UpsertDiscoverSourcePostsState :exec
-- Every success resets the failure ladder: a recovered source deserves a fresh backoff clock next time it fails.
INSERT INTO discover_source_posts_state (source_key, fetched_at, favicon_url, failure_count, next_retry_at) VALUES (?, ?, ?, 0, NULL)
ON CONFLICT (source_key) DO UPDATE SET fetched_at = excluded.fetched_at, favicon_url = excluded.favicon_url, failure_count = 0, next_retry_at = NULL;

-- name: RecordDiscoverSourcePostsFailure :exec
-- Never touches fetched_at/favicon_url on conflict: a transient failure must not erase a prior success's payload (stale-while-error).
INSERT INTO discover_source_posts_state (source_key, fetched_at, favicon_url, failure_count, next_retry_at) VALUES (?, NULL, NULL, ?, ?)
ON CONFLICT (source_key) DO UPDATE SET failure_count = excluded.failure_count, next_retry_at = excluded.next_retry_at;

-- name: GetDiscoverSourceFaviconURL :one
-- Fallback lookup for the favicon proxy when a candidate isn't in the feeds table yet (not subscribed).
SELECT favicon_url FROM discover_source_posts_state WHERE source_key = ?;

-- name: UpsertDiscoverSourceFaviconURL :exec
-- On-demand discovery success; only touches the favicon columns, never the posts ladder (fetched_at/failure_count/next_retry_at).
INSERT INTO discover_source_posts_state (source_key, favicon_url, favicon_failure_count, favicon_next_retry_at)
VALUES (?, ?, 0, NULL)
ON CONFLICT (source_key) DO UPDATE SET favicon_url = excluded.favicon_url, favicon_failure_count = 0, favicon_next_retry_at = NULL;

-- name: RecordDiscoverSourceFaviconDiscoveryFailure :exec
-- Never touches favicon_url or the posts ladder: a discovery failure is orthogonal to both.
INSERT INTO discover_source_posts_state (source_key, favicon_failure_count, favicon_next_retry_at)
VALUES (?, ?, ?)
ON CONFLICT (source_key) DO UPDATE SET favicon_failure_count = excluded.favicon_failure_count, favicon_next_retry_at = excluded.favicon_next_retry_at;
