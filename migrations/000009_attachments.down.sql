DROP TRIGGER IF EXISTS import_rows_attachment_gc ON import_rows;
DROP TRIGGER IF EXISTS import_rows_attachment_book ON import_rows;
DROP INDEX IF EXISTS import_rows_attachment;
ALTER TABLE import_rows DROP COLUMN IF EXISTS attachment_id;
DROP TABLE IF EXISTS transaction_attachments;
DROP TABLE IF EXISTS attachments;
DROP FUNCTION IF EXISTS drop_orphan_attachment();
DROP FUNCTION IF EXISTS check_attachment_book();

-- audit_row() as 000002 left it.
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
