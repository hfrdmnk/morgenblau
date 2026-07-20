-- +goose Up
-- +goose StatementBegin
-- One-row stamp of the trending batch's last successful run so restarts within
-- the interval (air hot-reload) skip the immediate startup run instead of
-- re-enumerating the relay on every save.
CREATE TABLE discover_batch_state (
    id          INTEGER PRIMARY KEY CHECK (id = 1),
    last_run_at TEXT NOT NULL
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS discover_batch_state;
-- +goose StatementEnd
