-- +goose Up
-- +goose StatementBegin

-- Local mirror of the reader-network repos we tap from the firehose. One row
-- per record, stored as the raw JSON the tap delivered so downstream readers
-- decode against the lexicon rather than a frozen column layout.
CREATE TABLE tap_records (
    did         TEXT NOT NULL,
    collection  TEXT NOT NULL,
    rkey        TEXT NOT NULL,
    cid         TEXT NOT NULL,
    record      TEXT NOT NULL, -- raw record JSON
    indexed_at  TEXT NOT NULL,
    PRIMARY KEY (did, collection, rkey)
);

-- Repos whose mirror changed since the last aggregate rebuild. marked_at is
-- compared on delete so a repo re-dirtied mid-rebuild keeps its row.
CREATE TABLE tap_dirty_repos (
    did       TEXT PRIMARY KEY,
    marked_at TEXT NOT NULL
);

-- Repos already backfilled from their PDS, so the seeder skips them on restart.
CREATE TABLE tap_seeder_state (
    did       TEXT PRIMARY KEY,
    seeded_at TEXT NOT NULL
);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS tap_seeder_state;
DROP TABLE IF EXISTS tap_dirty_repos;
DROP TABLE IF EXISTS tap_records;
-- +goose StatementEnd
