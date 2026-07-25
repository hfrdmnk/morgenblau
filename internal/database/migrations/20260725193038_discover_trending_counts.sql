-- +goose Up
-- +goose StatementBegin

-- Precomputed distinct-repo counts behind the trending quality bar (SPEC
-- <discovery> "Quality bar"). The reads used to re-derive these with a
-- correlated COUNT(DISTINCT) per row; the batch rebuilds both tables in one
-- pass instead, so the bar costs a keyed lookup at read time.
CREATE TABLE discover_trending_source_counts (
    source_key     TEXT PRIMARY KEY,
    distinct_repos INTEGER NOT NULL
);

CREATE TABLE discover_trending_follow_counts (
    subject_did    TEXT PRIMARY KEY,
    distinct_repos INTEGER NOT NULL
);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS discover_trending_follow_counts;
DROP TABLE IF EXISTS discover_trending_source_counts;
-- +goose StatementEnd
