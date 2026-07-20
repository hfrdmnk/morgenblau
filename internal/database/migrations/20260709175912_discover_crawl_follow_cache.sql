-- +goose Up
-- +goose StatementBegin

-- Reader-network follow crawl cache (SPEC <discovery> People "one hop
-- inside the reader network: people followed by their Morgenblau/Skyreader
-- follows"). Keyed by the crawled repo's own DID, same posture as the other
-- discover_crawl_* caches — shared across every viewer who follows that
-- person.
CREATE TABLE discover_crawl_follow_state (
    followed_did TEXT PRIMARY KEY,
    fetched_at   TEXT NOT NULL
);

-- One row per reader-network follow (blue.morgen.graph.follow or
-- app.skyreader.social.follow) found on a followed repo — the one-hop
-- People candidate set.
CREATE TABLE discover_crawl_follows (
    followed_did TEXT NOT NULL,
    subject_did  TEXT NOT NULL,
    fetched_at   TEXT NOT NULL,
    PRIMARY KEY (followed_did, subject_did)
);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS discover_crawl_follows;
DROP TABLE IF EXISTS discover_crawl_follow_state;
-- +goose StatementEnd
