-- +goose Up
-- +goose StatementBegin

-- Precomputed distinct-repo counts behind the trending source quality bar.
CREATE TABLE discover_trending_source_counts (
    source_key     TEXT PRIMARY KEY,
    distinct_repos INTEGER NOT NULL
);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS discover_trending_source_counts;
-- +goose StatementEnd
