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
    language         TEXT, -- detected from entry content at fetch time (feed tag is a hint only); NULL = undetermined, trending filter passes it
    etag             TEXT,
    last_modified    TEXT,
    last_fetched_at  TEXT,
    icon_url         TEXT,
    icon_fetched_at  TEXT,
    created_at       TEXT NOT NULL,
    updated_at       TEXT NOT NULL,
    consecutive_failures INTEGER NOT NULL DEFAULT 0, -- SPEC <feed-sources> failure handling: exponential backoff + 20-failure mute
    next_fetch_at    TEXT -- RFC3339 do-not-fetch-before stamp; NULL = eligible
);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS feeds;
-- +goose StatementEnd
