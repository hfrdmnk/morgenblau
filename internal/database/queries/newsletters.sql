-- name: GetNewsletterAddress :one
SELECT did, local_part, created_at
FROM newsletter_addresses
WHERE did = ?1;

-- name: GetNewsletterAddressByLocalPart :one
SELECT did, local_part, created_at
FROM newsletter_addresses
WHERE local_part = ?1;

-- name: CreateNewsletterAddress :exec
INSERT INTO newsletter_addresses (did, local_part, created_at)
VALUES (?1, ?2, ?3)
ON CONFLICT (did) DO NOTHING;

-- name: CreateNewsletterReceipt :exec
INSERT INTO newsletter_receipts (
    id, did, envelope_from, recipient, received_at, raw_mime, reserved_bytes, created_at
) VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8);

-- name: GetNextNewsletterReceipt :one
SELECT id, did, envelope_from, recipient, received_at, raw_mime,
       reserved_bytes, attempts, last_error, next_attempt_at, created_at
FROM newsletter_receipts
WHERE next_attempt_at IS NULL OR next_attempt_at <= ?1
ORDER BY created_at
LIMIT 1;

-- name: MarkNewsletterReceiptFailed :exec
UPDATE newsletter_receipts
SET attempts = attempts + 1, last_error = ?2, next_attempt_at = ?3
WHERE id = ?1;

-- name: DeleteNewsletterReceipt :exec
DELETE FROM newsletter_receipts WHERE id = ?1;

-- name: GetNewsletterSourceByKey :one
SELECT * FROM newsletter_sources WHERE did = ?1 AND source_key = ?2;

-- name: GetNewsletterSource :one
SELECT * FROM newsletter_sources WHERE did = ?1 AND id = ?2;

-- name: CreateNewsletterSource :exec
INSERT INTO newsletter_sources (
    id, did, source_key, identity_kind, identity_value, title,
    sender_name, sender_address, status, first_received_at,
    last_received_at, created_at, updated_at
) VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8, 'active', ?9, ?9, ?9, ?9)
ON CONFLICT (did, source_key) DO NOTHING;

-- name: CreateManualNewsletterSource :exec
INSERT INTO newsletter_sources (
    id, did, source_key, identity_kind, title, status, created_at, updated_at
) VALUES (?1, ?2, ?3, 'manual', ?4, 'active', ?5, ?5);

-- name: TouchNewsletterSource :exec
UPDATE newsletter_sources
SET sender_name = COALESCE(sqlc.arg(sender_name), sender_name),
    sender_address = CASE WHEN sqlc.arg(sender_address) = '' THEN sender_address ELSE sqlc.arg(sender_address) END,
    first_received_at = COALESCE(first_received_at, sqlc.arg(received_at)),
    last_received_at = sqlc.arg(received_at),
    updated_at = sqlc.arg(received_at)
WHERE did = sqlc.arg(did) AND id = sqlc.arg(id);

-- name: ListNewsletterSources :many
SELECT s.*,
       (SELECT COUNT(*) FROM newsletter_messages m WHERE m.did = s.did AND m.source_id = s.id) AS issue_count,
       (SELECT COUNT(*) FROM newsletter_saves sv WHERE sv.did = s.did AND sv.message_id IN
           (SELECT id FROM newsletter_messages m WHERE m.did = s.did AND m.source_id = s.id)) AS saved_count,
       (SELECT COUNT(*) FROM newsletter_messages m WHERE m.did = s.did AND m.source_id = s.id AND m.received_at >= ?2 AND m.received_at < ?3) AS count_7d,
       (SELECT COUNT(*) FROM newsletter_messages m WHERE m.did = s.did AND m.source_id = s.id AND m.received_at >= ?4 AND m.received_at < ?3) AS count_28d,
       (SELECT COUNT(*) FROM newsletter_messages m WHERE m.did = s.did AND m.source_id = s.id AND m.received_at >= ?5 AND m.received_at < ?3) AS count_56d,
       (SELECT COUNT(*) FROM newsletter_messages m WHERE m.did = s.did AND m.source_id = s.id AND m.received_at >= ?6 AND m.received_at < ?3) AS count_84d
FROM newsletter_sources s
WHERE s.did = ?1
ORDER BY s.title COLLATE NOCASE, s.id;

-- name: GetNewsletterSourceWithStats :one
SELECT s.*,
       (SELECT COUNT(*) FROM newsletter_messages m WHERE m.did = s.did AND m.source_id = s.id) AS issue_count,
       (SELECT COUNT(*) FROM newsletter_saves sv WHERE sv.did = s.did AND sv.message_id IN
           (SELECT id FROM newsletter_messages m WHERE m.did = s.did AND m.source_id = s.id)) AS saved_count,
       (SELECT COUNT(*) FROM newsletter_messages m WHERE m.did = s.did AND m.source_id = s.id AND m.received_at >= ?3 AND m.received_at < ?4) AS count_7d,
       (SELECT COUNT(*) FROM newsletter_messages m WHERE m.did = s.did AND m.source_id = s.id AND m.received_at >= ?5 AND m.received_at < ?4) AS count_28d,
       (SELECT COUNT(*) FROM newsletter_messages m WHERE m.did = s.did AND m.source_id = s.id AND m.received_at >= ?6 AND m.received_at < ?4) AS count_56d,
       (SELECT COUNT(*) FROM newsletter_messages m WHERE m.did = s.did AND m.source_id = s.id AND m.received_at >= ?7 AND m.received_at < ?4) AS count_84d
FROM newsletter_sources s
WHERE s.did = ?1 AND s.id = ?2;

-- name: PatchNewsletterSource :exec
UPDATE newsletter_sources
SET title = ?3, is_primary = ?4, tags = ?5, updated_at = ?6
WHERE did = ?1 AND id = ?2;

-- name: SetNewsletterSourceStatus :exec
UPDATE newsletter_sources SET status = ?3, updated_at = ?4
WHERE did = ?1 AND id = ?2;

-- name: EnableNewsletterSource :exec
UPDATE newsletter_sources
SET status = 'active', accept_after = ?3, updated_at = ?3
WHERE did = ?1 AND id = ?2;

-- name: DeleteUnsavedNewsletterMessages :exec
DELETE FROM newsletter_messages AS m
WHERE m.did = ?1 AND m.source_id = ?2
  AND NOT EXISTS (
      SELECT 1 FROM newsletter_saves s
      WHERE s.did = m.did
        AND s.message_id = m.id
  );

-- name: CreateNewsletterMessage :one
INSERT INTO newsletter_messages (
    id, did, source_id, entry_slug, dedupe_key, message_id, title,
    sender_name, sender_address, sent_at, received_at,
    body_html_blocked, body_html_remote, body_text,
    has_blocked_remote_images, attachment_summary, storage_bytes, created_at, updated_at
) VALUES (
    ?1, ?2, ?3, ?4, ?5, ?6, ?7,
    ?8, ?9, ?10, ?11,
    ?12, ?13, ?14, ?15, ?16, ?17, ?18, ?18
)
ON CONFLICT (did, dedupe_key) DO NOTHING
RETURNING id;

-- name: GetNewsletterGlobalStorageBytes :one
SELECT CAST(
    COALESCE((SELECT SUM(reserved_bytes) FROM newsletter_receipts), 0) +
    COALESCE((SELECT SUM(storage_bytes) FROM newsletter_messages), 0)
AS INTEGER) AS storage_bytes;

-- name: GetNewsletterOwnerStorageBytes :one
SELECT CAST(
    COALESCE((SELECT SUM(r.reserved_bytes) FROM newsletter_receipts r WHERE r.did = ?1), 0) +
    COALESCE((SELECT SUM(m.storage_bytes) FROM newsletter_messages m WHERE m.did = ?1), 0)
AS INTEGER) AS storage_bytes;

-- name: GetNewsletterMessageBySlug :one
SELECT m.*, s.title AS source_title, s.status AS source_status, s.is_primary AS source_primary,
       sv.id AS save_id, sv.created_at AS saved_at
FROM newsletter_messages m
JOIN newsletter_sources s ON s.id = m.source_id AND s.did = m.did
LEFT JOIN newsletter_saves sv ON sv.message_id = m.id AND sv.did = m.did
WHERE m.did = ?1 AND m.entry_slug = ?2;

-- name: GetNewsletterMessage :one
SELECT m.*, s.title AS source_title, s.status AS source_status, s.is_primary AS source_primary,
       sv.id AS save_id, sv.created_at AS saved_at
FROM newsletter_messages m
JOIN newsletter_sources s ON s.id = m.source_id AND s.did = m.did
LEFT JOIN newsletter_saves sv ON sv.message_id = m.id AND sv.did = m.did
WHERE m.did = ?1 AND m.id = ?2;

-- name: ListNewsletterMessagesForSource :many
SELECT m.*, s.title AS source_title, s.status AS source_status, s.is_primary AS source_primary,
       sv.id AS save_id, sv.created_at AS saved_at
FROM newsletter_messages m
JOIN newsletter_sources s ON s.id = m.source_id AND s.did = m.did
LEFT JOIN newsletter_saves sv ON sv.message_id = m.id AND sv.did = m.did
WHERE m.did = ?1 AND m.source_id = ?2
ORDER BY m.received_at DESC, m.id DESC;

-- name: ListNewsletterMessagesForDigest :many
SELECT m.*, s.title AS source_title, s.status AS source_status, s.is_primary AS source_primary,
       sv.id AS save_id, sv.created_at AS saved_at
FROM newsletter_messages m
JOIN newsletter_sources s ON s.id = m.source_id AND s.did = m.did
LEFT JOIN newsletter_saves sv ON sv.message_id = m.id AND sv.did = m.did
WHERE m.did = ?1 AND m.received_at >= ?2 AND m.received_at < ?3
  AND s.status = 'active'
ORDER BY m.received_at DESC, m.id DESC;

-- name: AllowNewsletterRemoteImages :exec
UPDATE newsletter_messages SET remote_images_allowed = 1, updated_at = ?3
WHERE did = ?1 AND id = ?2;

-- name: MoveNewsletterMessage :exec
UPDATE newsletter_messages SET source_id = ?3, updated_at = ?4
WHERE did = ?1 AND id = ?2;

-- name: CreateNewsletterInlineAsset :exec
INSERT INTO newsletter_inline_assets (
    token, message_id, content_id, media_type, data, content_hash, created_at
) VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7)
ON CONFLICT (message_id, content_id) DO NOTHING;

-- name: GetNewsletterInlineAsset :one
SELECT a.token, a.message_id, a.content_id, a.media_type, a.data, a.content_hash, a.created_at
FROM newsletter_inline_assets a
JOIN newsletter_messages m ON m.id = a.message_id
WHERE m.did = ?1 AND a.token = ?2;

-- name: CreateNewsletterSave :exec
INSERT INTO newsletter_saves (id, did, message_id, created_at)
VALUES (?1, ?2, ?3, ?4)
ON CONFLICT (did, message_id) DO NOTHING;

-- name: GetNewsletterSaveForMessage :one
SELECT * FROM newsletter_saves WHERE did = ?1 AND message_id = ?2;

-- name: DeleteNewsletterSave :exec
DELETE FROM newsletter_saves WHERE did = ?1 AND id = ?2;

-- name: GetNewsletterSave :one
SELECT * FROM newsletter_saves WHERE did = ?1 AND id = ?2;

-- name: DeleteStoppedUnsavedNewsletterMessage :exec
DELETE FROM newsletter_messages AS m
WHERE m.did = ?1 AND m.id = ?2
  AND NOT EXISTS (SELECT 1 FROM newsletter_saves s WHERE s.did = ?1 AND s.message_id = ?2)
  AND EXISTS (SELECT 1 FROM newsletter_sources src WHERE src.did = ?1 AND src.id = m.source_id AND src.status = 'stopped');

-- name: ListNewsletterSaves :many
SELECT sv.id, sv.did, sv.message_id, sv.created_at,
       m.entry_slug, m.title, m.received_at, src.title AS source_title
FROM newsletter_saves sv
JOIN newsletter_messages m ON m.id = sv.message_id AND m.did = sv.did
JOIN newsletter_sources src ON src.id = m.source_id AND src.did = m.did
WHERE sv.did = ?1
ORDER BY sv.created_at DESC, sv.id DESC;
