-- name: UpsertPrice :one
INSERT INTO prices (commodity, quote, date, rate, source, created_by)
VALUES (@commodity, @quote, @date, @rate, @source, sqlc.narg(created_by))
ON CONFLICT (commodity, quote, date, source) DO UPDATE SET rate = EXCLUDED.rate, created_by = EXCLUDED.created_by
RETURNING *;

-- name: ListPrices :many
SELECT * FROM prices
WHERE (sqlc.narg(commodity)::text IS NULL OR commodity = sqlc.narg(commodity)::text OR quote = sqlc.narg(commodity)::text)
ORDER BY date DESC, commodity, quote
LIMIT @lim;

-- name: LatestPrice :one
-- The newest rate on or before the date; on the same day a manual rate wins.
SELECT * FROM prices
WHERE commodity = @commodity AND quote = @quote AND date <= @on_date
ORDER BY date DESC, (source = 'manual') DESC
LIMIT 1;

-- name: DeletePrice :execrows
DELETE FROM prices WHERE id = $1 AND source = 'manual';
