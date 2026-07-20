-- +goose Up
-- +goose StatementBegin

-- Daily batch trending aggregate (SPEC <discovery> "Global/Trending"
-- acquisition; PRD module 3). One row per (repo_did, source_key): the
-- contributing repo's single strongest signal for that source. The batch
-- replaces a repo's whole row set on every pass (delete then reinsert), so
-- a same-day rerun — or a repo whose signals changed — never double-counts
-- (diff/replace, not blind accumulate).
CREATE TABLE discover_trending_signals (
    repo_did    TEXT NOT NULL,
    source_key  TEXT NOT NULL,
    kind        TEXT NOT NULL, -- 'rss' | 'standardfeed'
    title       TEXT,
    site_url    TEXT,
    signal_kind TEXT NOT NULL, -- 'author' | 'subscribe' | 'share' | 'save'
    signal_at   TEXT,          -- RFC3339; NULL = unknown recency
    fetched_at  TEXT NOT NULL,
    PRIMARY KEY (repo_did, source_key)
);

-- Speeds the trending read's group-by-source-key scan over the whole table.
CREATE INDEX discover_trending_signals_source_idx
    ON discover_trending_signals (source_key);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS discover_trending_signals_source_idx;
DROP TABLE IF EXISTS discover_trending_signals;
-- +goose StatementEnd
