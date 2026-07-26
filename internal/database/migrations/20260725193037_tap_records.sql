-- +goose Up
-- +goose StatementBegin

-- Local mirror of reader-network records delivered by tap.
CREATE TABLE tap_records (
    did         TEXT NOT NULL,
    collection  TEXT NOT NULL,
    rkey        TEXT NOT NULL,
    cid         TEXT NOT NULL,
    record      TEXT NOT NULL, -- raw record JSON
    indexed_at  TEXT NOT NULL,
    PRIMARY KEY (did, collection, rkey)
);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS tap_records;
-- +goose StatementEnd
