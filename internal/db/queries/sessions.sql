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

-- name: CreateSessionWithIP :one
INSERT INTO sessions (user_id, token_hash, kind, label, user_agent, expires_at, ip)
VALUES (@user_id, @token_hash, @kind, @label, @user_agent, @expires_at, sqlc.narg(ip)::inet)
RETURNING *;

-- name: ListUserSessions :many
SELECT id, kind, label, user_agent, coalesce(host(ip), '')::TEXT AS ip, created_at, last_used_at, expires_at
FROM sessions
WHERE user_id = @user_id AND expires_at > now()
ORDER BY last_used_at DESC;

-- name: DeleteUserSession :execrows
DELETE FROM sessions WHERE id = @id AND user_id = @user_id;

-- name: DeleteUserSessions :exec
DELETE FROM sessions WHERE user_id = @user_id;
