-- name: CreateSession :one
INSERT INTO sessions (user_id, token_hash, kind, label, user_agent, expires_at)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: GetSessionUser :one
-- The session and its user, only while both are valid.
SELECT sqlc.embed(s), sqlc.embed(u)
FROM sessions s
JOIN users u ON u.id = s.user_id
WHERE s.token_hash = $1
  AND s.expires_at > now()
  AND u.is_active;

-- name: TouchSession :exec
UPDATE sessions SET last_used_at = now(), expires_at = $2 WHERE id = $1;

-- name: DeleteSessionByTokenHash :exec
DELETE FROM sessions WHERE token_hash = $1;

-- name: DeleteUserSessionsExcept :exec
-- After a password change: sign out every other device.
DELETE FROM sessions WHERE user_id = $1 AND id <> $2;

-- name: DeleteExpiredSessions :execrows
DELETE FROM sessions WHERE expires_at <= now();
