-- name: ListDiscoverTrendingFollows :many
-- Whole-table read; kept for callers that genuinely need every row (see
-- internal/discoverbatch's write-path tests). The trending handler reads
-- ListDiscoverTrendingFollowsAboveBar instead (see that query's comment).
SELECT repo_did, subject_did, fetched_at
FROM discover_trending_follows;

-- name: ListDiscoverTrendingFollowsAboveBar :many
-- Bounds the trending-people read to subject_dids with followers from at
-- least min_distinct_repos distinct repos (SPEC <discovery> People
-- "Global/Trending": "same >=3-distinct-repos bar") instead of loading the
-- whole network table; scoring for the surviving candidates still happens
-- in Go via RankPeopleTrending, which keeps its own MinDistinctRepos check
-- as defense in depth. Written as a correlated WHERE subquery rather than
-- GROUP BY/HAVING because sqlc's SQLite parameter binder (v1.31.1) doesn't
-- detect placeholders inside a HAVING clause and silently drops them,
-- producing a param-less query that would panic at call time.
SELECT f.repo_did, f.subject_did, f.fetched_at
FROM discover_trending_follows f
WHERE (
    SELECT COUNT(DISTINCT f2.repo_did)
    FROM discover_trending_follows f2
    WHERE f2.subject_did = f.subject_did
) >= CAST(sqlc.arg(min_distinct_repos) AS BIGINT);

-- name: DeleteDiscoverTrendingFollowsForRepo :exec
DELETE FROM discover_trending_follows WHERE repo_did = ?;

-- name: InsertDiscoverTrendingFollow :exec
INSERT INTO discover_trending_follows (
    repo_did, subject_did, fetched_at
) VALUES (?, ?, ?);
