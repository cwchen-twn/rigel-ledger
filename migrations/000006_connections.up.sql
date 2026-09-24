-- P4c-1: each user links their own sources (Settings -> Connections). The
-- credentials are sealed in the browser to the sync runner's X25519 key
-- (internal/sealing), so this database holds ciphertext the app itself
-- cannot open; only the runner, holding the private key, can.

-- A system-level runner token: reaches /api/runner/* and nothing else.
ALTER TABLE sessions DROP CONSTRAINT sessions_kind_check;
ALTER TABLE sessions ADD CONSTRAINT sessions_kind_check CHECK (kind IN ('web', 'api', 'token', 'runner'));

-- The runner's public keys. The newest unretired one is what browsers seal
-- to; a connection sealed to a retired key must be re-entered.
CREATE TABLE runner_keys (
    id         BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    public_key BYTEA NOT NULL UNIQUE CHECK (length(public_key) = 32),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    retired_at TIMESTAMPTZ
);

-- What the runner can connect to, as it last published it. Field labels are
-- English defaults; the UI prefers connector.<id>.* translation keys.
CREATE TABLE runner_connectors (
    id         TEXT PRIMARY KEY CHECK (id ~ '^[a-z0-9][a-z0-9_-]{0,39}$'),
    name       TEXT NOT NULL,
    country    TEXT NOT NULL DEFAULT '',
    fields     JSONB NOT NULL DEFAULT '[]',
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE connections (
    id               BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id          BIGINT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    book_id          BIGINT NOT NULL REFERENCES books (id) ON DELETE CASCADE,
    connector        TEXT NOT NULL REFERENCES runner_connectors (id),
    label            TEXT NOT NULL DEFAULT '',
    sealed           BYTEA NOT NULL,
    key_id           BIGINT NOT NULL REFERENCES runner_keys (id),
    enabled          BOOLEAN NOT NULL DEFAULT true,
    interval_hours   SMALLINT NOT NULL DEFAULT 24 CHECK (interval_hours BETWEEN 1 AND 168),
    status           TEXT NOT NULL DEFAULT 'new'
                     CHECK (status IN ('new', 'ok', 'needs_user_action', 'failed')),
    -- A stable code from the runner (bad_credentials, challenge_expired, ...),
    -- never the institution's own message.
    last_error       TEXT NOT NULL DEFAULT '',
    last_run_at      TIMESTAMPTZ,
    run_requested_at TIMESTAMPTZ,
    claimed_at       TIMESTAMPTZ,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX connections_user ON connections (user_id);
CREATE INDEX connections_book ON connections (book_id);

-- An OTP, CAPTCHA or new-device check the runner cannot pass alone. The
-- person answers in the app; the answer is sealed to the runner as well.
CREATE TABLE connection_challenges (
    id            BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    connection_id BIGINT NOT NULL REFERENCES connections (id) ON DELETE CASCADE,
    kind          TEXT NOT NULL CHECK (kind IN ('otp', 'captcha', 'device')),
    prompt        TEXT NOT NULL DEFAULT '',
    image         BYTEA,
    answer_sealed BYTEA,
    expires_at    TIMESTAMPTZ NOT NULL,
    answered_at   TIMESTAMPTZ,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX connection_challenges_connection ON connection_challenges (connection_id);

-- The audit copy of a connection leaves the ciphertext out: an audit row is
-- read by people, and the blob is no business of theirs.
CREATE FUNCTION audit_connection() RETURNS trigger AS $$
DECLARE
    v_user BIGINT := nullif(current_setting('app.current_user', true), '')::BIGINT;
BEGIN
    INSERT INTO audit_log (book_id, table_name, row_id, action, old_values, new_values, changed_by)
    VALUES (
        CASE WHEN TG_OP = 'DELETE' THEN OLD.book_id ELSE NEW.book_id END,
        TG_TABLE_NAME,
        CASE WHEN TG_OP = 'DELETE' THEN OLD.id ELSE NEW.id END,
        TG_OP,
        CASE WHEN TG_OP IN ('UPDATE', 'DELETE') THEN to_jsonb(OLD) - 'sealed' END,
        CASE WHEN TG_OP IN ('INSERT', 'UPDATE') THEN to_jsonb(NEW) - 'sealed' END,
        v_user
    );
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

-- Only changes a person makes are audited: the runner's bookkeeping
-- (claimed_at, status, last_run_at) would bury them.
CREATE TRIGGER connections_audit AFTER INSERT OR DELETE OR UPDATE OF
    book_id, connector, label, sealed, key_id, enabled, interval_hours ON connections
    FOR EACH ROW EXECUTE FUNCTION audit_connection();
