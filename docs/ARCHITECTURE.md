# RigelLedger target architecture

Status: **accepted direction; P1 backend implemented** (2026-09-22). The schema
(`migrations/000001_init.up.sql`), sqlc data layer, sessions and the book-scoped JSON
API follow this document, and so does the SolidJS frontend. The app has never been deployed, so nothing here
needs a data migration path.

## Goals

- Double-entry bookkeeping for **personal and family** use, not for a company.
- Follow IFRS where it makes sense for a household, and stay as simple as possible.
- Balance sheet, income statement and cash flow statement.
- Multi-currency (TWD, USD, PYG, ...) with exchange rates updated automatically.
- Daily manual entry from web and phone.
- Import credit card, bank and futures-broker PDF statements. Parsing stays local, and no
  statement data leaves the cluster.
- Stock investments, including Firstrade sync.
- Per-user settings (language, display currency, ...) that can be changed at any time.
- Packaged as a container, deployed by the hcloud repo.

## Shape

```
PostgreSQL 18 (hcloud host, 10.0.1.1)
        |
Go binary  -- chi JSON API, embedded SolidJS SPA, migrations on start,
        |     in-process scheduler (exchange rates, prices)
        |
   +----+-----------------+----------------------+
SolidJS PWA (web)   Flutter (later)     firstrade-sync CronJob (Python)
```

- **One JSON API for every client.** This is why the web stays a SolidJS SPA rather than
  htmx: with htmx, every endpoint would have to be written twice, once as HTML fragments
  and once as JSON for mobile. SolidStart was rejected because it needs a Node SSR server
  next to Go.
- **UI: SolidJS + Tailwind v4 + Kobalte, in shadcn/ui's design.** shadcn/ui itself is
  React-only; switching to React for it was considered and declined to keep Solid.
  Instead the shadcn look lives in this repo, the same copy-in way shadcn works:
  - design tokens as CSS variables (`web/src/styles/globals.css`), with dark mode as one
    class on `<html>`;
  - our own components in `web/src/components/ui/`, using `cva` variants and `cn()`;
  - Kobalte (Solid's counterpart of Radix) only where accessibility is hard: Dialog/Sheet,
    Combobox, DropdownMenu and Toast. Selects and checkboxes are native elements.

  Bootstrap is gone.
- **PWA first.** It covers phone entry and receipt photos. Build Flutter only if a native
  feature is actually needed.

## Why the schema is reset

The current migrations (000001-000005) were reviewed against the goals.

| Need | Current schema | Why |
|---|---|---|
| Double entry | Partial | `check_journal_balance` runs on INSERT only, and it sums raw `amount` across currencies. |
| Multi-currency | Broken | There is one `exchange_rate` per journal, with no direction and no base currency. A USD card paying for a TWD expense is rejected as unbalanced. The base currency is effectively `users.main_currency`, which is a UI preference. |
| Balance sheet | Drifts | The stored `user_ledgers.balance` changes only on posting INSERT, and `PUT /ledgers` lets the client overwrite it. There is no FX revaluation, no retained earnings and no opening-balance equity. |
| Income statement | Wrong if multi-currency | It sums raw amounts in mixed currencies. |
| Cash flow | Impossible | There is no is-cash flag and no operating/investing/financing classification. |
| Stocks | Impossible | No quantity, cost basis or security identity. `ref_stock_prices` has a symbol with no exchange and no currency. `journal.stock_price_ref` attaches one market price to a whole journal. |
| Daily entry | Heavy | The user picks one of 254 business account types first. |
| Imports | Missing | No external id or dedupe key, no staging area, no statement link. |
| Family | Impossible | Everything is keyed to one `username`. |
| Settings | Conflated | `main_currency` is both the display preference and the accounting base. |
| Rates | Buggy | `ref_*` BIGINT PKs have no sequence. NUMERIC(20,6) truncates small rates: PYG->USD 0.000136986 is stored as 0.000137. There is no `source` column. |

### The 4-grade account reference tables are dropped

`ref_ledger_first_grade`, `ref_ledger_second_grade`, `ref_ledger_third_grade` and
`ref_ledger_types` do not fit the goal.

- **Wrong source.** They are Taiwan's GCIS business chart of accounts (商業會計項目表),
  not an IFRS taxonomy, and most of it is company-only:
  - cost of goods sold (5xxx, 25 rows);
  - operating expenses split three ways into selling, G&A and R&D (6xxx, about 60 rows);
  - agency revenue, minority interest and depletable assets.

  The household part is the bolted-on non-numeric grades `A` and `B`.
- **Rigid in the wrong place.** Four levels are global and frozen. A user can only hang
  leaf accounts under a level-4 type and cannot nest their own accounts, e.g.
  Food > Groceries > Costco.
- **The codes encode layout, not meaning.** "Current assets" appears twice, as `11` and
  `12`.
- **Redundant and unconstrained.** `first_grade` and `second_grade` are repeated on child
  rows, and nothing checks that they agree with the code prefix.
- **Reports need very little from them.** They need only four facts per account: class,
  current or non-current, is-cash, and cash-flow class. Those become columns on
  `accounts`. Grouping comes from the user's own `parent_id` tree. An optional free-text
  `code` column keeps numbering such as `1113` for anyone who wants it.

### No trial-balance feature, no period close

A trial balance historically did two jobs.

1. **Arithmetic control.** It proved that hand posting kept debits equal to credits.
   Here a deferred constraint trigger enforces `SUM(base_amount) = 0` per transaction on
   INSERT, UPDATE and DELETE, so the books balance **by construction**. Checking again
   is redundant, and so is a closing workflow with closing entries.
2. **Working view.** It lists every account's balance at a date. This is still useful for
   review: wrong signs, or a forgotten "Uncategorised". It is provided as one
   `account_balances(book, as_of)` query behind an "All accounts" page. The balance sheet
   and income statement are grouped projections of the same query.

Retained earnings are cumulative net income up to the period start, computed on the fly.
The only closing concept kept is a per-book **lock date**, which refuses edits before
that date and so protects reconciled history.

**Volume check.** A family books roughly 10-20k postings a year, so twenty years is at
most about 400k rows. An indexed SUM over `(account_id, date)` takes milliseconds. So
there is **no stored balance and no materialised view.**

### What survives from the old design

- The split between journal header and postings.
- `NUMERIC` in the database with `shopspring/decimal` in Go.
- The idea of a deferred constraint trigger.
- `set_config('app.current_user', ...)` feeding the audit triggers.
- ISO reference data.
- i18n keyed by stable codes.
- Stricter currency rules for balance-sheet accounts than for income and expense
  accounts.

### Other defects that the reset removes

- FKs on the `username` text (`ledger_owner`, `user_ledger_journal.user_id`, which
  actually holds a username, and `changed_by`), mixed with `users.id` elsewhere.
- A generic RBAC system (roles, permissions and two join tables for four permissions)
  that no code checks.
- `transac_id`, a manual per-user sequence with no generator, so concurrent inserts race.
- `is_reconciled` on the journal header. Reconciliation is per account statement: a card
  payment reconciles on the bank statement and on the card statement separately.
- `photo_addr TEXT[]` in place of an attachments table.
- An `updated_at` trigger on `user_ledger_postings`, which has no such column, so any
  UPDATE fails.
- `posting_type` D/C plus a non-negative amount. A signed amount makes every SUM trivial,
  and the UI still shows Dr/Cr columns.
- The unused `ref_countries_iso3166_1` table, and only 3 currencies seeded.

## Data model (one fresh `000001` migration)

```
users(id, username, email, password_hash, display_name, is_admin,
      language, display_currency, timezone, date_format, default_book_id, ...)
sessions(id, user_id, token_hash, kind web|api, label, expires_at, last_used_at)
books(id, name, base_currency, lock_date, dividend_cf_class, ...)
book_members(book_id, user_id, role owner|editor|viewer)      PK(book_id, user_id)
commodities(id, kind currency|security, code, name, quote_currency,
            exchange_mic, decimals)        -- every ISO 4217 code seeded as currency
prices(commodity_id, quote_id, date, rate NUMERIC, source manual|er-api|fawaz|...)
      UNIQUE(commodity_id, quote_id, date, source)  -- manual wins on read
accounts(id, book_id, parent_id, name, code NULL, class, commodity_id NULL,
         is_current, is_cash, cf_class, is_placeholder, template_key, archived_at)
         -- commodity required for asset/liability/equity; NULL = multi for I/E
transactions(id, book_id, date, payee, memo, source, external_id, import_row_id,
             created_by, updated_by, ...)   UNIQUE(book_id, source, external_id)
postings(id, transaction_id, account_id, commodity_id, amount NUMERIC signed,
         base_amount NUMERIC signed, quantity NULL, unit_cost NULL,
         status uncleared|cleared|reconciled, cleared_on, memo)
  -- deferred constraint trigger: SUM(base_amount) = 0 per txn on I/U/D
  -- trigger: commodity = account.commodity when set; date > book.lock_date
tags(id, book_id, name, kind);  transaction_tags(transaction_id, tag_id)
attachments(id, book_id, sha256, filename, mime, bytes BYTEA, transaction_id NULL)
import_batches(id, book_id, account_id, source, parser, attachment_id,
               period_start, period_end, statement_closing_balance)
import_rows(id, batch_id, date, amount, currency, description, raw JSONB,
            fingerprint, status pending|posted|ignored|duplicate, transaction_id)
rules(id, book_id, priority, match JSONB, account_id, tag_ids)
audit_log(id, book_id, table_name, row_id, action, old JSONB, new JSONB,
          changed_by, changed_at)
```

Precision and naming rules:

- Amounts are `NUMERIC(24,8)`. Rates are unscaled `NUMERIC`.
- `base_amount` is rounded to the base currency's minor unit. A rounding residue of at
  most one minor unit goes to an "FX rounding" account, so the sum is exactly zero.
- Signed amounts follow the convention debit > 0, credit < 0.

Family and account setup:

- **Books and members.** A book is one set of accounts in one base currency. A family
  can keep a shared book and personal books side by side, and every domain table
  carries `book_id`.
- **"Who spent it"** is a tag (person, trip, project), not a separate account, so the
  chart of accounts stays small.
- **The chart of accounts** starts from a ~40-account personal template that is copied
  into a new book and is fully editable afterwards. Template names are i18n keys.

## Multi-currency

- Every balance-sheet account has exactly one commodity: TWD cash, USD brokerage cash,
  PYG savings, AAPL shares, and so on. Income and expense accounts may receive any
  currency.
- A transaction may mix currencies. Each posting stores its native `amount` and a
  `base_amount` computed at the transaction-date rate. The UI pre-fills the rate from
  `prices`, and the user can override it.
- Balance is enforced on `base_amount`. The stored base amount is deliberate: IAS 21
  books a transaction at its **historical** rate, and computing it on the fly would
  silently move history whenever a rate is corrected.
- Realised FX differences on settlement, and unrealised FX differences at the report
  date, are computed report lines, not hand entries.

## Exchange rates

- A Go `RateProvider` interface, run by a daily in-process scheduler, plus an admin
  "refresh now" endpoint. The binary is always running, so no CronJob is needed.
- **Default provider:** `https://open.er-api.com/v6/latest/{base}`. It is free and needs
  no key, updates daily, and covers 160+ currencies including TWD and PYG. It requires
  an attribution link in the UI.
- **Fallback provider:** `fawazahmed0/exchange-api` via cdn.jsdelivr.net. It is free and
  keyless, covers 200+ currencies, and keeps dated snapshots, which lets the job backfill
  a missed day or an old transaction date.
- **Frankfurter/ECB was rejected** as the default because it has no TWD or PYG.
- The job fetches only the currencies enabled across books and stores them in `prices`
  with a `source`. A `manual` row always wins over a scraped one.
- Stock prices later use the same table and scheduler with another provider.

## User settings

- Each user has a UI language (en/zh/es), a **display currency**, a timezone, a date and
  number format, and a default book.
- These are columns on `users`, changed through `PATCH /api/me/settings`. The SPA
  switches locale and number formatting without a reload.
- Two currencies are deliberately different things:
  - **Display currency** is a per-user preference. Reports are translated into it at the
    report-date rate, so changing it is instant and lossless.
  - **Book base (functional) currency** is an accounting fact of the book. It can still
    be changed after creation, but only through an explicit **rebase** action:
    - Every `base_amount` is recomputed from historical `prices` in one transaction.
    - The action refuses, and lists the gaps, if any rate is missing.
    - The rebase is written to `audit_log`.

    This mirrors IAS 21's change of functional currency.

## IFRS, applied where it fits a household

- **Accrual, simply.** Expenses are recognised at purchase (card swipe), not at payment.
  Prepaid amortisation is possible but is not in the template.
- **Functional currency** per book. Foreign monetary balances are retranslated at the
  closing rate, and the difference is FX gain or loss (IAS 21).
- **Listed securities at fair value through profit or loss** (IFRS 9 FVTPL), so there is
  no OCI.
  - Fair value = quantity x latest price on or before the date x FX rate.
  - Unrealised gains are **computed in reports only** and never posted as journal entries.
- **Balance sheet** as of a date, split into current and non-current (IAS 1). Equity is
  opening balance + retained earnings + current-period result.
- **Income statement** for a period. It includes unrealised valuation and FX differences.
- **Cash flow statement by the direct method** (IAS 7).
  - It is derived from postings to `is_cash` accounts, classified by the counter-legs'
    `cf_class`.
  - A multi-leg transaction (a paycheck: gross, tax, insurance, net; or a stock sale:
    proceeds, cost, gain) allocates its cash movement pro rata.
  - Income and expense counter-legs default to operating.
  - Where dividends and interest go is a per-book setting, since IAS 7 allows either.
- **Realised gains** use FIFO over prior buy postings, which carry `quantity` and
  `unit_cost`. When a sale is entered, Go proposes the cost-release and gain lines, so the
  stored postings remain the only truth. There is no separate lots table.
- **Futures-broker statements** are recorded as a margin account (asset) plus daily
  settlement P&L postings.

## Backend choices

- **pgx/v5 plus sqlc** replace `lib/pq` and sqlx. sqlc gives typed queries; today's
  hand-written `Journal` struct mis-scans `TEXT[]` and NULL columns.
- **Opaque session tokens, stored hashed in PG**, replace the JWT access/refresh pair.
  - Web uses a cookie. Mobile and the sync job use a bearer token.
  - Sessions are revocable, and there is no refresh dance.
  - This also closes today's gaps: no algorithm, issuer or audience check, and refresh
    tokens accepted as access tokens.
- **Authorisation is by book membership on every query.** Today `DeleteLedger` and
  `Ledgers.Edit` do not check the owner at all.
- Keep chi, golang-migrate (migrations embedded, run on start) and `caarlos0/env`.

## Imports

- **Stays local, by decision.** Statement data never goes to an external service.
- **CSV first.**
- **PDF.**
  1. `pdftotext -layout` (poppler, installed in the image) extracts the text.
  2. A Go `Parser` interface has one implementation per institution and statement type:
     credit card, bank, futures.
  3. Parsed rows go to `import_rows` with a dedupe fingerprint. The original PDF is kept
     in `attachments`.
- **Review queue.** `rules` pre-fill the account and tags. The user confirms, and only
  then are transactions posted. **Nothing auto-posts.**
- **Reconciliation.** The statement's closing balance is checked against the account
  balance at `period_end`, and matched postings become `reconciled`.

### Card purchases: record now, settle later

A foreign-currency card purchase is not final for days: the issuer's rate, the FX fee
and any cash back arrive with the settlement. The flow is to record it on the day and
correct the same transaction later. It is covered by
`TestCardPurchaseEstimatedThenSettled`.

1. **Day 1:** `Travel 300 USD` (base amount pre-filled from the day's rate) against the
   TWD card for the estimate. Both lines are `uncleared`.
2. **Settlement:** edit the transaction.
   - Pin the expense's `base_amount` to what the issuer charged (9,468 TWD, so Visa's
     31.56 becomes the rate for that purchase). There is no FX gain or loss, because the
     card is a TWD account.
   - Add `Fees 142`, and set the card line to -9,610 with status `cleared` and
     `cleared_on` = the posting date.
   - Per-purchase cash back is `card +189 / Card rewards -189`. A monthly lump sum is
     its own transaction.
   - The audit log keeps the estimate.
3. **Payment:** a separate transfer, bank to card.

`uncleared` doubles as "estimated": a charge is final the moment it posts. The P4 work
that makes this nearly automatic:

- an "estimates to finalize" view (uncleared card lines older than a few days);
- optional `fx_fee_rate` and `cashback_rate` on card accounts, so the form proposes the
  fee and cash-back lines;
- statement import matches uncleared lines by date window, amount tolerance and payee,
  and offers to rewrite the estimate to the statement amount.

## Firstrade

- `MaxxRK/firstrade-api` is an **unofficial, reverse-engineered** Python library (MIT).
  - It logs in with a TOTP MFA secret and saves cookies.
  - It exposes `account_balances`, `get_positions`, `get_account_history` and quotes.
  - Firstrade can break it at any time.
- `integrations/firstrade/` will be a small uv project, built as its own image and run as
  a k8s CronJob.
  - It pulls history and positions, then POSTs them to `/api/books/{id}/imports` with a
    bearer token.
  - The rows land in the same review queue as PDF imports.
  - Credentials (username, password, TOTP secret) live in the SOPS secret.
- **Firstrade CSV export** is the fallback when the scraper breaks.

## Deployment

hcloud keeps every chart local under `k3s/helm/`. It has no OCI chart registry and
deploys with `k3s/upgrade.sh`. So **the chart lives in hcloud**, and this repo only
builds images.

**This repo owns** (done, see CLAUDE.md "CI and releases"):
- A multi-stage `Dockerfile`: bun builds the SPA, Go builds static binaries with
  `-tags prod`, and the result runs on distroless static. P4 switches the runtime stage
  to Debian slim with `poppler-utils` for `pdftotext`.
- Identical pipelines on both forges: `ci` on every push, `image` on `main` and tags,
  and `release` (GoReleaser) on `v*` tags. Gitea pushes to
  `git.chenantunez.com/cwchen-twn/rigel-ledger`, and GitHub pushes to
  `ghcr.io/cwchen-twn/rigel-ledger`.

**hcloud owns** `k3s/helm/rigel-ledger/`, modelled on `k3s/helm/navidrome/`:
- A stateless Deployment (RollingUpdate, no PVC), plus the firstrade CronJob.
- A SOPS secret `k3s/secrets/rigel-ledger-secrets.enc.yaml` that holds `DATABASE_URL`,
  pointing at `10.0.1.1:5432`.
- A Postgres role and database, following `k3s/postgres/README.md`.
- An entry in the backup chart's `databases` list.
- An entry in the `NAMESPACES` map in `upgrade.sh`.
- A pull secret, or a public package.
- **Tailnet-only access** through a Terraform `private_records` entry, not a public
  `subdomains` entry. Personal finance data stays off the internet, and the PWA and
  mobile app reach it over Tailscale.

## Roadmap

| Phase | Scope |
|---|---|
| P1 | ~~Schema reset, sessions, sqlc; books, accounts, multi-currency transactions API and UI; the "All accounts" balances page; the user Settings page~~ (done) |
| P2 | ~~Dockerfile, Gitea/GitHub CI and release~~ (done); hcloud chart; deploy and start daily entry |
| P3 | Exchange-rate scheduler (open.er-api plus fawazahmed0 fallback), book rebase, the three statements with FX revaluation and display-currency translation |
| P4 | CSV and PDF import, review queue, rules, reconciliation. Confirm whether "future transactions pdf" means futures-broker statements or scheduled transactions |
| P5 | Securities: FIFO realised gains, price scheduler, Firstrade CronJob |
| P6 | PWA polish, then Flutter if a native feature is needed |
