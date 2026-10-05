-- name: GetSystemSettings :one
SELECT * FROM system_settings WHERE id;

-- name: UpdateSystemSettings :one
UPDATE system_settings SET
    registration             = @registration,
    mfa_required             = @mfa_required,
    mfa_methods              = @mfa_methods,
    default_language         = @default_language,
    default_display_currency = @default_display_currency,
    default_timezone         = @default_timezone,
    default_date_format      = @default_date_format,
    default_theme            = @default_theme,
    session_ttl_seconds      = sqlc.narg(session_ttl_seconds),
    invite_ttl_seconds       = @invite_ttl_seconds,
    login_max_failures       = @login_max_failures,
    login_ip_max_failures    = @login_ip_max_failures,
    login_user_max_failures  = @login_user_max_failures,
    login_window_seconds     = @login_window_seconds,
    updated_by               = sqlc.narg(updated_by)
WHERE id
RETURNING *;

-- name: UpdateMailSettings :one
-- smtp_pass_enc is kept when the new value is NULL: the API never returns
-- the password, so an unchanged form sends none.
UPDATE system_settings SET
    mail_configured = true,
    mail_driver     = @mail_driver,
    smtp_host       = @smtp_host,
    smtp_port       = @smtp_port,
    smtp_security   = @smtp_security,
    smtp_user       = @smtp_user,
    smtp_pass_enc   = coalesce(sqlc.narg(smtp_pass_enc), smtp_pass_enc),
    mail_from       = @mail_from,
    mail_from_name  = @mail_from_name,
    updated_by      = sqlc.narg(updated_by)
WHERE id
RETURNING *;

-- name: ClearSMTPPassword :exec
UPDATE system_settings SET smtp_pass_enc = NULL WHERE id;
