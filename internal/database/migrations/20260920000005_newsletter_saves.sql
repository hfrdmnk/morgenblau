-- +goose Up
-- +goose StatementBegin
CREATE TABLE newsletter_saves (
    id         TEXT PRIMARY KEY,
    did        TEXT NOT NULL,
    message_id TEXT NOT NULL,
    created_at TEXT NOT NULL,
    FOREIGN KEY (did, message_id) REFERENCES newsletter_messages(did, id) ON DELETE CASCADE,
    UNIQUE (did, message_id)
);

CREATE INDEX newsletter_saves_did_created_idx
    ON newsletter_saves (did, created_at DESC);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS newsletter_saves;
-- +goose StatementEnd
