-- +goose Up
-- +goose StatementBegin

-- marked_at prevents a rebuild from clearing a newer dirty generation.
CREATE TABLE tap_dirty_repos (
    did       TEXT PRIMARY KEY,
    marked_at TEXT NOT NULL
);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS tap_dirty_repos;
-- +goose StatementEnd
