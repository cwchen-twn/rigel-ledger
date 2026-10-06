-- #37: securities from a sync. A broker account (集保, later Shioaji and
-- Firstrade) is a source account of kind 'brokerage': mapped to a parent
-- account under which each security gets its own account (one commodity per
-- account), and to the bank account its trades settle through.
ALTER TABLE source_accounts
    ADD COLUMN kind TEXT NOT NULL DEFAULT 'cash' CHECK (kind IN ('cash', 'brokerage')),
    ADD COLUMN settlement_account_id BIGINT REFERENCES accounts (id) ON DELETE SET NULL;

-- Which account holds a security for a broker account: made the first time
-- the security appears, found again by this, wherever it is moved or renamed.
CREATE TABLE source_securities (
    source_account_id BIGINT NOT NULL REFERENCES source_accounts (id) ON DELETE CASCADE,
    commodity         TEXT NOT NULL REFERENCES commodities (code),
    account_id        BIGINT NOT NULL REFERENCES accounts (id) ON DELETE CASCADE,
    PRIMARY KEY (source_account_id, commodity)
);

-- Two more kinds of staged row:
--   holding  units of a security a broker account holds on a date: an
--            assertion on the security's account, as a balance row is
--   trade    units in (> 0) or out (< 0), at price per unit in the
--            security's quote currency when the source knows it, and the
--            cash it settled for (signed on the settlement account: a buy
--            < 0) when it knows that; the person confirms the cash on accept
-- Their amount is 0; units, price and cash are their own columns.
ALTER TABLE import_rows DROP CONSTRAINT import_rows_kind_check;
ALTER TABLE import_rows
    ADD CONSTRAINT import_rows_kind_check CHECK (kind IN ('transaction', 'balance', 'holding', 'trade')),
    ADD COLUMN security TEXT REFERENCES commodities (code),
    ADD COLUMN units    NUMERIC(24,8),
    ADD COLUMN price    NUMERIC CHECK (price > 0),
    ADD COLUMN cash     NUMERIC(24,8),
    ADD CONSTRAINT import_rows_shape CHECK (CASE kind
        WHEN 'holding' THEN security IS NOT NULL AND units >= 0 AND amount = 0 AND cash IS NULL
        WHEN 'trade' THEN security IS NOT NULL AND units <> 0 AND amount = 0
        ELSE security IS NULL AND units IS NULL AND price IS NULL AND cash IS NULL
    END);
