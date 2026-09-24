DROP TABLE IF EXISTS balance_assertions;
ALTER TABLE IF EXISTS import_rows DROP CONSTRAINT IF EXISTS import_rows_rule_fk;
DROP TABLE IF EXISTS import_rules;
DROP TABLE IF EXISTS import_rows;
DROP TABLE IF EXISTS import_batches;
DROP TABLE IF EXISTS source_accounts;
DELETE FROM sessions WHERE kind = 'token';
ALTER TABLE sessions DROP CONSTRAINT sessions_kind_check;
ALTER TABLE sessions ADD CONSTRAINT sessions_kind_check CHECK (kind IN ('web', 'api'));
