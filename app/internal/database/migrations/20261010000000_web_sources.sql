-- +goose Up
-- +goose StatementBegin
CREATE TABLE web_sources (
    id                   TEXT PRIMARY KEY,
    -- One web source per site: listing_key is the normalized listing URL, listing_url the one generation fetches.
    listing_key          TEXT NOT NULL UNIQUE,
    listing_url          TEXT NOT NULL,
    site_url             TEXT NOT NULL,
    title                TEXT NOT NULL DEFAULT '',
    recipe               TEXT NOT NULL,
    health               TEXT NOT NULL CHECK (health IN ('ok', 'needs_attention', 'blocked_by_site', 'broken')),
    health_reason        TEXT NOT NULL DEFAULT '',
    setup_cost           TEXT NOT NULL DEFAULT '{}',
    created_by_did       TEXT NOT NULL,
    created_at           TEXT NOT NULL,
    updated_at           TEXT NOT NULL,
    -- The setup in force: when it ran, its own verdict and a counter a poll checks so it never writes over a newer one.
    setup_at             TEXT NOT NULL,
    setup_health         TEXT NOT NULL CHECK (setup_health IN ('ok', 'needs_attention', 'blocked_by_site', 'broken')),
    setup_health_reason  TEXT NOT NULL DEFAULT '',
    setup_seq            INTEGER NOT NULL DEFAULT 0,
    -- The last setup run, kept or not, so a reader retrying at once does not pay for another.
    setup_tried_at       TEXT NOT NULL,
    last_polled_at       TEXT,
    next_poll_at         TEXT,
    etag                 TEXT,
    last_modified        TEXT,
    link_set_hash        TEXT,
    consecutive_failures INTEGER NOT NULL DEFAULT 0,
    -- The weekly model check counts from here, or from setup_at before the first one.
    last_checked_at      TEXT,
    -- The reader who pointed at a post for the setup in force, and when; kept for audit, never served.
    anchored_by_did      TEXT,
    anchored_at          TEXT
);

CREATE INDEX web_sources_next_poll_at_idx ON web_sources (next_poll_at);
CREATE INDEX web_sources_created_by_idx ON web_sources (created_by_did, created_at);

CREATE TABLE web_source_items (
    source_id     TEXT NOT NULL REFERENCES web_sources (id) ON DELETE CASCADE,
    url           TEXT NOT NULL,
    title         TEXT NOT NULL,
    published_at  TEXT,
    excerpt       TEXT NOT NULL DEFAULT '',
    first_seen_at TEXT NOT NULL,
    history       INTEGER NOT NULL DEFAULT 0,
    position      INTEGER NOT NULL,
    PRIMARY KEY (source_id, url)
);

-- Only good polls land here; the detector compares a new poll against the latest few.
CREATE TABLE web_source_polls (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    source_id    TEXT NOT NULL REFERENCES web_sources (id) ON DELETE CASCADE,
    polled_at    TEXT NOT NULL,
    item_count   INTEGER NOT NULL,
    urls         TEXT NOT NULL,
    titles       TEXT NOT NULL,
    url_template TEXT NOT NULL,
    date_frac    REAL NOT NULL,
    title_shape  REAL NOT NULL
);

CREATE INDEX web_source_polls_source_idx ON web_source_polls (source_id, id);

-- Every run of the repair ladder: the model step's daily and weekly budgets count here, and free false alarms stay on record.
CREATE TABLE web_source_repairs (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    source_id    TEXT NOT NULL REFERENCES web_sources (id) ON DELETE CASCADE,
    attempted_at TEXT NOT NULL,
    cause        TEXT NOT NULL CHECK (cause IN ('detector', 'weekly_check')),
    outcome      TEXT NOT NULL CHECK (outcome IN ('accepted', 'false_alarm', 'unresolved')),
    step         TEXT NOT NULL DEFAULT '',
    rule         TEXT NOT NULL DEFAULT '',
    reason       TEXT NOT NULL DEFAULT '',
    used_model   INTEGER NOT NULL DEFAULT 0,
    usd          REAL NOT NULL DEFAULT 0
);

CREATE INDEX web_source_repairs_source_idx ON web_source_repairs (source_id, attempted_at);

-- Every model call's cost across all sources, for the global daily budget; it outlives the sources it was spent on.
CREATE TABLE web_model_spend (
    id       INTEGER PRIMARY KEY AUTOINCREMENT,
    spent_at TEXT NOT NULL,
    kind     TEXT NOT NULL CHECK (kind IN ('setup', 'repair', 'check', 'anchor_check')),
    usd      REAL NOT NULL
);

CREATE INDEX web_model_spend_spent_at_idx ON web_model_spend (spent_at);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS web_model_spend;
DROP TABLE IF EXISTS web_source_repairs;
DROP TABLE IF EXISTS web_source_polls;
DROP TABLE IF EXISTS web_source_items;
DROP TABLE IF EXISTS web_sources;
-- +goose StatementEnd
