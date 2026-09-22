-- name: ListCurrencies :many
SELECT * FROM commodities WHERE kind = 'currency' ORDER BY code;

-- name: GetCommodity :one
SELECT * FROM commodities WHERE code = $1;
