-- +goose Up
-- +goose StatementBegin

-- Tier-1: per-user derived index of blue.morgen.graph.follow records on the
-- user's PDS. Reconciled, not authoritative; PDS is canonical.
CREATE TABLE user_follows (
    did         TEXT NOT NULL,
    rkey        TEXT NOT NULL,
    at_uri      TEXT NOT NULL,
    subject_did TEXT NOT NULL,
    created_at  TEXT NOT NULL,
    updated_at  TEXT NOT NULL,
    PRIMARY KEY (did, rkey)
);

CREATE UNIQUE INDEX user_follows_did_subject_did_idx
    ON user_follows (did, subject_did);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS user_follows_did_subject_did_idx;
DROP TABLE IF EXISTS user_follows;
-- +goose StatementEnd
