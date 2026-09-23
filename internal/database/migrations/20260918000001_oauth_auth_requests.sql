-- +goose Up
-- +goose StatementBegin
CREATE TABLE oauth_auth_requests (
    state       TEXT PRIMARY KEY,
    -- Request material is AEAD-encrypted by the OAuth store before persistence.
    data        BLOB NOT NULL,
    created_at  TEXT NOT NULL,
    expires_at  TEXT NOT NULL
);

CREATE INDEX oauth_auth_requests_expires_at_idx
    ON oauth_auth_requests (expires_at);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS oauth_auth_requests;
-- +goose StatementEnd
