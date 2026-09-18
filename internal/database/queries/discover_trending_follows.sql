-- name: ListDiscoverTrendingFollows :many
-- Whole-table read; kept for callers that genuinely need every row (see
-- internal/discoveringest's write-path tests). The trending handler reads
-- ListDiscoverTrendingFollowsAboveBar instead (see that query's comment).
SELECT repo_did, subject_did, fetched_at
FROM discover_trending_follows;

-- name: ListDiscoverTrendingFollowsAboveBar :many
-- Bounds the trending-people read to subject_dids with followers from at
-- least min_distinct_repos distinct repos (SPEC <discovery> People
-- "Global/Trending": "same >=3-distinct-repos bar") instead of loading the
-- whole network table; scoring for the surviving candidates still happens
-- in Go via RankPeopleTrending, which keeps its own MinDistinctRepos check
-- as defense in depth. The bar reads the counts table the batch rebuilds,
-- so it costs a keyed lookup per row rather than a correlated
-- COUNT(DISTINCT) over the whole follows table.
SELECT f.repo_did, f.subject_did, f.fetched_at
FROM discover_trending_follows f
JOIN discover_trending_follow_counts c ON c.subject_did = f.subject_did
WHERE c.distinct_repos >= CAST(sqlc.arg(min_distinct_repos) AS BIGINT);

-- name: DeleteDiscoverTrendingFollowCounts :exec
DELETE FROM discover_trending_follow_counts;

-- name: RebuildDiscoverTrendingFollowCounts :exec
-- Paired with DeleteDiscoverTrendingFollowCounts inside one transaction: the
-- aggregation ListDiscoverTrendingFollowsAboveBar used to run per read, hoisted
-- to once per batch pass.
INSERT INTO discover_trending_follow_counts (subject_did, distinct_repos)
SELECT subject_did, COUNT(DISTINCT repo_did)
FROM discover_trending_follows
GROUP BY subject_did;

-- name: DeleteDiscoverTrendingFollowsForRepo :exec
DELETE FROM discover_trending_follows WHERE repo_did = ?;

-- name: InsertDiscoverTrendingFollow :exec
INSERT INTO discover_trending_follows (
    repo_did, subject_did, fetched_at
) VALUES (?, ?, ?);
