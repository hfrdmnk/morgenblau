-- +goose Up
-- +goose StatementBegin

-- Discover hide/snooze store (SPEC <discovery> "Hiding and rotation"; PRD
-- module 6). One mechanism for both target kinds — never a PDS record, so
-- negative taste signals stay private (SPEC: "publishing it would leak taste
-- and pollute the repo").
CREATE TABLE discover_hides (
    did          TEXT NOT NULL,
    target_kind  TEXT NOT NULL, -- 'source' | 'person'
    target_key   TEXT NOT NULL, -- canonical source key or person DID
    hidden_until TEXT NOT NULL,
    hide_count   INTEGER NOT NULL DEFAULT 1,
    created_at   TEXT NOT NULL,
    updated_at   TEXT NOT NULL,
    PRIMARY KEY (did, target_kind, target_key)
);

-- Speeds the exclusion-set read every suggestion request does: active hides
-- for one user, one target kind.
CREATE INDEX discover_hides_active_idx
    ON discover_hides (did, target_kind, hidden_until);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS discover_hides_active_idx;
DROP TABLE IF EXISTS discover_hides;
-- +goose StatementEnd
