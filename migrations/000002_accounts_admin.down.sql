DROP TRIGGER IF EXISTS system_settings_audit ON system_settings;
DROP TABLE IF EXISTS auth_events;
DROP TABLE IF EXISTS access_requests;
DROP TABLE IF EXISTS email_tokens;
DROP TABLE IF EXISTS system_settings;

-- audit_row() as 000001 defined it.
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

ALTER TABLE sessions DROP COLUMN IF EXISTS ip;

-- Users who never got an address or a password cannot survive the old
-- constraints; they had no way to sign in anyway.
DELETE FROM users WHERE email = '';
ALTER TABLE users
    DROP CONSTRAINT IF EXISTS users_initialized_email,
    DROP COLUMN IF EXISTS invited_by,
    DROP COLUMN IF EXISTS password_must_change,
    DROP COLUMN IF EXISTS initialized_at,
    DROP COLUMN IF EXISTS email_verified_at;
DROP INDEX IF EXISTS users_email_key;
ALTER TABLE users ADD CONSTRAINT users_email_key UNIQUE (email);
ALTER TABLE users ALTER COLUMN email DROP DEFAULT;
ALTER TABLE users ALTER COLUMN password_hash DROP DEFAULT;
