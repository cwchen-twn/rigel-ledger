-- name: CreateEmailToken :one
INSERT INTO email_tokens (user_id, kind, email, token_hash, code_hash, payload, expires_at, created_by)
VALUES (sqlc.narg(user_id), @kind, @email, sqlc.narg(token_hash), sqlc.narg(code_hash), @payload,
        @expires_at, sqlc.narg(created_by))
RETURNING *;

-- name: GetEmailTokenByHash :one
-- A link token that can still be used.
SELECT * FROM email_tokens
WHERE token_hash = @token_hash AND used_at IS NULL AND expires_at > now();

-- name: GetOpenEmailToken :one
-- The newest open token of a kind for a user (the pending email change, the
-- open invitation).
SELECT * FROM email_tokens
WHERE user_id = @user_id AND kind = @kind AND used_at IS NULL AND expires_at > now()
ORDER BY id DESC
LIMIT 1;

-- name: GetEmailToken :one
SELECT * FROM email_tokens WHERE id = @id;

-- name: BumpEmailTokenAttempts :one
UPDATE email_tokens SET attempts = attempts + 1 WHERE id = @id RETURNING attempts;

-- name: UseEmailToken :execrows
UPDATE email_tokens SET used_at = now() WHERE id = @id AND used_at IS NULL;

-- name: RevokeEmailTokens :exec
-- Closing a kind for a user: a new code replaces the old, a revoked
-- invitation stops working.
UPDATE email_tokens SET used_at = now()
WHERE user_id = @user_id AND kind = @kind AND used_at IS NULL;

-- name: OpenRegistrationExists :one
-- Another sign-up still waiting for its link with this username or address.
SELECT EXISTS (
    SELECT 1 FROM email_tokens
    WHERE kind = 'register' AND used_at IS NULL AND expires_at > now()
      AND (lower(email) = lower(@email) OR payload ->> 'username' = @username::TEXT)
);

-- name: DeleteStaleEmailTokens :execrows
DELETE FROM email_tokens WHERE expires_at < now() - INTERVAL '30 days';

-- name: CountRecentEmailTokens :one
-- How many codes or links of a kind were sent to a user lately: the resend cap.
SELECT count(*) FROM email_tokens
WHERE user_id = @user_id AND kind = @kind
  AND created_at > now() - make_interval(secs => @window_seconds::BIGINT);
