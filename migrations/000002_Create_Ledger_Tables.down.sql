-- ========================================
-- DROP IFRS COMPLIANT LEDGER SCHEMA
-- ========================================

-- Drop triggers first
DROP TRIGGER IF EXISTS trigger_audit_user_ledgers ON user_ledgers;
DROP TRIGGER IF EXISTS trigger_update_ledger_balance ON user_ledger_postings;
DROP TRIGGER IF EXISTS update_user_ledger_postings_updated_at ON user_ledger_postings;
DROP TRIGGER IF EXISTS update_user_ledger_journal_updated_at ON user_ledger_journal;
DROP TRIGGER IF EXISTS update_user_ledgers_updated_at ON user_ledgers;
DROP TRIGGER IF EXISTS update_ref_ledger_types_updated_at ON ref_ledger_types;
DROP TRIGGER IF EXISTS update_ref_ledger_third_grade_updated_at ON ref_ledger_third_grade;
DROP TRIGGER IF EXISTS update_ref_ledger_second_grade_updated_at ON ref_ledger_second_grade;
DROP TRIGGER IF EXISTS update_ref_ledger_first_grade_updated_at ON ref_ledger_first_grade;

-- Drop functions
DROP FUNCTION IF EXISTS audit_ledger_changes();
DROP FUNCTION IF EXISTS update_ledger_balance();

-- Drop indexes
DROP INDEX IF EXISTS idx_ledger_audit_trail_date;
DROP INDEX IF EXISTS idx_ledger_audit_trail_changed_by;
DROP INDEX IF EXISTS idx_ledger_audit_trail_ledger;
DROP INDEX IF EXISTS idx_user_ledger_postings_journal;
DROP INDEX IF EXISTS idx_user_ledger_postings_ledger;
DROP INDEX IF EXISTS idx_user_ledger_journal_date;
DROP INDEX IF EXISTS idx_user_ledger_journal_user;
DROP INDEX IF EXISTS idx_user_ledgers_status;
DROP INDEX IF EXISTS idx_user_ledgers_currency;
DROP INDEX IF EXISTS idx_user_ledgers_type;
DROP INDEX IF EXISTS idx_user_ledgers_owner;

-- Drop tables in reverse order (due to foreign key constraints)
DROP TABLE IF EXISTS ledger_audit_trail;
DROP TABLE IF EXISTS user_ledger_postings;
DROP TABLE IF EXISTS user_ledger_journal;
DROP TABLE IF EXISTS user_ledgers;
DROP TABLE IF EXISTS ref_ledger_types;
DROP TABLE IF EXISTS ref_ledger_third_grade;
DROP TABLE IF EXISTS ref_ledger_second_grade;
DROP TABLE IF EXISTS ref_ledger_first_grade;
DROP TABLE IF EXISTS ref_exchange_rates;
DROP TABLE IF EXISTS ref_stock_prices;
