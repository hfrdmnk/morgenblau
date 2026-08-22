-- name: ListDiscoverTrendingSignals :many
-- Whole-table read; kept for callers that genuinely need every row (see
-- internal/discoveringest's write-path tests). The trending handler reads
-- ListDiscoverTrendingSignalsAboveBar instead (see that query's comment).
SELECT repo_did, source_key, kind, title, site_url, signal_kind, signal_at, fetched_at
FROM discover_trending_signals;

-- name: ListDiscoverTrendingSignalsAboveBar :many
-- Bounds the trending-sources read to source_keys with signals from at
-- least min_distinct_repos distinct repos (SPEC <discovery> "Quality bar")
-- instead of loading the whole network table; grouping/scoring for the
-- surviving candidates still happens in Go via RankTrending, which keeps
-- its own MinDistinctRepos check as defense in depth. The bar reads the
-- counts table the batch rebuilds, so it costs a keyed lookup per row
-- rather than a correlated COUNT(DISTINCT) over the whole signals table.
SELECT s.repo_did, s.source_key, s.kind, s.title, s.site_url, s.signal_kind, s.signal_at, s.fetched_at
FROM discover_trending_signals s
JOIN discover_trending_source_counts c ON c.source_key = s.source_key
WHERE c.distinct_repos >= CAST(sqlc.arg(min_distinct_repos) AS BIGINT);

-- name: ListDiscoverTrendingSignalsForEligibleSubjects :many
-- Bounds the People trending eligibility read (SPEC <discovery> People
-- "Eligibility") to repos that are themselves subject_dids clearing the
-- follower quality bar, instead of loading the whole signals table to test
-- each candidate's reader-network presence. Same counts-table join as
-- ListDiscoverTrendingSignalsAboveBar, against the follower counts.
-- signal_kind != 'save' is the save-privacy invariant: a save-only subject
-- must never clear eligibility (SPEC <discovery> People "Eligibility":
-- "Saves don't confer eligibility").
SELECT s.repo_did, s.source_key, s.kind, s.title, s.site_url, s.signal_kind, s.signal_at, s.fetched_at
FROM discover_trending_signals s
JOIN discover_trending_follow_counts c ON c.subject_did = s.repo_did
WHERE s.signal_kind != 'save'
AND c.distinct_repos >= CAST(sqlc.arg(min_distinct_repos) AS BIGINT);

-- name: GetDiscoverTrendingSignalTitle :one
-- Backfills a reaction-only rss candidate's title/siteUrl (SPEC <discovery>);
-- deliberately not gated by the min-3-repo trending bar used in
-- ListDiscoverTrendingSignalsAboveBar, since a title only needs one
-- contributing repo, unlike a trending card.
SELECT title, site_url FROM discover_trending_signals
WHERE source_key = ? AND title IS NOT NULL AND title != ''
LIMIT 1;

-- name: ListDiscoverTrendingSignalTitles :many
-- Batched form of GetDiscoverTrendingSignalTitle for backfilling a whole page
-- of reaction-only rss candidates. Returns every titled row for the requested
-- keys; the caller keeps the first row per source_key, so ordering only has to
-- group the keys together. Same deliberate absence of the min-repo trending
-- bar: a title needs one contributing repo, unlike a trending card.
SELECT source_key, title, site_url FROM discover_trending_signals
WHERE source_key IN (sqlc.slice('source_keys'))
  AND title IS NOT NULL AND title != ''
ORDER BY source_key;

-- name: DeleteDiscoverTrendingSourceCounts :exec
DELETE FROM discover_trending_source_counts;

-- name: RebuildDiscoverTrendingSourceCounts :exec
-- Paired with DeleteDiscoverTrendingSourceCounts inside one transaction: the
-- aggregation ListDiscoverTrendingSignalsAboveBar used to run per read, hoisted
-- to once per batch pass.
INSERT INTO discover_trending_source_counts (source_key, distinct_repos)
SELECT source_key, COUNT(DISTINCT repo_did)
FROM discover_trending_signals
GROUP BY source_key;

-- name: DeleteDiscoverTrendingSignalsForRepo :exec
DELETE FROM discover_trending_signals WHERE repo_did = ?;

-- name: InsertDiscoverTrendingSignal :exec
INSERT INTO discover_trending_signals (
    repo_did, source_key, kind, title, site_url, signal_kind, signal_at, fetched_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?);

