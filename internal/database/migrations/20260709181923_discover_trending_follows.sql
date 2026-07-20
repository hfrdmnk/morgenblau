-- +goose Up
-- +goose StatementBegin

-- Daily batch reader-network follower aggregate (SPEC <discovery> People
-- "Global/Trending": "ranked by reader-network follower count"; PRD module
-- 3 extension). One row per (repo_did, subject_did): repo_did follows
-- subject_did via blue.morgen.graph.follow or app.skyreader.social.follow.
-- Same diff/replace-per-repo idempotency as discover_trending_signals.
CREATE TABLE discover_trending_follows (
    repo_did    TEXT NOT NULL,
    subject_did TEXT NOT NULL,
    fetched_at  TEXT NOT NULL,
    PRIMARY KEY (repo_did, subject_did)
);

-- Speeds the trending-people read's group-by-subject scan over the whole table.
CREATE INDEX discover_trending_follows_subject_idx
    ON discover_trending_follows (subject_did);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS discover_trending_follows_subject_idx;
DROP TABLE IF EXISTS discover_trending_follows;
-- +goose StatementEnd
