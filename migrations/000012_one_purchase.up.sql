-- #53-#55: one purchase, however many sources see it. Two more proposals:
--   same_invoice  an invoice for a purchase another source's invoice already
--                 added its lines to (an order email and its 電子發票):
--                 accepting keeps only its file. match_transaction_id, or
--                 match_row_id while that other invoice waits.
--   waiting       an invoice nothing paid for yet, younger than a week: a
--                 card charge may still come. Booked as cash only by choice.
-- And 'duplicate' may now name a waiting row (match_row_id): the same charge
-- sent by another source, settled when that row is accepted.
ALTER TABLE import_rows DROP CONSTRAINT import_rows_proposal_check;
ALTER TABLE import_rows ADD CONSTRAINT import_rows_proposal_check
    CHECK (proposal IN ('new', 'duplicate', 'clears', 'transfer', 'enrich', 'same_invoice', 'waiting'));
