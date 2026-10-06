DELETE FROM import_rows WHERE kind IN ('holding', 'trade');
ALTER TABLE import_rows
    DROP CONSTRAINT import_rows_shape,
    DROP COLUMN cash,
    DROP COLUMN price,
    DROP COLUMN units,
    DROP COLUMN security,
    DROP CONSTRAINT import_rows_kind_check;
ALTER TABLE import_rows ADD CONSTRAINT import_rows_kind_check CHECK (kind IN ('transaction', 'balance'));
DROP TABLE IF EXISTS source_securities;
ALTER TABLE source_accounts DROP COLUMN settlement_account_id, DROP COLUMN kind;
