-- Fix update_ledger_balance trigger: change from BEFORE to AFTER INSERT
DROP TRIGGER IF EXISTS trigger_update_ledger_balance ON user_ledger_postings;

CREATE OR REPLACE FUNCTION update_ledger_balance()
RETURNS TRIGGER AS $$
DECLARE
    ledger_type VARCHAR(4);
    old_ledger_value JSONB;
    new_ledger_value JSONB;
BEGIN
    SELECT first_grade INTO ledger_type
    FROM ref_ledger_types
    WHERE ledger_type_id = (
        SELECT ledger_type_id FROM user_ledgers WHERE ledger_id = NEW.ledger_id
    );

    -- Skip balance update for income/expense accounts (multi-currency, calculated via reports)
    IF ledger_type IN ('4', '5', '6', '7', 'A', 'B') THEN
        RETURN NEW;
    END IF;

    IF (SELECT currency FROM user_ledgers WHERE ledger_id = NEW.ledger_id) != NEW.currency_code THEN
        RAISE EXCEPTION 'Posting currency does not match ledger currency';
    END IF;

    SELECT TO_JSONB(t) INTO old_ledger_value FROM (
        SELECT ledger_id, balance, updated_at FROM user_ledgers WHERE ledger_id = NEW.ledger_id
    ) t;

    IF NEW.posting_type = 'D' AND ledger_type = '1' THEN
        UPDATE user_ledgers SET balance = balance + NEW.amount, updated_at = CURRENT_TIMESTAMP WHERE ledger_id = NEW.ledger_id;
    ELSIF NEW.posting_type = 'C' AND ledger_type = '1' THEN
        UPDATE user_ledgers SET balance = balance - NEW.amount, updated_at = CURRENT_TIMESTAMP WHERE ledger_id = NEW.ledger_id;
    ELSIF NEW.posting_type = 'D' AND ledger_type IN ('2', '3') THEN
        UPDATE user_ledgers SET balance = balance - NEW.amount, updated_at = CURRENT_TIMESTAMP WHERE ledger_id = NEW.ledger_id;
    ELSIF NEW.posting_type = 'C' AND ledger_type IN ('2', '3') THEN
        UPDATE user_ledgers SET balance = balance + NEW.amount, updated_at = CURRENT_TIMESTAMP WHERE ledger_id = NEW.ledger_id;
    END IF;

    SELECT TO_JSONB(t) INTO new_ledger_value FROM (
        SELECT ledger_id, balance, updated_at FROM user_ledgers WHERE ledger_id = NEW.ledger_id
    ) t;

    INSERT INTO ledger_audit_trail (ledger_id, journal_id, posting_id, action, old_values, new_values, changed_by, changed_at)
    VALUES (NEW.ledger_id, NEW.journal_id, NEW.posting_id, 'UPDATE', old_ledger_value, new_ledger_value,
            current_setting('app.current_user', true), CURRENT_TIMESTAMP);

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trigger_update_ledger_balance
    AFTER INSERT ON user_ledger_postings
    FOR EACH ROW EXECUTE FUNCTION update_ledger_balance();

-- Add debit=credit balance constraint via a trigger on journal insert
CREATE OR REPLACE FUNCTION check_journal_balance()
RETURNS TRIGGER AS $$
DECLARE
    debit_sum  NUMERIC;
    credit_sum NUMERIC;
BEGIN
    SELECT COALESCE(SUM(amount), 0) INTO debit_sum
    FROM user_ledger_postings WHERE journal_id = NEW.journal_id AND posting_type = 'D';

    SELECT COALESCE(SUM(amount), 0) INTO credit_sum
    FROM user_ledger_postings WHERE journal_id = NEW.journal_id AND posting_type = 'C';

    IF debit_sum != credit_sum THEN
        RAISE EXCEPTION 'Journal % is unbalanced: debits=% credits=%', NEW.journal_id, debit_sum, credit_sum;
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- This trigger fires when a posting row is committed; the check runs after each posting.
-- For multi-posting journals, callers must insert all postings within the same transaction
-- and can use SET CONSTRAINTS ALL DEFERRED if needed.
CREATE CONSTRAINT TRIGGER trigger_check_journal_balance
    AFTER INSERT ON user_ledger_postings
    DEFERRABLE INITIALLY DEFERRED
    FOR EACH ROW EXECUTE FUNCTION check_journal_balance();
