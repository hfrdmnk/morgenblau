-- +goose Up
-- +goose StatementBegin

-- Tier-1: per-user derived index of blue.morgen.feed.save records on the
-- user's PDS. Reconciled, not authoritative; PDS is canonical.
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

-- Tier-1: per-user derived index of shares. rkey holds the existence record's
-- rkey: blue.morgen.feed.share (rss) or site.standard.graph.recommend
-- (standardfeed). Reconciled, not authoritative; PDS is canonical.
CREATE TABLE user_shares (
    did          TEXT NOT NULL,
    rkey         TEXT NOT NULL,
    at_uri       TEXT NOT NULL,
    kind         TEXT NOT NULL, -- 'rss' | 'standardfeed'
    item_url     TEXT, -- NULL only for path-less documents
    document     TEXT, -- site.standard.document at-uri (standardfeed only)
    comment      TEXT,
    feed_url     TEXT,
    sidecar_rkey TEXT, -- blue.morgen.feed.share rkey when a comment sidecar exists
    created_at   TEXT NOT NULL,
    updated_at   TEXT NOT NULL,
    PRIMARY KEY (did, rkey)
);

CREATE UNIQUE INDEX user_shares_did_document_idx
    ON user_shares (did, document) WHERE document IS NOT NULL;

CREATE INDEX user_shares_did_created_idx
    ON user_shares (did, created_at DESC);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS user_shares_did_created_idx;
DROP INDEX IF EXISTS user_shares_did_document_idx;
DROP TABLE IF EXISTS user_shares;
DROP INDEX IF EXISTS user_saves_did_item_url_idx;
DROP TABLE IF EXISTS user_saves;
-- +goose StatementEnd
