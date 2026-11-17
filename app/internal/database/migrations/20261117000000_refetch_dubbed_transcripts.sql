-- +goose Up
-- Auto-dubbed videos stored a dub's caption track instead of the original audio's; drop every stored transcript so the reader fetches it again.
DELETE FROM video_transcripts
WHERE status = 'ready';

-- +goose Down
-- Stored transcripts are a cache and will be fetched again when the reader opens the video.
SELECT 1;
