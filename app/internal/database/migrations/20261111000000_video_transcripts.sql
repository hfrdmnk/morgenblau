-- +goose Up
-- +goose StatementBegin
CREATE TABLE video_transcripts (
    entry_id   INTEGER PRIMARY KEY,
    video_id   TEXT NOT NULL,
    status     TEXT NOT NULL CHECK (status IN ('ready', 'none')),
    transcript TEXT,
    checked_at TEXT NOT NULL,
    CHECK ((status = 'ready') = (transcript IS NOT NULL)),
    FOREIGN KEY (entry_id) REFERENCES feed_entries(id) ON DELETE CASCADE
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS video_transcripts;
-- +goose StatementEnd
