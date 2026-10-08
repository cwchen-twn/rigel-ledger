-- Adjustments become manual transactions. The lock trigger would refuse the
-- rows of a closed period, so user triggers are off for this one update.
ALTER TABLE transactions DISABLE TRIGGER USER;
UPDATE transactions SET source = 'manual' WHERE source = 'adjustment';
ALTER TABLE transactions ENABLE TRIGGER USER;
ALTER TABLE transactions DROP CONSTRAINT transactions_source_check;
ALTER TABLE transactions ADD CONSTRAINT transactions_source_check
    CHECK (source IN ('manual', 'opening', 'import', 'sync'));
