-- name: ListDiscoverTrendingSignals :many
-- Whole-table read; kept for callers that genuinely need every row (see
-- internal/discoverbatch's write-path tests). The trending handler reads
-- ListDiscoverTrendingSignalsAboveBar instead (see that query's comment).
SELECT repo_did, source_key, kind, title, site_url, signal_kind, signal_at, fetched_at
FROM discover_trending_signals;

-- name: ListDiscoverTrendingSignalsAboveBar :many
-- Bounds the trending-sources read to source_keys with signals from at
-- least min_distinct_repos distinct repos (SPEC <discovery> "Quality bar")
-- instead of loading the whole network table; grouping/scoring for the
-- surviving candidates still happens in Go via RankTrending, which keeps
-- its own MinDistinctRepos check as defense in depth. Written as a
-- correlated WHERE subquery rather than GROUP BY/HAVING because sqlc's
-- SQLite parameter binder (v1.31.1) doesn't detect placeholders inside a
-- HAVING clause and silently drops them, producing a param-less query that
-- would panic at call time.
SELECT s.repo_did, s.source_key, s.kind, s.title, s.site_url, s.signal_kind, s.signal_at, s.fetched_at
FROM discover_trending_signals s
WHERE (
    SELECT COUNT(DISTINCT s2.repo_did)
    FROM discover_trending_signals s2
    WHERE s2.source_key = s.source_key
) >= CAST(sqlc.arg(min_distinct_repos) AS BIGINT);

-- name: ListDiscoverTrendingSignalsForEligibleSubjects :many
-- Bounds the People trending eligibility read (SPEC <discovery> People
-- "Eligibility") to repos that are themselves subject_dids clearing the
-- follower quality bar in discover_trending_follows, instead of loading the
-- whole signals table to test each candidate's reader-network presence.
-- Same correlated-subquery shape as ListDiscoverTrendingSignalsAboveBar,
-- for the same sqlc HAVING-placeholder reason.
-- signal_kind != 'save' is the save-privacy invariant: a save-only subject
-- must never clear eligibility (SPEC <discovery> People "Eligibility":
-- "Saves don't confer eligibility").
SELECT s.repo_did, s.source_key, s.kind, s.title, s.site_url, s.signal_kind, s.signal_at, s.fetched_at
FROM discover_trending_signals s
WHERE s.signal_kind != 'save'
AND (
    SELECT COUNT(DISTINCT f.repo_did)
    FROM discover_trending_follows f
    WHERE f.subject_did = s.repo_did
) >= CAST(sqlc.arg(min_distinct_repos) AS BIGINT);

-- name: GetDiscoverTrendingSignalTitle :one
-- Backfills a reaction-only rss candidate's title/siteUrl (SPEC <discovery>);
-- deliberately not gated by the min-3-repo trending bar used in
-- ListDiscoverTrendingSignalsAboveBar, since a title only needs one
-- contributing repo, unlike a trending card.
SELECT title, site_url FROM discover_trending_signals
WHERE source_key = ? AND title IS NOT NULL AND title != ''
LIMIT 1;

-- name: DeleteDiscoverTrendingSignalsForRepo :exec
DELETE FROM discover_trending_signals WHERE repo_did = ?;

-- name: InsertDiscoverTrendingSignal :exec
INSERT INTO discover_trending_signals (
    repo_did, source_key, kind, title, site_url, signal_kind, signal_at, fetched_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?);

-- name: GetDiscoverBatchState :one
SELECT id, last_run_at FROM discover_batch_state WHERE id = 1;

-- name: UpsertDiscoverBatchState :exec
INSERT INTO discover_batch_state (id, last_run_at) VALUES (1, ?)
ON CONFLICT (id) DO UPDATE SET last_run_at = excluded.last_run_at;
