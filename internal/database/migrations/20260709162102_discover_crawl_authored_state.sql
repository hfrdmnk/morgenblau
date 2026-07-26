-- +goose Up
-- +goose StatementBegin

-- Authored-publication crawl freshness, keyed by the crawled repo's own DID.
CREATE TABLE discover_crawl_authored_state (
    followed_did TEXT PRIMARY KEY,
    fetched_at   TEXT NOT NULL
);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS discover_crawl_authored_state;
-- +goose StatementEnd
