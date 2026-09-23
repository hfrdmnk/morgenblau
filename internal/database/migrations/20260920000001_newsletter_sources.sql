-- +goose Up
-- +goose StatementBegin
CREATE TABLE newsletter_sources (
    id                TEXT PRIMARY KEY,
    did               TEXT NOT NULL,
    source_key        TEXT NOT NULL,
    identity_kind     TEXT NOT NULL CHECK (identity_kind IN ('list_id', 'from', 'manual')),
    identity_value    TEXT,
    title             TEXT NOT NULL,
    sender_name       TEXT,
    sender_address    TEXT NOT NULL DEFAULT '',
    status            TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'stopped')),
    accept_after      TEXT,
    is_primary        INTEGER NOT NULL DEFAULT 0 CHECK (is_primary IN (0, 1)),
    tags              TEXT NOT NULL DEFAULT '[]',
    first_received_at TEXT,
    last_received_at  TEXT,
    created_at        TEXT NOT NULL,
    updated_at        TEXT NOT NULL,
    UNIQUE (did, source_key),
    UNIQUE (did, id),
    FOREIGN KEY (did) REFERENCES newsletter_addresses(did) ON DELETE RESTRICT
);

CREATE INDEX newsletter_sources_did_status_title_idx
    ON newsletter_sources (did, status, title COLLATE NOCASE);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS newsletter_sources;
-- +goose StatementEnd
