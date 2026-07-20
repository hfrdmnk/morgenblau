-- +goose Up
-- +goose StatementBegin

-- Cross-crawl cache for publication at-uri resolution (SPEC <discovery>):
-- keyed by the raw at-uri found in a subscription record, one row per uri
-- regardless of which followed repo referenced it. canonical_key/kind/title/
-- site_url/icon_url NULL means a deterministic skip (not discoverable, not an error).
-- failure_count/next_retry_at back an exponential backoff on transient
-- resolve failures (internal/backoff); next_retry_at also gates deterministic
-- skips at a flat 24h so a publication later mirrored into site.standard
-- gets picked up without exponential growth.
CREATE TABLE discover_publication_resolutions (
    publication_uri TEXT PRIMARY KEY,
    canonical_key   TEXT,
    kind            TEXT,
    title           TEXT,
    site_url        TEXT,
    icon_url        TEXT,
    failure_count   INTEGER NOT NULL DEFAULT 0,
    fetched_at      TEXT NOT NULL,
    next_retry_at   TEXT NOT NULL
);

-- Handle-form and DID-form publication uris resolve to the same canonical_key
-- but sit on two different PK rows; the favicon path looks up by canonical_key.
CREATE INDEX discover_publication_resolutions_canonical_key_idx ON discover_publication_resolutions (canonical_key);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS discover_publication_resolutions_canonical_key_idx;
DROP TABLE IF EXISTS discover_publication_resolutions;
-- +goose StatementEnd
