-- +goose Up
-- +goose StatementBegin

-- Personal discovery crawl cache (SPEC <discovery> "Personal" acquisition):
-- on-demand listRecords crawls of followed people's repos, TTL'd. Keyed by
-- the crawled repo's own DID, not the viewer.
CREATE TABLE discover_crawl_state (
    followed_did TEXT PRIMARY KEY,
    fetched_at   TEXT NOT NULL
);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS discover_crawl_state;
-- +goose StatementEnd
