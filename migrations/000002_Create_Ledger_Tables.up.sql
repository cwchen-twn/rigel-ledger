-- ========================================
-- IFRS COMPLIANT LEDGER SCHEMA
-- ========================================

CREATE TABLE ref_stock_prices (
    stock_price_id BIGINT PRIMARY KEY,
    stock_symbol VARCHAR(10) NOT NULL,
    price_date DATE NOT NULL,
    price_value NUMERIC(20, 6) NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(stock_symbol, price_date)
);
COMMENT ON TABLE ref_stock_prices IS 'Reference table for stock prices';
COMMENT ON COLUMN ref_stock_prices.stock_price_id IS 'Unique identifier for the stock price';
COMMENT ON COLUMN ref_stock_prices.stock_symbol IS 'Stock symbol, e.g., AAPL';
COMMENT ON COLUMN ref_stock_prices.price_date IS 'Date of the stock price';
COMMENT ON COLUMN ref_stock_prices.price_value IS 'Price value of the stock';

CREATE TABLE ref_exchange_rates (
    exchange_rate_id BIGINT PRIMARY KEY,
    from_currency VARCHAR(3) NOT NULL REFERENCES ref_currencies_iso4217(alphabetic_code) ON DELETE RESTRICT,
    to_currency VARCHAR(3) NOT NULL REFERENCES ref_currencies_iso4217(alphabetic_code) ON DELETE RESTRICT,
    rate_date DATE NOT NULL,
    rate NUMERIC(20, 6) NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(from_currency, to_currency, rate_date)
);
COMMENT ON TABLE ref_exchange_rates IS 'Reference table for exchange rates';
COMMENT ON COLUMN ref_exchange_rates.exchange_rate_id IS 'Unique identifier for the exchange rate';
COMMENT ON COLUMN ref_exchange_rates.from_currency IS 'Ref: ref_currencies_iso4217';
COMMENT ON COLUMN ref_exchange_rates.to_currency IS 'Ref: ref_currencies_iso4217';
COMMENT ON COLUMN ref_exchange_rates.rate IS 'Exchange rate value';
COMMENT ON COLUMN ref_exchange_rates.rate_date IS 'Date of the exchange rate';

-- Create ledger hierarchy reference tables
CREATE TABLE ref_ledger_first_grade (
    first_grade    VARCHAR(1) PRIMARY KEY,
    type_name      TEXT NOT NULL,
    type_name_zh   TEXT NOT NULL,
    description_en TEXT,
    description_zh TEXT,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE NULL
);
COMMENT ON TABLE ref_ledger_first_grade IS 'First level of IFRS chart of accounts hierarchy';

CREATE TABLE ref_ledger_second_grade (
    second_grade   VARCHAR(2) PRIMARY KEY,
    first_grade    VARCHAR(1) NOT NULL REFERENCES ref_ledger_first_grade(first_grade) ON DELETE CASCADE,
    type_name      TEXT NOT NULL,
    type_name_zh   TEXT,
    description_en TEXT,
    description_zh TEXT,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE NULL
);
COMMENT ON TABLE ref_ledger_second_grade IS 'Second level of IFRS chart of accounts hierarchy';

CREATE TABLE ref_ledger_third_grade (
    third_grade    VARCHAR(3) PRIMARY KEY,
    first_grade    VARCHAR(1) NOT NULL REFERENCES ref_ledger_first_grade(first_grade) ON DELETE CASCADE,
    second_grade   VARCHAR(2) NOT NULL REFERENCES ref_ledger_second_grade(second_grade) ON DELETE CASCADE,
    type_name      TEXT NOT NULL,
    type_name_zh   TEXT,
    description_en TEXT,
    description_zh TEXT,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE NULL
);
COMMENT ON TABLE ref_ledger_third_grade IS 'Third level of IFRS chart of accounts hierarchy';

CREATE TABLE ref_ledger_types (
    ledger_type_id VARCHAR(4) PRIMARY KEY,
    first_grade    VARCHAR(1) NOT NULL REFERENCES ref_ledger_first_grade(first_grade) ON DELETE CASCADE,
    second_grade   VARCHAR(2) NOT NULL REFERENCES ref_ledger_second_grade(second_grade) ON DELETE CASCADE,
    third_grade    VARCHAR(3) NOT NULL REFERENCES ref_ledger_third_grade(third_grade) ON DELETE CASCADE,
    type_name      TEXT NOT NULL,
    type_name_zh   TEXT,
    description_en TEXT,
    description_zh TEXT,
    is_active      BOOLEAN DEFAULT true,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE NULL
);
COMMENT ON TABLE ref_ledger_types IS 'Fourth level of IFRS chart of accounts hierarchy - individual account types';
COMMENT ON COLUMN ref_ledger_types.ledger_type_id IS 'Fourth grade, e.g., A111 Salary revenue';
COMMENT ON COLUMN ref_ledger_types.first_grade IS 'Ref: ref_ledger_first_grade';
COMMENT ON COLUMN ref_ledger_types.second_grade IS 'Ref: ref_ledger_second_grade';
COMMENT ON COLUMN ref_ledger_types.third_grade IS 'Ref: ref_ledger_third_grade';

-- Create user ledgers table with improved structure
CREATE TABLE user_ledgers (
    ledger_id       SERIAL PRIMARY KEY,
    ledger_owner    TEXT NOT NULL REFERENCES users(username) ON DELETE RESTRICT,
    ledger_name     TEXT NOT NULL,
    ledger_type_id  VARCHAR(4) NOT NULL REFERENCES ref_ledger_types(ledger_type_id) ON DELETE RESTRICT,
    currency        VARCHAR(3) REFERENCES ref_currencies_iso4217(alphabetic_code) ON DELETE RESTRICT,
    balance         NUMERIC(20, 6) DEFAULT 0,
    ledger_status   SMALLINT DEFAULT 1 CHECK (ledger_status IN (0, 1)), -- 0: Inactive 1: Active
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE NULL
);
COMMENT ON TABLE user_ledgers IS 'User-defined ledger accounts following IFRS chart of accounts structure';
COMMENT ON COLUMN user_ledgers.ledger_owner IS 'e.g., cwchen.twn';
COMMENT ON COLUMN user_ledgers.ledger_name IS 'Under the ledger_type_name, e.g., Operating Expenses, Cash, Tax, Living Expense';
COMMENT ON COLUMN user_ledgers.ledger_type_id IS 'Related to ledger_types, e.g., A111';

-- Create journal entries table with improved structure
CREATE TABLE user_ledger_journal (
    journal_id      BIGSERIAL PRIMARY KEY,
    user_id         TEXT NOT NULL REFERENCES users(username) ON DELETE RESTRICT,
    transac_id      BIGINT NOT NULL,
    transac_date    DATE NOT NULL,
    posting_date    DATE DEFAULT CURRENT_DATE,
    photo_addr      TEXT[],
    description     VARCHAR(500),
    reference_no    VARCHAR(100),
    is_reconciled   BOOLEAN DEFAULT false,
    -- In case at least one posting has a different currency from the base currency,
    -- we need to use the FX rate to convert the amount to the base currency.
    -- Remember, when the base currency is changed, the exchange_rate should be updated too.
    -- To simplify the system, consider not permitting the base currency to be changed.
    exchange_rate   NUMERIC(20, 6) DEFAULT 1.0,
    stock_price_ref BIGINT REFERENCES ref_stock_prices(stock_price_id) ON DELETE RESTRICT,
    created_at      TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at      TIMESTAMP WITH TIME ZONE NULL,
    UNIQUE(transac_id, user_id)
);
COMMENT ON COLUMN user_ledger_journal.transac_id IS 'Should be interpreted as in the frontend like TX00000001';
COMMENT ON COLUMN user_ledger_journal.transac_date IS 'Transaction date could be different from the creation date';
COMMENT ON COLUMN user_ledger_journal.posting_date IS 'Date when the transaction was posted to the ledger';

-- Create ledger postings table
CREATE TABLE user_ledger_postings (
    posting_id      SERIAL PRIMARY KEY,
    journal_id      BIGINT NOT NULL REFERENCES user_ledger_journal(journal_id) ON DELETE RESTRICT,
    ledger_id       INT NOT NULL REFERENCES user_ledgers(ledger_id) ON DELETE RESTRICT,
    posting_type    VARCHAR(1) NOT NULL CHECK (posting_type IN ('D', 'C')), -- D: Debit, C: Credit

    -- original amount in the transaction currency
    amount          NUMERIC(20, 6) NOT NULL CHECK (amount >= 0),
    currency_code   CHAR(3) NOT NULL REFERENCES ref_currencies_iso4217(alphabetic_code),
    description     VARCHAR(500),
    created_at      TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);
COMMENT ON TABLE user_ledger_postings IS 'Used to generate the income statement and trial balance for a given period';
COMMENT ON COLUMN user_ledger_postings.amount IS 'Original amount in the transaction currency';
COMMENT ON COLUMN user_ledger_postings.currency_code IS 'Currency of the transaction amount';

-- Create audit trail table for compliance
CREATE TABLE ledger_audit_trail (
    audit_id        BIGSERIAL PRIMARY KEY,
    ledger_id       INT NOT NULL REFERENCES user_ledgers(ledger_id) ON DELETE RESTRICT,
    journal_id      BIGINT REFERENCES user_ledger_journal(journal_id) ON DELETE RESTRICT,
    posting_id      BIGINT REFERENCES user_ledger_postings(posting_id) ON DELETE RESTRICT,
    action          VARCHAR(20) NOT NULL CHECK (action IN ('INSERT', 'UPDATE', 'DELETE')),
    old_values      JSONB,
    new_values      JSONB,
    changed_by      TEXT NOT NULL REFERENCES users(username) ON DELETE RESTRICT,
    changed_at      TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);
COMMENT ON TABLE ledger_audit_trail IS 'Audit trail for all ledger changes to ensure IFRS compliance and data integrity';

-- Create indexes for better performance
CREATE INDEX idx_user_ledgers_owner ON user_ledgers(ledger_owner);
CREATE INDEX idx_user_ledgers_type ON user_ledgers(ledger_type_id);
CREATE INDEX idx_user_ledgers_currency ON user_ledgers(currency);
CREATE INDEX idx_user_ledgers_status ON user_ledgers(ledger_status);
CREATE INDEX idx_user_ledger_journal_user ON user_ledger_journal(user_id);
CREATE INDEX idx_user_ledger_journal_date ON user_ledger_journal(transac_date);
CREATE INDEX idx_user_ledger_postings_ledger ON user_ledger_postings(ledger_id);
CREATE INDEX idx_user_ledger_postings_journal ON user_ledger_postings(journal_id);
CREATE INDEX idx_ledger_audit_trail_ledger ON ledger_audit_trail(ledger_id);
CREATE INDEX idx_ledger_audit_trail_changed_by ON ledger_audit_trail(changed_by);
CREATE INDEX idx_ledger_audit_trail_date ON ledger_audit_trail(changed_at);

-- Create triggers for updated_at on new tables
CREATE TRIGGER update_ref_ledger_first_grade_updated_at BEFORE UPDATE ON ref_ledger_first_grade
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_ref_ledger_second_grade_updated_at BEFORE UPDATE ON ref_ledger_second_grade
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_ref_ledger_third_grade_updated_at BEFORE UPDATE ON ref_ledger_third_grade
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_ref_ledger_types_updated_at BEFORE UPDATE ON ref_ledger_types
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_user_ledgers_updated_at BEFORE UPDATE ON user_ledgers
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_user_ledger_journal_updated_at BEFORE UPDATE ON user_ledger_journal
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_user_ledger_postings_updated_at BEFORE UPDATE ON user_ledger_postings
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- Create function to update ledger balances
CREATE OR REPLACE FUNCTION update_ledger_balance()
RETURNS TRIGGER AS $$
DECLARE
    ledger_type VARCHAR(4);
    old_ledger_value JSONB;
    new_ledger_value JSONB;
BEGIN
    IF TG_OP != 'INSERT' THEN
        RETURN NULL;
    END IF;

    SELECT
        first_grade INTO ledger_type
    FROM
        ref_ledger_types
    WHERE ledger_type_id = (
        SELECT ledger_type_id FROM user_ledgers WHERE ledger_id = NEW.ledger_id
    );

    -- Skip balance update for EXPENSES and REVENUES accounts.
    -- Since those accounts would have several currencies in the postings,
    -- and the balance will be calculated by the income statement with currency specified.
    IF ledger_type IN ('4', '5', '6', '7', 'A', 'B') THEN
        RETURN NULL;
    END IF;

    -- Only update balance for ASSETS, LIABILITIES, and EQUITY accounts.
    IF (
        SELECT currency FROM user_ledgers
        WHERE ledger_id = NEW.ledger_id
    ) != NEW.currency_code
    THEN
        RAISE EXCEPTION 'Posting currency does not match ledger currency';
    END IF;

    SELECT TO_JSONB(t) INTO old_ledger_value FROM (
        SELECT
            ledger_id,
            balance,
            updated_at
        FROM user_ledgers
        WHERE ledger_id = NEW.ledger_id
    ) t;

    IF NEW.posting_type = 'D' AND ledger_type = '1' THEN
        -- Debit ASSETS
        UPDATE user_ledgers
        SET balance = balance + NEW.amount,
            updated_at = CURRENT_TIMESTAMP
        WHERE ledger_id = NEW.ledger_id;
    ELSIF NEW.posting_type = 'C' AND ledger_type = '1' THEN
        -- Credit ASSETS
        UPDATE user_ledgers
        SET balance = balance - NEW.amount,
            updated_at = CURRENT_TIMESTAMP
        WHERE ledger_id = NEW.ledger_id;
    ELSIF NEW.posting_type = 'D' AND ledger_type IN ('2', '3') THEN
        -- Debit LIABILITIES and EQUITY
        UPDATE user_ledgers
        SET balance = balance - NEW.amount,
            updated_at = CURRENT_TIMESTAMP
        WHERE ledger_id = NEW.ledger_id;
    ELSIF NEW.posting_type = 'C' AND ledger_type IN ('2', '3') THEN
        -- Credit LIABILITIES and EQUITY
        UPDATE user_ledgers
        SET balance = balance + NEW.amount,
            updated_at = CURRENT_TIMESTAMP
        WHERE ledger_id = NEW.ledger_id;
    END IF;

    SELECT TO_JSONB(t) INTO new_ledger_value FROM (
        SELECT
            ledger_id,
            balance,
            updated_at
        FROM user_ledgers
        WHERE ledger_id = NEW.ledger_id
    ) t;

    INSERT INTO ledger_audit_trail (
        ledger_id,
        journal_id,
        posting_id,
        action,
        old_values,
        new_values,
        changed_by,
        changed_at
    ) VALUES (
        NEW.ledger_id,
        NEW.journal_id,
        NEW.posting_id,
        'UPDATE',
        old_ledger_value,
        new_ledger_value,
        current_setting('app.current_user', true),
        CURRENT_TIMESTAMP
    );

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Create trigger to automatically update ledger balances
CREATE TRIGGER trigger_update_ledger_balance
    BEFORE INSERT ON user_ledger_postings
    FOR EACH ROW EXECUTE FUNCTION update_ledger_balance();

-- Create function to maintain audit trail
CREATE OR REPLACE FUNCTION audit_user_ledgers_changes()
RETURNS TRIGGER AS $$
BEGIN
    IF TG_OP = 'INSERT' THEN
        INSERT INTO ledger_audit_trail (
            ledger_id,
            action,
            new_values,
            changed_by,
            changed_at
        ) VALUES (
            NEW.ledger_id,
            'INSERT',
            to_jsonb(NEW),
            current_setting('app.current_user', true),
            CURRENT_TIMESTAMP
        );
        RETURN NEW;
    ELSIF TG_OP = 'UPDATE' THEN
        INSERT INTO ledger_audit_trail (
            ledger_id,
            action,
            old_values,
            new_values,
            changed_by,
            changed_at
        ) VALUES (
            NEW.ledger_id,
            'UPDATE',
            to_jsonb(OLD),
            to_jsonb(NEW),
            current_setting('app.current_user', true),
            CURRENT_TIMESTAMP
        );
        RETURN NEW;
    ELSIF TG_OP = 'DELETE' THEN
        INSERT INTO ledger_audit_trail (
            ledger_id,
            action,
            old_values,
            changed_by,
            changed_at
        ) VALUES (
            OLD.ledger_id,
            'DELETE',
            to_jsonb(OLD),
            current_setting('app.current_user', true),
            CURRENT_TIMESTAMP
        );
        RETURN OLD;
    END IF;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

-- Create triggers for audit trail
CREATE TRIGGER trigger_audit_user_ledgers
    AFTER INSERT OR UPDATE OR DELETE ON user_ledgers
    FOR EACH ROW EXECUTE FUNCTION audit_user_ledgers_changes();
