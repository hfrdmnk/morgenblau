-- +goose Up
-- +goose StatementBegin
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
    UNIQUE (did, id),
    FOREIGN KEY (did, source_id) REFERENCES newsletter_sources(did, id) ON DELETE RESTRICT,
    UNIQUE (did, dedupe_key)
);

CREATE INDEX newsletter_messages_did_received_idx
    ON newsletter_messages (did, received_at DESC);

CREATE INDEX newsletter_messages_source_received_idx
    ON newsletter_messages (source_id, received_at DESC);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS newsletter_messages;
-- +goose StatementEnd
