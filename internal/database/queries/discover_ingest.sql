-- name: GetDiscoverIngestCursor :one
SELECT seq, updated_at
FROM discover_ingest_cursor
WHERE id = 1;

-- name: UpsertDiscoverIngestCursor :exec
INSERT INTO discover_ingest_cursor (id, seq, updated_at)
VALUES (1, ?, ?)
ON CONFLICT (id) DO UPDATE SET
    seq        = excluded.seq,
    updated_at = excluded.updated_at;
