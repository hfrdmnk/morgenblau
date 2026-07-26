-- +goose Up
-- +goose StatementBegin

-- Precomputed distinct-repo counts behind the trending people quality bar.
CREATE TABLE discover_trending_follow_counts (
    subject_did    TEXT PRIMARY KEY,
    distinct_repos INTEGER NOT NULL
);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS discover_trending_follow_counts;
-- +goose StatementEnd
