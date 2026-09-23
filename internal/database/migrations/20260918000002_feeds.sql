-- +goose Up
-- +goose StatementBegin
CREATE TABLE feeds (
    feed_url             TEXT PRIMARY KEY,
    kind                 TEXT NOT NULL DEFAULT 'rss',
    site_url             TEXT,
    title                TEXT,
    etag                 TEXT,
    last_modified        TEXT,
    last_fetched_at      TEXT,
    icon_url             TEXT,
    icon_fetched_at      TEXT,
    created_at           TEXT NOT NULL,
    updated_at           TEXT NOT NULL,
    consecutive_failures INTEGER NOT NULL DEFAULT 0,
    next_fetch_at        TEXT
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS feeds;
-- +goose StatementEnd
