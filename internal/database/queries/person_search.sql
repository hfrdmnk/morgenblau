-- name: PersonSearchPresence :many
-- Reader-network presence for a batch of AppView typeahead DIDs (SPEC
-- <discovery> People "Search" + "Eligibility"). Returns only the DIDs that
-- have at least one non-save signal row; the caller folds the result into a
-- map[string]bool, so a DID absent from the result is absent from the network.
-- signal_kind != 'save' is the save-privacy invariant: a save-only person must
-- not badge, or the badge would leak that they save (SPEC <discovery> People
-- "Eligibility": "Saves don't confer eligibility").
--
-- Uses sqlc.slice for a single batched read. The repo has never used
-- sqlc.slice before; if `make sqlc` mishandles it, switch the caller to
-- PersonSearchPresenceOne below, called once per DID (typeahead is capped at
-- ~10 results, so the sequential cost is trivial).
SELECT DISTINCT repo_did FROM discover_trending_signals
WHERE repo_did IN (sqlc.slice('dids')) AND signal_kind != 'save';

-- name: PersonSearchPresenceOne :one
-- Per-DID fallback for PersonSearchPresence when sqlc.slice codegen misbehaves;
-- same save-free presence test, called sequentially over the <=10 hits.
SELECT EXISTS(
    SELECT 1 FROM discover_trending_signals
    WHERE repo_did = ? AND signal_kind != 'save'
) AS present;

-- name: PersonSearchTasteHints :many
-- Up to `limit` publication titles for a present person's search badge (SPEC
-- <discovery> People "Search"). Drawn from subscribe/author signal rows only:
-- never 'share', never 'save'. A taste hint previews what the person reads or
-- publishes, and save rows are invisible under save privacy. Distinct titles,
-- most-recent signal first with a title tiebreak so the pick is deterministic.
SELECT title FROM discover_trending_signals
WHERE repo_did = ?
  AND signal_kind IN ('subscribe', 'author')
  AND title IS NOT NULL AND title != ''
GROUP BY title
ORDER BY MAX(signal_at) DESC, title ASC
LIMIT ?;
