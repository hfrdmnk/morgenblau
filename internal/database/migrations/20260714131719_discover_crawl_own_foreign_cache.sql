-- +goose Up
-- +goose StatementBegin

-- Own-repo foreign (Skyreader/Glean) subscription crawl cache (SPEC
-- <discovery> self trust tier). Keyed by the viewing user's own DID, same
-- posture as discover_crawl_adjacent_*: crawls the viewer's own repo, so a
-- long shared-TTL cache buys nothing.
CREATE TABLE discover_crawl_own_foreign_state (
    did        TEXT PRIMARY KEY,
    fetched_at TEXT NOT NULL
);

-- One row per foreign subscription found on the user's own repo.
CREATE TABLE discover_crawl_own_foreign_subscriptions (
    did           TEXT NOT NULL,
    canonical_key TEXT NOT NULL,
    kind          TEXT NOT NULL, -- 'rss' | 'standardfeed'
    app           TEXT NOT NULL, -- 'skyreader' | 'glean'
    title         TEXT,
    site_url      TEXT,
    created_at    TEXT,
    fetched_at    TEXT NOT NULL,
    PRIMARY KEY (did, canonical_key)
);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS discover_crawl_own_foreign_subscriptions;
DROP TABLE IF EXISTS discover_crawl_own_foreign_state;
-- +goose StatementEnd
