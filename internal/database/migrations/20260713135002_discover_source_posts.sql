-- +goose Up
-- +goose StatementBegin

-- position 0 is newest and the row set is capped by discoverposts.PreviewCap.
CREATE TABLE discover_source_posts (
    source_key   TEXT NOT NULL,
    position     INTEGER NOT NULL,
    title        TEXT NOT NULL,
    published_at TEXT,
    url          TEXT,
    post_key     TEXT NOT NULL,
    PRIMARY KEY (source_key, position)
);

-- post_key includes source_key at generation time.
CREATE UNIQUE INDEX idx_discover_source_posts_post_key
    ON discover_source_posts (post_key);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS idx_discover_source_posts_post_key;
DROP TABLE IF EXISTS discover_source_posts;
-- +goose StatementEnd
