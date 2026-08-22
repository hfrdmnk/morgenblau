-- name: GetDiscoverIngestCursor :one
SELECT seq, bootstrap_tip_seq, bootstrap_through_seq, updated_at
FROM discover_ingest_cursor
WHERE id = 1;

-- name: UpsertDiscoverIngestCursor :exec
-- Writes the whole position at once: advancing the live seq must clear the
-- bootstrap columns in the same statement, or a restart would re-enter a
-- backfill that already finished.
INSERT INTO discover_ingest_cursor (id, seq, bootstrap_tip_seq, bootstrap_through_seq, updated_at)
VALUES (1, ?, ?, ?, ?)
ON CONFLICT (id) DO UPDATE SET
    seq                   = excluded.seq,
    bootstrap_tip_seq     = excluded.bootstrap_tip_seq,
    bootstrap_through_seq = excluded.bootstrap_through_seq,
    updated_at            = excluded.updated_at;
