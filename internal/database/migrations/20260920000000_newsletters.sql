-- +goose Up
-- +goose StatementBegin
CREATE TABLE newsletter_addresses (
    did        TEXT PRIMARY KEY,
    local_part TEXT NOT NULL UNIQUE,
    created_at TEXT NOT NULL
);

CREATE TABLE newsletter_sources (
    id               TEXT PRIMARY KEY,
    did              TEXT NOT NULL,
    source_key       TEXT NOT NULL,
    identity_kind    TEXT NOT NULL CHECK (identity_kind IN ('list_id', 'from', 'manual')),
    identity_value   TEXT,
    title            TEXT NOT NULL,
    sender_name      TEXT,
    sender_address   TEXT NOT NULL DEFAULT '',
    status           TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'stopped')),
    accept_after     TEXT,
    is_primary       INTEGER NOT NULL DEFAULT 0 CHECK (is_primary IN (0, 1)),
    tags             TEXT NOT NULL DEFAULT '[]',
    first_received_at TEXT,
    last_received_at TEXT,
    created_at       TEXT NOT NULL,
    updated_at       TEXT NOT NULL,
    UNIQUE (did, source_key)
);

CREATE INDEX newsletter_sources_did_status_title_idx
    ON newsletter_sources (did, status, title COLLATE NOCASE);

CREATE TABLE newsletter_receipts (
    id             TEXT PRIMARY KEY,
    did            TEXT NOT NULL,
    envelope_from  TEXT NOT NULL,
    recipient      TEXT NOT NULL,
    received_at    TEXT NOT NULL,
    raw_mime       BLOB NOT NULL,
    reserved_bytes INTEGER NOT NULL CHECK (reserved_bytes >= 0),
    attempts       INTEGER NOT NULL DEFAULT 0,
    last_error     TEXT,
    next_attempt_at TEXT,
    created_at     TEXT NOT NULL
);

CREATE INDEX newsletter_receipts_ready_idx
    ON newsletter_receipts (next_attempt_at, created_at);

CREATE TABLE newsletter_messages (
    id                        TEXT PRIMARY KEY,
    did                       TEXT NOT NULL,
    source_id                 TEXT NOT NULL,
    entry_slug                TEXT NOT NULL UNIQUE,
    dedupe_key                TEXT NOT NULL,
    message_id                TEXT,
    title                     TEXT,
    sender_name               TEXT,
    sender_address            TEXT NOT NULL DEFAULT '',
    sent_at                   TEXT,
    received_at               TEXT NOT NULL,
    body_html_blocked         TEXT,
    body_html_remote          TEXT,
    body_text                 TEXT,
    has_blocked_remote_images INTEGER NOT NULL DEFAULT 0 CHECK (has_blocked_remote_images IN (0, 1)),
    remote_images_allowed     INTEGER NOT NULL DEFAULT 0 CHECK (remote_images_allowed IN (0, 1)),
    attachment_summary        TEXT,
    storage_bytes             INTEGER NOT NULL DEFAULT 0 CHECK (storage_bytes >= 0),
    created_at                TEXT NOT NULL,
    updated_at                TEXT NOT NULL,
    FOREIGN KEY (source_id) REFERENCES newsletter_sources(id) ON DELETE RESTRICT,
    UNIQUE (did, dedupe_key)
);

CREATE INDEX newsletter_messages_did_received_idx
    ON newsletter_messages (did, received_at DESC);

CREATE INDEX newsletter_messages_source_received_idx
    ON newsletter_messages (source_id, received_at DESC);

CREATE TABLE newsletter_inline_assets (
    token        TEXT PRIMARY KEY,
    message_id   TEXT NOT NULL,
    content_id   TEXT NOT NULL,
    media_type   TEXT NOT NULL,
    data         BLOB NOT NULL,
    content_hash TEXT NOT NULL,
    created_at   TEXT NOT NULL,
    FOREIGN KEY (message_id) REFERENCES newsletter_messages(id) ON DELETE CASCADE,
    UNIQUE (message_id, content_id)
);

CREATE TABLE newsletter_saves (
    id         TEXT PRIMARY KEY,
    did        TEXT NOT NULL,
    message_id TEXT NOT NULL,
    created_at TEXT NOT NULL,
    FOREIGN KEY (message_id) REFERENCES newsletter_messages(id) ON DELETE CASCADE,
    UNIQUE (did, message_id)
);

CREATE INDEX newsletter_saves_did_created_idx
    ON newsletter_saves (did, created_at DESC);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS newsletter_saves;
DROP TABLE IF EXISTS newsletter_inline_assets;
DROP TABLE IF EXISTS newsletter_messages;
DROP TABLE IF EXISTS newsletter_receipts;
DROP TABLE IF EXISTS newsletter_sources;
DROP TABLE IF EXISTS newsletter_addresses;
-- +goose StatementEnd
