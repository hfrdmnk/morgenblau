-- +goose Up
-- +goose StatementBegin
-- Mirrors the save record's tags as a JSON array, NULL when untagged (internal/tags).
ALTER TABLE user_saves ADD COLUMN tags TEXT;

-- Private newsletter saves keep their tags here only, in the same shape as newsletter_sources.tags.
ALTER TABLE newsletter_saves ADD COLUMN tags TEXT NOT NULL DEFAULT '[]';
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE newsletter_saves DROP COLUMN tags;
ALTER TABLE user_saves DROP COLUMN tags;
-- +goose StatementEnd
