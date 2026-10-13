-- Reading a whole book at once for its export file (ledger/bookfile.go),
-- and the one write an import needs that the other queries do not offer.

-- name: ExportTransactions :many
SELECT * FROM transactions WHERE book_id = @book_id ORDER BY date, id;

-- name: ExportPostings :many
SELECT p.* FROM postings p JOIN transactions t ON t.id = p.transaction_id
WHERE t.book_id = @book_id
ORDER BY p.transaction_id, p.position, p.id;

-- name: ExportTransactionTags :many
SELECT tt.transaction_id, g.name FROM transaction_tags tt JOIN tags g ON g.id = tt.tag_id
WHERE g.book_id = @book_id
ORDER BY tt.transaction_id, lower(g.name);

-- name: ExportItems :many
SELECT * FROM transaction_items WHERE book_id = @book_id ORDER BY transaction_id, position;

-- name: ExportAssertions :many
SELECT * FROM balance_assertions WHERE book_id = @book_id ORDER BY account_id, date, source;

-- name: ExportSourceSecurities :many
SELECT ss.* FROM source_securities ss JOIN source_accounts s ON s.id = ss.source_account_id
WHERE s.book_id = @book_id
ORDER BY ss.source_account_id, ss.commodity;

-- name: ExportPrices :many
-- Every rate between two commodities the book uses, from any source.
SELECT * FROM prices
WHERE commodity = ANY(@codes::text[]) AND quote = ANY(@codes::text[])
ORDER BY commodity, quote, date, source;

-- name: ImportPrice :execrows
-- Prices are shared by every book, so an imported one only fills a day that
-- has no rate for the pair at all, and never a day a locked book has frozen
-- (the same condition as check_price_lock, which would otherwise abort the
-- whole import).
INSERT INTO prices (commodity, quote, date, rate, source, created_by)
SELECT @commodity::text, @quote::text, @date::date, @rate::numeric, 'import', sqlc.narg(created_by)::bigint
WHERE NOT EXISTS (SELECT 1 FROM prices p
                  WHERE p.commodity = @commodity::text AND p.quote = @quote::text AND p.date = @date::date)
  AND NOT EXISTS (SELECT 1 FROM books b
                  WHERE b.lock_date >= @date::date
                    AND (b.base_currency IN (@commodity::text, @quote::text)
                         OR EXISTS (SELECT 1 FROM accounts a
                                    WHERE a.book_id = b.id AND a.commodity IN (@commodity::text, @quote::text))));
