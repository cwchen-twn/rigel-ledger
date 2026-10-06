-- #38: invoices with line items (電子發票 載具, receipts from Gmail). An
-- invoice enriches; it does not create: it matches the card, bank or cash
-- transaction that paid it and adds its items, which can split the expense
-- by category. Only an invoice nothing paid for (a cash purchase) proposes
-- a transaction of its own.

-- An invoice row: amount is its total, signed on the account that paid
-- (a purchase < 0); counterparty the seller; items its lines, as
-- [{description, quantity, unit_price, amount, account_id}] where
-- account_id is the category a rule gives the line (null when none does).
ALTER TABLE import_rows DROP CONSTRAINT import_rows_kind_check;
ALTER TABLE import_rows
    ADD CONSTRAINT import_rows_kind_check CHECK (kind IN ('transaction', 'balance', 'holding', 'trade', 'invoice')),
    ADD COLUMN items JSONB,
    ADD CONSTRAINT import_rows_items CHECK ((kind = 'invoice') = (items IS NOT NULL AND jsonb_typeof(items) = 'array'));

-- enrich  the invoice belongs to match_transaction_id (booked), or waits
--         for match_row_id (a card row not accepted yet)
ALTER TABLE import_rows DROP CONSTRAINT import_rows_proposal_check;
ALTER TABLE import_rows ADD CONSTRAINT import_rows_proposal_check
    CHECK (proposal IN ('new', 'duplicate', 'clears', 'transfer', 'enrich'));

-- A transaction's lines as its invoice lists them. Display and evidence:
-- the postings hold the money (a split by category writes postings too).
CREATE TABLE transaction_items (
    id             BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    book_id        BIGINT NOT NULL REFERENCES books (id) ON DELETE CASCADE,
    transaction_id BIGINT NOT NULL REFERENCES transactions (id) ON DELETE CASCADE,
    position       SMALLINT NOT NULL,
    description    TEXT NOT NULL DEFAULT '',
    quantity       NUMERIC,
    unit_price     NUMERIC,
    amount         NUMERIC(24,8) NOT NULL,
    -- The category the line was booked to, when the expense was split.
    account_id     BIGINT REFERENCES accounts (id) ON DELETE SET NULL,
    -- Where it came from: "<connector>:<invoice id>".
    source         TEXT NOT NULL DEFAULT '',
    UNIQUE (transaction_id, position)
);
CREATE TRIGGER transaction_items_audit AFTER INSERT OR UPDATE OR DELETE ON transaction_items
    FOR EACH ROW EXECUTE FUNCTION audit_row();
