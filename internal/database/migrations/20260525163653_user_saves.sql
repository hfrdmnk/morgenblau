-- +goose Up
-- +goose StatementBegin

-- Tier-1: per-user derived index of blue.morgen.feed.save records on the
-- user's PDS. Reconciled, not authoritative — PDS is canonical.
CREATE TABLE user_saves (
    did         TEXT NOT NULL,
    rkey        TEXT NOT NULL,
    at_uri      TEXT NOT NULL,
    item_url    TEXT NOT NULL,
    feed_url    TEXT,
    created_at  TEXT NOT NULL,
    updated_at  TEXT NOT NULL,
    PRIMARY KEY (did, rkey)
);

CREATE UNIQUE INDEX user_saves_did_item_url_idx
    ON user_saves (did, item_url);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS user_saves_did_item_url_idx;
DROP TABLE IF EXISTS user_saves;
-- +goose StatementEnd
