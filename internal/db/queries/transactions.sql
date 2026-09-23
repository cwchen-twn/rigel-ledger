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
