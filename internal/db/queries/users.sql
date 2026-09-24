-- name: CreateUser :one
INSERT INTO users (username, email, password_hash, display_name, language, display_currency, timezone, is_admin)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;

-- name: GetUserByID :one
SELECT * FROM users WHERE id = $1;

-- name: GetUserByUsername :one
SELECT * FROM users WHERE username = $1;

-- name: UpdateUserSettings :one
UPDATE users SET
    display_name     = @display_name,
    language         = @language,
    display_currency = @display_currency,
    timezone         = @timezone,
    date_format      = @date_format,
    theme            = @theme,
    default_book_id  = sqlc.narg(default_book_id)
WHERE id = @id
RETURNING *;

-- name: UpdateUserPassword :exec
UPDATE users SET password_hash = $2 WHERE id = $1;

-- name: SetUserAdmin :exec
UPDATE users SET is_admin = $2 WHERE id = $1;

-- name: TouchUserLogin :exec
UPDATE users SET last_login_at = now() WHERE id = $1;

-- name: SetDefaultBookIfUnset :exec
UPDATE users SET default_book_id = $2 WHERE id = $1 AND default_book_id IS NULL;

-- name: UpdateUserIdentity :one
UPDATE users SET username = @username, email = @email WHERE id = @id
RETURNING *;

-- name: GetUserByEmail :one
SELECT * FROM users WHERE email <> '' AND lower(email) = lower(@email);

-- name: UsernameTaken :one
SELECT EXISTS (SELECT 1 FROM users WHERE username = @username AND id <> @except_id);

-- name: EmailTaken :one
SELECT EXISTS (SELECT 1 FROM users WHERE email <> '' AND lower(email) = lower(@email) AND id <> @except_id);

-- name: CountAdmins :one
SELECT count(*) FROM users WHERE is_admin AND is_active;

-- name: CreateInitialUser :one
-- An invited or bootstrapped user: no password or no address yet, and the
-- first-login wizard still ahead.
INSERT INTO users (username, email, password_hash, display_name, language, display_currency,
                   timezone, date_format, theme, is_admin, password_must_change, invited_by)
VALUES (@username, @email, @password_hash, @display_name, @language, @display_currency,
        @timezone, @date_format, @theme, @is_admin, @password_must_change, sqlc.narg(invited_by))
RETURNING *;

-- name: SetUserEmail :one
UPDATE users SET email = @email, email_verified_at = sqlc.narg(verified_at) WHERE id = @id
RETURNING *;

-- name: SetUserUsername :one
UPDATE users SET username = @username WHERE id = @id
RETURNING *;

-- name: SetUserPassword :exec
UPDATE users SET password_hash = @password_hash, password_must_change = false WHERE id = @id;

-- name: MarkUserInitialized :one
UPDATE users SET initialized_at = now() WHERE id = @id AND initialized_at IS NULL
RETURNING *;

-- name: SetUserActive :exec
UPDATE users SET is_active = @is_active WHERE id = @id;

-- name: ListUsers :many
-- The admin's list, with whether an invitation is still open.
SELECT sqlc.embed(u),
       EXISTS (SELECT 1 FROM email_tokens t
               WHERE t.user_id = u.id AND t.kind = 'invite' AND t.used_at IS NULL
                 AND t.expires_at > now())::BOOLEAN AS invite_pending
FROM users u
ORDER BY u.id;

-- name: DeleteUser :exec
DELETE FROM users WHERE id = @id;
