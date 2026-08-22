-- +goose Up
-- +goose StatementBegin

-- One-row Jetstream stream position. seq is the live-tail resume cursor; the
-- bootstrap columns are non-null only while the Replay backfill is still
-- running, so a restart can tell "never ingested" from "mid-bootstrap".
CREATE TABLE discover_ingest_cursor (
    id                    INTEGER PRIMARY KEY CHECK (id = 1),
    seq                   INTEGER NOT NULL,
    bootstrap_tip_seq     INTEGER,
    bootstrap_through_seq INTEGER,
    updated_at            TEXT NOT NULL
);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS discover_ingest_cursor;
-- +goose StatementEnd
