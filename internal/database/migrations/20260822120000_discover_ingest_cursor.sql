-- +goose Up
-- +goose StatementBegin

CREATE TABLE discover_ingest_cursor (
    id         INTEGER PRIMARY KEY CHECK (id = 1),
    seq        INTEGER NOT NULL,
    updated_at TEXT NOT NULL
);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS discover_ingest_cursor;
-- +goose StatementEnd
