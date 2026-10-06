-- name: InsertTransactionItem :exec
INSERT INTO transaction_items (book_id, transaction_id, position, description, quantity, unit_price, amount, account_id, source)
VALUES (@book_id, @transaction_id, @position, @description, sqlc.narg(quantity), sqlc.narg(unit_price), @amount,
        sqlc.narg(account_id), @source);

-- name: ListTransactionItems :many
SELECT * FROM transaction_items WHERE transaction_id = ANY(@transaction_ids::BIGINT[]) ORDER BY transaction_id, position;
