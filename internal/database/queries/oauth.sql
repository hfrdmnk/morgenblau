-- name: PutSession :exec
INSERT INTO oauth_sessions (did, session_id, data, updated_at)
VALUES (?, ?, ?, ?)
ON CONFLICT (did, session_id) DO UPDATE SET
    data = excluded.data,
    updated_at = excluded.updated_at;

-- name: GetSession :one
SELECT data FROM oauth_sessions
WHERE did = ? AND session_id = ?;

-- name: DeleteSession :exec
DELETE FROM oauth_sessions
WHERE did = ? AND session_id = ?;

-- name: PutAuthRequest :exec
INSERT INTO oauth_auth_requests (state, data, created_at, expires_at)
VALUES (?, ?, ?, ?);

-- name: GetAuthRequest :one
SELECT data, expires_at FROM oauth_auth_requests
WHERE state = ?;

-- name: DeleteAuthRequest :exec
DELETE FROM oauth_auth_requests
WHERE state = ?;

-- name: DeleteExpiredAuthRequests :execrows
DELETE FROM oauth_auth_requests
WHERE expires_at < ?;
