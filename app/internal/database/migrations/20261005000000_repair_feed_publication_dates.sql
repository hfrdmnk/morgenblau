-- +goose Up
-- Legacy fallback and fetch timestamps were sampled separately; bypass 304 for all RSS feeds once.
UPDATE feeds
SET etag = NULL, last_modified = NULL
WHERE kind = 'rss';

-- +goose Down
-- HTTP validators are disposable and will be repopulated by the next fetch.
SELECT 1;
