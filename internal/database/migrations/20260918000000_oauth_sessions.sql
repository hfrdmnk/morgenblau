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
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS oauth_sessions;
-- +goose StatementEnd
