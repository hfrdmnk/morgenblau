-- +goose Up
-- +goose StatementBegin
-- Consecutive failures of a YouTube feed while YouTube's other feeds fetched; an outage fails them all and does not count.
ALTER TABLE feeds ADD COLUMN lone_failures INTEGER NOT NULL DEFAULT 0;

-- When YouTube's channel page said the channel does not exist; NULL until then and again after the next successful fetch.
ALTER TABLE feeds ADD COLUMN channel_gone_at TEXT;

-- The old ladder left channels caught by a YouTube outage waiting up to a day; retry them on the next sweep.
UPDATE feeds SET next_fetch_at = NULL
WHERE consecutive_failures > 0
  AND (feed_url LIKE 'https://www.youtube.com/feeds/videos.xml?%' OR feed_url LIKE 'https://youtube.com/feeds/videos.xml?%');
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE feeds DROP COLUMN channel_gone_at;
ALTER TABLE feeds DROP COLUMN lone_failures;
-- +goose StatementEnd
