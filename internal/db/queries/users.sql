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
