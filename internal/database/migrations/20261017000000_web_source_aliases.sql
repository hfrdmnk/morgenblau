-- +goose Up
-- +goose StatementBegin
-- A listing pasted earlier whose setup moved to the section page that lists its posts; it finds that page's source.
CREATE TABLE web_source_aliases (
    listing_key TEXT PRIMARY KEY,
    source_id   TEXT NOT NULL REFERENCES web_sources (id) ON DELETE CASCADE
);

CREATE INDEX web_source_aliases_source_idx ON web_source_aliases (source_id);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS web_source_aliases;
-- +goose StatementEnd
