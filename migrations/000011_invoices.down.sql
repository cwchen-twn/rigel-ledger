DROP TABLE IF EXISTS transaction_items;
DELETE FROM import_rows WHERE kind = 'invoice';
UPDATE import_rows SET proposal = 'new', match_transaction_id = NULL, match_row_id = NULL WHERE proposal = 'enrich';
ALTER TABLE import_rows DROP CONSTRAINT import_rows_proposal_check;
ALTER TABLE import_rows ADD CONSTRAINT import_rows_proposal_check
    CHECK (proposal IN ('new', 'duplicate', 'clears', 'transfer'));
ALTER TABLE import_rows DROP CONSTRAINT import_rows_items, DROP COLUMN items, DROP CONSTRAINT import_rows_kind_check;
ALTER TABLE import_rows ADD CONSTRAINT import_rows_kind_check CHECK (kind IN ('transaction', 'balance', 'holding', 'trade'));
