-- name: SetPublicFollowing :exec
INSERT INTO user_settings (did, public_following, updated_at) VALUES (?, ?, ?)
ON CONFLICT (did) DO UPDATE SET
    public_following = excluded.public_following,
    updated_at       = excluded.updated_at;

-- name: PublicFollowingEnabled :one
SELECT EXISTS (SELECT 1 FROM user_settings WHERE did = ? AND public_following = 1);
