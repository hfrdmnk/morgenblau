-- +goose Up
-- +goose StatementBegin

-- One row per merged share/recommend found on a followed repo. dedupe_key is
-- the document AT-URI for standardfeed or itemUrl for rss/skyreader.
CREATE TABLE discover_crawl_shares (
    followed_did TEXT NOT NULL,
    dedupe_key   TEXT NOT NULL,
    kind         TEXT NOT NULL, -- 'rss' | 'standardfeed' | 'skyreader'
    item_url     TEXT,
    document     TEXT,
    feed_url     TEXT,
    comment      TEXT,
    created_at   TEXT NOT NULL, -- record's own createdAt, for newest-first ordering
    fetched_at   TEXT NOT NULL,
    PRIMARY KEY (followed_did, dedupe_key)
);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS discover_crawl_shares;
-- +goose StatementEnd
