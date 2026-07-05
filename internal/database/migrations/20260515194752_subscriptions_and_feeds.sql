-- +goose Up
-- +goose StatementBegin

-- Tier-2: shared catalog of upstream sources, deduped by canonical key across
-- all Morgenblau users. One row per key; many user_subscriptions point here.
-- feed_url is the generic catalog key: canonical feed URL (kind 'rss') or
-- publication at-uri (kind 'standardfeed'). Local is canonical for entries.
CREATE TABLE feeds (
    feed_url         TEXT PRIMARY KEY,
    kind             TEXT NOT NULL DEFAULT 'rss', -- 'rss' | 'standardfeed'
    site_url         TEXT,
    title            TEXT, -- cached publication.name (standardfeed); NULL for rss
    etag             TEXT,
    last_modified    TEXT,
    last_fetched_at  TEXT,
    icon_url         TEXT,
    icon_fetched_at  TEXT,
    created_at       TEXT NOT NULL,
    updated_at       TEXT NOT NULL
);

-- Tier-1: per-user derived index of subscriptions. rkey holds the existence
-- record's rkey: blue.morgen.feed.subscription (rss) or
-- site.standard.graph.subscription (standardfeed). Reconciled, not
-- authoritative; PDS is canonical.
CREATE TABLE user_subscriptions (
    did              TEXT NOT NULL,
    rkey             TEXT NOT NULL,
    at_uri           TEXT NOT NULL,
    feed_url         TEXT NOT NULL,
    kind             TEXT NOT NULL DEFAULT 'rss', -- 'rss' | 'standardfeed'
    sidecar_rkey     TEXT, -- blue.morgen sidecar rkey when kind='standardfeed' and a metadata sidecar exists
    title            TEXT,
    is_primary       INTEGER NOT NULL DEFAULT 0, -- `primary` on the wire; renamed to dodge the SQL keyword
    tags             TEXT, -- JSON array string e.g. ["a","b"]; NULL when none
    created_at       TEXT NOT NULL,
    updated_at       TEXT NOT NULL,
    PRIMARY KEY (did, rkey),
    FOREIGN KEY (feed_url) REFERENCES feeds(feed_url) ON DELETE RESTRICT
);

CREATE UNIQUE INDEX user_subscriptions_did_feed_url_idx
    ON user_subscriptions (did, feed_url);

CREATE INDEX user_subscriptions_feed_url_idx
    ON user_subscriptions (feed_url);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS user_subscriptions_feed_url_idx;
DROP INDEX IF EXISTS user_subscriptions_did_feed_url_idx;
DROP TABLE IF EXISTS user_subscriptions;
DROP TABLE IF EXISTS feeds;
-- +goose StatementEnd
