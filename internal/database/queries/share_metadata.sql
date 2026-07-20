-- name: GetFeedEntryShareMetadataByDocument :one
SELECT title, entry_slug, url
FROM feed_entries
WHERE guid = ?
LIMIT 1;

-- name: GetFeedEntryShareMetadataByItemURL :one
SELECT title, entry_slug, url
FROM feed_entries
WHERE url = ?
LIMIT 1;

-- name: GetShareMetadataCache :one
SELECT target_key, title, target_url, fetched_at, failure_count, next_retry_at
FROM share_metadata_cache
WHERE target_key = ?;

-- name: UpsertShareMetadataSuccess :exec
INSERT INTO share_metadata_cache (
    target_key, title, target_url, fetched_at, failure_count, next_retry_at
) VALUES (?, ?, ?, ?, 0, NULL)
ON CONFLICT (target_key) DO UPDATE SET
    title = excluded.title,
    target_url = excluded.target_url,
    fetched_at = excluded.fetched_at,
    failure_count = 0,
    next_retry_at = NULL;

-- name: RecordShareMetadataFailure :exec
-- Preserve the last successful payload so a transient failure serves stale metadata.
INSERT INTO share_metadata_cache (
    target_key, title, target_url, fetched_at, failure_count, next_retry_at
) VALUES (?, NULL, NULL, NULL, ?, ?)
ON CONFLICT (target_key) DO UPDATE SET
    failure_count = excluded.failure_count,
    next_retry_at = excluded.next_retry_at;
