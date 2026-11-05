-- +goose Up
-- +goose StatementBegin
ALTER TABLE newsletter_sources ADD COLUMN unsubscribe_url TEXT;
ALTER TABLE newsletter_sources ADD COLUMN deleted_at TEXT;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE newsletter_sources DROP COLUMN deleted_at;
ALTER TABLE newsletter_sources DROP COLUMN unsubscribe_url;
-- +goose StatementEnd
