-- Two-factor sign-in: email codes, TOTP, passkeys and recovery codes, with
-- the session's assurance level. Enforced by system_settings.mfa_required
-- (000002 stored the switch; this makes it bite).

-- 1 = password (or a link) only; 2 = a second factor, or a passkey with
-- user verification. Existing sessions are level 1.
ALTER TABLE sessions ADD COLUMN aal SMALLINT NOT NULL DEFAULT 1 CHECK (aal IN (1, 2));

ALTER TABLE users
    -- The WebAuthn user handle: random, never shown, created on the first passkey.
    ADD COLUMN webauthn_id    BYTEA UNIQUE,
    -- Mail the user when a sign-in comes from an address not seen lately.
    ADD COLUMN signin_alerts  BOOLEAN NOT NULL DEFAULT true;

-- Email and TOTP factors, one row each per user. confirmed_at NULL is an
-- enrolment in progress, which counts for nothing.
CREATE TABLE mfa_factors (
    id           BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id      BIGINT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    kind         TEXT NOT NULL CHECK (kind IN ('email', 'totp')),
    -- TOTP seed, AES-GCM sealed with APP_ENCRYPTION_KEY.
    secret_enc   BYTEA,
    -- The last TOTP time step accepted: a code is never accepted twice.
    last_step    BIGINT NOT NULL DEFAULT 0,
    confirmed_at TIMESTAMPTZ,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (user_id, kind),
    CHECK (kind <> 'totp' OR secret_enc IS NOT NULL)
);

CREATE TABLE webauthn_credentials (
    id            BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id       BIGINT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    credential_id BYTEA NOT NULL UNIQUE,
    -- The library's Credential record (public key, sign count, flags).
    data          JSONB NOT NULL,
    name          TEXT NOT NULL DEFAULT '',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_used_at  TIMESTAMPTZ
);
CREATE INDEX webauthn_credentials_user ON webauthn_credentials (user_id);

-- Ten single-use codes, stored as SHA-256.
CREATE TABLE mfa_recovery_codes (
    id         BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id    BIGINT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    code_hash  BYTEA NOT NULL UNIQUE,
    used_at    TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX mfa_recovery_codes_user ON mfa_recovery_codes (user_id);

-- Short-lived state between two requests of a ceremony: the second step of a
-- sign-in, a WebAuthn registration or assertion, an email-factor enrolment.
-- Only the token's hash is stored, as for sessions.
CREATE TABLE auth_challenges (
    id         BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id    BIGINT REFERENCES users (id) ON DELETE CASCADE,
    kind       TEXT NOT NULL CHECK (kind IN ('login', 'passkey_register', 'passkey_login', 'email_enroll')),
    token_hash BYTEA NOT NULL UNIQUE,
    -- "web" or "api": what the finished sign-in opens.
    client     TEXT NOT NULL DEFAULT 'web',
    -- WebAuthn SessionData, or the kind's own state.
    payload    JSONB NOT NULL DEFAULT '{}',
    -- An emailed 6-digit code, hashed.
    code_hash  BYTEA,
    code_sent_at TIMESTAMPTZ,
    attempts   INT NOT NULL DEFAULT 0,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX auth_challenges_expires ON auth_challenges (expires_at);
