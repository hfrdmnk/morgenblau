-- +goose Up
-- +goose StatementBegin
CREATE TABLE user_subscriptions (
    did          TEXT NOT NULL,
    rkey         TEXT NOT NULL,
    at_uri       TEXT NOT NULL,
    feed_url     TEXT NOT NULL,
    kind         TEXT NOT NULL DEFAULT 'rss',
    sidecar_rkey TEXT,
    title        TEXT,
    is_primary   INTEGER NOT NULL DEFAULT 0,
    tags         TEXT,
    created_at   TEXT NOT NULL,
    updated_at   TEXT NOT NULL,
    PRIMARY KEY (did, rkey),
    FOREIGN KEY (feed_url) REFERENCES feeds(feed_url) ON DELETE RESTRICT
);

CREATE UNIQUE INDEX user_subscriptions_did_feed_url_idx
    ON user_subscriptions (did, feed_url);

CREATE INDEX user_subscriptions_feed_url_idx
    ON user_subscriptions (feed_url);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS user_subscriptions;
-- +goose StatementEnd
