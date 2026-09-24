-- Accounts and administration: invitations, registration modes, verified
-- email, the first-login wizard, system settings, sign-in throttling and its
-- audit. The first migration written after the first deployment: 000001 is
-- frozen from here on.

-- ---------------------------------------------------------------------------
-- Users
-- ---------------------------------------------------------------------------

-- '' means "not set": an invited user has no password until they accept, and
-- a bootstrap admin created without ADMIN_EMAIL has no address until the
-- wizard. bcrypt never matches '', so neither can sign in with a password
-- they do not have.
ALTER TABLE users ALTER COLUMN password_hash SET DEFAULT '';
ALTER TABLE users ALTER COLUMN email SET DEFAULT '';

-- Addresses are unique regardless of case, and '' may repeat.
ALTER TABLE users DROP CONSTRAINT users_email_key;
CREATE UNIQUE INDEX users_email_key ON users (lower(email)) WHERE email <> '';

ALTER TABLE users
    ADD COLUMN email_verified_at    TIMESTAMPTZ,
    -- NULL until the first-login wizard is finished. Existing users walk it
    -- once, which is also how their address gets verified.
    ADD COLUMN initialized_at       TIMESTAMPTZ,
    ADD COLUMN password_must_change BOOLEAN NOT NULL DEFAULT false,
    ADD COLUMN invited_by           BIGINT REFERENCES users (id) ON DELETE SET NULL,
    ADD CONSTRAINT users_initialized_email CHECK (initialized_at IS NULL OR email <> '');

ALTER TABLE sessions ADD COLUMN ip INET;

-- ---------------------------------------------------------------------------
-- System settings: one row, edited on the Administration page
-- ---------------------------------------------------------------------------

CREATE TABLE system_settings (
    id                       BOOLEAN PRIMARY KEY DEFAULT true CHECK (id),

    registration             TEXT NOT NULL DEFAULT 'closed'
                             CHECK (registration IN ('closed', 'request', 'open')),
    -- Enforced from the two-factor migration on.
    mfa_required             BOOLEAN NOT NULL DEFAULT true,
    mfa_methods              TEXT[] NOT NULL DEFAULT '{email,totp,passkey}'
                             CHECK (mfa_methods <@ '{email,totp,passkey}'::TEXT[]),

    -- Defaults for new users (invited, registered, bootstrapped).
    default_language         TEXT NOT NULL DEFAULT 'en' CHECK (default_language IN ('en', 'zh', 'es')),
    default_display_currency TEXT NOT NULL DEFAULT 'USD' REFERENCES commodities (code),
    default_timezone         TEXT NOT NULL DEFAULT 'UTC',
    default_date_format      TEXT NOT NULL DEFAULT 'YYYY-MM-DD'
                             CHECK (default_date_format IN ('YYYY-MM-DD', 'DD/MM/YYYY', 'MM/DD/YYYY', 'YYYY/MM/DD')),
    default_theme            TEXT NOT NULL DEFAULT 'system' CHECK (default_theme IN ('system', 'light', 'dark')),

    -- Durations are whole seconds. NULL session_ttl: the SESSION_TTL environment variable.
    session_ttl_seconds      BIGINT CHECK (session_ttl_seconds >= 300),
    invite_ttl_seconds       BIGINT NOT NULL DEFAULT 604800 CHECK (invite_ttl_seconds >= 3600),

    -- Failed sign-ins tolerated inside login_window, per key.
    login_max_failures       INT NOT NULL DEFAULT 5  CHECK (login_max_failures > 0),      -- username + ip
    login_ip_max_failures    INT NOT NULL DEFAULT 20 CHECK (login_ip_max_failures > 0),   -- ip
    login_user_max_failures  INT NOT NULL DEFAULT 20 CHECK (login_user_max_failures > 0), -- username
    login_window_seconds     BIGINT NOT NULL DEFAULT 900 CHECK (login_window_seconds >= 60),

    -- Mail. The password is AES-GCM sealed with APP_ENCRYPTION_KEY, so a dump
    -- or a backup alone does not reveal it. mail_configured stays false until
    -- an admin saves (or the environment seeds) the mail settings once.
    mail_configured          BOOLEAN NOT NULL DEFAULT false,
    mail_driver              TEXT NOT NULL DEFAULT 'log' CHECK (mail_driver IN ('smtp', 'log', 'off')),
    smtp_host                TEXT NOT NULL DEFAULT '',
    smtp_port                INT NOT NULL DEFAULT 587 CHECK (smtp_port BETWEEN 1 AND 65535),
    smtp_security            TEXT NOT NULL DEFAULT 'starttls' CHECK (smtp_security IN ('starttls', 'tls', 'none')),
    smtp_user                TEXT NOT NULL DEFAULT '',
    smtp_pass_enc            BYTEA,
    mail_from                TEXT NOT NULL DEFAULT '',
    mail_from_name           TEXT NOT NULL DEFAULT 'RigelLedger',

    updated_by               BIGINT REFERENCES users (id) ON DELETE SET NULL,
    updated_at               TIMESTAMPTZ NOT NULL DEFAULT now()
);
INSERT INTO system_settings DEFAULT VALUES;
CREATE TRIGGER system_settings_updated_at BEFORE UPDATE ON system_settings
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- ---------------------------------------------------------------------------
-- Email tokens: links for signed-out flows, 6-digit codes for signed-in ones
-- ---------------------------------------------------------------------------

-- Only hashes are stored, as for sessions. A 'register' row carries the
-- would-be user in payload, so an unverified sign-up never creates a user.
CREATE TABLE email_tokens (
    id         BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id    BIGINT REFERENCES users (id) ON DELETE CASCADE,
    kind       TEXT NOT NULL CHECK (kind IN ('invite', 'verify', 'register')),
    email      TEXT NOT NULL,
    token_hash BYTEA UNIQUE,
    code_hash  BYTEA,
    payload    JSONB NOT NULL DEFAULT '{}',
    attempts   INT NOT NULL DEFAULT 0,
    expires_at TIMESTAMPTZ NOT NULL,
    used_at    TIMESTAMPTZ,
    created_by BIGINT REFERENCES users (id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (token_hash IS NOT NULL OR code_hash IS NOT NULL),
    CHECK (kind = 'register' OR user_id IS NOT NULL)
);
CREATE INDEX email_tokens_user ON email_tokens (user_id, kind);

-- ---------------------------------------------------------------------------
-- Access requests (registration = 'request')
-- ---------------------------------------------------------------------------

CREATE TABLE access_requests (
    id         BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    username   TEXT NOT NULL,
    email      TEXT NOT NULL,
    message    TEXT NOT NULL DEFAULT '',
    ip         INET,
    status     TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'approved', 'rejected')),
    decided_by BIGINT REFERENCES users (id) ON DELETE SET NULL,
    decided_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX access_requests_status ON access_requests (status, created_at DESC);

-- ---------------------------------------------------------------------------
-- Sign-in events: the throttle's source and the security audit
-- ---------------------------------------------------------------------------

CREATE TABLE auth_events (
    id         BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    username   TEXT NOT NULL DEFAULT '',
    user_id    BIGINT REFERENCES users (id) ON DELETE SET NULL,
    ip         INET,
    user_agent TEXT NOT NULL DEFAULT '',
    event      TEXT NOT NULL,
    -- Counts against the throttle.
    failure    BOOLEAN NOT NULL DEFAULT false,
    detail     JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX auth_events_username ON auth_events (username, created_at) WHERE failure;
CREATE INDEX auth_events_ip ON auth_events (ip, created_at) WHERE failure;
CREATE INDEX auth_events_user ON auth_events (user_id, created_at DESC);

-- ---------------------------------------------------------------------------
-- Audit: system_settings has a boolean id and no book
-- ---------------------------------------------------------------------------

CREATE OR REPLACE FUNCTION audit_row() RETURNS trigger AS $$
DECLARE
    r        JSONB;
    v_book   BIGINT;
    v_row    BIGINT;
    v_user   BIGINT;
BEGIN
    r := CASE WHEN TG_OP = 'DELETE' THEN to_jsonb(OLD) ELSE to_jsonb(NEW) END;
    v_user := nullif(current_setting('app.current_user', true), '')::BIGINT;

    IF TG_TABLE_NAME = 'postings' THEN
        SELECT book_id INTO v_book FROM transactions WHERE id = (r ->> 'transaction_id')::BIGINT;
        v_row := (r ->> 'id')::BIGINT;
    ELSIF TG_TABLE_NAME = 'book_members' THEN
        v_book := (r ->> 'book_id')::BIGINT;
        v_row := (r ->> 'user_id')::BIGINT;
    ELSIF TG_TABLE_NAME = 'books' THEN
        v_book := (r ->> 'id')::BIGINT;
        v_row := v_book;
    ELSIF TG_TABLE_NAME = 'system_settings' THEN
        -- The sealed SMTP password never reaches the audit log.
        v_book := NULL;
        v_row := NULL;
        IF TG_OP IN ('UPDATE', 'DELETE') THEN
            INSERT INTO audit_log (book_id, table_name, row_id, action, old_values, new_values, changed_by)
            VALUES (NULL, TG_TABLE_NAME, NULL, TG_OP,
                    to_jsonb(OLD) - 'smtp_pass_enc',
                    CASE WHEN TG_OP = 'UPDATE' THEN to_jsonb(NEW) - 'smtp_pass_enc' END,
                    v_user);
        END IF;
        RETURN NULL;
    ELSE
        v_book := (r ->> 'book_id')::BIGINT;
        v_row := (r ->> 'id')::BIGINT;
    END IF;

    INSERT INTO audit_log (book_id, table_name, row_id, action, old_values, new_values, changed_by)
    VALUES (
        v_book, TG_TABLE_NAME, v_row, TG_OP,
        CASE WHEN TG_OP IN ('UPDATE', 'DELETE') THEN to_jsonb(OLD) END,
        CASE WHEN TG_OP IN ('INSERT', 'UPDATE') THEN to_jsonb(NEW) END,
        v_user
    );
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER system_settings_audit AFTER UPDATE ON system_settings
    FOR EACH ROW EXECUTE FUNCTION audit_row();
