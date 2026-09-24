# RigelLedger target architecture

Status: **accepted direction; P1, P2 and P2.5 done** (2026-09-24). The schema,
sqlc data layer, sessions and the book-scoped JSON API follow this document, and so does
the SolidJS frontend. **The app is deployed** (ledger.chenantunez.com, tailnet-only), so
`000001_init` is frozen: every schema change is a new migration pair.

## Goals

- Double-entry bookkeeping for **personal and family** use, not for a company.
- Follow IFRS where it makes sense for a household, and stay as simple as possible.
- Balance sheet, income statement and cash flow statement.
- Multi-currency (TWD, USD, PYG, ...) with exchange rates updated automatically.
- Daily manual entry from web and phone.
- Sync Taiwan banks, cards, brokers, futures and e-invoices, and Firstrade, with statement
  import as the fallback. No statement data leaves the cluster (see Data sources and sync).
- Stock and futures investments: 永豐 via its official Shioaji API, Taiwan holdings via
  集保 e存摺, Firstrade via its unofficial library.
- Per-user settings (language, display currency, ...) that can be changed at any time.
- A yearly tax workbook for Taiwan and Paraguay, later (designed in Tax; P7).
- Packaged as a container, deployed by the hcloud repo.

## Shape

```
PostgreSQL 18 (hcloud host, 10.0.1.1)
        |
Go binary  -- chi JSON API, embedded SolidJS SPA, migrations on start,
        |     in-process scheduler (exchange rates, prices)
        |
   +----+-----------------+----------------------+
SolidJS PWA (web)   Flutter (later)     sync runners (CronJobs):
                                          tw-sync (Node, all-set-tw connectors)
                                          py-sync (Python: Shioaji, Firstrade)
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
commodities(code, kind currency|security|points, name, decimals,
            quote_currency, exchange_mic, contract_size)
            -- every ISO 4217 code seeded; "XNAS:AAPL", "MILES:EVA" added at runtime
prices(commodity_id, quote_id, date, rate NUMERIC, source manual|er-api|fawaz|...)
      UNIQUE(commodity_id, quote_id, date, source)  -- manual wins on read
accounts(id, book_id, parent_id, name, code NULL, class, commodity_id NULL,
         is_current, is_cash, cf_class, is_placeholder, template_key, archived_at)
         -- commodity required for asset/liability/equity; NULL = multi for I/E
transactions(id, book_id, date, payee, memo, source, external_id, import_row_id,
             created_by, updated_by, ...)   UNIQUE(book_id, source, external_id)
postings(id, transaction_id, account_id, commodity_id, amount NUMERIC signed,
         base_amount NUMERIC signed, unit_cost NULL,
         -- amount is units of the account's commodity: TWD, shares, miles
         status uncleared|cleared|reconciled, cleared_on, memo)
  -- deferred constraint trigger: SUM(base_amount) = 0 per txn on I/U/D
  -- trigger: commodity = account.commodity when set; date > book.lock_date
  -- prices trigger: no rate on or before the lock date of a book using it
tags(id, book_id, name, kind);  transaction_tags(transaction_id, tag_id)
attachments(id, book_id, sha256, filename, mime, bytes BYTEA, transaction_id NULL)
source_accounts(book_id, connector, external_id, account_id)       -- P4 sync mapping
balance_assertions(account_id, date, amount, source)               -- P4 reconciliation
import_batches(id, book_id, account_id, source, parser, attachment_id,
               period_start, period_end, statement_closing_balance)
import_rows(id, batch_id, kind transaction|balance|holding|bill|invoice|trade|settlement|margin,
            date, amount, currency, description, raw JSONB, external_id,
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

Implemented in P3a (`internal/rates`).

- **What is stored:** once a day, USD against every ISO currency in `commodities` (about
  150 rows), in `prices` with the provider as `source`. `ledger.RateOn` crosses any
  pair through USD, so there is no per-book or per-user currency list to keep in step:
  every book's currencies and every display currency are covered by construction.
- **Default provider:** `https://open.er-api.com/v6/latest/USD` (free, keyless, daily,
  160+ currencies including TWD and PYG). Its terms ask for an attribution link wherever
  rates are shown; the book settings and admin pages carry "Rates By Exchange Rate API".
- **Fallback and history:** `fawazahmed0/exchange-api` (jsDelivr, then its pages.dev
  mirror) when er-api fails, and for dated snapshots.
- **Schedule:** an in-process job, no CronJob.
  - It starts 30 s after boot, then runs hourly.
  - It fetches the latest rates when no fetch has succeeded in 12 h.
  - It backfills any of the last 14 days still missing, giving up on a day after 3
    failures in a day.
  - Every attempt is a `rate_fetches` row (migration `000004`), shown on
    Administration -> Exchange rates, with "Fetch the latest now" and "Fetch that day".
  - `RATES_ENABLED=false` turns it off.
- **Precedence:** a `manual` row wins on its own date (`LatestPrice` orders it first);
  otherwise the newest date on or before the lookup wins.
- **Locked periods:** a snapshot for a date a book has locked is refused by the
  `prices_lock` trigger, and the job skips that one rate (`skipped` in the record)
  rather than failing the day.
- **Frankfurter/ECB was rejected** as the default because it has no TWD or PYG.
- Stock prices later use the same table with another provider (P5).

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

The three statements are implemented (P3b, `internal/ledger/reports.go`, page
`/b/:id/reports`). Each is computed per request, returns the exact rates it used, and
can be shown in any currency: figures are kept in the book's base and translated at the
report date's rate (default: the viewer's display currency). A currency with no rate
falls back to the base and is listed as missing, as is a security with no price.

- **Accrual, simply.** Expenses are recognised at purchase (card swipe), not at payment.
  Prepaid amortisation is possible but is not in the template.
- **Functional currency** per book. Foreign monetary balances are retranslated at the
  closing rate, and the difference is FX gain or loss (IAS 21).
- **Listed securities at fair value through profit or loss** (IFRS 9 FVTPL), so there is
  no OCI.
  - Fair value = units held x latest price on or before the date x FX rate.
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
- **Realised gains** release cost at the **weighted average** of what the account holds
  (implemented). A sale's cost line is computed by the server and the gain line is
  entered against it, so the stored postings remain the only truth. There is no lots
  table. FIFO lots for US tax reporting are a P5 addition, driven by `unit_cost`.
- **Futures-broker statements** are recorded as a margin account (asset) plus daily
  settlement P&L postings.

## Holdings, valuation and what stays out of the ledger

Decisions from reviewing the schema against securities, futures, miles, insurance and
memberships (2026-09-23).

### How accounts, transactions and postings relate

```
account      a bucket that holds ONE commodity     "Visa card (TWD)", "Travel", "EVA miles"
transaction  one event: date, payee, memo, tags    "EVA award ticket, 2026-09-23"
posting      one leg of that event                 EVA miles  -10,500 miles (cost -6,300)
                                                   Visa card   -2,000 TWD
                                                   Travel      +8,300 TWD
```

- A transaction has two or more postings. Each posting belongs to exactly one account.
- The postings' `base_amount`s sum to exactly zero; the database refuses anything else.
- An account's balance is the sum of its postings up to a date.
- `amount` is always in the account's commodity: TWD, shares or miles.
- There are no from/to columns, because real events have more legs than two: a
  paycheck, a card charge with fee and cash back, a miles ticket with tax.
- Status lives on the posting: a card payment clears on two statements separately.

### Securities, futures and points are commodities

| Kind | Examples | Price | `base_amount` means |
|---|---|---|---|
| `currency` | TWD, USD, PYG | exchange rates | translated value |
| `security` | `XNAS:AAPL`, `XTAF:TX` | quotes in `quote_currency` | cost |
| `points` | `MILES:EVA`, `PTS:CATHAY` | none, by design | cost |

**Coming in:**
- A buy with a unit price is priced as units × `unit_cost` × the quote-currency rate.
- Anything else needs its cost entered. Zero is fine for miles earned.

**Going out** (miles spent, shares sold): the server releases the weighted-average cost
of what the account holds, and exactly the remaining cost when everything goes.
- The editor fetches the same figure from `GET /accounts/{id}/cost`, and "Put the
  remainder here" fills the balancing line.
- Example ticket: 10,500 of 20,000 miles bought for 12,000 TWD leave at 6,300, and
  Travel is 6,300 + 2,000 tax = 8,300.

**Card points to miles:** `Points -X (base -cost) / Miles +Y (base +cost)`. The cost
moves across unchanged.

**Futures** carry `contract_size` (TX = 200).
- Their balance-sheet value is **margin plus unrealised P&L**.
- Contract value (units × size × price) is **exposure**: shown beside the position,
  never added to assets. Adding it would overstate net worth many times over.

### Market prices and the display currency (P3, P5)

- The same `prices` table holds exchange rates and share quotes.
- A scheduler fetches:
  - quotes for every security with a non-zero holding, from the Firstrade sync, TWSE's
    open API for Taiwan listings, and an end-of-day source such as Stooq for the rest;
  - rates for every book's currencies plus every user's `display_currency`.
- Value shown = units × latest price on or before the date × rate to the base currency.
  Converting to the display currency multiplies once more by the base→display rate at
  the same date.
- The display currency is a view. Changing it touches no stored amount.
- **Rule for P3: reports are computed per request and never persisted in a display
  currency.** A new display currency therefore shows in every report on its next
  render, with nothing to regenerate. Any cache added later is keyed by currency (and
  date, and book) and dropped with the prices it read.

### A balance sheet bound to its date's rates (P3b, implemented)

- The report "as of D" converts foreign cash and debts, and securities at fair value, at
  the **latest rate on or before D**. Points, property, futures (contract value is
  exposure) and equity accounts stay at cost.
- **Equity** = the equity accounts + the result accumulated in income and expense + the
  unrealised revaluation. The last is derived as whatever balances the statement, which
  in the base currency is exactly sum(value - cost), and translated also absorbs the
  per-line rounding, so assets - liabilities - equity is zero by construction.
- **Income statement** for a period = income and expense at their stored
  (transaction-date) base amounts, plus the change in the unrealised revaluation between
  the day before the period and its last day.
- **Cash flow** (direct method): each transaction that moves a cash account attributes
  that movement to its other legs, each contributing minus its own base amount (pro rata
  by construction); a cash-to-cash transfer moves nothing. Legs are classified by their
  account's `cf_class`; interest and dividends received follow the book's choice. Cash
  arriving through the opening-balances account is part of the opening position, not a
  flow. An "effect of exchange rates" line reconciles opening to closing cash.
- Accounts holding a security default to `investing`, so a share purchase is not an
  operating outflow.
- The difference from historical base amounts is a computed unrealised-FX or valuation
  line (IAS 21, IFRS 9).
- The income statement stays at transaction-date rates, which are already stored.
- The report returns the exact rates it used, so every statement shows its inputs.
- **Implemented now:** `prices` obeys the lock date. No rate dated on or before the lock
  date of a book that uses either currency can be inserted, changed or deleted.
  Otherwise, correcting a March rate in September would silently rewrite March's closed
  balance sheet.

### Tags

- Tags are for questions that cut across payment methods, like "what did Japan 2026
  cost?".
- A trip report sums the **expense postings** of tagged transactions by category. Cash,
  bank, card and miles all count, the miles at their cost.
- Tags also cover "who spent it" and projects.
- Tags attach to the whole transaction. Per-posting tags are the known extension, if one
  statement import ever needs splitting.
- The report itself is P3; filtering the transaction list by tag works today.

### Insurance: the money here, the paperwork elsewhere

- **Term, health and car premiums:** an expense, or prepaid and spread over the
  coverage period if precision matters.
- **Savings-type or USD policies (儲蓄險):** an asset in the policy's currency.
  - Premiums go into it.
  - Periodic revaluation brings it to the insurer's cash value, with the difference to
    an insurance gain or loss.
  - The cost of cover is the part that is not cash value.
- **Claims:** income, or a reduction of the expense they cover.
- **Out of the ledger:** policy numbers, coverage, beneficiaries, renewals and
  documents are a later "policies" module in this app. It would share users, books and
  login, and link to the asset account, but it is not bookkeeping.

### Frequent-flyer status: out of the ledger

- Tier, qualifying segments and status expiry cannot be spent, so they are not value.
- The **award-miles balance** is a points commodity above. Status tracking is a separate
  tool, or a small "memberships" module later.
- Tagging flights by airline already gives a segment count.

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

## Data sources and sync

Decided 2026-09-23 against the sources the household actually uses (fifteen, with
Paraguay and email added the same day). The reference
is [TedLin1993/all-set-tw](https://github.com/TedLin1993/all-set-tw) (MIT), a self-hosted
Taiwan finance hub whose connectors this design reuses.

### Principle

- **Every source produces the same staged records**, whether it is a bank login, an
  official API, an app API, or a CSV or PDF statement.
- **Nothing auto-posts.** Rules and matching propose; the user confirms in the review
  queue.
- **Sources are evidence; the ledger is the truth.** Balance and holding snapshots become
  **assertions** checked against the ledger, never overwrites.

### Coverage

| # | Source | What | Mechanism | Status |
|---|---|---|---|---|
| 1 | 永豐銀行 | card overview, recent bills, unbilled spend | all-set-tw `sinopac` (browser login + CAPTCHA) | reuse |
| 1 | 永豐銀行 | deposits, balances, transactions | not in all-set-tw: a new connector, or the balance via 集保's settlement-bank list (unverified for 永豐) | new |
| 2 | 永豐證券 | stocks, ETF, funds: positions and trades | **Shioaji**, official, read-only key | new (Python) |
| 3 | 永豐期貨 | futures positions, margin, P&L | **Shioaji** futures account | new (Python) |
| 4 | 國泰世華 | deposits, balances, transactions; card bills and spend | all-set-tw `cathaybk` (browser per sync; extra verification is manual) | reuse |
| 5 | 國泰證券 | holdings and trades | via 集保 e存摺; retires when brokerage consolidates into 永豐 | via 集保 |
| 6 | 國泰期貨 | futures trades | statement PDF until consolidated into 永豐 | PDF |
| 7, 10 | 國泰人壽, 南山人壽 | policies | **out of the ledger** (see Insurance); premiums arrive through bank and card | not synced |
| 8 | 將來銀行 | deposits, balances, transactions; 信貸 | new connector (app API) or export; the loan is a liability with principal/interest split | new |
| 9 | 兆豐銀行 | deposits, balances, transactions | new connector or export | new |
| 11 | 電子發票載具 | carrier invoices with line items | all-set-tw `einvoice` (app login). The MOF's own API route could not be verified | reuse |
| 12 | 集保 e存摺 | settlement-bank balances; TW stocks, ETF and funds, holdings and trades across brokers | all-set-tw `tdcc` (device OTP on first login) | reuse; **source of truth for TW holdings** |
| 13 | Firstrade | trades, positions, value, history | `MaxxRK/firstrade-api` (unofficial Python; TOTP, saved cookies); its CSV export is the fallback | new (Python) |
| 14 | Banco Continental (Paraguay) | USD account, PYG account, PYG credit card | no known API or open-source connector; start from its online-banking statement export (format to be checked), then a browser connector in the tw-sync shape if the export is poor | new |
| 15 | Gmail | order confirmations, receipts, subscription renewals, bank/card alert emails | IMAP, read-only, one label (see Email below) | new |

What is known about the sources, and what is not:

- **Open Banking (開放銀行)** phase 3 is TSP-only and voluntary. It is not an option for
  an individual, so bank data means logins or statements.
- **Shioaji** (sinotrade.github.io) is the one official brokerage API.
  - Read-only queries for stock and futures accounts: `list_positions` (with unrealised
    P&L), `list_profit_loss(begin, end)` (with fees and tax), `settlements` (T..T+2),
    `margin()` (equity, margins, settle P&L), `account_balance`.
  - The API key has per-permission and IP scopes. The CA certificate is needed only to
    place orders.
  - Limits: 25 queries per 5 s, 1000 logins per day.
  - **Fills (`list_trades`) are current-day only**, so the runner must sync every
    trading day after the close, or history is lost.
- Consolidating 國泰 securities and futures into 永豐 (planned) turns rows 5–6 into
  Shioaji too.
- **Banco Continental** needs no model change.
  - It maps to three accounts: an asset in USD, an asset in PYG, and a liability in PYG
    for the card. PYG has zero minor units, which the schema already handles.
  - A USD purchase on the PYG card follows the card-settlement flow, the same as a
    foreign charge on a Taiwan card.
  - Whether these live in the TWD book or a separate PYG book is a household choice.
    One book works; reports translate.

### Runners

Both are CronJobs in the hcloud chart. They hold the institution credentials; the Go app
does not.

- **`integrations/tw-sync/` (Node).**
  - It pins all-set-tw's `@taiwan-fin-hub/connectors` and `core`. The connectors
    package depends only on zod and node-forge.
  - A small adapter replaces the Cloudflare-only pieces: Browser Run becomes local
    Puppeteer/Chromium, and Workers AI CAPTCHA recognition becomes local OCR.
  - It maps all-set-tw's `SyncResult` (accounts, balance snapshots, pending/posted
    transactions, card bills, invoices with items, positions, trades) to our import
    payload.
  - Upstream fixes arrive by bumping the pin. New connectors (將來, 兆豐, 永豐
    deposits) follow all-set-tw's connector contract, so they could be upstreamed.
- **`integrations/py-sync/` (Python, uv).** Shioaji and Firstrade, both Python SDKs.

Both POST to `/api/books/{id}/imports` with an `api` session token, and the rows land
in one review queue.

### Ingestion contract (Go side)

- **Account mapping.** `source_accounts(book_id, connector, external_id -> account_id)`.
  One external account maps to one ledger account; an unmapped one appears in the review
  queue until the user maps it.
- **Staged rows.** `import_rows` gain a `kind`:
  - `transaction` (with `pending` or `posted`);
  - `balance`, `holding`, `bill`;
  - `invoice` (with items);
  - `trade`, `settlement`, `margin`.
- **Idempotent.** `transactions.external_id = "<connector>:<sourceId>"`, under the
  existing unique `(book_id, source, external_id)`. A re-sync never duplicates.
- **Matching** is the core of P4. all-set-tw documents its pending-to-posted card
  matching: same card and day, then amount and merchant-name score, one-to-one. That
  informs ours:
  - **Card pending = our uncleared estimate.** The posted row rewrites the amount and
    clears the line. This is the card-settlement flow below, automated.
  - **Card payment** in the bank and the card's credit become one transfer.
  - **Transfers between own accounts** (bank to bank, bank to broker settlement) match
    on amount and a date window. Each is proposed once, never counted twice.
  - **Invoices enrich; they do not create.**
    - An invoice matches an existing card or cash transaction on amount, date and
      seller. Its items attach to that transaction and can split the expense by
      category.
    - Only an unmatched cash invoice proposes a new cash transaction. Otherwise every
      card purchase would be booked twice.
  - **Broker trades** (集保, Shioaji, Firstrade) become buy and sell transactions with
    `unit_cost`, using the existing average-cost rules. Dividends, fees and tax are their
    own lines.
  - **Futures.**
    - Daily settle P&L from `margin()` becomes margin-account ↔ futures P&L postings.
    - Equity is an assertion.
    - Contract value stays exposure (see Holdings).
- **Assertions.** `balance_assertions(account_id, date, amount, source)` hold bank
  balances, card outstanding, and units per security.
  - A mismatch shows as drift on the account, with the date it began.
  - Nothing is overwritten. This replaces "statement closing balance" as the general
    reconciliation.
- **Challenges.**
  - When a CAPTCHA defeats local OCR, or an OTP or new-device check appears, the run
    stops with `needs_user_action`, as all-set-tw does.
  - The challenge goes to the UI with a TTL. The user answers it there, and the run
    resumes.
  - The runner never bypasses a security check.

### Receipts

- A receipt photo or PDF can be attached to a transaction when it is entered, or later.
  On a phone the file input opens the camera.
- Storage is the planned `attachments` table:
  - `bytea` in PostgreSQL, so the nightly `pg_dump` backs it up with the books;
  - de-duplicated by SHA-256;
  - images downscaled in the browser before upload, and a size cap per file.
- **Optional local OCR** can suggest date, amount and seller for a new entry. It runs in
  the cluster; receipts never go to a cloud service.
- Synced evidence attaches the same way: an order email, an e-invoice or a statement page
  becomes an attachment of the transaction it matched.

### Email (Gmail) as a source

Email is **evidence**, like e-invoices: it enriches and proposes, it does not post.

- **Access: IMAP with a Google app password**, read-only, on one Gmail label (e.g.
  `rigel`) that a Gmail filter fills.
  - The Gmail API is the alternative, but a personal OAuth app left in "Testing" issues
    refresh tokens that expire after 7 days. Publishing it needs Google verification for
    the restricted read scope.
  - The app password lives in the runner's SOPS secret like every other credential.
- **Parsing is local.**
  - Many merchants embed schema.org `Order`/`Invoice` markup, which is read first.
  - Next come per-sender parsers (card alert emails, app stores, airlines, e-commerce).
  - Anything else is left for review.
  - No mail content goes to an external model.
- **What it produces:**
  - **Card alert emails** (刷卡通知 and similar) become pending card rows the moment the
    charge happens, which is the earliest estimate in the card-settlement flow.
  - **Order confirmations and receipts** match an existing card transaction on amount,
    date and merchant, and attach the email (as an attachment) and its line items. They
    create nothing when matched.
  - **Subscriptions.** Renewal emails, plus recurring card charges from the same
    merchant at a steady interval, feed a **recurring list**: merchant, amount, cadence,
    next date, and price-change alerts. This later drives recurring-transaction templates.
- The runner is a `mail` connector in tw-sync. IMAP is plain Node, so no browser is
  needed.

### Security and risk

- **Credentials** live in the SOPS/age k8s secret mounted only into the runner pods:
  bank and app logins, the Shioaji key (Account permission only, IP-scoped), the
  Firstrade TOTP secret and the Gmail app password. They are never in the database, a log, or `raw`.
- **Local-only rule.** Statements, invoices, receipts, emails and CAPTCHA images never
  leave the cluster.
  The only outbound traffic is the login to the institution itself. The app is
  tailnet-only.
- **Caveats, stated plainly.**
  - Bank logins are unofficial: a site change breaks them, they can trip fraud checks,
    and some banks end the user's other sessions (all-set-tw notes this).
  - Terms of service may not permit automated access; that risk is the user's.
  - Every connector has an off switch, and **CSV/PDF statement import stays** as the
    fallback: `pdftotext -layout` plus a Go parser per institution, the original file
    kept in `attachments`.

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

## Tax: Taiwan and Paraguay (designed, not built)

The goal is **a yearly tax workbook per taxpayer and country**. It gathers income by tax
category, deductions with their evidence, tax already withheld, and capital gains, in
the country's currency. The workbook is handed to the official software (Taiwan's 綜所稅
filing software, with its pre-filled data) or to a contador in Paraguay. **The ledger
prepares and cross-checks; it does not file.** The design needs only a few hooks, and
most of them are already there.

### What the ledger must record so the workbook is possible

- **Who the income belongs to.**
  - Tax is per person, not per book. A `person` tag kind names the taxpayer on
    transactions, the same tags as "who spent it".
  - `tax_profiles(user_or_person, jurisdiction TW|PY, residency, filing_unit)` records
    where each person files. Taiwan's household filing (夫妻合併申報, with separate
    computation choices) is a filing unit, not a book.
- **What each line is for tax.**
  - `tax_categories(jurisdiction, year, key)` define the categories. Examples:
    - Taiwan: 薪資所得, 利息所得, 股利所得, 財產交易所得, deductible 保險費/醫藥費/捐贈/房租;
    - Paraguay: the IRP categories for personal services and for capital income.
  - A mapping `account -> category` per jurisdiction, with a per-posting override. The
    salary account maps once; an odd line can say otherwise.
- **Tax paid in advance is an asset, not an expense.**
  - Withholding on salary, on dividends (for example US withholding on Firstrade
    dividends) and on interest goes to a `tax_withheld` asset (tax receivable) per
    jurisdiction.
  - The final assessment moves the year's liability to the tax expense. The refund or
    payment settles the difference.
  - Foreign tax paid is kept separately so it can be claimed as a credit where the rules
    allow.
- **Evidence.**
  - Deductions need proof: receipts and e-invoices (attachments), insurance premiums (the
    policies module), donations.
  - The workbook lists each deduction with its attachment, so nothing is claimed without
    a document.
- **Capital gains** reuse the holdings rules.
  - Cost basis is weighted average today, FIFO lots in P5. The method is chosen per
    jurisdiction, because the country's rule wins over the ledger's default.
  - Transaction taxes already paid on sales (such as Taiwan's securities and futures
    transaction taxes) are their own expense lines, and the workbook lists them.
- **The country's currency and the country's rate.**
  - Taiwan reports in TWD, Paraguay in PYG. Foreign income is translated with the rate
    source each tax authority requires.
  - That source is another `prices.source` (for example a central-bank or tax-authority
    rate), not the ledger's own scraped rate.
  - Stored postings keep their historical base amounts. The workbook is a separate
    translation, the same way the display currency is.

### Rules are data, per year

- Rates, brackets, thresholds and limits change every year. They include Taiwan's
  basic-income (最低稅負) thresholds for overseas income, the dividend-taxation options,
  and deduction caps.
- They live in versioned files, one per jurisdiction and year, e.g.
  `tax/rules/tw/2026.yaml`. They are reviewed by a person and never hard-coded.
- **Nothing in this section is tax advice**, and the figures are deliberately absent.
  Each year's file is filled from the official source and checked against what the
  authority pre-fills.

### Cross-border points to confirm before building

- **Taiwan residents** include overseas income in the basic-income (AMT) computation
  above a threshold. Paraguay and US income is overseas income here. Confirm the current
  rule and threshold.
- **Paraguay's IRP** (Ley 6380) has separate treatment for personal-service income and
  for capital income. Whether a given foreign-source item is taxable there, and at which
  rate, needs a contador's confirmation.
- **Credits for foreign tax paid** (US dividend withholding, Paraguayan tax on
  Taiwan-source items or the reverse) depend on each country's rules and any treaty.
  Record the facts now; decide the treatment later.

### What stays out

- **Filing, e-signing and payment** belong to the official channels.
- **Tax optimisation advice.** The workbook shows numbers and where they came from.

## Accounts, administration and sign-in security (P2.5)

Decided 2026-09-24, before the app is ever reachable outside the tailnet.

### Accounts

- **Every user walks a first-login wizard** once: username, a verified email address,
  display name, then language, display currency, time zone, date format and theme.
  `users.initialized_at` stays NULL until it is done, and the API answers
  `403 onboarding_required` to everything except `/api/me`, logout, the email-code
  endpoints and `/api/me/onboarding` meanwhile.
- **The email address changes only when a 6-digit code comes back.** The code goes to
  the new address; the old one is told afterwards. Addresses are unique regardless of
  case (`users_email_key` on `lower(email)`).
- **The bootstrap admin** comes from `ADMIN_USERNAME` / `ADMIN_INITIAL_PASSWORD`
  (and optionally `ADMIN_EMAIL`). It is created at startup only while no admin exists,
  never overwrites a user, and must replace the password inside the wizard.
- **Invitations**: an admin enters a username and an address; the user row is created
  without a password, and a one-time link (7 days by default) leads to settings and a
  password, then a signed-in session. Clicking the link verifies the address.
- **Registration mode** (admin): `closed` (invitations only, the default), `request`
  (a public form files an access request; approving it sends the invitation) or `open`
  (a mailed link; the user row is created only when it is clicked, so an unverified
  sign-up reserves nothing and creates nobody).
- **CLI-created users** (`rigel-ledger-cli create-user`) walk the wizard too.

### System settings and mail

- One `system_settings` row, edited on the Administration page: registration mode,
  two-factor switches, defaults for new users, session and invitation lifetimes, and
  the sign-in limits. `SESSION_TTL` is the fallback while the row leaves it empty.
- **SMTP is configured on the Administration page.** The password is sealed with
  AES-256-GCM under `APP_ENCRYPTION_KEY` (SOPS), never returned by the API, and kept
  out of `audit_log`. `SMTP_*` / `MAIL_*` environment variables seed the row on the first
  start only.
- **Until an admin configures mail, the driver is `log`**: codes and links are written
  to the pod log. That is how the first admin verifies their address in the wizard
  before the Mail tab is reachable (`kubectl logs ... | grep 'mail (log driver'`), and
  why SMTP must be set before inviting anyone.
- Mail templates are server-side (`internal/mail/templates/{en,zh,es}`): the one
  exception to "UI text lives in the frontend i18n".

### Throttling and audit

- `auth_events` is both the security audit (sign-ins, failures, codes, invitations,
  admin changes) and the throttle's source, so limits survive restarts and rollouts.
- Failed sign-ins inside a window (15 min) are counted per (username, address) (5),
  per address (20) and per username (20). Past a limit the answer is
  `429 too_many_attempts` with `Retry-After`, the same for unknown usernames. The same
  throttle covers codes, invitation links and anonymous sign-ups and requests.
- **The client address** is the rightmost `X-Forwarded-For` entry not in
  `TRUSTED_PROXIES` (the pod network by default). Cloudflare's ranges join that list if
  it ever fronts the app.
- Users see their own sessions (and sign any out) and their sign-in history; admins
  see everyone's.

### Two-factor sign-in (P2.5b, implemented)

- **Sessions carry an assurance level**: 1 after a password (or an emailed link),
  2 after a second factor or a passkey with user verification. With
  `system_settings.mfa_required` (the default) a level-1 session reaches only
  `/api/me`, logout, the wizard and enrolment (`403 mfa_enrollment_required`
  elsewhere); adding a factor raises the session in place.
- **Sign-in in two steps.** A correct password for a user with a factor returns a
  challenge (5 minutes, 5 attempts) and the methods to offer; `POST /api/auth/mfa`
  finishes it. A passkey also signs in on its own (discoverable, user verification
  required).
- **Factors**, any of the ones the admin allows (`mfa_methods`):
  - **passkeys** (go-webauthn; RP ID and origin from `APP_ORIGIN`, so a new host
    orphans them);
  - **TOTP** (pquerna/otp, SHA-1/6/30 s, ±1 step; the seed is sealed like the SMTP
    password; `last_step` refuses a replayed code, atomically);
  - **email codes** to the verified address (enrolment proves the mailbox; weaker,
    and labelled so);
  - **ten recovery codes**, shown once, SHA-256 stored, single use, minted with the
    first factor.
- **Step-up:** removing a factor or making new codes needs the password in a
  two-factor session; the last factor stays while two-factor is required.
- **Break-glass:** admin "Reset two-factor sign-in", or
  `rigel-ledger-cli reset-mfa -u <user>`: every factor, code and session goes.
- **Lockout exemption:** an address that opened a full session (`signed_in`) for a
  username in the last 30 days is exempt from that username's global limit, so a
  stranger guessing elsewhere cannot lock the owner out. A password alone does not
  earn it.
- **New-sign-in alerts** mail the user when a session opens from an address not seen
  in 90 days (per-user switch, on by default).

## Deployment

hcloud keeps every chart local under `k3s/helm/`. It has no OCI chart registry and
deploys with `k3s/upgrade.sh`. So **the chart lives in hcloud**, and this repo only
builds images.

**This repo owns** (done, see CLAUDE.md "CI and releases"):
- A multi-stage `Dockerfile`: bun builds the SPA, Go builds static binaries with
  `-tags prod`, and the result runs on distroless static. P4 switches the runtime stage
  to Debian slim with `poppler-utils` for `pdftotext`.
- The sync runner images (`tw-sync`, `py-sync`), built by the same pipelines from P4.
- Identical pipelines on both forges: `ci` on every push, `image` on `main` and tags,
  and `release` (GoReleaser) on `v*` tags. Gitea pushes to
  `git.chenantunez.com/cwchen-twn/rigel-ledger`, and GitHub pushes to
  `ghcr.io/cwchen-twn/rigel-ledger`.

**hcloud owns** `k3s/helm/rigel-ledger/`, modelled on `k3s/helm/navidrome/`:
- A stateless Deployment (RollingUpdate, no PVC), plus the sync-runner CronJobs
  (`tw-sync`, `py-sync`), each mounting only its own SOPS secret of institution
  credentials.
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
| P2 | ~~Dockerfile, Gitea/GitHub CI and release; probes and the hcloud chart (tailnet-only ipAllowList, own Postgres role, nightly backup); release `v0.1.0` and deploy~~ (done, 2026-09-24) |
| P2.5 | **Accounts and sign-in security.** a: first-login wizard, verified email, invitations, registration modes, bootstrap admin, the Administration page (users, requests, sign-in rules, defaults, SMTP), throttling and the sign-in audit, sessions (migration `000002`). b: ~~two-factor sign-in -- email codes, TOTP, passkeys, recovery codes, enforcement (`000003`)~~ (done). Both before any public exposure |
| P3 | ~~Exchange-rate scheduler (open.er-api plus fawazahmed0 fallback, and every display currency)~~ (P3a, done); ~~the three statements bound to closing rates with `rates_used`, display-currency translation~~ (P3b, done); book rebase, tag (trip) report |
| P4 | **Sync and review**: import API with `import_rows` kinds, `source_accounts`, review queue, rules, matching (pending/posted, transfers, invoices, order emails), assertions, challenges; receipt attachments (upload, camera, optional local OCR); the tw-sync runner (國泰世華, 永豐 card, 集保 e存摺, 電子發票, Gmail); CSV/PDF fallback, including Banco Continental's statement export |
| P5 | Securities and futures: py-sync (Shioaji daily, Firstrade), quote scheduler, fair value and futures exposure in reports, futures margin postings, FIFO lots for tax. New connectors: 將來, 兆豐, 永豐 deposits, Banco Continental (if its export is not enough). Recurring list and subscription templates. (Points, average cost and the security commodity itself are done.) |
| P6 | PWA polish, then Flutter if a native feature is needed |
| P7 | **Tax workbooks (TW, PY)**: `person` tags and `tax_profiles`; tax categories and account mappings per jurisdiction; `tax_withheld` accounts and foreign-tax-paid records; per-year rule files; workbook export (income by category, deductions with evidence, withholding, capital gains in the country's currency and rate). Needs P3 reports, P4 attachments and P5 lots. Prepares and cross-checks; does not file. |
