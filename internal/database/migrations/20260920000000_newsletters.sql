-- +goose Up
-- +goose StatementBegin
CREATE TABLE newsletter_addresses (
    did        TEXT PRIMARY KEY,
    local_part TEXT NOT NULL UNIQUE,
    created_at TEXT NOT NULL,
    UNIQUE (did, local_part)
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS newsletter_addresses;
-- +goose StatementEnd
