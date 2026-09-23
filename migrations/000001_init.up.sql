-- RigelLedger schema. See docs/ARCHITECTURE.md for the reasoning behind it.
--
-- Until the first deployment this file is edited in place. After that, every
-- change is a new numbered migration pair.
--
-- Conventions:
--   * Money is NUMERIC, never float. amount / base_amount are signed:
--     debit > 0, credit < 0.
--   * Go validates first and returns field-level errors; the triggers here are
--     the backstop that keeps the books consistent no matter who writes.
--   * Mutations run inside a transaction that has called
--     set_config('app.current_user', <user id>, true); the audit trigger reads it.
--   * Written to run on PostgreSQL 14+ (hcloud and CI run 18).

-- ---------------------------------------------------------------------------
-- Types
-- ---------------------------------------------------------------------------

CREATE TYPE account_class  AS ENUM ('asset', 'liability', 'equity', 'income', 'expense');
CREATE TYPE cf_class       AS ENUM ('operating', 'investing', 'financing');
-- Order matters: roles compare with < and >=.
CREATE TYPE member_role    AS ENUM ('viewer', 'editor', 'owner');
CREATE TYPE posting_status AS ENUM ('uncleared', 'cleared', 'reconciled');
-- currency: ISO 4217. security: shares, ETFs, futures -- priced in a quote currency.
-- points: airline miles, card points -- no market price, carried at cost.
CREATE TYPE commodity_kind AS ENUM ('currency', 'security', 'points');

CREATE FUNCTION set_updated_at() RETURNS trigger AS $$
BEGIN
    NEW.updated_at = now();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- ---------------------------------------------------------------------------
-- Commodities: every currency, securities, and loyalty points
-- ---------------------------------------------------------------------------

-- An account holds exactly one commodity, and a posting's `amount` is in it:
-- TWD for a bank account, shares for a brokerage position, miles for a
-- frequent-flyer balance. `base_amount` is always the book-currency value
-- (for securities and points: what they cost).
CREATE TABLE commodities (
    -- ISO 4217 for currencies; "<NAMESPACE>:<SYMBOL>" otherwise, e.g.
    -- "XNAS:AAPL", "XTAF:TX" (TAIEX future), "MILES:EVA", "PTS:CATHAY".
    code           TEXT PRIMARY KEY,
    kind           commodity_kind NOT NULL,
    name           TEXT NOT NULL,
    decimals       SMALLINT NOT NULL CHECK (decimals BETWEEN 0 AND 8),
    -- Securities only: the currency their price is quoted in.
    quote_currency TEXT REFERENCES commodities (code),
    exchange_mic   TEXT,
    -- Futures only: units of the underlying per contract (TX = 200). Contract
    -- value is exposure, shown beside the position -- never the asset value,
    -- which is margin plus unrealised P&L.
    contract_size  NUMERIC CHECK (contract_size > 0),
    CHECK (kind <> 'currency' OR code ~ '^[A-Z]{3}$'),
    CHECK (kind = 'currency' OR code ~ '^[A-Z0-9]+:[A-Z0-9._-]+$'),
    CHECK ((kind = 'security') = (quote_currency IS NOT NULL)),
    CHECK (kind = 'security' OR contract_size IS NULL)
);

-- ---------------------------------------------------------------------------
-- Users and sessions
-- ---------------------------------------------------------------------------

CREATE TABLE users (
    id               BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    username         TEXT NOT NULL UNIQUE CHECK (username ~ '^[a-z0-9][a-z0-9._-]{1,49}$'),
    email            TEXT NOT NULL UNIQUE,
    password_hash    TEXT NOT NULL,
    display_name     TEXT NOT NULL DEFAULT '',
    is_admin         BOOLEAN NOT NULL DEFAULT false,
    is_active        BOOLEAN NOT NULL DEFAULT true,
    -- Preferences. All of them can change at any time; none affects stored data.
    language         TEXT NOT NULL DEFAULT 'en' CHECK (language IN ('en', 'zh', 'es')),
    display_currency TEXT NOT NULL DEFAULT 'USD' REFERENCES commodities (code),
    timezone         TEXT NOT NULL DEFAULT 'UTC',
    date_format      TEXT NOT NULL DEFAULT 'YYYY-MM-DD'
                     CHECK (date_format IN ('YYYY-MM-DD', 'DD/MM/YYYY', 'MM/DD/YYYY', 'YYYY/MM/DD')),
    theme            TEXT NOT NULL DEFAULT 'system' CHECK (theme IN ('system', 'light', 'dark')),
    default_book_id  BIGINT, -- FK added after books
    last_login_at    TIMESTAMPTZ,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TRIGGER users_updated_at BEFORE UPDATE ON users
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Opaque session tokens. Only the SHA-256 of the token is stored, so a
-- database dump cannot be replayed as a login.
CREATE TABLE sessions (
    id           BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id      BIGINT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    token_hash   BYTEA NOT NULL UNIQUE,
    kind         TEXT NOT NULL CHECK (kind IN ('web', 'api')),
    label        TEXT NOT NULL DEFAULT '',
    user_agent   TEXT NOT NULL DEFAULT '',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_used_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at   TIMESTAMPTZ NOT NULL
);
CREATE INDEX sessions_user_id ON sessions (user_id);

-- ---------------------------------------------------------------------------
-- Books and members
-- ---------------------------------------------------------------------------

CREATE TABLE books (
    id                         BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name                       TEXT NOT NULL CHECK (name <> ''),
    -- The functional currency (IAS 21). Every posting's base_amount is in it.
    base_currency              TEXT NOT NULL REFERENCES commodities (code),
    -- Transactions dated on or before this are frozen.
    lock_date                  DATE,
    -- IAS 7 lets interest and dividends be operating or investing; one choice per book.
    interest_dividend_cf_class cf_class NOT NULL DEFAULT 'operating',
    created_by                 BIGINT REFERENCES users (id) ON DELETE SET NULL,
    created_at                 TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at                 TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TRIGGER books_updated_at BEFORE UPDATE ON books
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

ALTER TABLE users ADD CONSTRAINT users_default_book_id_fkey
    FOREIGN KEY (default_book_id) REFERENCES books (id) ON DELETE SET NULL;

CREATE TABLE book_members (
    book_id    BIGINT NOT NULL REFERENCES books (id) ON DELETE CASCADE,
    user_id    BIGINT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    role       member_role NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (book_id, user_id)
);
CREATE INDEX book_members_user_id ON book_members (user_id);

-- ---------------------------------------------------------------------------
-- Prices: exchange rates now, security prices from P5
-- ---------------------------------------------------------------------------

-- One unit of `commodity` is worth `rate` units of `quote` on `date`.
-- Rates are global facts, not per book. A 'manual' row wins over a scraped
-- one for the same day (enforced in the query, not here).
CREATE TABLE prices (
    id         BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    commodity  TEXT NOT NULL REFERENCES commodities (code),
    quote      TEXT NOT NULL REFERENCES commodities (code),
    date       DATE NOT NULL,
    rate       NUMERIC NOT NULL CHECK (rate > 0),
    source     TEXT NOT NULL DEFAULT 'manual',
    created_by BIGINT REFERENCES users (id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (commodity <> quote),
    UNIQUE (commodity, quote, date, source)
);
CREATE INDEX prices_lookup ON prices (commodity, quote, date DESC);

-- ---------------------------------------------------------------------------
-- Accounts
-- ---------------------------------------------------------------------------

CREATE TABLE accounts (
    id             BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    book_id        BIGINT NOT NULL REFERENCES books (id) ON DELETE CASCADE,
    parent_id      BIGINT REFERENCES accounts (id) ON DELETE RESTRICT,
    class          account_class NOT NULL,
    -- NULL name means "show the translation of template_key".
    name           TEXT,
    template_key   TEXT,
    code           TEXT,
    -- Balance-sheet accounts hold exactly one commodity. Income and expense
    -- accounts may receive any currency, so theirs is NULL.
    commodity      TEXT REFERENCES commodities (code),
    is_current     BOOLEAN NOT NULL DEFAULT true,  -- IAS 1 presentation
    is_cash        BOOLEAN NOT NULL DEFAULT false, -- cash and cash equivalents, IAS 7
    cf_class       cf_class NOT NULL DEFAULT 'operating',
    is_placeholder BOOLEAN NOT NULL DEFAULT false, -- grouping only, no postings
    archived_at    TIMESTAMPTZ,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (name IS NOT NULL OR template_key IS NOT NULL),
    CHECK (name IS NULL OR name <> ''),
    CHECK (class IN ('income', 'expense') OR commodity IS NOT NULL),
    CHECK (class NOT IN ('income', 'expense') OR commodity IS NULL),
    CHECK (NOT is_cash OR class = 'asset'),
    UNIQUE (book_id, template_key)
);
CREATE INDEX accounts_book_id ON accounts (book_id);
CREATE INDEX accounts_parent_id ON accounts (parent_id);
CREATE TRIGGER accounts_updated_at BEFORE UPDATE ON accounts
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- A parent must be in the same book and the same class, and not create a cycle.
CREATE FUNCTION check_account_parent() RETURNS trigger AS $$
DECLARE
    p accounts%ROWTYPE;
    cursor_id BIGINT;
BEGIN
    IF NEW.parent_id IS NULL THEN
        RETURN NEW;
    END IF;
    SELECT * INTO p FROM accounts WHERE id = NEW.parent_id;
    IF p.book_id <> NEW.book_id OR p.class <> NEW.class THEN
        RAISE EXCEPTION 'account parent must be in the same book and class'
            USING ERRCODE = 'check_violation', CONSTRAINT = 'account_parent';
    END IF;
    cursor_id := NEW.parent_id;
    WHILE cursor_id IS NOT NULL LOOP
        IF cursor_id = NEW.id THEN
            RAISE EXCEPTION 'account parent would create a cycle'
                USING ERRCODE = 'check_violation', CONSTRAINT = 'account_parent';
        END IF;
        SELECT parent_id INTO cursor_id FROM accounts WHERE id = cursor_id;
    END LOOP;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER accounts_check_parent BEFORE INSERT OR UPDATE OF parent_id, class, book_id ON accounts
    FOR EACH ROW EXECUTE FUNCTION check_account_parent();

-- ---------------------------------------------------------------------------
-- Transactions and postings
-- ---------------------------------------------------------------------------

CREATE TABLE transactions (
    id          BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    book_id     BIGINT NOT NULL REFERENCES books (id) ON DELETE CASCADE,
    date        DATE NOT NULL,
    payee       TEXT NOT NULL DEFAULT '',
    memo        TEXT NOT NULL DEFAULT '',
    -- 'opening' rows are excluded from the cash flow statement.
    source      TEXT NOT NULL DEFAULT 'manual' CHECK (source IN ('manual', 'opening', 'import', 'sync')),
    -- Dedupe key for imports and sync; NULL for manual entries.
    external_id TEXT,
    created_by  BIGINT REFERENCES users (id) ON DELETE SET NULL,
    updated_by  BIGINT REFERENCES users (id) ON DELETE SET NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (book_id, source, external_id)
);
CREATE INDEX transactions_book_date ON transactions (book_id, date DESC, id DESC);
CREATE TRIGGER transactions_updated_at BEFORE UPDATE ON transactions
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE postings (
    id             BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    transaction_id BIGINT NOT NULL REFERENCES transactions (id) ON DELETE CASCADE,
    account_id     BIGINT NOT NULL REFERENCES accounts (id) ON DELETE RESTRICT,
    position       SMALLINT NOT NULL DEFAULT 0,
    -- The account's commodity for balance-sheet accounts; any currency for I/E.
    commodity      TEXT NOT NULL REFERENCES commodities (code),
    amount         NUMERIC(24, 8) NOT NULL,
    -- amount translated into the book's base currency at the transaction-date rate.
    base_amount    NUMERIC(24, 8) NOT NULL,
    -- Securities: the price paid per unit in the quote currency (USD per share),
    -- kept for tax cost basis. The quantity is `amount` itself.
    unit_cost      NUMERIC(24, 8),
    status         posting_status NOT NULL DEFAULT 'uncleared',
    cleared_on     DATE,
    memo           TEXT NOT NULL DEFAULT '',
    CHECK (amount <> 0 OR base_amount <> 0),
    CHECK (sign(amount) * sign(base_amount) >= 0)
);
CREATE INDEX postings_transaction_id ON postings (transaction_id);
CREATE INDEX postings_account_id ON postings (account_id);

-- Per-posting rules that need other tables.
CREATE FUNCTION check_posting() RETURNS trigger AS $$
DECLARE
    a    accounts%ROWTYPE;
    t    transactions%ROWTYPE;
    base TEXT;
BEGIN
    SELECT * INTO t FROM transactions WHERE id = NEW.transaction_id;
    SELECT * INTO a FROM accounts WHERE id = NEW.account_id;
    SELECT base_currency INTO base FROM books WHERE id = t.book_id;

    IF a.book_id <> t.book_id THEN
        RAISE EXCEPTION 'posting account belongs to another book'
            USING ERRCODE = 'check_violation', CONSTRAINT = 'posting_account_book';
    END IF;
    IF TG_OP = 'INSERT' OR NEW.account_id <> OLD.account_id THEN
        IF a.is_placeholder THEN
            RAISE EXCEPTION 'account % is a placeholder and cannot hold postings', a.id
                USING ERRCODE = 'check_violation', CONSTRAINT = 'posting_account_placeholder';
        END IF;
        IF a.archived_at IS NOT NULL THEN
            RAISE EXCEPTION 'account % is archived', a.id
                USING ERRCODE = 'check_violation', CONSTRAINT = 'posting_account_archived';
        END IF;
    END IF;
    IF a.commodity IS NOT NULL AND a.commodity <> NEW.commodity THEN
        RAISE EXCEPTION 'posting commodity % does not match account commodity %', NEW.commodity, a.commodity
            USING ERRCODE = 'check_violation', CONSTRAINT = 'posting_commodity';
    END IF;
    IF NEW.commodity = base AND NEW.amount <> NEW.base_amount THEN
        RAISE EXCEPTION 'a posting in the base currency must have base_amount = amount'
            USING ERRCODE = 'check_violation', CONSTRAINT = 'posting_base_amount';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER postings_check BEFORE INSERT OR UPDATE ON postings
    FOR EACH ROW EXECUTE FUNCTION check_posting();

-- Double entry: every transaction has at least two postings whose base amounts
-- sum to exactly zero. Deferred to commit so the postings of one transaction
-- can be written one row at a time.
CREATE FUNCTION check_transaction_balance(txn_id BIGINT) RETURNS void AS $$
DECLARE
    n     INTEGER;
    total NUMERIC;
BEGIN
    IF NOT EXISTS (SELECT 1 FROM transactions WHERE id = txn_id) THEN
        RETURN; -- deleted along with its postings
    END IF;
    SELECT count(*), coalesce(sum(base_amount), 0) INTO n, total
    FROM postings WHERE transaction_id = txn_id;
    IF n < 2 THEN
        RAISE EXCEPTION 'transaction % has % posting(s); at least 2 are required', txn_id, n
            USING ERRCODE = 'check_violation', CONSTRAINT = 'transaction_min_postings';
    END IF;
    IF total <> 0 THEN
        RAISE EXCEPTION 'transaction % is unbalanced by % in the base currency', txn_id, total
            USING ERRCODE = 'check_violation', CONSTRAINT = 'transaction_balanced';
    END IF;
END;
$$ LANGUAGE plpgsql;

CREATE FUNCTION postings_balance_trigger() RETURNS trigger AS $$
BEGIN
    IF TG_OP IN ('INSERT', 'UPDATE') THEN
        PERFORM check_transaction_balance(NEW.transaction_id);
    END IF;
    IF TG_OP IN ('UPDATE', 'DELETE') AND (TG_OP = 'DELETE' OR OLD.transaction_id <> NEW.transaction_id) THEN
        PERFORM check_transaction_balance(OLD.transaction_id);
    END IF;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

CREATE CONSTRAINT TRIGGER postings_balanced
    AFTER INSERT OR UPDATE OR DELETE ON postings
    DEFERRABLE INITIALLY DEFERRED
    FOR EACH ROW EXECUTE FUNCTION postings_balance_trigger();

-- A transaction inserted with no postings at all never fires the trigger above.
CREATE FUNCTION transactions_balance_trigger() RETURNS trigger AS $$
BEGIN
    PERFORM check_transaction_balance(NEW.id);
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

CREATE CONSTRAINT TRIGGER transactions_balanced
    AFTER INSERT ON transactions
    DEFERRABLE INITIALLY DEFERRED
    FOR EACH ROW EXECUTE FUNCTION transactions_balance_trigger();

-- Lock date: nothing dated on or before books.lock_date may be created,
-- changed, moved into the locked period, or deleted.
CREATE FUNCTION check_lock_date(p_book_id BIGINT, p_date DATE) RETURNS void AS $$
DECLARE
    lock DATE;
BEGIN
    SELECT lock_date INTO lock FROM books WHERE id = p_book_id;
    IF lock IS NOT NULL AND p_date <= lock THEN
        RAISE EXCEPTION 'the book is locked up to %', lock
            USING ERRCODE = 'check_violation', CONSTRAINT = 'book_locked';
    END IF;
END;
$$ LANGUAGE plpgsql;

CREATE FUNCTION transactions_lock_trigger() RETURNS trigger AS $$
BEGIN
    IF TG_OP IN ('UPDATE', 'DELETE') THEN
        PERFORM check_lock_date(OLD.book_id, OLD.date);
    END IF;
    IF TG_OP IN ('INSERT', 'UPDATE') THEN
        PERFORM check_lock_date(NEW.book_id, NEW.date);
        RETURN NEW;
    END IF;
    RETURN OLD;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER transactions_lock BEFORE INSERT OR UPDATE OR DELETE ON transactions
    FOR EACH ROW EXECUTE FUNCTION transactions_lock_trigger();

CREATE FUNCTION postings_lock_trigger() RETURNS trigger AS $$
DECLARE
    t transactions%ROWTYPE;
BEGIN
    IF TG_OP IN ('UPDATE', 'DELETE') THEN
        SELECT * INTO t FROM transactions WHERE id = OLD.transaction_id;
        IF FOUND THEN
            PERFORM check_lock_date(t.book_id, t.date);
        END IF;
    END IF;
    IF TG_OP IN ('INSERT', 'UPDATE') THEN
        SELECT * INTO t FROM transactions WHERE id = NEW.transaction_id;
        PERFORM check_lock_date(t.book_id, t.date);
        RETURN NEW;
    END IF;
    RETURN OLD;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER postings_lock BEFORE INSERT OR UPDATE OR DELETE ON postings
    FOR EACH ROW EXECUTE FUNCTION postings_lock_trigger();

-- Rates are global, but a locked period's statements must not move when an
-- old rate is corrected. A rate is frozen if any book locked on or after its
-- date uses either side of it -- as base currency or in one of its accounts.
-- (Cross rates through USD are covered: USD->TWD involves TWD.)
CREATE FUNCTION check_price_lock(p_commodity TEXT, p_quote TEXT, p_date DATE) RETURNS void AS $$
DECLARE
    lock DATE;
BEGIN
    SELECT max(b.lock_date) INTO lock
    FROM books b
    WHERE b.lock_date >= p_date
      AND (b.base_currency IN (p_commodity, p_quote)
           OR EXISTS (SELECT 1 FROM accounts a
                      WHERE a.book_id = b.id AND a.commodity IN (p_commodity, p_quote)));
    IF lock IS NOT NULL THEN
        RAISE EXCEPTION 'a book using % or % is locked up to %', p_commodity, p_quote, lock
            USING ERRCODE = 'check_violation', CONSTRAINT = 'book_locked';
    END IF;
END;
$$ LANGUAGE plpgsql;

CREATE FUNCTION prices_lock_trigger() RETURNS trigger AS $$
BEGIN
    IF TG_OP IN ('UPDATE', 'DELETE') THEN
        PERFORM check_price_lock(OLD.commodity, OLD.quote, OLD.date);
    END IF;
    IF TG_OP IN ('INSERT', 'UPDATE') THEN
        PERFORM check_price_lock(NEW.commodity, NEW.quote, NEW.date);
        RETURN NEW;
    END IF;
    RETURN OLD;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER prices_lock BEFORE INSERT OR UPDATE OR DELETE ON prices
    FOR EACH ROW EXECUTE FUNCTION prices_lock_trigger();

-- ---------------------------------------------------------------------------
-- Tags ("who spent it", trips, projects)
-- ---------------------------------------------------------------------------

CREATE TABLE tags (
    id      BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    book_id BIGINT NOT NULL REFERENCES books (id) ON DELETE CASCADE,
    name    TEXT NOT NULL CHECK (name <> '')
);
CREATE UNIQUE INDEX tags_book_name ON tags (book_id, lower(name));

CREATE TABLE transaction_tags (
    transaction_id BIGINT NOT NULL REFERENCES transactions (id) ON DELETE CASCADE,
    tag_id         BIGINT NOT NULL REFERENCES tags (id) ON DELETE CASCADE,
    PRIMARY KEY (transaction_id, tag_id)
);
CREATE INDEX transaction_tags_tag_id ON transaction_tags (tag_id);

-- ---------------------------------------------------------------------------
-- Audit log
-- ---------------------------------------------------------------------------

CREATE TABLE audit_log (
    id         BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    book_id    BIGINT,
    table_name TEXT NOT NULL,
    row_id     BIGINT,
    action     TEXT NOT NULL CHECK (action IN ('INSERT', 'UPDATE', 'DELETE')),
    old_values JSONB,
    new_values JSONB,
    -- NULL when the change did not come through the API (CLI, psql).
    changed_by BIGINT,
    changed_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX audit_log_book ON audit_log (book_id, changed_at DESC);

CREATE FUNCTION audit_row() RETURNS trigger AS $$
DECLARE
    r        JSONB;
    v_book   BIGINT;
    v_row    BIGINT;
    v_user   BIGINT;
BEGIN
    r := CASE WHEN TG_OP = 'DELETE' THEN to_jsonb(OLD) ELSE to_jsonb(NEW) END;
    v_user := nullif(current_setting('app.current_user', true), '')::BIGINT;

    IF TG_TABLE_NAME = 'postings' THEN
        SELECT book_id INTO v_book FROM transactions WHERE id = (r ->> 'transaction_id')::BIGINT;
        v_row := (r ->> 'id')::BIGINT;
    ELSIF TG_TABLE_NAME = 'book_members' THEN
        v_book := (r ->> 'book_id')::BIGINT;
        v_row := (r ->> 'user_id')::BIGINT;
    ELSIF TG_TABLE_NAME = 'books' THEN
        v_book := (r ->> 'id')::BIGINT;
        v_row := v_book;
    ELSE
        v_book := (r ->> 'book_id')::BIGINT;
        v_row := (r ->> 'id')::BIGINT;
    END IF;

    INSERT INTO audit_log (book_id, table_name, row_id, action, old_values, new_values, changed_by)
    VALUES (
        v_book, TG_TABLE_NAME, v_row, TG_OP,
        CASE WHEN TG_OP IN ('UPDATE', 'DELETE') THEN to_jsonb(OLD) END,
        CASE WHEN TG_OP IN ('INSERT', 'UPDATE') THEN to_jsonb(NEW) END,
        v_user
    );
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER books_audit AFTER INSERT OR UPDATE OR DELETE ON books
    FOR EACH ROW EXECUTE FUNCTION audit_row();
CREATE TRIGGER book_members_audit AFTER INSERT OR UPDATE OR DELETE ON book_members
    FOR EACH ROW EXECUTE FUNCTION audit_row();
CREATE TRIGGER accounts_audit AFTER INSERT OR UPDATE OR DELETE ON accounts
    FOR EACH ROW EXECUTE FUNCTION audit_row();
CREATE TRIGGER transactions_audit AFTER INSERT OR UPDATE OR DELETE ON transactions
    FOR EACH ROW EXECUTE FUNCTION audit_row();
CREATE TRIGGER postings_audit AFTER INSERT OR UPDATE OR DELETE ON postings
    FOR EACH ROW EXECUTE FUNCTION audit_row();

-- ---------------------------------------------------------------------------
-- Reference data
-- ---------------------------------------------------------------------------

-- ISO 4217 list one, published 2026-09-17 (six-group.com). Funds and codes without
-- minor units (precious metals, SDR, test codes) are excluded.
INSERT INTO commodities (code, kind, name, decimals) VALUES
    ('AED', 'currency', 'UAE Dirham', 2),
    ('AFN', 'currency', 'Afghani', 2),
    ('ALL', 'currency', 'Lek', 2),
    ('AMD', 'currency', 'Armenian Dram', 2),
    ('AOA', 'currency', 'Kwanza', 2),
    ('ARS', 'currency', 'Argentine Peso', 2),
    ('AUD', 'currency', 'Australian Dollar', 2),
    ('AWG', 'currency', 'Aruban Florin', 2),
    ('AZN', 'currency', 'Azerbaijan Manat', 2),
    ('BAM', 'currency', 'Convertible Mark', 2),
    ('BBD', 'currency', 'Barbados Dollar', 2),
    ('BDT', 'currency', 'Taka', 2),
    ('BHD', 'currency', 'Bahraini Dinar', 3),
    ('BIF', 'currency', 'Burundi Franc', 0),
    ('BMD', 'currency', 'Bermudian Dollar', 2),
    ('BND', 'currency', 'Brunei Dollar', 2),
    ('BOB', 'currency', 'Boliviano', 2),
    ('BRL', 'currency', 'Brazilian Real', 2),
    ('BSD', 'currency', 'Bahamian Dollar', 2),
    ('BTN', 'currency', 'Ngultrum', 2),
    ('BWP', 'currency', 'Pula', 2),
    ('BYN', 'currency', 'Belarusian Ruble', 2),
    ('BZD', 'currency', 'Belize Dollar', 2),
    ('CAD', 'currency', 'Canadian Dollar', 2),
    ('CDF', 'currency', 'Congolese Franc', 2),
    ('CHF', 'currency', 'Swiss Franc', 2),
    ('CLP', 'currency', 'Chilean Peso', 0),
    ('CNY', 'currency', 'Yuan Renminbi', 2),
    ('COP', 'currency', 'Colombian Peso', 2),
    ('CRC', 'currency', 'Costa Rican Colon', 2),
    ('CUP', 'currency', 'Cuban Peso', 2),
    ('CVE', 'currency', 'Cabo Verde Escudo', 2),
    ('CZK', 'currency', 'Czech Koruna', 2),
    ('DJF', 'currency', 'Djibouti Franc', 0),
    ('DKK', 'currency', 'Danish Krone', 2),
    ('DOP', 'currency', 'Dominican Peso', 2),
    ('DZD', 'currency', 'Algerian Dinar', 2),
    ('EGP', 'currency', 'Egyptian Pound', 2),
    ('ERN', 'currency', 'Nakfa', 2),
    ('ETB', 'currency', 'Ethiopian Birr', 2),
    ('EUR', 'currency', 'Euro', 2),
    ('FJD', 'currency', 'Fiji Dollar', 2),
    ('FKP', 'currency', 'Falkland Islands Pound', 2),
    ('GBP', 'currency', 'Pound Sterling', 2),
    ('GEL', 'currency', 'Lari', 2),
    ('GHS', 'currency', 'Ghana Cedi', 2),
    ('GIP', 'currency', 'Gibraltar Pound', 2),
    ('GMD', 'currency', 'Dalasi', 2),
    ('GNF', 'currency', 'Guinean Franc', 0),
    ('GTQ', 'currency', 'Quetzal', 2),
    ('GYD', 'currency', 'Guyana Dollar', 2),
    ('HKD', 'currency', 'Hong Kong Dollar', 2),
    ('HNL', 'currency', 'Lempira', 2),
    ('HTG', 'currency', 'Gourde', 2),
    ('HUF', 'currency', 'Forint', 2),
    ('IDR', 'currency', 'Rupiah', 2),
    ('ILS', 'currency', 'New Israeli Sheqel', 2),
    ('INR', 'currency', 'Indian Rupee', 2),
    ('IQD', 'currency', 'Iraqi Dinar', 3),
    ('IRR', 'currency', 'Iranian Rial', 2),
    ('ISK', 'currency', 'Iceland Krona', 0),
    ('JMD', 'currency', 'Jamaican Dollar', 2),
    ('JOD', 'currency', 'Jordanian Dinar', 3),
    ('JPY', 'currency', 'Yen', 0),
    ('KES', 'currency', 'Kenyan Shilling', 2),
    ('KGS', 'currency', 'Som', 2),
    ('KHR', 'currency', 'Riel', 2),
    ('KMF', 'currency', 'Comorian Franc ', 0),
    ('KPW', 'currency', 'North Korean Won', 2),
    ('KRW', 'currency', 'Won', 0),
    ('KWD', 'currency', 'Kuwaiti Dinar', 3),
    ('KYD', 'currency', 'Cayman Islands Dollar', 2),
    ('KZT', 'currency', 'Tenge', 2),
    ('LAK', 'currency', 'Lao Kip', 2),
    ('LBP', 'currency', 'Lebanese Pound', 2),
    ('LKR', 'currency', 'Sri Lanka Rupee', 2),
    ('LRD', 'currency', 'Liberian Dollar', 2),
    ('LSL', 'currency', 'Loti', 2),
    ('LYD', 'currency', 'Libyan Dinar', 3),
    ('MAD', 'currency', 'Moroccan Dirham', 2),
    ('MDL', 'currency', 'Moldovan Leu', 2),
    ('MGA', 'currency', 'Malagasy Ariary', 2),
    ('MKD', 'currency', 'Denar', 2),
    ('MMK', 'currency', 'Kyat', 2),
    ('MNT', 'currency', 'Tugrik', 2),
    ('MOP', 'currency', 'Pataca', 2),
    ('MRU', 'currency', 'Ouguiya', 2),
    ('MUR', 'currency', 'Mauritius Rupee', 2),
    ('MVR', 'currency', 'Rufiyaa', 2),
    ('MWK', 'currency', 'Malawi Kwacha', 2),
    ('MXN', 'currency', 'Mexican Peso', 2),
    ('MYR', 'currency', 'Malaysian Ringgit', 2),
    ('MZN', 'currency', 'Mozambique Metical', 2),
    ('NAD', 'currency', 'Namibia Dollar', 2),
    ('NGN', 'currency', 'Naira', 2),
    ('NIO', 'currency', 'Cordoba Oro', 2),
    ('NOK', 'currency', 'Norwegian Krone', 2),
    ('NPR', 'currency', 'Nepalese Rupee', 2),
    ('NZD', 'currency', 'New Zealand Dollar', 2),
    ('OMR', 'currency', 'Rial Omani', 3),
    ('PAB', 'currency', 'Balboa', 2),
    ('PEN', 'currency', 'Sol', 2),
    ('PGK', 'currency', 'Kina', 2),
    ('PHP', 'currency', 'Philippine Peso', 2),
    ('PKR', 'currency', 'Pakistan Rupee', 2),
    ('PLN', 'currency', 'Zloty', 2),
    ('PYG', 'currency', 'Guarani', 0),
    ('QAR', 'currency', 'Qatari Rial', 2),
    ('RON', 'currency', 'Romanian Leu', 2),
    ('RSD', 'currency', 'Serbian Dinar', 2),
    ('RUB', 'currency', 'Russian Ruble', 2),
    ('RWF', 'currency', 'Rwanda Franc', 0),
    ('SAR', 'currency', 'Saudi Riyal', 2),
    ('SBD', 'currency', 'Solomon Islands Dollar', 2),
    ('SCR', 'currency', 'Seychelles Rupee', 2),
    ('SDG', 'currency', 'Sudanese Pound', 2),
    ('SEK', 'currency', 'Swedish Krona', 2),
    ('SGD', 'currency', 'Singapore Dollar', 2),
    ('SHP', 'currency', 'Saint Helena Pound', 2),
    ('SLE', 'currency', 'Leone', 2),
    ('SOS', 'currency', 'Somali Shilling', 2),
    ('SRD', 'currency', 'Surinam Dollar', 2),
    ('SSP', 'currency', 'South Sudanese Pound', 2),
    ('STN', 'currency', 'Dobra', 2),
    ('SVC', 'currency', 'El Salvador Colon', 2),
    ('SYP', 'currency', 'Syrian Pound', 2),
    ('SZL', 'currency', 'Lilangeni', 2),
    ('THB', 'currency', 'Baht', 2),
    ('TJS', 'currency', 'Somoni', 2),
    ('TMT', 'currency', 'Turkmenistan New Manat', 2),
    ('TND', 'currency', 'Tunisian Dinar', 3),
    ('TOP', 'currency', 'Pa’anga', 2),
    ('TRY', 'currency', 'Turkish Lira', 2),
    ('TTD', 'currency', 'Trinidad and Tobago Dollar', 2),
    ('TWD', 'currency', 'New Taiwan Dollar', 2),
    ('TZS', 'currency', 'Tanzanian Shilling', 2),
    ('UAH', 'currency', 'Hryvnia', 2),
    ('UGX', 'currency', 'Uganda Shilling', 0),
    ('USD', 'currency', 'US Dollar', 2),
    ('UYU', 'currency', 'Peso Uruguayo', 2),
    ('UZS', 'currency', 'Uzbekistan Sum', 2),
    ('VED', 'currency', 'Bolívar Soberano', 2),
    ('VES', 'currency', 'Bolívar Soberano', 2),
    ('VND', 'currency', 'Dong', 0),
    ('VUV', 'currency', 'Vatu', 0),
    ('WST', 'currency', 'Tala', 2),
    ('XAF', 'currency', 'CFA Franc BEAC', 0),
    ('XCD', 'currency', 'East Caribbean Dollar', 2),
    ('XCG', 'currency', 'Caribbean Guilder', 2),
    ('XOF', 'currency', 'CFA Franc BCEAO', 0),
    ('XPF', 'currency', 'CFP Franc', 0),
    ('YER', 'currency', 'Yemeni Rial', 2),
    ('ZAR', 'currency', 'Rand', 2),
    ('ZMW', 'currency', 'Zambian Kwacha', 2),
    ('ZWG', 'currency', 'Zimbabwe Gold', 2);
