-- name: UpsertSourceAccount :one
INSERT INTO source_accounts (book_id, connector, external_id, label, currency, kind)
VALUES (@book_id, @connector, @external_id, @label, sqlc.narg(currency), @kind)
ON CONFLICT (book_id, connector, external_id) DO UPDATE
    SET label = CASE WHEN EXCLUDED.label <> '' THEN EXCLUDED.label ELSE source_accounts.label END,
        currency = coalesce(source_accounts.currency, EXCLUDED.currency),
        kind = EXCLUDED.kind
RETURNING *;

-- name: ListSourceAccounts :many
SELECT s.*,
       (SELECT count(*) FROM import_rows r WHERE r.source_account_id = s.id AND r.status = 'pending')::BIGINT AS pending
FROM source_accounts s WHERE s.book_id = @book_id
-- A stable order: a row must not jump away when it is mapped.
ORDER BY s.connector, s.label, s.id;

-- name: GetSourceAccount :one
SELECT * FROM source_accounts WHERE book_id = @book_id AND id = @id;

-- name: MapSourceAccount :one
UPDATE source_accounts SET account_id = sqlc.narg(account_id), settlement_account_id = sqlc.narg(settlement_account_id)
WHERE book_id = @book_id AND id = @id RETURNING *;

-- name: CreateImportBatch :one
INSERT INTO import_batches (book_id, connector, label, created_by) VALUES (@book_id, @connector, @label, sqlc.narg(created_by))
RETURNING *;

-- name: SetBatchCounts :exec
UPDATE import_batches SET received = @received, duplicates = @duplicates WHERE id = @id;

-- name: InsertImportRow :one
-- Nothing on a row already staged (the same external id): that is what
-- makes a re-sync harmless. The caller counts the missing RETURNING.
INSERT INTO import_rows (book_id, batch_id, source_account_id, kind, external_id, date, amount, currency,
                         description, counterparty, pending, raw, security, units, price, cash, items)
VALUES (@book_id, @batch_id, @source_account_id, @kind, @external_id, @date, @amount, @currency,
        @description, @counterparty, @pending, @raw, sqlc.narg(security), sqlc.narg(units), sqlc.narg(price), sqlc.narg(cash),
        sqlc.narg(items))
ON CONFLICT (book_id, external_id) DO NOTHING
RETURNING id;

-- name: ListQueue :many
-- The review queue: pending rows with their source account's mapping.
SELECT r.id, r.kind, r.external_id, r.date, r.amount, r.currency, r.description, r.counterparty, r.pending,
       r.proposal, r.proposed_account_id, r.match_transaction_id, r.match_row_id, r.rule_id, r.attachment_id,
       r.security, c.name AS security_name, r.units, r.price, r.cash, r.items,
       s.id AS source_account_id, s.connector, s.label AS source_label, s.account_id, s.settlement_account_id
FROM import_rows r JOIN source_accounts s ON s.id = r.source_account_id
LEFT JOIN commodities c ON c.code = r.security
WHERE r.book_id = @book_id AND r.status = 'pending'
ORDER BY r.date DESC, r.id DESC
LIMIT @lim;

-- name: GetImportRow :one
SELECT r.*, s.account_id, s.connector, s.settlement_account_id
FROM import_rows r JOIN source_accounts s ON s.id = r.source_account_id
WHERE r.book_id = @book_id AND r.id = @id;

-- name: PendingRowsOfSource :many
SELECT id FROM import_rows WHERE source_account_id = @source_account_id AND status = 'pending' ORDER BY date, id;

-- name: SetRowProposal :exec
UPDATE import_rows SET proposal = @proposal, proposed_account_id = sqlc.narg(proposed_account_id),
    match_transaction_id = sqlc.narg(match_transaction_id), match_row_id = sqlc.narg(match_row_id),
    rule_id = sqlc.narg(rule_id)
WHERE id = @id;

-- name: DecideRow :execrows
UPDATE import_rows SET status = @status, transaction_id = sqlc.narg(transaction_id),
    decided_by = sqlc.narg(decided_by), decided_at = now()
WHERE id = @id AND status = 'pending';

-- name: FindDuplicate :one
-- A transaction already in the books for this row: same account, same
-- amount, within a few days, not already claimed by a row of this account
-- (a typed-in transfer is claimed once from each side).
SELECT t.id FROM postings p JOIN transactions t ON t.id = p.transaction_id
WHERE t.book_id = @book_id AND p.account_id = @account_id AND p.amount = @amount
  AND t.date BETWEEN @from_date AND @to_date
  AND NOT EXISTS (SELECT 1 FROM import_rows r JOIN source_accounts s ON s.id = r.source_account_id
                  WHERE r.book_id = @book_id AND r.status <> 'ignored' AND s.account_id = @account_id
                    AND (r.transaction_id = t.id OR r.match_transaction_id = t.id))
ORDER BY abs(t.date - @on_date::DATE), t.id
LIMIT 1;

-- name: FindEstimate :one
-- An uncleared entry this posted row settles: same account and sign, the
-- amount within the tolerance, a few days either side (a card charge
-- entered as an estimate, or a pending row accepted earlier).
SELECT t.id FROM postings p JOIN transactions t ON t.id = p.transaction_id
WHERE t.book_id = @book_id AND p.account_id = @account_id AND p.status = 'uncleared'
  AND p.amount BETWEEN @lo AND @hi
  AND t.date BETWEEN @from_date AND @to_date
  AND (SELECT count(*) FROM postings x WHERE x.transaction_id = t.id) = 2
  AND NOT EXISTS (SELECT 1 FROM import_rows r WHERE r.book_id = @book_id AND r.status = 'pending'
                  AND r.match_transaction_id = t.id)
ORDER BY abs(p.amount - @amount::NUMERIC), abs(t.date - @on_date::DATE), t.id
LIMIT 1;

-- name: FindTransferPartner :one
-- The other side of a transfer between two of the book's own accounts:
-- another pending row, from another mapped account, the opposite amount in
-- the same currency, a few days apart, not already paired.
SELECT r.id FROM import_rows r JOIN source_accounts s ON s.id = r.source_account_id
WHERE r.book_id = @book_id AND r.status = 'pending' AND r.kind = 'transaction' AND r.id <> @row_id
  AND s.account_id IS NOT NULL AND s.account_id <> @account_id
  AND r.currency = @currency AND r.amount + @amount::NUMERIC = 0
  AND r.date BETWEEN @from_date AND @to_date
  AND r.proposal IN ('new', 'transfer') AND (r.match_row_id IS NULL OR r.match_row_id = @row_id)
ORDER BY abs(r.date - @on_date::DATE), r.id
LIMIT 1;

-- name: ListRules :many
SELECT * FROM import_rules WHERE book_id = @book_id ORDER BY priority, id;

-- name: CreateRule :one
INSERT INTO import_rules (book_id, priority, pattern, source_account_id, account_id, created_by)
VALUES (@book_id, @priority, @pattern, sqlc.narg(source_account_id), @account_id, sqlc.narg(created_by))
RETURNING *;

-- name: DeleteRule :execrows
DELETE FROM import_rules WHERE book_id = @book_id AND id = @id;

-- name: UpsertAssertion :exec
INSERT INTO balance_assertions (book_id, account_id, date, amount, source)
VALUES (@book_id, @account_id, @date, @amount, @source)
ON CONFLICT (account_id, date, source) DO UPDATE SET amount = EXCLUDED.amount;

-- name: AccountDrift :many
-- The newest assertion of each account against what the books say on that
-- date (in the account's own commodity).
SELECT DISTINCT ON (ba.account_id)
       ba.account_id, ba.date, ba.amount AS asserted, ba.source,
       coalesce((SELECT sum(p.amount) FROM postings p JOIN transactions t ON t.id = p.transaction_id
                 WHERE p.account_id = ba.account_id AND t.date <= ba.date), 0)::numeric AS booked
FROM balance_assertions ba
WHERE ba.book_id = @book_id
ORDER BY ba.account_id, ba.date DESC, ba.created_at DESC;

-- name: SetPostingStatus :exec
UPDATE postings SET status = @status, cleared_on = sqlc.narg(cleared_on)
WHERE transaction_id = @transaction_id AND account_id = @account_id AND status <> 'reconciled';

-- name: PendingRowsOfAccount :many
-- Waiting transaction rows on a ledger account, near a date: matched again
-- once a trade has booked the cash they may duplicate.
SELECT r.id FROM import_rows r JOIN source_accounts s ON s.id = r.source_account_id
WHERE r.book_id = @book_id AND r.status = 'pending' AND r.kind = 'transaction'
  AND s.account_id = @account_id AND r.date BETWEEN @from_date AND @to_date
ORDER BY r.date, r.id;

-- name: GetSourceSecurity :one
SELECT account_id FROM source_securities WHERE source_account_id = @source_account_id AND commodity = @commodity;

-- name: SetSourceSecurity :exec
INSERT INTO source_securities (source_account_id, commodity, account_id)
VALUES (@source_account_id, @commodity, @account_id)
ON CONFLICT (source_account_id, commodity) DO UPDATE SET account_id = EXCLUDED.account_id;

-- name: FindInvoiceMatch :one
-- The booked payment an invoice belongs to: a card, bank or cash line of
-- its total, from two days before to five after (a card posts late), on a
-- transaction that has no invoice yet and that no other waiting invoice
-- claims. The seller's name in the payee wins a tie, then the nearest day.
SELECT t.id FROM postings p
JOIN transactions t ON t.id = p.transaction_id
JOIN accounts a ON a.id = p.account_id
WHERE t.book_id = @book_id AND a.class IN ('asset', 'liability')
  AND p.commodity = @currency AND p.amount = @amount
  AND t.date BETWEEN @from_date AND @to_date
  AND NOT EXISTS (SELECT 1 FROM transaction_items i WHERE i.transaction_id = t.id)
  AND NOT EXISTS (SELECT 1 FROM import_rows o WHERE o.book_id = @book_id AND o.kind = 'invoice' AND o.status = 'pending'
                  AND o.match_transaction_id = t.id AND o.id <> @row_id)
ORDER BY (@seller::TEXT <> '' AND t.payee ILIKE '%' || @seller::TEXT || '%') DESC, abs(t.date - @on_date::DATE), t.id
LIMIT 1;

-- name: FindInvoiceRow :one
-- A card or bank row still waiting in the queue that will pay for it.
SELECT r.id FROM import_rows r
WHERE r.book_id = @book_id AND r.status = 'pending' AND r.kind = 'transaction'
  AND r.currency = @currency AND r.amount = @amount AND r.date BETWEEN @from_date AND @to_date
  AND NOT EXISTS (SELECT 1 FROM import_rows o WHERE o.book_id = @book_id AND o.kind = 'invoice' AND o.status = 'pending'
                  AND o.match_row_id = r.id AND o.id <> @row_id)
ORDER BY (@seller::TEXT <> '' AND (r.description ILIKE '%' || @seller::TEXT || '%' OR r.counterparty ILIKE '%' || @seller::TEXT || '%')) DESC,
         abs(r.date - @on_date::DATE), r.id
LIMIT 1;

-- name: PendingInvoiceRows :many
SELECT id FROM import_rows WHERE book_id = @book_id AND status = 'pending' AND kind = 'invoice' ORDER BY date, id LIMIT 500;

-- name: SetRowItems :exec
UPDATE import_rows SET items = @items WHERE book_id = @book_id AND id = @id;

-- name: FindDuplicateRow :one
-- The same charge waiting from another source: a pending transaction row
-- on the same ledger account, the same amount and currency, a few days
-- apart, from another source account (one source's two identical coffees
-- are two), not already paired with a third.
SELECT r.id, r.pending FROM import_rows r JOIN source_accounts s ON s.id = r.source_account_id
WHERE r.book_id = @book_id AND r.status = 'pending' AND r.kind = 'transaction' AND r.id <> @row_id
  AND s.account_id = @account_id AND r.source_account_id <> @source_account_id
  AND r.currency = @currency AND r.amount = @amount AND r.date BETWEEN @from_date AND @to_date
  AND (r.match_row_id IS NULL OR r.match_row_id = @row_id)
  AND NOT EXISTS (SELECT 1 FROM import_rows o WHERE o.book_id = @book_id AND o.status = 'pending' AND o.id <> @row_id
                  AND o.proposal = 'duplicate' AND o.match_row_id = r.id)
ORDER BY abs(r.date - @on_date::DATE), r.id
LIMIT 1;

-- name: RowsMatchingRow :many
-- Waiting rows that point at a row: matched again once it is decided.
SELECT id FROM import_rows WHERE book_id = @book_id AND status = 'pending' AND match_row_id = @row_id;

-- name: FindSameInvoiceTx :one
-- A payment another source's invoice already added its lines to: a card,
-- bank or cash line of this total, in the window, carrying items from a
-- connector that is not this one.
SELECT t.id FROM postings p
JOIN transactions t ON t.id = p.transaction_id
JOIN accounts a ON a.id = p.account_id
WHERE t.book_id = @book_id AND a.class IN ('asset', 'liability')
  AND p.commodity = @currency AND p.amount = @amount
  AND t.date BETWEEN @from_date AND @to_date
  AND EXISTS (SELECT 1 FROM transaction_items i WHERE i.transaction_id = t.id AND i.source NOT LIKE @connector::TEXT || ':%')
ORDER BY abs(t.date - @on_date::DATE), t.id
LIMIT 1;

-- name: FindSameInvoiceRow :one
-- Another source's invoice for it, still waiting and staged earlier: the
-- first keeps its proposal, so two never wait on each other.
SELECT o.id FROM import_rows o JOIN source_accounts s ON s.id = o.source_account_id
WHERE o.book_id = @book_id AND o.status = 'pending' AND o.kind = 'invoice' AND o.id < @row_id
  AND s.connector <> @connector AND o.currency = @currency AND o.amount = @amount
  AND o.date BETWEEN @from_date AND @to_date AND o.proposal IN ('enrich', 'new', 'waiting')
ORDER BY abs(o.date - @on_date::DATE), o.id
LIMIT 1;
