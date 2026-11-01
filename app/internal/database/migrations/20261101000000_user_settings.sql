-- +goose Up
-- +goose StatementBegin
CREATE TABLE user_settings (
    did              TEXT PRIMARY KEY,
    public_following INTEGER NOT NULL DEFAULT 0 CHECK (public_following IN (0, 1)),
    updated_at       TEXT NOT NULL
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS user_settings;
-- +goose StatementEnd
