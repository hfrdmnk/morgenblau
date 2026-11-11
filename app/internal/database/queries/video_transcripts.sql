-- name: GetVideoTranscript :one
SELECT video_id, status, transcript, checked_at FROM video_transcripts WHERE entry_id = ?;

-- name: UpsertVideoTranscript :exec
INSERT INTO video_transcripts (entry_id, video_id, status, transcript, checked_at)
VALUES (?, ?, ?, ?, ?)
ON CONFLICT (entry_id) DO UPDATE SET
    video_id = excluded.video_id,
    status = excluded.status,
    transcript = excluded.transcript,
    checked_at = excluded.checked_at;
