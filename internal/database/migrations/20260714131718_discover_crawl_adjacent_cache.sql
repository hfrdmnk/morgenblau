-- +goose Up
-- +goose StatementBegin

-- Adjacent-graph (Bluesky/Tangled) follow crawl cache for the session
-- user's own repo (SPEC <discovery> weak trust tier). Keyed by the viewing
-- user's own DID, unlike the followed-repo discover_crawl_* caches: this
-- crawls the viewer, not someone they follow, so there's no cross-viewer
-- sharing to gain from a longer TTL.
CREATE TABLE discover_crawl_adjacent_state (
    did        TEXT PRIMARY KEY,
    fetched_at TEXT NOT NULL
);

-- One row per adjacent-graph follow found on the user's own repo.
CREATE TABLE discover_crawl_adjacent_follows (
    did         TEXT NOT NULL,
    subject_did TEXT NOT NULL,
    network     TEXT NOT NULL, -- 'bluesky' | 'tangled'
    fetched_at  TEXT NOT NULL,
    PRIMARY KEY (did, subject_did)
);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS discover_crawl_adjacent_follows;
DROP TABLE IF EXISTS discover_crawl_adjacent_state;
-- +goose StatementEnd
