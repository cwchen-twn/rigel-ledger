-- #36: files on transactions -- the receipt a person photographs, and the
-- evidence a source sends with a row (an order email, an e-invoice, a
-- statement). The bytes live in PostgreSQL so the nightly pg_dump keeps them
-- with the books; a book holds one copy per content (sha256), however many
-- transactions point at it. Only images and PDFs: never HTML or SVG, which
-- would run script when opened from the app's origin.
CREATE TABLE attachments (
    id         BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    book_id    BIGINT NOT NULL REFERENCES books (id) ON DELETE CASCADE,
    sha256     BYTEA NOT NULL CHECK (octet_length(sha256) = 32),
    filename   TEXT NOT NULL CHECK (filename <> '' AND char_length(filename) <= 200),
    mime       TEXT NOT NULL CHECK (mime IN ('image/jpeg', 'image/png', 'image/webp', 'application/pdf')),
    size       INT NOT NULL CHECK (size > 0 AND size <= 10485760),
    bytes      BYTEA NOT NULL,
    created_by BIGINT REFERENCES users (id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (size = octet_length(bytes)),
    UNIQUE (book_id, sha256)
);
-- JPEG, PNG, WebP and PDF are compressed already: store, do not recompress.
ALTER TABLE attachments ALTER COLUMN bytes SET STORAGE EXTERNAL;

CREATE TABLE transaction_attachments (
    id             BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    book_id        BIGINT NOT NULL REFERENCES books (id) ON DELETE CASCADE,
    transaction_id BIGINT NOT NULL REFERENCES transactions (id) ON DELETE CASCADE,
    attachment_id  BIGINT NOT NULL REFERENCES attachments (id) ON DELETE CASCADE,
    created_by     BIGINT REFERENCES users (id) ON DELETE SET NULL,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (transaction_id, attachment_id)
);
CREATE INDEX transaction_attachments_attachment ON transaction_attachments (attachment_id);

-- A staged row's evidence; accepting the row links it to the transaction.
ALTER TABLE import_rows ADD COLUMN attachment_id BIGINT REFERENCES attachments (id) ON DELETE SET NULL;
CREATE INDEX import_rows_attachment ON import_rows (attachment_id) WHERE attachment_id IS NOT NULL;

-- A link, or a row's evidence, stays inside one book.
CREATE FUNCTION check_attachment_book() RETURNS trigger AS $$
BEGIN
    IF TG_TABLE_NAME = 'transaction_attachments' AND NOT EXISTS (
        SELECT 1 FROM transactions WHERE id = NEW.transaction_id AND book_id = NEW.book_id
    ) OR NEW.attachment_id IS NOT NULL AND NOT EXISTS (
        SELECT 1 FROM attachments WHERE id = NEW.attachment_id AND book_id = NEW.book_id
    ) THEN
        RAISE EXCEPTION 'an attachment belongs to the book of what it is attached to'
            USING ERRCODE = 'check_violation', CONSTRAINT = 'attachment_book';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
CREATE TRIGGER transaction_attachments_book BEFORE INSERT OR UPDATE ON transaction_attachments
    FOR EACH ROW EXECUTE FUNCTION check_attachment_book();
CREATE TRIGGER import_rows_attachment_book BEFORE INSERT OR UPDATE OF attachment_id ON import_rows
    FOR EACH ROW EXECUTE FUNCTION check_attachment_book();

-- A file nothing points at any more (its last link removed, its transaction
-- or staged row deleted) goes with it.
CREATE FUNCTION drop_orphan_attachment() RETURNS trigger AS $$
BEGIN
    DELETE FROM attachments a
    WHERE a.id = OLD.attachment_id
      AND NOT EXISTS (SELECT 1 FROM transaction_attachments l WHERE l.attachment_id = a.id)
      AND NOT EXISTS (SELECT 1 FROM import_rows r WHERE r.attachment_id = a.id);
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;
CREATE TRIGGER transaction_attachments_gc AFTER DELETE ON transaction_attachments
    FOR EACH ROW EXECUTE FUNCTION drop_orphan_attachment();
CREATE TRIGGER import_rows_attachment_gc AFTER DELETE OR UPDATE OF attachment_id ON import_rows
    FOR EACH ROW WHEN (OLD.attachment_id IS NOT NULL) EXECUTE FUNCTION drop_orphan_attachment();

CREATE OR REPLACE FUNCTION audit_row() RETURNS trigger AS $$
DECLARE
    r        JSONB;
    v_book   BIGINT;
    v_row    BIGINT;
    v_user   BIGINT;
BEGIN
    -- An attachment's bytes never reach the audit log (a receipt would be
    -- copied into it, hex-encoded): its metadata does, built without them.
    IF TG_TABLE_NAME = 'attachments' THEN
        INSERT INTO audit_log (book_id, table_name, row_id, action, old_values, new_values, changed_by)
        VALUES (
            CASE WHEN TG_OP = 'DELETE' THEN OLD.book_id ELSE NEW.book_id END,
            TG_TABLE_NAME,
            CASE WHEN TG_OP = 'DELETE' THEN OLD.id ELSE NEW.id END,
            TG_OP,
            CASE WHEN TG_OP IN ('UPDATE', 'DELETE') THEN jsonb_build_object(
                'id', OLD.id, 'book_id', OLD.book_id, 'sha256', encode(OLD.sha256, 'hex'),
                'filename', OLD.filename, 'mime', OLD.mime, 'size', OLD.size) END,
            CASE WHEN TG_OP IN ('INSERT', 'UPDATE') THEN jsonb_build_object(
                'id', NEW.id, 'book_id', NEW.book_id, 'sha256', encode(NEW.sha256, 'hex'),
                'filename', NEW.filename, 'mime', NEW.mime, 'size', NEW.size) END,
            nullif(current_setting('app.current_user', true), '')::BIGINT
        );
        RETURN NULL;
    END IF;

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

CREATE TRIGGER attachments_audit AFTER INSERT OR UPDATE OR DELETE ON attachments
    FOR EACH ROW EXECUTE FUNCTION audit_row();
CREATE TRIGGER transaction_attachments_audit AFTER INSERT OR UPDATE OR DELETE ON transaction_attachments
    FOR EACH ROW EXECUTE FUNCTION audit_row();
