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

-- name: EnsureSecurity :exec
-- A security a source names, the first time it does; an existing code is
-- left as it is (its name may have been corrected by hand).
INSERT INTO commodities (code, kind, name, decimals, quote_currency)
VALUES (@code, 'security', @name, @decimals, @quote_currency)
ON CONFLICT (code) DO NOTHING;
