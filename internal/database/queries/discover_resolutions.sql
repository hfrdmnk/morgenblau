-- name: GetDiscoverPublicationResolution :one
SELECT publication_uri, canonical_key, kind, title, site_url, icon_url, failure_count, fetched_at, next_retry_at
FROM discover_publication_resolutions WHERE publication_uri = ?;

-- name: GetDiscoverPublicationResolutionByCanonicalKey :one
SELECT publication_uri, canonical_key, kind, title, site_url, icon_url, failure_count, fetched_at, next_retry_at
FROM discover_publication_resolutions WHERE canonical_key = ? LIMIT 1;

-- name: UpsertDiscoverPublicationResolution :exec
INSERT INTO discover_publication_resolutions (
    publication_uri, canonical_key, kind, title, site_url, icon_url, failure_count, fetched_at, next_retry_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (publication_uri) DO UPDATE SET
    canonical_key = excluded.canonical_key,
    kind          = excluded.kind,
    title         = excluded.title,
    site_url      = excluded.site_url,
    icon_url      = excluded.icon_url,
    failure_count = excluded.failure_count,
    fetched_at    = excluded.fetched_at,
    next_retry_at = excluded.next_retry_at;
