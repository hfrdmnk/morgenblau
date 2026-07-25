-- +goose Up
-- +goose StatementBegin

-- Personal discovery crawl cache (SPEC <discovery> "Personal" acquisition):
-- on-demand listRecords crawls of followed people's repos, TTL'd. Keyed by
-- the crawled repo's own DID, not the viewer — shared across every viewer who
-- follows that person, same posture as the Tier-2 feeds catalog.
CREATE TABLE discover_crawl_state (
    followed_did TEXT PRIMARY KEY,
    fetched_at   TEXT NOT NULL
);

-- One row per canonical source key found on a followed repo across the four
-- reader-network subscription lexicons. canonical_key is the feed URL (rss)
-- or the DID-normalized publication at-uri (standardfeed) — the same keys
-- Tier-2 dedupes on, so cross-reader variants of one source collapse here.
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

-- Key-only probes (favicon site-url fallback, source lookups) can't use the
-- composite PK: canonical_key isn't leftmost, so they'd full-scan the cache.
CREATE INDEX discover_crawl_subscriptions_key_idx
    ON discover_crawl_subscriptions (canonical_key);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS discover_crawl_subscriptions_key_idx;
DROP TABLE IF EXISTS discover_crawl_subscriptions;
DROP TABLE IF EXISTS discover_crawl_state;
-- +goose StatementEnd
