-- +goose Up
-- +goose StatementBegin

-- One row per canonical source key found on a followed repo across the four
-- reader-network subscription lexicons. canonical_key matches Tier-2's key.
CREATE TABLE discover_crawl_subscriptions (
    followed_did  TEXT NOT NULL,
    canonical_key TEXT NOT NULL,
    kind          TEXT NOT NULL, -- 'rss' | 'standardfeed'
    title         TEXT,
    site_url      TEXT,
    created_at    TEXT,          -- record's own createdAt for the standing-signal recency lean; NULL = unknown recency (no lean bonus)
    fetched_at    TEXT NOT NULL,
    PRIMARY KEY (followed_did, canonical_key)
);

-- canonical_key isn't leftmost in the primary key.
CREATE INDEX discover_crawl_subscriptions_key_idx
    ON discover_crawl_subscriptions (canonical_key);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS discover_crawl_subscriptions_key_idx;
DROP TABLE IF EXISTS discover_crawl_subscriptions;
-- +goose StatementEnd
