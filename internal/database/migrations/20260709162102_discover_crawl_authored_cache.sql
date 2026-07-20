-- +goose Up
-- +goose StatementBegin

-- Authored-publication discovery crawl cache (SPEC <discovery> Signal
-- ordering: "authors the publication" is the strongest per-source signal).
-- Keyed by the crawled repo's own DID, same posture as
-- discover_crawl_subscriptions and discover_crawl_shares.
CREATE TABLE discover_crawl_authored_state (
    followed_did TEXT PRIMARY KEY,
    fetched_at   TEXT NOT NULL
);

-- One row per site.standard.publication the followed DID owns in their own
-- repo. last_published_at is a best-effort recency proxy (the newest
-- site.standard.document found in the same repo) driving the standing-signal
-- recency lean ("actively publishing author outranks dormant"); NULL when it
-- can't be determined.
CREATE TABLE discover_crawl_authored (
    followed_did      TEXT NOT NULL,
    canonical_key      TEXT NOT NULL,
    kind                TEXT NOT NULL, -- always 'standardfeed' in v1
    title               TEXT,
    site_url            TEXT,
    last_published_at   TEXT,
    fetched_at          TEXT NOT NULL,
    verification        TEXT NOT NULL, -- always 'verified' today; column exists for a future retry policy
    PRIMARY KEY (followed_did, canonical_key)
);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS discover_crawl_authored;
DROP TABLE IF EXISTS discover_crawl_authored_state;
-- +goose StatementEnd
