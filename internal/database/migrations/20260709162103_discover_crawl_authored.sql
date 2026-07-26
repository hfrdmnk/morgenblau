-- +goose Up
-- +goose StatementBegin

-- One row per site.standard.publication the followed DID owns. The recency
-- proxy comes from the newest site.standard.document in the same repo.
CREATE TABLE discover_crawl_authored (
    followed_did      TEXT NOT NULL,
    canonical_key      TEXT NOT NULL,
    kind                TEXT NOT NULL, -- always 'standardfeed' in v1
    title               TEXT,
    site_url            TEXT,
    last_published_at   TEXT,
    fetched_at          TEXT NOT NULL,
    verification        TEXT NOT NULL, -- always 'verified' today; column exists for a future retry policy
    PRIMARY KEY (followed_did, canonical_key)
);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS discover_crawl_authored;
-- +goose StatementEnd
