-- +goose Up
-- +goose StatementBegin

-- Own-repo foreign subscription crawl freshness is keyed by the viewer DID.
CREATE TABLE discover_crawl_own_foreign_state (
    did        TEXT PRIMARY KEY,
    fetched_at TEXT NOT NULL
);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS discover_crawl_own_foreign_state;
-- +goose StatementEnd
