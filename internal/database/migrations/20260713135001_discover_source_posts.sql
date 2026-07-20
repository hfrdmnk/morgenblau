-- +goose Up
-- +goose StatementBegin

-- Posts-preview cache for discover source candidates (SPEC <discovery>: a
-- card preview must never block on a live fetch). Keyed by candidate key, not
-- feed_url, since candidates aren't in the feeds table until subscribed.
-- fetched_at is nullable: "never succeeded yet" is a real state, distinct
-- from a stale success, once failure_count/next_retry_at track backoff.
CREATE TABLE discover_source_posts_state (
    source_key    TEXT PRIMARY KEY,
    fetched_at    TEXT,
    favicon_url   TEXT,
    failure_count INTEGER NOT NULL DEFAULT 0,
    next_retry_at TEXT,
    -- independent backoff ladder so a favicon failure never delays a posts retry or vice versa (different call paths now)
    favicon_failure_count INTEGER NOT NULL DEFAULT 0,
    favicon_next_retry_at TEXT
);

-- position 0 = newest, capped at discoverposts.PreviewCap; no FKs since
-- candidates aren't in feeds.
CREATE TABLE discover_source_posts (
    source_key   TEXT NOT NULL,
    position     INTEGER NOT NULL,
    title        TEXT NOT NULL,
    published_at TEXT,
    url          TEXT,
    post_key     TEXT NOT NULL,
    PRIMARY KEY (source_key, position)
);

-- post_key is salted with source_key at generation time, so this index also guards against cross-source collisions.
CREATE UNIQUE INDEX idx_discover_source_posts_post_key ON discover_source_posts(post_key);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS discover_source_posts;
DROP TABLE IF EXISTS discover_source_posts_state;
-- +goose StatementEnd
