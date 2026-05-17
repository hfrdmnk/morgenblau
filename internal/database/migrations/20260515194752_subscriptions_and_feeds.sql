-- +goose Up
-- +goose StatementBegin

-- Tier-2: shared catalog of upstream feeds, deduped by canonical URL across
-- all Morgenblau users. One row per feed_url; many user_subscriptions point
-- here. Local is canonical (no PDS path for entries).
CREATE TABLE feeds (
    feed_url         TEXT PRIMARY KEY,
    site_url         TEXT,
    etag             TEXT,
    last_modified    TEXT,
    last_fetched_at  TEXT,
    icon_url         TEXT,
    icon_fetched_at  TEXT,
    created_at       TEXT NOT NULL,
    updated_at       TEXT NOT NULL
);

-- Tier-1: per-user derived index of app.skyreader.feed.subscription records
-- on the user's PDS. Reconciled, not authoritative — PDS is canonical.
CREATE TABLE user_subscriptions (
    did              TEXT NOT NULL,
    rkey             TEXT NOT NULL,
    at_uri           TEXT NOT NULL,
    feed_url         TEXT NOT NULL,
    title            TEXT,
    custom_title     TEXT,
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
