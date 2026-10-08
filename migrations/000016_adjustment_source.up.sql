-- #87: a transaction that closes the gap between the books and a balance
-- the person states (cash spent that no source saw, a missing statement)
-- is booked with its own source, so lists and reports can tell it apart.
ALTER TABLE transactions DROP CONSTRAINT transactions_source_check;
ALTER TABLE transactions ADD CONSTRAINT transactions_source_check
    CHECK (source IN ('manual', 'opening', 'import', 'sync', 'adjustment'));
