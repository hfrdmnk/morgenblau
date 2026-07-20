-- name: GetDiscoverCrawlState :one
SELECT followed_did, fetched_at FROM discover_crawl_state WHERE followed_did = ?;

-- name: UpsertDiscoverCrawlState :exec
INSERT INTO discover_crawl_state (followed_did, fetched_at) VALUES (?, ?)
ON CONFLICT (followed_did) DO UPDATE SET fetched_at = excluded.fetched_at;

-- name: ListDiscoverCrawlSubscriptions :many
-- Column order matches the table's physical layout so sqlc reuses the
-- DiscoverCrawlSubscription model instead of minting a one-off row type.
SELECT followed_did, canonical_key, kind, title, site_url, created_at, fetched_at
FROM discover_crawl_subscriptions WHERE followed_did = ?;

-- name: DeleteDiscoverCrawlSubscriptions :exec
DELETE FROM discover_crawl_subscriptions WHERE followed_did = ?;

-- name: InsertDiscoverCrawlSubscription :exec
INSERT INTO discover_crawl_subscriptions (
    followed_did, canonical_key, kind, title, site_url, fetched_at, created_at
) VALUES (?, ?, ?, ?, ?, ?, ?);

-- name: GetDiscoverCrawlShareState :one
SELECT followed_did, fetched_at FROM discover_crawl_share_state WHERE followed_did = ?;

-- name: UpsertDiscoverCrawlShareState :exec
INSERT INTO discover_crawl_share_state (followed_did, fetched_at) VALUES (?, ?)
ON CONFLICT (followed_did) DO UPDATE SET fetched_at = excluded.fetched_at;

-- name: ListDiscoverCrawlShares :many
SELECT followed_did, dedupe_key, kind, item_url, document, feed_url, comment, created_at, fetched_at
FROM discover_crawl_shares WHERE followed_did = ?;

-- name: DeleteDiscoverCrawlShares :exec
DELETE FROM discover_crawl_shares WHERE followed_did = ?;

-- name: InsertDiscoverCrawlShare :exec
INSERT INTO discover_crawl_shares (
    followed_did, dedupe_key, kind, item_url, document, feed_url, comment, created_at, fetched_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: GetDiscoverCrawlAuthoredState :one
SELECT followed_did, fetched_at FROM discover_crawl_authored_state WHERE followed_did = ?;

-- name: UpsertDiscoverCrawlAuthoredState :exec
INSERT INTO discover_crawl_authored_state (followed_did, fetched_at) VALUES (?, ?)
ON CONFLICT (followed_did) DO UPDATE SET fetched_at = excluded.fetched_at;

-- name: ListDiscoverCrawlAuthored :many
-- Returns every cached outcome, verified and not; callers filter to verified-only in Go (authored_store.go), keeping this query free to surface all outcomes for a future retry policy.
SELECT followed_did, canonical_key, kind, title, site_url, last_published_at, fetched_at, verification
FROM discover_crawl_authored WHERE followed_did = ?;

-- name: DeleteDiscoverCrawlAuthored :exec
DELETE FROM discover_crawl_authored WHERE followed_did = ?;

-- name: InsertDiscoverCrawlAuthored :exec
INSERT INTO discover_crawl_authored (
    followed_did, canonical_key, kind, title, site_url, last_published_at, fetched_at, verification
) VALUES (?, ?, ?, ?, ?, ?, ?, ?);

-- name: GetDiscoverCrawlFollowState :one
SELECT followed_did, fetched_at FROM discover_crawl_follow_state WHERE followed_did = ?;

-- name: UpsertDiscoverCrawlFollowState :exec
INSERT INTO discover_crawl_follow_state (followed_did, fetched_at) VALUES (?, ?)
ON CONFLICT (followed_did) DO UPDATE SET fetched_at = excluded.fetched_at;

-- name: ListDiscoverCrawlFollows :many
SELECT followed_did, subject_did, fetched_at
FROM discover_crawl_follows WHERE followed_did = ?;

-- name: DeleteDiscoverCrawlFollows :exec
DELETE FROM discover_crawl_follows WHERE followed_did = ?;

-- name: InsertDiscoverCrawlFollow :exec
INSERT INTO discover_crawl_follows (followed_did, subject_did, fetched_at) VALUES (?, ?, ?);

-- name: GetDiscoverCrawlAdjacentState :one
SELECT did, fetched_at FROM discover_crawl_adjacent_state WHERE did = ?;

-- name: UpsertDiscoverCrawlAdjacentState :exec
INSERT INTO discover_crawl_adjacent_state (did, fetched_at) VALUES (?, ?)
ON CONFLICT (did) DO UPDATE SET fetched_at = excluded.fetched_at;

-- name: ListDiscoverCrawlAdjacentFollows :many
SELECT did, subject_did, network, fetched_at
FROM discover_crawl_adjacent_follows WHERE did = ?;

-- name: DeleteDiscoverCrawlAdjacentFollows :exec
DELETE FROM discover_crawl_adjacent_follows WHERE did = ?;

-- name: InsertDiscoverCrawlAdjacentFollow :exec
INSERT INTO discover_crawl_adjacent_follows (did, subject_did, network, fetched_at) VALUES (?, ?, ?, ?);

-- name: GetDiscoverCrawlOwnForeignState :one
SELECT did, fetched_at FROM discover_crawl_own_foreign_state WHERE did = ?;

-- name: UpsertDiscoverCrawlOwnForeignState :exec
INSERT INTO discover_crawl_own_foreign_state (did, fetched_at) VALUES (?, ?)
ON CONFLICT (did) DO UPDATE SET fetched_at = excluded.fetched_at;

-- name: ListDiscoverCrawlOwnForeignSubscriptions :many
-- Column order matches the table's physical layout so sqlc reuses the
-- DiscoverCrawlOwnForeignSubscription model instead of minting a one-off row type.
SELECT did, canonical_key, kind, app, title, site_url, created_at, fetched_at
FROM discover_crawl_own_foreign_subscriptions WHERE did = ?;

-- name: DeleteDiscoverCrawlOwnForeignSubscriptions :exec
DELETE FROM discover_crawl_own_foreign_subscriptions WHERE did = ?;

-- name: InsertDiscoverCrawlOwnForeignSubscription :exec
INSERT INTO discover_crawl_own_foreign_subscriptions (
    did, canonical_key, kind, app, title, site_url, created_at, fetched_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?);

-- name: GetDiscoverCrawlSubscriptionSiteURLByKey :one
-- Site-URL fallback for on-demand favicon discovery when the resolution cache misses.
SELECT site_url FROM discover_crawl_subscriptions WHERE canonical_key = ? AND site_url IS NOT NULL AND site_url != '' LIMIT 1;
