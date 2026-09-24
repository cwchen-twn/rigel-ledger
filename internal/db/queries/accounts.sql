-- name: CreateAccount :one
INSERT INTO accounts (book_id, parent_id, class, name, template_key, code, commodity,
                      is_current, is_cash, cf_class, is_placeholder)
VALUES (@book_id, sqlc.narg(parent_id), @class, sqlc.narg(name), sqlc.narg(template_key), sqlc.narg(code),
        sqlc.narg(commodity), @is_current, @is_cash, @cf_class, @is_placeholder)
RETURNING *;

-- name: ListAccounts :many
SELECT * FROM accounts WHERE book_id = $1 ORDER BY class, coalesce(code, ''), id;

-- name: GetAccount :one
SELECT * FROM accounts WHERE book_id = $1 AND id = $2;

-- name: GetAccountByTemplateKey :one
SELECT * FROM accounts WHERE book_id = $1 AND template_key = $2;

-- name: UpdateAccount :one
UPDATE accounts SET
    parent_id      = sqlc.narg(parent_id),
    name           = sqlc.narg(name),
    code           = sqlc.narg(code),
    is_current     = @is_current,
    is_cash        = @is_cash,
    cf_class       = @cf_class,
    is_placeholder = @is_placeholder
WHERE book_id = @book_id AND id = @id
RETURNING *;

-- name: SetAccountArchived :one
UPDATE accounts SET archived_at = CASE WHEN @archived::boolean THEN now() END
WHERE book_id = @book_id AND id = @id
RETURNING *;

-- name: DeleteAccount :execrows
DELETE FROM accounts WHERE book_id = $1 AND id = $2;

-- name: AccountHasPostings :one
SELECT EXISTS (SELECT 1 FROM postings WHERE account_id = $1);

-- name: AccountHasChildren :one
SELECT EXISTS (SELECT 1 FROM accounts WHERE parent_id = $1);

-- name: AccountSums :many
-- Balances as of a date, per account and commodity. Income and expense
-- accounts can hold several currencies, hence the commodity grouping.
SELECT p.account_id,
       p.commodity,
       coalesce(sum(p.amount), 0)::numeric      AS amount,
       coalesce(sum(p.base_amount), 0)::numeric AS base_amount
FROM postings p
JOIN transactions t ON t.id = p.transaction_id
WHERE t.book_id = @book_id AND t.date <= @as_of
GROUP BY p.account_id, p.commodity;

-- name: AccountCostBasis :one
-- What an account holds and what it cost, up to a date: the input to average
-- cost when miles, points or shares leave it. Excludes one transaction, so
-- editing a redemption does not count the redemption itself.
SELECT coalesce(sum(p.amount), 0)::numeric      AS quantity,
       coalesce(sum(p.base_amount), 0)::numeric AS cost
FROM postings p
JOIN transactions t ON t.id = p.transaction_id
WHERE p.account_id = @account_id
  AND t.date <= @as_of
  AND t.id <> @exclude_transaction_id;

-- name: AccountSumsBetween :many
-- Movements in [from, to], per account and commodity: the income statement.
SELECT p.account_id,
       p.commodity,
       coalesce(sum(p.amount), 0)::numeric      AS amount,
       coalesce(sum(p.base_amount), 0)::numeric AS base_amount
FROM postings p
JOIN transactions t ON t.id = p.transaction_id
WHERE t.book_id = @book_id AND t.date BETWEEN @from_date AND @to_date
GROUP BY p.account_id, p.commodity;

-- name: CashFlowPostings :many
-- Every posting of the transactions in [from, to] that move a cash account:
-- the direct-method cash flow statement attributes each one's cash movement
-- to its other legs.
SELECT p.transaction_id, p.account_id, p.base_amount::numeric AS base_amount, a.is_cash
FROM postings p
JOIN transactions t ON t.id = p.transaction_id
JOIN accounts a ON a.id = p.account_id
WHERE t.book_id = @book_id AND t.date BETWEEN @from_date AND @to_date
  AND EXISTS (
      SELECT 1 FROM postings p2 JOIN accounts a2 ON a2.id = p2.account_id
      WHERE p2.transaction_id = t.id AND a2.is_cash
  )
ORDER BY p.transaction_id, p.id;
