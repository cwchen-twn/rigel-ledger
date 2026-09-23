-- name: ListCurrencies :many
SELECT * FROM commodities WHERE kind = 'currency' ORDER BY code;

-- name: ListCommodities :many
-- Everything an account can hold: currencies first, then securities and points.
SELECT * FROM commodities ORDER BY (kind <> 'currency'), kind, code;

-- name: GetCommodity :one
SELECT * FROM commodities WHERE code = $1;

-- name: CreateCommodity :one
INSERT INTO commodities (code, kind, name, decimals, quote_currency, exchange_mic, contract_size)
VALUES (@code, @kind, @name, @decimals, sqlc.narg(quote_currency), sqlc.narg(exchange_mic), sqlc.narg(contract_size))
RETURNING *;
