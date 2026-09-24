-- name: CreateTransaction :one
INSERT INTO transactions (book_id, date, payee, memo, source, external_id, created_by, updated_by)
VALUES (@book_id, @date, @payee, @memo, @source, sqlc.narg(external_id), @user_id, @user_id)
RETURNING *;

-- name: UpdateTransaction :one
UPDATE transactions SET date = @date, payee = @payee, memo = @memo, updated_by = @user_id
WHERE book_id = @book_id AND id = @id
RETURNING *;

-- name: GetTransaction :one
SELECT * FROM transactions WHERE book_id = $1 AND id = $2;

-- name: DeleteTransaction :execrows
DELETE FROM transactions WHERE book_id = $1 AND id = $2;

-- name: CreatePosting :one
INSERT INTO postings (transaction_id, account_id, position, commodity, amount, base_amount,
                      unit_cost, status, cleared_on, memo)
VALUES (@transaction_id, @account_id, @position, @commodity, @amount, @base_amount,
        sqlc.narg(unit_cost), @status, sqlc.narg(cleared_on), @memo)
RETURNING *;

-- name: DeletePostings :exec
DELETE FROM postings WHERE transaction_id = $1;

-- name: ListPostings :many
SELECT * FROM postings WHERE transaction_id = ANY(@transaction_ids::bigint[])
ORDER BY transaction_id, position, id;

-- name: ListTransactions :many
-- Newest first, keyset-paginated on (date, id). account_ids is the selected
-- account plus its descendants, computed in Go; empty means no filter.
SELECT t.*
FROM transactions t
WHERE t.book_id = @book_id
  AND (sqlc.narg(from_date)::date IS NULL OR t.date >= sqlc.narg(from_date)::date)
  AND (sqlc.narg(to_date)::date IS NULL OR t.date <= sqlc.narg(to_date)::date)
  AND (cardinality(@account_ids::bigint[]) = 0 OR EXISTS (
        SELECT 1 FROM postings p WHERE p.transaction_id = t.id AND p.account_id = ANY(@account_ids::bigint[])))
  AND (sqlc.narg(q)::text IS NULL
       OR t.payee ILIKE '%' || sqlc.narg(q)::text || '%'
       OR t.memo ILIKE '%' || sqlc.narg(q)::text || '%')
  AND (sqlc.narg(tag_id)::bigint IS NULL OR EXISTS (
        SELECT 1 FROM transaction_tags tt WHERE tt.transaction_id = t.id AND tt.tag_id = sqlc.narg(tag_id)::bigint))
  AND (sqlc.narg(cursor_date)::date IS NULL
       OR (t.date, t.id) < (sqlc.narg(cursor_date)::date, sqlc.narg(cursor_id)::bigint))
ORDER BY t.date DESC, t.id DESC
LIMIT @lim;

-- name: UpsertTag :one
INSERT INTO tags (book_id, name) VALUES ($1, $2)
ON CONFLICT (book_id, lower(name)) DO UPDATE SET name = tags.name
RETURNING *;

-- name: ListTags :many
SELECT * FROM tags WHERE book_id = $1 ORDER BY lower(name);

-- name: ClearTransactionTags :exec
DELETE FROM transaction_tags WHERE transaction_id = $1;

-- name: AddTransactionTag :exec
INSERT INTO transaction_tags (transaction_id, tag_id) VALUES ($1, $2) ON CONFLICT DO NOTHING;

-- name: ListTransactionTags :many
SELECT tt.transaction_id, t.name
FROM transaction_tags tt
JOIN tags t ON t.id = tt.tag_id
WHERE tt.transaction_id = ANY(@transaction_ids::bigint[])
ORDER BY lower(t.name);

-- name: TagSummaries :many
-- Every tag of a book with its span and what its transactions spent
-- (expense postings at their stored base amounts: miles at cost).
SELECT tg.name,
       count(DISTINCT t.id)::BIGINT                                          AS transactions,
       min(t.date)::DATE                                                     AS first_date,
       max(t.date)::DATE                                                     AS last_date,
       coalesce(sum(p.base_amount) FILTER (WHERE a.class = 'expense'), 0)::numeric AS expenses
FROM tags tg
JOIN transaction_tags tt ON tt.tag_id = tg.id
JOIN transactions t ON t.id = tt.transaction_id
JOIN postings p ON p.transaction_id = t.id
JOIN accounts a ON a.id = p.account_id
WHERE tg.book_id = @book_id
GROUP BY tg.name
ORDER BY max(t.date) DESC, tg.name;

-- name: TagExpenses :many
-- One tag's expense postings, per account.
SELECT p.account_id, coalesce(sum(p.base_amount), 0)::numeric AS base_amount
FROM tags tg
JOIN transaction_tags tt ON tt.tag_id = tg.id
JOIN postings p ON p.transaction_id = tt.transaction_id
JOIN accounts a ON a.id = p.account_id
WHERE tg.book_id = @book_id AND lower(tg.name) = lower(@name) AND a.class = 'expense'
GROUP BY p.account_id;

-- name: RebasePostings :many
-- Every posting of a book with its transaction date, for a change of base.
SELECT p.id, p.transaction_id, p.account_id, p.commodity, p.amount, p.base_amount, t.date
FROM postings p JOIN transactions t ON t.id = p.transaction_id
WHERE t.book_id = @book_id
ORDER BY t.id, p.position;

-- name: SetPostingBase :exec
UPDATE postings SET base_amount = @base_amount WHERE id = @id;

-- name: MaxPostingPosition :one
SELECT coalesce(max(position), 0)::INT FROM postings WHERE transaction_id = @transaction_id;
