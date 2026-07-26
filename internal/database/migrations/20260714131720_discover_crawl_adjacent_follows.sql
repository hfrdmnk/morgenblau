-- +goose Up
-- +goose StatementBegin

-- One row per Bluesky or Tangled follow found on the user's own repo.
CREATE TABLE discover_crawl_adjacent_follows (
    did         TEXT NOT NULL,
    subject_did TEXT NOT NULL,
    network     TEXT NOT NULL, -- 'bluesky' | 'tangled'
    fetched_at  TEXT NOT NULL,
    PRIMARY KEY (did, subject_did)
);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS discover_crawl_adjacent_follows;
-- +goose StatementEnd
