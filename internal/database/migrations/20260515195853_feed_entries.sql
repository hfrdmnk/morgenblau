-- +goose Up
-- +goose StatementBegin

-- Tier-2 entries. One row per (feed_url, guid). Body is pre-sanitized HTML;
-- raw upstream HTML is never persisted. Per SPEC <content-types>, content type
-- and sanitization are decided at fetch time, not at render.
CREATE TABLE feed_entries (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    feed_url        TEXT NOT NULL,
    guid            TEXT NOT NULL,
    entry_slug      TEXT NOT NULL,
    url             TEXT NOT NULL,
    title           TEXT,
    content_html    TEXT,
    content_type    TEXT NOT NULL,
    published_at    TEXT NOT NULL,
    fetched_at      TEXT NOT NULL,
    metadata        TEXT,
    extracted_body  TEXT,
    record_cid      TEXT, -- site.standard.document CID for the sweep diff; NULL for rss
    UNIQUE (feed_url, guid),
    UNIQUE (entry_slug),
    FOREIGN KEY (feed_url) REFERENCES feeds(feed_url) ON DELETE CASCADE
);

CREATE INDEX feed_entries_published_at_idx
    ON feed_entries (published_at DESC);

CREATE INDEX feed_entries_feed_url_published_at_idx
    ON feed_entries (feed_url, published_at DESC);

-- Guid-only lookup: ListUserShares joins on guid = document, and reconcile
-- backfills a share's item_url from the cached entry by guid. The composite
-- UNIQUE(feed_url, guid) can't serve a guid-only probe (guid isn't leftmost).
CREATE INDEX feed_entries_guid_idx
    ON feed_entries (guid);

-- Tier-2 itemUrl fallback resolution (SPEC <discovery> Signal ordering: an
-- item reaction with no feedUrl/document provenance resolves via its cached
-- entry's own url). Without this index that lookup is a full table scan.
CREATE INDEX feed_entries_url_idx
    ON feed_entries (url);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS feed_entries_url_idx;
DROP INDEX IF EXISTS feed_entries_guid_idx;
DROP INDEX IF EXISTS feed_entries_feed_url_published_at_idx;
DROP INDEX IF EXISTS feed_entries_published_at_idx;
DROP TABLE IF EXISTS feed_entries;
-- +goose StatementEnd
