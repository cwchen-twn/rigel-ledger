-- Exchange-rate scheduler: one row per attempt to fetch a day's rates, so
-- the job knows what it has and the Administration page can show it.
CREATE TABLE rate_fetches (
    id         BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    source     TEXT NOT NULL,
    -- The day the provider says its rates are for (NULL when it failed).
    rate_date  DATE,
    -- A backfill asks for a day; a refresh asks for "latest" (NULL).
    requested  DATE,
    rates      INT NOT NULL DEFAULT 0,
    skipped    INT NOT NULL DEFAULT 0, -- refused by a book's lock date
    error      TEXT NOT NULL DEFAULT '',
    fetched_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX rate_fetches_recent ON rate_fetches (fetched_at DESC);
CREATE INDEX rate_fetches_day ON rate_fetches (rate_date) WHERE error = '';
