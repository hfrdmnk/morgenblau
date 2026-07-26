-- +goose Up
-- +goose StatementBegin

-- Inactive repos retain raw records for reactivation; deletion purges them.
CREATE TABLE tap_repo_states (
    did        TEXT PRIMARY KEY,
    handle     TEXT NOT NULL,
    is_active  INTEGER NOT NULL CHECK (is_active IN (0, 1)),
    status     TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS tap_repo_states;
-- +goose StatementEnd
