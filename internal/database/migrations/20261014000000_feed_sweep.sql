-- +goose Up
-- +goose StatementBegin
-- When the sweep last found nobody following the feed; NULL while followed. Every UpsertFeed clears it.
ALTER TABLE feeds ADD COLUMN orphaned_at TEXT;

-- When a reader was last offered the web source to follow; an unfollowed one is kept this long after.
ALTER TABLE web_sources ADD COLUMN offered_at TEXT NOT NULL DEFAULT '';
UPDATE web_sources SET offered_at = updated_at;

-- When a feed reader outside Morgenblau last fetched /web/<id>; such readers follow the source like a subscription does.
ALTER TABLE web_sources ADD COLUMN served_at TEXT;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE web_sources DROP COLUMN served_at;
ALTER TABLE web_sources DROP COLUMN offered_at;
ALTER TABLE feeds DROP COLUMN orphaned_at;
-- +goose StatementEnd
