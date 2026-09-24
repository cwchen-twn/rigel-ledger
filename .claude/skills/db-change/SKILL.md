---
name: db-change
description: How to change the rigel-ledger database schema or SQL queries safely - where the schema and queries live, regenerating sqlc, the pre-deploy rule of editing 000001_init in place, trigger conventions, WithTx and audit, and the DB tests every rule needs. Use when adding or altering a table, column, trigger, enum or query, or when a sqlc/migration/CI "sqlc diff" failure appears.
---

# Changing the schema or queries (rigel-ledger)

## Where things live

| What | Where |
|---|---|
| The schema | `migrations/000001_init.up.sql`, then one pair per change (`000002_accounts_admin`, ...) |
| Queries | `internal/db/queries/<domain>.sql` |
| Generated Go | `internal/db/*.sql.go`, `models.go`, `db.go` -- **never edit by hand** |
| sqlc config | `sqlc.yaml` (numeric -> `decimal.Decimal`, date/timestamptz -> `time.Time`) |
| Pool, `WithTx`, `Migrate` | `internal/db/store.go` |
| Bookkeeping rules | `internal/ledger/` |
| Trigger tests (raw SQL) | `internal/db/schema_test.go` |

## The migration rule

**The app is deployed (2026-09-24): never edit an applied migration.**
`000001_init` and `000002_accounts_admin` are frozen. A schema change is a new
pair: `make migrations/new name=<what>`, with both up and down, and the down
must leave the database the previous migration expects (see `000002`'s down for
how to restore a replaced function). Add the pair to
`internal/db/migrations_test.go`'s round trip when it touches existing data.

Before the deploy the rule was the opposite (one readable `000001`, edited in
place, `make db/reset` after each change); that is history now.

Keep the SQL PostgreSQL-14 compatible (local dev); CI and hcloud run 18.

## Steps

1. Change the schema and/or the query files.
2. `make sqlc`. It compiles sqlc with cgo the first time (~3 min), then is fast.
   CI runs `sqlc diff` with the release binary and fails on any drift, so always
   commit the regenerated files with the SQL that caused them.
3. If a new column or query uses a new type, check the generated struct: money
   must come out as `decimal.Decimal`/`NullDecimal`, never `pgtype.Numeric` or
   `float64`. Add an override in `sqlc.yaml` if needed.
4. Use the new query from `internal/ledger`, not from handlers.
5. Add tests (below), then `make audit` with `TEST_DATABASE_URL` set.

## Conventions the schema already follows -- keep them

- **Money** is `NUMERIC(24,8)`, rates unscaled `NUMERIC`. Amounts are signed:
  debit > 0, credit < 0.
- **Triggers name their failures.** `RAISE EXCEPTION ... USING ERRCODE =
  'check_violation', CONSTRAINT = '<code>'`. `ledger.translate` turns the
  constraint name into the API error code, so the frontend can translate it as
  `error.<code>`. Pick a short snake_case code and add it to the i18n files.
- **Go validates first, the trigger is the backstop.** A rule the user can
  trip belongs in `internal/ledger` too, with a field-level error.
- **Deferred constraint triggers** run at COMMIT, so their error surfaces from
  `WithTx`'s commit, not from the INSERT.
- **Every write goes through `store.WithTx(ctx, userID, fn)`**, which sets
  `app.current_user` for `audit_row()`. A new table that users edit gets an
  `audit_row` trigger (and a branch in `audit_row()` if its book id is not a
  `book_id` column).
- **Book scoping**: every domain table carries `book_id` (or reaches it through
  its parent), and every query filters by it. Access is resolved once per request
  into `ledger.Access`; never trust an id from the client without the book filter.
- **Lock date covers `prices`.** Rates are global, so `check_price_lock` freezes a rate
  for any book that uses either currency and is locked on or after its date. Anything
  new that feeds a closed period's statements needs the same protection.
- **Translations are not data.** Store stable keys; names live in
  `web/src/i18n/*.json`.
- **Secrets the database must hold** (the SMTP password; TOTP seeds next) are sealed
  with `internal/secretbox` under `APP_ENCRYPTION_KEY`, are never returned by the
  API, and are stripped from `audit_log` (see the `system_settings` branch of
  `audit_row()` in `000002`).
- **A nullable `host(ip)::TEXT` must be `coalesce`d**: sqlc types a computed text
  column as `string`, and a NULL fails the scan at runtime (the admin log did).

## Tests every change needs

- A new or changed **trigger**: a test in `internal/db/schema_test.go` that
  writes raw SQL and asserts the constraint name with `wantConstraint`. This
  proves the rule holds without Go.
- A new **ledger rule**: a test in `internal/ledger/ledger_test.go` asserting the
  `*ledger.Error` code (and field) with `wantCode`.
- A new **endpoint**: a round trip in `internal/routes/api_test.go`.

DB tests use `dbtest.New(t)`, which creates and drops a fresh migrated database.
Without `TEST_DATABASE_URL` they skip. CI sets `REQUIRE_DB_TESTS=1` so they
cannot silently skip there.
