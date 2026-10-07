# RigelLedger target architecture

Status: **accepted direction; P1-P3 done, P4 nearly** (2026-10-06; v1.0.0, the first deploy since, is #45). The schema,
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
SolidJS PWA (web)   Flutter (later)     sync runners (one per person):
                                          rigel-sync (Node; every institution,
                                          all-set-tw's Taiwan connectors vendored)
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
attachments(id, book_id, sha256, filename, mime, size, bytes BYTEA)  -- 000009, linked by
transaction_attachments(transaction_id, attachment_id) and import_rows.attachment_id
-- P4a (migration 000005, implemented; see "The import core" below)
source_accounts(book_id, connector, external_id, label, currency, account_id NULL)
import_batches(id, book_id, connector, label, received, duplicates, created_by)
import_rows(id, batch_id, source_account_id, kind transaction|balance|holding|trade|invoice, external_id UNIQUE per book,
            date, amount signed, currency, description, counterparty, pending, raw JSONB,
            security, units, price, cash  -- holding and trade rows (000010),
            items JSONB  -- invoice rows (000011), proposal also enrich,
            proposal new|duplicate|clears|transfer, proposed_account_id, match_transaction_id,
            match_row_id, rule_id, status pending|accepted|ignored, transaction_id)
import_rules(id, book_id, priority, pattern, source_account_id NULL, account_id)
balance_assertions(account_id, date, amount, source)
  -- later kinds (holding, bill, invoice, trade, settlement, margin) and
  -- attachment_id arrive with P4b/P4c and P5
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
  - **A bank account, card or currency is an account, never a book.** The first live
    use made the opposite mistake (thirteen books, one per bank and currency), so the
    UI now says so:
    - onboarding explains what a book holds and goes straight to "Add an account";
    - the book switcher is a plain label while there is one book;
    - "New book" warns that it is for finances kept completely apart;
    - the quick "Add an account" form needs only a kind (bank, card, cash, e-wallet,
      broker, loan), a name and a currency, and files the account in the right place.
  - Books can be deleted by an owner, who types the name to confirm, unless locked.
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

    **Implemented (P3c, `ledger.Rebase`, Book settings -> Base currency):**
    - Money in another currency is re-translated from its own amount at its date's
      rate.
    - Old-base amounts, securities and points have their cost converted.
    - Each transaction's difference is posted to FX gain/loss.
    - It refuses while the book has a lock date.
    - A dry run ("Check") lists every missing (currency, date) pair.

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
- **Implemented (P3c):** Reports -> "Trips and tags" lists every tag with its span,
  count and spend, and one tag's spend by expense account. Stored base amounts are
  historical, so a trip's cost does not move with later rates; the view is translated
  to the chosen currency at today's rate.

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
| 1 | 永豐銀行 | deposits, balances, transactions | all-set-tw `sinopac` covers deposits too since 2026-10 (the mobile app's JSON API after the browser login) | reuse |
| 2 | 永豐證券 | stocks, ETF, funds: positions and trades | **Shioaji**, official, read-only key | new (Python) |
| 3 | 永豐期貨 | futures positions, margin, P&L | **Shioaji** futures account | new (Python) |
| 4 | 國泰世華 | deposits, balances, transactions; card bills and spend | all-set-tw `cathaybk` (browser per sync; extra verification is manual) | reuse |
| 5 | 國泰證券 | holdings and trades | via 集保 e存摺; retires when brokerage consolidates into 永豐 | via 集保 |
| 6 | 國泰期貨 | futures history | the emailed 月對帳單 PDF, read by `xx-mail` (#42); history only, since futures move to 永豐 Shioaji | PDF |
| 7, 10 | 國泰人壽, 南山人壽 | policies | **out of the ledger** (see Insurance); premiums arrive through bank and card | not synced |
| 8 | 將來銀行 | deposits, balances, transactions; 信貸 | all-set-tw `nextbank` (the bank's web API: CAPTCHA of letters and digits read in the runner), as `tw-nextbank` (#66): main account and 口袋; 信貸 not yet (upstream reads deposits only) | reuse |
| 9 | 兆豐銀行 | deposits, balances, transactions | all-set-tw `megabank` (the mobile app's API: CAPTCHA read in the runner, SMS on an unusual sign-in, the virtual device kept), as `tw-megabank` (#65); 月對帳單 PDFs come a month late | reuse |
| 11 | 電子發票載具 | carrier invoices with line items | all-set-tw `einvoice` (app login). The MOF's own API route could not be verified | reuse |
| 12 | 集保 e存摺 | settlement-bank balances; TW stocks, ETF and funds, holdings and trades across brokers | all-set-tw `tdcc` (device OTP on first login) | reuse; **source of truth for TW holdings** |
| 13 | Firstrade | trades, positions, value, history | `MaxxRK/firstrade-api` (unofficial Python; TOTP, saved cookies); its CSV export is the fallback | new (Python) |
| 14 | Banco Continental (Paraguay) | USD account, PYG account, PYG credit card | its online banking's PDF statements, uploaded on Imports (#43): Movimientos de Cuenta for each account, the card's monthly extracto; `py-continental` (#67) reads ContiWeb for the month so far: each account's XLS export and the card page, the Contimóvil QR as a device challenge, the device remembered by its `_cdip` cookie | PDF, browser |
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

Each person runs their own (see "One runner per person" below): the owner's next to
the app, anyone else's on a computer they leave on. Since P4c-1 a runner **does not
hold any institution login in its own config**: its person links their institutions
under Connections, the browser seals the credentials to that runner's key, and the
runner works through that person's connections as a job queue (see "Connections"
below). The only secrets a runner holds are its runner token and its X25519 private
key.

- **`integrations/rigel-sync/` (Node), the one runner.** It was `tw-sync` until
  2026-10-06; renamed because it is meant for every institution a person has --
  Taiwan, Paraguay, the US, banks, brokers, crypto -- and a person links one runner.
  Connector ids are `<country>-<institution>` (ISO 3166 alpha-2, lower case:
  `tw-cathaybk`, `tw-sinopac`, `py-continental`, `us-firstrade`; `xx-` for one with
  no home country), and the code sits under `src/connectors/<country>/`.
  - all-set-tw publishes no package: its connectors live inside its Cloudflare
    Worker (`apps/worker/src/sources/<id>/`), are rewritten often, and hand amounts
    over as JS numbers. rigel-sync **vendors the connectors it uses at a pinned commit**
    (with its MIT licence), and converts every amount to a decimal string at the
    boundary (`src/money.ts`, which throws rather than round).
  - How vendoring works (P4c-2b, implemented): `scripts/vendor.ts <commit>` copies
    the upstream files, unedited, under `vendor/all-set-tw/` (with `UPSTREAM`
    naming the commit), and Bun bundles each connector into
    `vendor/all-set-tw/<id>.js`, because Node's type stripping runs neither
    upstream's extensionless imports nor its parameter properties. npm packages
    stay imports, pinned at upstream's lockfile versions. CI checks the bundle is
    what the copies give (`bun run vendor:check`). A newer upstream is one command
    and a reviewed diff.
  - A small adapter replaces the Cloudflare-only pieces: `@cloudflare/puppeteer`
    (Browser Run, with detached sessions kept alive between the OTP or CAPTCHA
    round-trips) becomes `src/browser/cloudflare.ts` over puppeteer-core and a local
    Chrome (`CHROME_PATH`), which keeps the browser running between disconnect and
    connect. Upstream's throw-and-call-again OTP steps run in one sync through
    `ctx.ask` (`src/connectors/cathaybk.ts`), so the person answers in the app. The
    Workers AI CAPTCHA callback becomes local OCR, then a `captcha` challenge
    (below). D1, KV and Queues sit outside the connectors and are not needed.
  - 國泰世華 (P4c-2b): deposits and the credit card. Account ids keep the last four
    digits only. Card rows are signed from the statement (upstream signs refunds and
    payments as charges), and the card's balance is not sent: upstream's is the
    unpaid statement, not what is owed today. The one-time code goes by SMS or
    email, a `choice` field on the connect form; once answered, the bank trusts the
    runner's browser (a cookie kept in the connection's state) and later runs ask
    nothing. The tests drive the whole flow against a pretend bank in Chrome
    (`test/fake-cathay.ts`).
  - 永豐銀行 (P4c-2c): deposits in every currency (`deposit-<last4>`, plus the
    currency when not TWD: one account number holds several) and the card, posted
    rows and authorisations not yet posted (`pending`, with their own ids, so the
    posted row clears them). Sign-in is a six-digit image CAPTCHA, then the
    mobile app's JSON API with the session's cookies, kept in the connection's
    state and reused until the bank refuses them. Upstream reads the CAPTCHA with
    Gemma on Workers AI; here **the runner reads it itself** (decided 2026-10-06,
    over always asking the person or sending the image to a vision API):
    `src/ocr/captcha.ts` runs ddddocr's `common_old.onnx` (MIT, 13 MB, downloaded
    once from a pinned commit, sha256-checked) on `onnxruntime-web`, with only
    blank and digits allowed at each step. Measured on 12 real 永豐 images: 9
    read right, and every miss came out short, so it is rejected before the bank
    sees it. After three images the person gets one under "Needs you"; three
    wrong answers are `bad_captcha`. Card balances are not sent, as for 國泰.
  - 集保 e存摺 (`tw-tdcc`, #39): the e存摺 app's API (no browser), through upstream's
    `initializeTdccSnapshot` and `syncTdccTradeHistory` rather than its connector's
    `sync`, so the wrapper reads 集保's own strings: share counts and prices stay exact,
    and upstream's dates -- built in the runner's time zone, printed in UTC, a day early
    in Taiwan -- are not used. Broker accounts are `brokerage` sources (#37) with
    holdings and trades, funds `funds-<org>`, settlement accounts `bank-<bank>-<last4>`
    with balances and movements (the same money a bank connector may also send: the
    duplicate matcher keeps it once). A trade's direction is read from its name, from a
    fixed list; one not on it is logged and left out. Stocks are `XTAI:<symbol>`
    (TPEx listings too: symbols are unique across both), funds `FUND:<code>`. The
    device id is kept in the connection's state before the first sign-in, so an email
    code (and the SMS that may follow) is answered once.
  - 電子發票 (`tw-einvoice`, #40): the government e-invoice app's API (mobile number and
    carrier password, no code), through upstream's `initializeEInvoiceSync` and
    `fetchEInvoiceInvoiceDetail`. Each invoice is an `invoice` row (#38) on one
    `carrier` source account; its id is its number and its Taiwan day, because
    upstream's embeds a date computed in the runner's time zone (a laptop and a
    server would disagree). Line amounts are read from the API's strings, not
    upstream's truncated integers. The app's session is kept and tried first.
  - Balance rows are keyed `<account>:<day>:balance`: the import key is
    `<connector>:<id>` per book, so a bare day collided across accounts.
  - The runner itself (P4c-2a, implemented): Node 22.18+ running its TypeScript
    directly; puppeteer-core drives Chrome, onnxruntime-web reads CAPTCHAs. `rigel-sync run` is the daemon; `rigel-sync try
    <connector>` runs one connector from a terminal with no app, which is how a real
    login is checked before a connection is made. Its key and each connection's
    session state (cookies, trusted device) stay in its `DATA_DIR`.
  - It maps all-set-tw's `SyncResult` (accounts, balance snapshots, pending/posted
    transactions, card bills, invoices with items, positions, trades) to our import
    payload.
  - Upstream fixes arrive by bumping the pin. New connectors (將來, 兆豐) follow all-set-tw's connector contract, so they could be upstreamed.
- **Shioaji and Firstrade are Python SDKs, but not a second runner.** A person links
  one runner (`RegisterKeys` retires every key a runner does not list, so two would
  retire each other's), so a Python SDK runs inside rigel-sync's image, called by
  its connector as a subprocess (the protocol stays in Node). To be designed with
  P5; this replaces the planned `integrations/py-sync`.

The runner speaks the runner protocol (`/api/runner/*`, with the `runner` token its person
made in Settings -> Sync runner): claim due
connections, open their sealed credentials, raise challenges, and post batches to
`/api/runner/connections/{id}/imports`, which lands them in that connection's book's
review queue. `integrations/fake-runner` (Go, not shipped) is the reference
implementation against one pretend institution, for development and end-to-end tests.

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
    - Implemented (#38, migration `000011`, `internal/ledger/invoices.go`): an
      `invoice` row's amount is its total signed on the account that paid (a purchase
      < 0), its counterparty the seller, `items` its lines. The matcher looks for a
      booked asset or liability line of that amount from 2 days before to 5 after (a
      card posts late), on a transaction with no invoice yet and claimed by no other
      waiting invoice, the seller in the payee winning a tie -- proposal `enrich`; then
      for a card or bank row still waiting (it waits for that row: `match_pending`);
      else it is a cash purchase against the account its carrier (載具) source is mapped
      to. Invoices are matched again whenever rows are staged or accepted, and match
      whether or not their source is mapped.
    - Each item gets the category of the first rule its description contains. Accepting
      with `split` rewrites the transaction's one income/expense line into a line per
      item category, the rest staying where it was -- only when that is plain (one
      category line of the invoice's whole total, no group turning the other way). The
      lines are kept as `transaction_items` and shown in the transaction sheet; the
      invoice's file, if the source sent one, attaches as evidence.
    - One purchase, several invoices (#54): an order email and its 電子發票. An invoice
      whose payment already carries another connector's lines is `same_invoice`:
      accepting adds only its file. While the first waits, the later one waits for it
      (only an earlier invoice can be waited for, so two never wait on each other).
    - Not cash too soon (#55): an invoice nothing paid for is `waiting` for a week
      after its date -- a card statement lags the 電子發票 -- and becomes a cash purchase
      only then (the queue proposes it again when read), or at once if the person
      chooses a category.
    - Another currency (#56, migration `000013`): a EUR invoice charged in TWD on the
      card. When nothing in the invoice's own currency matches, the matcher looks at
      payments (booked, then waiting rows) in other ISO currencies in the same window,
      the same way: the charge must be within 5% of the invoice at the day's rate
      (`RateOn`; the card's own rate and its 1.5% foreign fee sit inside), and its
      payee or description must name the seller (a word of 4+ letters of its name,
      not one as common as GmbH, Online or Services). Without a rate, or without a
      name, it does not match: an amount in another currency is too weak alone. The
      closest one wins. Accepting adds the items in their own currency
      (`transaction_items.currency`) and the file; the booking stays as the card
      charged it, and split is not offered (the lines are in another currency). The
      queue shows the charge beside the invoice (`match_amount`, `match_currency`).
      A rate added later does not re-propose an invoice already proposed `new`.
  - **Broker trades** (集保, Shioaji, Firstrade) become buy and sell transactions with
    `unit_cost`, using the existing average-cost rules. Implemented for `holding` and
    `trade` rows (#37, migration `000010`, `internal/ledger/securities.go`):
    - A broker account is a source account of kind `brokerage`, mapped to a **parent**
      (an asset account or group) and to the **settlement account** its trades' cash
      goes through. Each security it reports gets its own account under the parent the
      first time (`2330 台積電`, cash-flow class investing), remembered in
      `source_securities` wherever it is moved or renamed; a security the book has never
      seen is registered from the row (`XTAI:2330`, its name, its quote currency).
    - `holding` rows are unit assertions on the security's account, applied when the
      broker account is mapped: drift shows in units.
    - `trade` rows wait for the person, because 集保 sends no fees, no tax and sometimes
      a placeholder price: the queue pre-fills the cash from units x price (or the cash
      the source sent) and the person confirms it. **A buy's cost is the cash paid, fees
      included; a sale releases average cost and books the rest to Realized gains**, both
      priced with the same `resolveLine` the editor uses, so any currency balances.
    - After a trade is booked, the settlement account's waiting rows within 5 days are
      matched again, so the bank's own 交割 line shows as the duplicate it is instead of
      a second cash movement.
    - Units that move with no cash -- a transfer between brokers, a stock dividend
      (taxed at par in Taiwan) -- are refused on accept: ignore the row and enter them by
      hand. Dividends paid in cash arrive as the bank's own rows.
  - **Futures.**
    - Daily settle P&L from `margin()` becomes margin-account ↔ futures P&L postings.
    - Equity is an assertion.
    - Contract value stays exposure (see Holdings).
    - 國泰期貨 history (#42, the owner's call 2026-10-06: Cathay is for history only,
      Shioaji takes over) is booked a month at a time the way Shioaji will book a day,
      from the 月對帳單's 保證金及權利金專戶餘額 table alone, on the margin account
      (`tw-cathayfut-<account>`): realised P&L (權利金 + 平倉 + 到期履約), 手續費 and
      期交稅 as rows of their own (rules give them categories), 存提, unrealised P&L
      (權益總值 - 本月餘額: open futures and option market values) on the last day,
      reversed the next, and an assertion of 權益總值. 原始/維持保證金 are thresholds the
      broker requires, not balances: never booked. The table must add up or nothing is
      sent. The PDF is encrypted with the holder's 身分證字號: `xx-mail`'s optional
      `id_number` field opens it in the runner's process (pdf.js; no file, no other
      program). The daily 買賣報告書 are left alone (the month has the same fills). The
      first month needs an opening balance entered by hand, or it shows as drift.
- **Assertions.** `balance_assertions(account_id, date, amount, source)` hold bank
  balances, card outstanding, and units per security.
  - A mismatch shows as drift on the account, with the date it began.
  - Nothing is overwritten. This replaces "statement closing balance" as the general
    reconciliation.
- **Challenges** (implemented in P4c-1).
  - When a CAPTCHA defeats local OCR, or an OTP or new-device check appears, the runner
    raises a challenge (`otp`, `captcha` with its image, `device`) with a TTL of at most
    15 minutes; the connection shows `needs_user_action`, as all-set-tw does.
  - The owner answers on Connections (a banner on every page points there). The answer
    is sealed to the runner like the credentials, and handed to the runner once.
  - An unanswered challenge expires; the runner ends the run `challenge_expired`.
  - The runner never bypasses a security check.

### The import core (P4a, implemented)

What every source feeds, built before any connector so the runners have one
contract to target. Migration `000005`; `internal/ledger/imports.go`,
`internal/routes/handlers_imports.go`, `web/src/pages/Imports.tsx`.

- **Import tokens.** Settings -> API tokens makes a session of kind `token` (a new
  kind; `api` stays the full-access script login), for a person's own import script.
  It is shown once, lives 1-730 days without sliding, carries aal 2, and
  `auth.TokenScope` confines it to `GET /api/me`, `GET /api/books`,
  `POST /api/books/{id}/imports` and `GET .../imports/sources`. A leaked one can add
  rows to the queue and nothing else: it cannot read the books or accept its own rows.
  (The sync runner has its own `runner` token since P4c-1; see Connections.)
- **The batch.** `POST /api/books/{id}/imports` takes
  `{connector, label, accounts[{id, label, currency}], rows[{kind, account, id, date,
  amount, currency, description, counterparty, pending, raw}]}`, up to 5,000 rows and
  16 MiB. Amounts are signed on the account (money in > 0). A row's key is
  `<connector>:<id>`, unique per book, so resending a whole statement stages nothing
  twice (it is counted as a duplicate).
- **Matching, per row, in order** (on staging, on mapping its account, and for rows
  without a category when a rule is added):
  1. **duplicate**: a posting on the mapped account with the same amount within 3
     days, not already claimed by a row of that account. A hand-entered transfer is
     claimed once from each side.
  2. **clears**: a posted (not pending) row within 10% and 5 days of an uncleared
     two-posting entry -- the card estimate. Accepting books the posted figure; a
     foreign other leg keeps its units and takes the posted figure as its base.
  3. **the same charge from another source** (#53): another waiting row on the same
     mapped account, same amount and currency, within 3 days, from another source
     account (one source's two identical coffees stay two). The posted row books and
     the pending one (a card alert) is its `duplicate` (`match_row_id`); between two
     alike, the first staged books. The duplicate cannot be accepted on its own:
     accepting the other settles both as one transaction, and ignoring it sets the
     duplicate free (matched again).
  4. **transfer**: another waiting row on another mapped account, same currency,
     opposite amount, within 3 days. Accepting books one transaction and takes both
     rows off the queue. **An exchange between currencies** (#43: USD sold for PYG at
     Banco Continental) pairs by the bank's movement number instead: both rows carry
     it as `reference`, in different currencies, opposite ways. Accepting books both
     lines with one base value (the base side's amount, else the first side at the
     day's rate), so the bank's own rate is the transaction's, with no FX residue.
  5. **new**: the first rule (in priority order) whose pattern the description or
     counterparty contains, case-insensitive, picks the category.
- **Nothing auto-posts.** A row becomes a transaction (`source` `sync`, or `import`
  for CSV; `external_id` = the row's key) only when accepted. Ignored rows stay on
  record so they are never staged again. If what a row matched has been deleted, the
  accept re-matches it and answers `match_gone`.
- **Balance rows become assertions** as soon as their account is mapped. Drift is each
  account's newest assertion against the books' sum on that date, in the account's
  commodity; it shows on Imports and as a badge on Accounts. Nothing is overwritten.
- **CSV** is parsed in the browser (UTF-8, Big5 or Windows-1252; comma, semicolon or
  tab; one signed column or money out/in; ROC dates; decimal comma) and sent as
  connector `csv`. Row ids are a SHA-256 of account, date, amount and text plus an
  occurrence index, so re-importing the same or an overlapping export adds nothing.
- **Not yet:** `holding`, `bill`, `invoice`, `trade`, `settlement` and `margin` kinds;
  challenges; tags from rules; the runners themselves (P4c).

### Connections (P4c-1, implemented)

Decided 2026-09-24: with more than one household on the instance, each person links
their own institutions in the app, instead of an admin collecting everyone's bank
logins into the runner's SOPS secret. Migration `000006`; `internal/connections`,
`internal/sealing`, `internal/routes/handlers_{connections,runner}.go`,
`web/src/pages/Connections.tsx`, `web/src/lib/seal.ts`.

- **Sealed to the runner, not to the app.** The browser encrypts the connector's fields
  (a JSON object) to the runner's X25519 public key: an ephemeral X25519 key,
  HKDF-SHA256, AES-256-GCM, with additional data binding the blob to
  `credentials/<user id>/<connector>` (format in `internal/sealing`'s package comment;
  WebCrypto, the Go standard library and node:crypto all have it). The app stores
  ciphertext and holds no key that opens it; **a leaked database plus
  `APP_ENCRYPTION_KEY` exposes no bank login.** Sealing with the app's own `secretbox`
  key was rejected for exactly that reason.
- **Never shown again.** No response carries the blob back to a person, and the audit
  trigger (`audit_connection`) stores the row without it. Changing credentials means
  entering all of them again.
- **The runner.** A person links theirs with a `runner` token (Settings -> Sync runner,
  or `rigel-ledger-cli create-runner-token -u USER`); it reaches `/api/runner/*` only,
  and nobody else reaches that (404). The runner registers the keys it holds (the newest is what
  browsers seal to; keys it stops listing are retired, and connections sealed to them
  ask their owner to re-enter), publishes its connectors with a field schema (the UI
  renders the form from it), claims due connections (a claim lapses after 30 minutes),
  and ends each run `ok` or `failed` with a stable code, never the institution's text.
- **A connection** belongs to one person and feeds one book they edit; rows are imported
  as that person, under the connection's connector, into the P4a review queue. It syncs
  every 1-168 hours (24 by default), or at once on "Sync now".
- **The consent panel** says it plainly: an unofficial login that can break, may end
  the person's other sessions, may be against the institution's terms, and only reads.
- **Scale, stated plainly.** One runner for everyone would send every login from one
  cluster address, and for many people at one bank that trips fraud checks sooner than
  one household does -- hence one runner per person, below. Syncing bank data for people outside the family
  comes close to account aggregation in Taiwan (個資法, and the FSC's open-banking
  rules): fine for invited family and friends, a legal question before any public
  sign-up.

#### One runner per person (P4c-1.6, implemented)

Decided 2026-10-05, replacing P4c-1.5's sync modes (2026-09-30: an admin-set `server`
mode for the cluster's runner, `client` for a runner on the person's computer, two
token kinds). Migration `000008`.

- **Like a self-hosted CI runner.** Every person, admin or not, links their own runner
  under Settings -> Sync runner with a token the app makes for them. Where it runs is
  their business: the owner's happens to run on the same server as the app, anyone
  else's on a computer they leave on. There is no mode and no server runner: one
  `runner` session kind, and every key and connector has an owner.
- **Confined to its owner.** A runner registers keys and publishes connectors for its
  owner only, claims only its owner's connections sealed to one of its keys, and gets
  `not_claimed` for anyone else's. A person's Connections offers their own runner's
  connectors and seals to its newest key; without one, nothing can be linked
  (`no_runner`). `connections.Runner` is that scope, passed to every runner method.
- **Whoever hosts a runner can open its credentials.** Hosting someone else's runner
  (they hand the admin their token) is their explicit choice of trust, not a server
  default.
- **One per person.** Keys a runner stops listing are retired, so two runners of one
  person would retire each other's. Linking again revokes the token before; the same
  runner relinked keeps its key (it lives in the runner's volume), so its connections
  carry on. Several runners per person would need keys scoped per runner: not now.
- **The browser cannot be the runner.** The browser's same-origin rules keep
  ledger.chenantunez.com from calling a bank's site with its cookies, and several
  all-set-tw connectors drive a real Chromium. A native mobile app could host a runner
  later, a PWA cannot.
- **Packaged as a container, for every place.** rigel-sync (P4c-2) ships as one image
  with Node and Chromium inside, so a person installs Docker and nothing
  else -- no Node to install or keep up to date. The same image runs next to the app
  and on a laptop; the documented setup (Settings -> Sync runner shows it) is a
  compose file:

  ```yaml
  services:
    rigel-sync:
      image: ghcr.io/cwchen-twn/rigel-ledger-sync:latest
      platform: linux/amd64             # the only build; Apple silicon runs it under Rosetta
      restart: unless-stopped
      environment:
        RIGEL_URL: https://ledger.chenantunez.com
        RUNNER_TOKEN: ${RUNNER_TOKEN}   # Settings -> Sync runner -> Link a runner
      volumes:
        - rigel-sync:/data              # the runner's private key; never leaves this volume
  volumes:
    rigel-sync:
  ```

  The key is generated inside the volume on first start, so it stays on that machine.
  The image (`integrations/rigel-sync/Dockerfile`, P4c-2d) is Node 22 on Debian
  bookworm-slim with Debian's Chromium, tini as PID 1 (Chrome leaves renderer
  processes to reap), the CAPTCHA model baked in at its sha256, and the runner as
  the unprivileged `node` user. Chrome's sandbox is off (`RIGEL_SYNC_NO_SANDBOX=1`):
  it needs user namespaces, which Docker's and Kubernetes' default seccomp profiles
  do not give a container, so the container is the boundary. About 960 MB, 680 of
  them Chromium and its libraries.
  **linux/amd64 only, on both forges** (the owner's call, 2026-10-06): arm64 on
  Gitea would mean registering QEMU in the k3s node's kernel from a CI job, and
  building it on GitHub alone would make the registries differ. Docker Desktop on
  Apple silicon runs the amd64 image under Rosetta (`platform: linux/amd64` in the
  compose file says so). Revisit if Chrome under Rosetta proves unreliable.
- **For a runner next to the app:** the cluster is in a data centre abroad, and some
  Taiwanese banks flag or block foreign data-centre addresses. The owner's runner may
  need to leave through a Taiwan exit node (Tailscale): `rigel-sync try tw-cathaybk` from
  the server tells, and `CHROME_ARGS=--proxy-server=...` routes the browser alone.

### Receipts

- A receipt photo or PDF can be attached to a transaction when it is entered, or later.
  On a phone the file input opens the camera.
- Storage (#36, migration `000009`, `internal/ledger/attachments.go`):
  - `attachments` holds the bytes as `bytea` in PostgreSQL, so the nightly `pg_dump`
    backs them up with the books; one copy per book and SHA-256, however many
    transactions link it (`transaction_attachments`). A file nothing links any more
    (its last link removed, its transaction or staged row deleted) is deleted by a
    trigger. The audit log records its metadata, never its bytes.
  - Only JPEG, PNG, WebP and PDF, **decided from the bytes**, never the client's
    type: HTML and SVG would run script from the app's origin. Served with that
    type, `nosniff`, and a CSP sandbox for images; 10 MiB per file.
  - Images are downscaled in the browser before upload (2048 px, JPEG 0.85): a 3.9 MB
    camera photo arrives as about 430 KB.
  - The lock date does not stop attaching or removing a file: a receipt changes no
    figure.
  - API: `POST|DELETE /api/books/{id}/transactions/{tid}/attachments[/{aid}]`
    (multipart `file`) and `GET /api/books/{id}/attachments/{aid}`; transactions list
    their attachments. Import tokens reach none of it.
- **KuDE receipts (Paraguay, #44)** are read from their QR code, not by OCR: the code is
  a link to `ekuatia.set.gov.py` whose query carries the e-invoice (the CDC, which
  holds the seller's RUC, the number and the date; the total; the number of lines).
  Imports -> "Import PDF or receipt" takes a photo or a PDF, finds the QR in the tab
  (jsQR, loaded on first use like pdf.js) and stages one `invoice` row on the `kude`
  source: the CDC as id and reference, the total in guaranies as one line, the photo
  (downscaled like any attachment) as its file. It then matches the card or bank
  line that paid like any invoice. The parser is `integrations/rigel-sync/src/
  statements/kude.ts`, shared with the runner. The QR names no currency: a total
  with dots between thousands ("223.542") is read as guaranies, any other fraction
  is refused. Taiwan's paper receipts need nothing: they are 電子發票 on the carrier
  (`tw-einvoice`).
- **Optional local OCR** (#74, v1.1.0), for receipts with neither (the US, travel): the
  photo stays the transaction's attachment and OCR only suggests date, total and
  seller, which the person confirms or types over. It runs locally; receipts never go
  to a cloud service.
- Synced evidence attaches the same way: an order email, an e-invoice or a statement page
  becomes an attachment of the transaction it matched. A batch carries `files[{ref,
  filename, data}]` (base64, inside its 16 MiB) and a row names its `file`; the file is
  stored only for a row that is new, waits with it (`import_rows.attachment_id`, a
  paperclip in the queue), and is linked to whatever transaction accepting the row
  books or matches (a transfer takes both sides' evidence).

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
- The runner is the `xx-mail` connector in rigel-sync (#41; the owner's choices,
  2026-10-06): **any IMAP mailbox, Gmail by default**, one folder or label, opened
  read-only. IMAP is plain Node (imapflow, mailparser).
  - Each run rereads the last 14 days (120 the first time) and resends what it finds:
    rows are idempotent, so a batch the app refused is not lost. Only emails new since
    the last run (by uid, within the folder's uid validity) are printed.
  - **Evidence is the PDF the email attaches** (an invoice, a receipt: the one named
    like one first; #57). Its content is not read (the owner's call, 2026-10-06: an
    email only has to join the payment, not to state it again). An email with no
    attachment (Apple) is printed to PDF by the runner's Chrome, with script off and
    every request but inline `data:` refused, so a tracking pixel never reports the
    sync; the From, Subject and Date head the page. Orders go on one `orders` source
    account; card alerts on `card-<issuer>-<last4>`.
  - **An email that states no amount** (a 電子發票 notice, an order whose price is only
    in its PDF) is an `invoice` row of amount 0 with no items: evidence. It is matched
    to money out from 2 days before to 5 after whose payee or description names its
    seller (the nearest day; booked first, then waiting rows), it never becomes a
    purchase by itself (`amount_unknown`), and it claims nothing an invoice with an
    amount could match.
  - **An invoice number another source knows** (`reference`, migration `000014`): Apple
    and the 關貿 (tradevan) notices name the 電子發票 they issue. The row is then
    `same_invoice` of that 電子發票 (booked: its items' source; waiting: its external
    id), whatever the amounts; it adds only its file.
  - Parsers, each written from a real email and tested on invented ones
    (`src/mail/senders.ts`): Hetzner, DigitalOcean, Apple, Gotogate, 遠傳 (the amount
    due, 應繳金額, last period's unpaid amount included), 電子發票 notices (關貿), Trip.com;
    then schema.org Order and Invoice. Card alerts (國泰世華, 永豐), Taiwan shops (momo,
    PChome, Shopee, Uber Eats, foodpanda) and Google Play, Netflix follow, from samples.
  - `read_back` (optional, a day): the first run after it is set reads the label from
    that day, every email new again, to restore history (國泰期貨 statements, old
    invoices); afterwards the usual overlap.
  - Known gap: a bill paid by auto-debit (遠傳) is charged days after its email, maybe
    outside the 5-day window; it then waits like any invoice, and becomes a cash
    purchase only if a person says so.

### Security and risk

- **Credentials** are sealed in the browser to the runner's key and stored as ciphertext
  in `connections` (see Connections): bank and app logins, the Shioaji key (Account
  permission only, IP-scoped), the Firstrade TOTP secret, the Gmail app password. **They
  are never readable by the app, a log, or `raw`; only the runner opens them, only while
  it signs in.** The runner's own SOPS/age secret holds just its token and private key.
- **Local-only rule.** Statements, invoices, receipts, emails and CAPTCHA images never
  leave the cluster.
  The only outbound traffic is the login to the institution itself. The app is
  tailnet-only.
- **Caveats, stated plainly.**
  - Bank logins are unofficial: a site change breaks them, they can trip fraud checks,
    and some banks end the user's other sessions (all-set-tw notes this).
  - Terms of service may not permit automated access; that risk is the user's.
  - Every connector has an off switch, and **CSV/PDF statement import stays** as the
    fallback. A PDF is read in the browser with pdf.js (#42; no poppler, so the image
    stays distroless, and a statement's password, often the 身分證字號, never leaves the
    tab), by the sync runner's own parsers (`integrations/rigel-sync/src/statements`,
    shared through the `@sync` alias), so an uploaded PDF and an emailed one give the
    same rows. pdf.js is not in the bundle: its two prebuilt files load as module
    scripts when a PDF is chosen, the worker in the page's thread (no `worker-src`).
    The original file is kept in `attachments`. Known statements: 國泰期貨 月對帳單; Banco Continental's Movimientos de Cuenta (USD, PYG) and credit card extracto (#43).

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
  `-tags prod`, and the result runs on distroless static (statement PDFs are read in
  the browser, #42).
- The sync runner image (`rigel-ledger-sync`, from `integrations/rigel-sync`), built by
  the same pipelines from P4.
- Identical pipelines on both forges: `ci` on every push, `image` on `main` and tags,
  and `release` (GoReleaser) on `v*` tags. Gitea pushes to
  `git.chenantunez.com/cwchen-twn/rigel-ledger`, and GitHub pushes to
  `ghcr.io/cwchen-twn/rigel-ledger`.

**hcloud owns** `k3s/helm/rigel-ledger/`, modelled on `k3s/helm/navidrome/`:
- A stateless Deployment (RollingUpdate, no PVC), plus the owner's own sync runners
  (`rigel-ledger-sync`), with only its runner token in a SOPS secret and its
  private key in a small volume; institution logins live sealed in the app.
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
| P2.5 | **Accounts and sign-in security.** a: ~~first-login wizard, verified email, invitations, registration modes, bootstrap admin, the Administration page (users, requests, sign-in rules, defaults, SMTP), throttling and the sign-in audit, sessions (migration `000002`)~~ (done). b: ~~two-factor sign-in -- email codes, TOTP, passkeys, recovery codes, enforcement (`000003`)~~ (done). Both before any public exposure |
| P3 | ~~Exchange-rate scheduler (open.er-api plus fawazahmed0 fallback, and every display currency)~~ (P3a, done); ~~the three statements bound to closing rates with `rates_used`, display-currency translation~~ (P3b, done); ~~book rebase, tag (trip) report~~ (P3c, done) |
| P4 | **Sync and review**: ~~import API, runner tokens, `source_accounts`, review queue, rules, matching (duplicates, pending/posted, transfers), balance assertions and drift, CSV import~~ (P4a, done); ~~per-user Connections with credentials sealed to the runner, the runner protocol, challenges, the fake runner~~ (P4c-1, done); ~~server or client sync mode per person, device runners~~ (P4c-1.5, done); ~~one runner kind, every person links their own~~ (P4c-1.6, done); ~~the sync runner (then tw-sync, now rigel-sync): protocol, sealing, `try`~~ (P4c-2a, done); ~~vendored all-set-tw, the Chrome stand-in, 國泰世華~~ (P4c-2b, done); ~~all-set-tw moved to e303bf0: 國泰 foreign-currency accounts, Zod 4~~ (#64, done); ~~`tw-megabank`, 兆豐銀行~~ (#65, done); ~~`tw-nextbank`, 將來銀行~~ (#66, done); ~~永豐銀行 deposits and card, CAPTCHAs read in the runner~~ (P4c-2c, done); ~~rigel-sync, one container image for the cluster and for people's computers~~ (P4c-2d, done); ~~`import_rows` kinds for invoices, holdings and trades, order emails, challenges~~ (#36-#41, done); ~~receipt attachments (upload, camera) and synced evidence~~ (#36, done); ~~`holding` and `trade` rows, securities from a broker account~~ (#37, done); ~~`invoice` rows with items, matched to what paid, split by item category~~ (#38, done); ~~`tw-tdcc`, 集保 e存摺~~ (#39, done); ~~`tw-einvoice`, 電子發票 on a mobile barcode~~ (#40, done); ~~`xx-mail`, email over IMAP, schema.org orders~~ (#41 part 1, done); ~~國泰期貨 月對帳單 history over email, and statement PDFs uploaded on Imports, read in the browser~~ (#42, done); ~~Banco Continental PDF statements, exchanges between currencies as one transfer~~ (#43, done); ~~email evidence: the attached PDF, emails without an amount, invoice numbers, parsers for Hetzner, DigitalOcean, Apple, Gotogate, 遠傳, 電子發票 notices, Trip.com~~ (#57, done; card alerts come from each bank's connector, #41 closed); ~~one purchase from several sources: waiting rows matched to each other, a second invoice, invoices that wait~~ (#53-#55, done); ~~foreign-currency invoices matched to the card charge~~ (#56, done); ~~`py-continental`, ContiWeb for the month so far: each account's XLS export and the card page, the QR device check sent as a challenge~~ (#67, done); KuDE receipts read from their QR code (#44; general OCR moved to #74). The release itself: #45 |
| P5 | Securities and futures: Shioaji (daily) and Firstrade in rigel-sync (their Python SDKs as subprocesses), quote scheduler, fair value and futures exposure in reports, futures margin postings, FIFO lots for tax. (將來, 兆豐 and Banco Continental moved into P4 for v1.0.0: #43, #65-#67.) Recurring list and subscription templates. (Points, average cost and the security commodity itself are done.) |
| P6 | PWA polish, then Flutter if a native feature is needed |
| P7 | **Tax workbooks (TW, PY)**: `person` tags and `tax_profiles`; tax categories and account mappings per jurisdiction; `tax_withheld` accounts and foreign-tax-paid records; per-year rule files; workbook export (income by category, deductions with evidence, withholding, capital gains in the country's currency and rate). Needs P3 reports, P4 attachments and P5 lots. Prepares and cross-checks; does not file. |
