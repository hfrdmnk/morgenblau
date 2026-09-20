-- +goose Up
-- +goose StatementBegin
CREATE TABLE oauth_sessions (
    did         TEXT NOT NULL,
    session_id  TEXT NOT NULL,
    -- Session material is AEAD-encrypted by the OAuth store before persistence.
    data        BLOB NOT NULL,
    updated_at  TEXT NOT NULL,
    PRIMARY KEY (did, session_id)
);

CREATE TABLE oauth_auth_requests (
    state       TEXT PRIMARY KEY,
    -- Request material is AEAD-encrypted by the OAuth store before persistence.
    data        BLOB NOT NULL,
    created_at  TEXT NOT NULL,
    expires_at  TEXT NOT NULL
);

CREATE INDEX oauth_auth_requests_expires_at_idx
    ON oauth_auth_requests (expires_at);

CREATE TABLE feeds (
    feed_url             TEXT PRIMARY KEY,
    kind                 TEXT NOT NULL DEFAULT 'rss',
    site_url             TEXT,
    title                TEXT,
    etag                 TEXT,
    last_modified        TEXT,
    last_fetched_at      TEXT,
    icon_url             TEXT,
    icon_fetched_at      TEXT,
    created_at           TEXT NOT NULL,
    updated_at           TEXT NOT NULL,
    consecutive_failures INTEGER NOT NULL DEFAULT 0,
    next_fetch_at        TEXT
);

CREATE TABLE user_subscriptions (
    did          TEXT NOT NULL,
    rkey         TEXT NOT NULL,
    at_uri       TEXT NOT NULL,
    feed_url     TEXT NOT NULL,
    kind         TEXT NOT NULL DEFAULT 'rss',
    sidecar_rkey TEXT,
    title        TEXT,
    is_primary   INTEGER NOT NULL DEFAULT 0,
    tags         TEXT,
    created_at   TEXT NOT NULL,
    updated_at   TEXT NOT NULL,
    PRIMARY KEY (did, rkey),
    FOREIGN KEY (feed_url) REFERENCES feeds(feed_url) ON DELETE RESTRICT
);

CREATE UNIQUE INDEX user_subscriptions_did_feed_url_idx
    ON user_subscriptions (did, feed_url);

CREATE INDEX user_subscriptions_feed_url_idx
    ON user_subscriptions (feed_url);

CREATE TABLE feed_entries (
    id             INTEGER PRIMARY KEY AUTOINCREMENT,
    feed_url       TEXT NOT NULL,
    guid           TEXT NOT NULL,
    entry_slug     TEXT NOT NULL,
    url            TEXT NOT NULL,
    title          TEXT,
    content_html   TEXT,
    content_type   TEXT NOT NULL,
    published_at   TEXT NOT NULL,
    fetched_at     TEXT NOT NULL,
    metadata       TEXT,
    extracted_body TEXT,
    record_cid     TEXT,
    UNIQUE (feed_url, guid),
    UNIQUE (entry_slug),
    FOREIGN KEY (feed_url) REFERENCES feeds(feed_url) ON DELETE CASCADE
);

CREATE INDEX feed_entries_published_at_idx
    ON feed_entries (published_at DESC);

CREATE INDEX feed_entries_feed_url_published_at_idx
    ON feed_entries (feed_url, published_at DESC);

CREATE INDEX feed_entries_url_idx
    ON feed_entries (url);

CREATE TABLE user_saves (
    did        TEXT NOT NULL,
    rkey       TEXT NOT NULL,
    at_uri     TEXT NOT NULL,
    item_url   TEXT NOT NULL,
    feed_url   TEXT,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    PRIMARY KEY (did, rkey)
);

CREATE UNIQUE INDEX user_saves_did_item_url_idx
    ON user_saves (did, item_url);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS user_saves;
DROP TABLE IF EXISTS feed_entries;
DROP TABLE IF EXISTS user_subscriptions;
DROP TABLE IF EXISTS feeds;
DROP TABLE IF EXISTS oauth_auth_requests;
DROP TABLE IF EXISTS oauth_sessions;
-- +goose StatementEnd
