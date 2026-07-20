-- +goose Up
-- +goose StatementBegin

-- Personal discovery share crawl cache (SPEC <social-layer> Follow Contract:
-- "their shares appear in the user's Library"). Keyed by the crawled repo's
-- own DID, not the viewer — shared across every viewer who follows that
-- person, same posture as discover_crawl_subscriptions.
CREATE TABLE discover_crawl_share_state (
    followed_did TEXT PRIMARY KEY,
    fetched_at   TEXT NOT NULL
);

-- One row per merged share/recommend found on a followed repo across the
-- three share-shaped collections. dedupe_key is the document AT-URI for
-- standardfeed (joins a recommend existence record to its lazy comment
-- sidecar) or the itemUrl for rss/skyreader — the same per-kind identity the
-- own-repo share reconcile uses.
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
DROP TABLE IF EXISTS discover_crawl_share_state;
-- +goose StatementEnd
