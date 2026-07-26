-- +goose Up
-- +goose StatementBegin

-- One row per Skyreader or Glean subscription found on the user's own repo.
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
-- +goose StatementEnd
