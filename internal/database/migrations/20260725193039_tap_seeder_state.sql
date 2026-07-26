-- +goose Up
-- +goose StatementBegin

-- Repos already accepted by tap's idempotent backfill endpoint.
CREATE TABLE tap_seeder_state (
    did       TEXT PRIMARY KEY,
    seeded_at TEXT NOT NULL
);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS tap_seeder_state;
-- +goose StatementEnd
