UPDATE import_rows SET proposal = 'new', match_transaction_id = NULL, match_row_id = NULL
WHERE proposal IN ('same_invoice', 'waiting') OR (proposal = 'duplicate' AND match_transaction_id IS NULL);
ALTER TABLE import_rows DROP CONSTRAINT import_rows_proposal_check;
ALTER TABLE import_rows ADD CONSTRAINT import_rows_proposal_check
    CHECK (proposal IN ('new', 'duplicate', 'clears', 'transfer', 'enrich'));
