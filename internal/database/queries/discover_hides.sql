-- name: GetDiscoverHide :one
SELECT did, target_kind, target_key, hidden_until, hide_count, created_at, updated_at
FROM discover_hides WHERE did = ? AND target_kind = ? AND target_key = ?;

-- name: CountDiscoverHidesForUser :one
SELECT COUNT(*) FROM discover_hides WHERE did = ?;

-- name: UpsertDiscoverHide :exec
INSERT INTO discover_hides (did, target_kind, target_key, hidden_until, hide_count, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (did, target_kind, target_key) DO UPDATE SET
    hidden_until = excluded.hidden_until,
    hide_count   = excluded.hide_count,
    updated_at   = excluded.updated_at;

-- name: ListActiveDiscoverHides :many
-- Scoped by did in the query itself (never fetch-then-compare in handlers).
SELECT target_key
FROM discover_hides WHERE did = ? AND target_kind = ? AND hidden_until > ?;
