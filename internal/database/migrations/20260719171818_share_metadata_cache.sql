-- +goose Up
-- +goose StatementBegin

-- Derived display metadata for shared items that are not present in feed_entries.
-- Nullable payload fields preserve failure state without inventing a title or URL.
CREATE TABLE share_metadata_cache (
    target_key     TEXT PRIMARY KEY,
    title          TEXT,
    target_url     TEXT,
    fetched_at     TEXT,
    failure_count  INTEGER NOT NULL DEFAULT 0,
    next_retry_at  TEXT
);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS share_metadata_cache;
-- +goose StatementEnd
