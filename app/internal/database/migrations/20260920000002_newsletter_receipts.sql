-- +goose Up
-- +goose StatementBegin
CREATE TABLE newsletter_receipts (
    id                   TEXT PRIMARY KEY,
    did                  TEXT NOT NULL,
    envelope_from        TEXT NOT NULL,
    recipient            TEXT NOT NULL,
    recipient_local_part TEXT NOT NULL,
    received_at          TEXT NOT NULL,
    raw_mime             BLOB NOT NULL,
    reserved_bytes       INTEGER NOT NULL CHECK (reserved_bytes >= 0),
    attempts             INTEGER NOT NULL DEFAULT 0,
    last_error           TEXT,
    next_attempt_at      TEXT,
    created_at           TEXT NOT NULL,
    FOREIGN KEY (did, recipient_local_part) REFERENCES newsletter_addresses(did, local_part) ON DELETE RESTRICT
);

CREATE INDEX newsletter_receipts_ready_idx
    ON newsletter_receipts (next_attempt_at, created_at);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS newsletter_receipts;
-- +goose StatementEnd
