-- name: GetMFAState :one
-- What a user has enrolled, for the login step and the gate.
SELECT
    EXISTS (SELECT 1 FROM mfa_factors f WHERE f.user_id = @user_id AND f.kind = 'totp' AND f.confirmed_at IS NOT NULL)::BOOLEAN AS totp,
    EXISTS (SELECT 1 FROM mfa_factors f WHERE f.user_id = @user_id AND f.kind = 'email' AND f.confirmed_at IS NOT NULL)::BOOLEAN AS email,
    (SELECT count(*) FROM webauthn_credentials w WHERE w.user_id = @user_id)::BIGINT AS passkeys,
    (SELECT count(*) FROM mfa_recovery_codes r WHERE r.user_id = @user_id AND r.used_at IS NULL)::BIGINT AS recovery_left;

-- name: GetFactor :one
SELECT * FROM mfa_factors WHERE user_id = @user_id AND kind = @kind;

-- name: UpsertPendingTOTP :one
-- A new (unconfirmed) TOTP seed; replaces an unfinished enrolment, never a
-- confirmed one.
INSERT INTO mfa_factors (user_id, kind, secret_enc)
VALUES (@user_id, 'totp', @secret_enc)
ON CONFLICT (user_id, kind) DO UPDATE SET secret_enc = EXCLUDED.secret_enc, last_step = 0, created_at = now()
    WHERE mfa_factors.confirmed_at IS NULL
RETURNING *;

-- name: ConfirmFactor :exec
UPDATE mfa_factors SET confirmed_at = now(), last_step = @last_step WHERE id = @id;

-- name: EnableEmailFactor :exec
INSERT INTO mfa_factors (user_id, kind, confirmed_at) VALUES (@user_id, 'email', now())
ON CONFLICT (user_id, kind) DO UPDATE SET confirmed_at = now();

-- name: AdvanceTOTPStep :execrows
-- Accept a TOTP step only if it is newer than the last one: replay-proof
-- even when two requests race.
UPDATE mfa_factors SET last_step = @step WHERE id = @id AND last_step < @step;

-- name: DeleteFactor :execrows
DELETE FROM mfa_factors WHERE user_id = @user_id AND kind = @kind;

-- name: ListPasskeys :many
SELECT id, credential_id, data, name, created_at, last_used_at FROM webauthn_credentials
WHERE user_id = @user_id ORDER BY created_at;

-- name: GetPasskeyByCredentialID :one
SELECT * FROM webauthn_credentials WHERE credential_id = @credential_id;

-- name: CreatePasskey :one
INSERT INTO webauthn_credentials (user_id, credential_id, data, name)
VALUES (@user_id, @credential_id, @data, @name)
RETURNING id, credential_id, data, name, created_at, last_used_at;

-- name: TouchPasskey :exec
UPDATE webauthn_credentials SET data = @data, last_used_at = now() WHERE id = @id;

-- name: DeletePasskey :execrows
DELETE FROM webauthn_credentials WHERE id = @id AND user_id = @user_id;

-- name: SetWebAuthnID :one
UPDATE users SET webauthn_id = coalesce(webauthn_id, @webauthn_id) WHERE id = @id RETURNING webauthn_id;

-- name: GetUserByWebAuthnID :one
SELECT * FROM users WHERE webauthn_id = @webauthn_id;

-- name: DeleteRecoveryCodes :exec
DELETE FROM mfa_recovery_codes WHERE user_id = @user_id;

-- name: CreateRecoveryCode :exec
INSERT INTO mfa_recovery_codes (user_id, code_hash) VALUES (@user_id, @code_hash);

-- name: UseRecoveryCode :execrows
UPDATE mfa_recovery_codes SET used_at = now()
WHERE user_id = @user_id AND code_hash = @code_hash AND used_at IS NULL;

-- name: ResetMFA :exec
-- Break-glass: every factor, passkey and recovery code of a user.
WITH f AS (DELETE FROM mfa_factors WHERE mfa_factors.user_id = @user_id),
     w AS (DELETE FROM webauthn_credentials WHERE webauthn_credentials.user_id = @user_id)
DELETE FROM mfa_recovery_codes WHERE mfa_recovery_codes.user_id = @user_id;

-- name: CreateChallenge :one
INSERT INTO auth_challenges (user_id, kind, token_hash, client, payload, code_hash, expires_at)
VALUES (sqlc.narg(user_id), @kind, @token_hash, @client, @payload, sqlc.narg(code_hash), @expires_at)
RETURNING *;

-- name: GetChallenge :one
SELECT * FROM auth_challenges WHERE token_hash = @token_hash AND kind = @kind AND expires_at > now();

-- name: BumpChallengeAttempts :one
UPDATE auth_challenges SET attempts = attempts + 1 WHERE id = @id RETURNING attempts;

-- name: SetChallengeCode :exec
UPDATE auth_challenges SET code_hash = @code_hash, code_sent_at = now(), attempts = 0 WHERE id = @id;

-- name: DeleteChallenge :execrows
DELETE FROM auth_challenges WHERE id = @id;

-- name: DeleteExpiredChallenges :execrows
DELETE FROM auth_challenges WHERE expires_at < now();

-- name: SetSignInAlerts :exec
UPDATE users SET signin_alerts = @signin_alerts WHERE id = @id;

-- name: SeenFromIP :one
-- Whether this user signed in successfully from ip in the last 90 days (the
-- new-sign-in alert stays quiet then).
SELECT EXISTS (
    SELECT 1 FROM auth_events
    WHERE user_id = @user_id AND ip = sqlc.narg(ip)::inet
      AND event = 'signed_in'
      AND created_at > now() - INTERVAL '90 days'
)::BOOLEAN;
