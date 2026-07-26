-- +goose Up
-- +goose StatementBegin

-- One row per reader-network follow found on a followed repo.
CREATE TABLE discover_crawl_follows (
    followed_did TEXT NOT NULL,
    subject_did  TEXT NOT NULL,
    fetched_at   TEXT NOT NULL,
    PRIMARY KEY (followed_did, subject_did)
);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS discover_crawl_follows;
-- +goose StatementEnd
