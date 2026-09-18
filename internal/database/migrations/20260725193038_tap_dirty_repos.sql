-- +goose Up
-- +goose StatementBegin

CREATE TABLE tap_dirty_repos (
    did        TEXT PRIMARY KEY,
    marked_seq INTEGER NOT NULL
);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS tap_dirty_repos;
-- +goose StatementEnd
