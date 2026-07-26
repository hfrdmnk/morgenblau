-- +goose Up
-- +goose StatementBegin

-- Posts-preview cache state for discover candidates. Candidate keys do not
-- require a feeds row, and posts/favicon retries have independent backoff.
CREATE TABLE discover_source_posts_state (
    source_key    TEXT PRIMARY KEY,
    fetched_at    TEXT,
    favicon_url   TEXT,
    failure_count INTEGER NOT NULL DEFAULT 0,
    next_retry_at TEXT,
    favicon_failure_count INTEGER NOT NULL DEFAULT 0,
    favicon_next_retry_at TEXT
);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS discover_source_posts_state;
-- +goose StatementEnd
