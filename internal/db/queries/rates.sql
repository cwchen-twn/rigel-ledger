-- name: RecordRateFetch :one
INSERT INTO rate_fetches (source, rate_date, requested, rates, skipped, error)
VALUES (@source, sqlc.narg(rate_date), sqlc.narg(requested), @rates, @skipped, @error)
RETURNING *;

-- name: ListRateFetches :many
SELECT * FROM rate_fetches ORDER BY fetched_at DESC LIMIT @lim;

-- name: LastSuccessfulFetch :one
SELECT * FROM rate_fetches WHERE error = '' ORDER BY fetched_at DESC LIMIT 1;

-- name: FetchedDays :many
-- Days in [from, to] that already have a successful fetch.
SELECT DISTINCT rate_date::DATE AS day FROM rate_fetches
WHERE error = '' AND rate_date BETWEEN @from_date AND @to_date;

-- name: FailedTodayFor :one
-- Failed attempts for a requested day since midnight UTC: the backfill
-- gives up on a day after three.
SELECT count(*) FROM rate_fetches
WHERE error <> '' AND requested = @day AND fetched_at > date_trunc('day', now());

-- name: BookCurrencies :many
-- The currencies a book holds or reports in: its base, its accounts', and
-- its members' display currencies.
SELECT DISTINCT code FROM (
    SELECT b.base_currency AS code FROM books b WHERE b.id = @book_id
    UNION SELECT a.commodity FROM accounts a
        JOIN commodities c ON c.code = a.commodity
        WHERE a.book_id = @book_id AND c.kind = 'currency'
    UNION SELECT u.display_currency FROM book_members m JOIN users u ON u.id = m.user_id WHERE m.book_id = @book_id
) x WHERE code IS NOT NULL ORDER BY code;
