-- +goose Up
-- +goose StatementBegin
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
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS newsletter_inline_assets;
-- +goose StatementEnd
