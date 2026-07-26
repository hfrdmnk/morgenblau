-- +goose Up
-- +goose StatementBegin

-- Adjacent-graph crawl freshness is keyed by the viewing user's own DID.
CREATE TABLE discover_crawl_adjacent_state (
    did        TEXT PRIMARY KEY,
    fetched_at TEXT NOT NULL
);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS discover_crawl_adjacent_state;
-- +goose StatementEnd
