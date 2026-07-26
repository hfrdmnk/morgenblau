-- +goose Up
-- +goose StatementBegin

-- Reader-network follow crawl freshness, keyed by the crawled repo's DID.
CREATE TABLE discover_crawl_follow_state (
    followed_did TEXT PRIMARY KEY,
    fetched_at   TEXT NOT NULL
);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS discover_crawl_follow_state;
-- +goose StatementEnd
