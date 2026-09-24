# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

RigelLedger is a personal and family finance web application built with Go (backend) and SolidJS (frontend): double-entry bookkeeping, multi-currency, IFRS-flavoured reports, statement imports and stock investments. It is deployed (ledger.chenantunez.com, tailnet-only), so **applied migrations are frozen**: schema changes are new migration pairs.

**Read `docs/ARCHITECTURE.md` before designing anything.** It is the accepted target design and the roadmap P1-P7; P1, P2, P2.5 (accounts, administration, throttling, two-factor sign-in) and P3 (rates, statements, tags, rebase) are done; P4 (sync and review) is next.

## Common Commands

```bash
# Development
make run/live          # Hot-reload with Air
make run/livedebug     # Hot-reload + Delve debugger

# Build
make build/dev         # Dev build (with debug flags)
make build/prod        # Optimized production build (static binary)

# Quality
make tidy              # Format code and tidy modules
make audit             # tests + tidy check + gofmt + vet + staticcheck + govulncheck
make test              # All tests with -race; DB tests need TEST_DATABASE_URL
make test/cover        # HTML coverage report

# Database
make sqlc              # Regenerate internal/db after touching migrations/ or internal/db/queries/
make db/reset          # Drop the dev database's tables and re-migrate (destroys local data)
make migrations/new name=<name>   # New migration pair: the only way to change the schema now

# Docs
make swag              # Regenerate Swagger/OpenAPI docs (writes to api/)

# Admin CLI (also shipped in the image as rigel-ledger-cli)
go run ./cmd/cli create-user -u alice -e alice@example.com   # password read from stdin; walks the wizard at first sign-in
go run ./cmd/cli set-admin -u alice                          # promote (the Administration page needs an admin)
go run ./cmd/cli reset-mfa -u alice                          # break-glass: drop every second factor and session
go run ./cmd/cli create-runner-token -u alice -l tw-sync     # the sync runner's token, printed once
RUNNER_TOKEN=... go run ./integrations/fake-runner -url http://localhost:8080   # a pretend institution, for Connections
```

To run a single test: `go test -run TestName ./internal/ledger/` (with `TEST_DATABASE_URL` set, e.g. from `.env`).

## Architecture

### Package Structure

```
cmd/rigel-ledger/   # HTTP server entry point
cmd/cli/            # Admin CLI: create-user, reset-password, set-admin
internal/
  application.go    # Bootstrap: config -> migrate -> pgx pool -> mail seed, bootstrap admin -> router; sweeper
  config.go         # caarlos0/env config; .env fills only what the environment leaves unset
  db/               # sqlc-generated queries (DO NOT EDIT *.sql.go, models.go, db.go),
                    #   queries/*.sql, store.go (pool, WithTx, Migrate)
  dbtest/           # Throwaway migrated database per test
  ledger/           # Bookkeeping rules: books, accounts, transactions, balances, rates, preferences
  identity/         # Who may use the app: system settings, mail, bootstrap admin, invitations,
                    #   registration, email verification, the first-login wizard, admin users
  auth/             # Opaque session tokens, CSRF header, client IP, sign-in throttle + auth_events
  mail/             # SMTP/log senders and the embedded mail templates (en, zh, es)
  rates/            # Exchange-rate providers and the hourly scheduler (USD snapshots into prices)
  connections/      # Per-user institution connections and the sync runner's job queue
  sealing/          # X25519 + AES-GCM blobs sealed to the runner (the browser seals; only the runner opens)
  secretbox/        # AES-256-GCM sealing of secrets the DB holds (APP_ENCRYPTION_KEY)
  routes/           # chi router, handlers, JSON DTOs
  response/         # JSON helpers and the SPA shell template
web/
  efs.go            # Embeds static/ and templates/ into the binary (!dev); efs_dev.go reads disk
  src/              # SolidJS app
  static/           # Vite build output (static/dist/, gitignored) and icons
  templates/        # Thin Go html/template shell -- renders <div id="app"> only
migrations/         # golang-migrate SQL, embedded; 000001_init, then one pair per change (frozen once applied)
docs/               # ARCHITECTURE.md -- target design and roadmap
api/                # Generated Swagger output (do not edit manually)
```

### Request Flow

1. `internal/routes/router.go`: `auth.ResolveClientIP` (rightmost untrusted `X-Forwarded-For`, see `TRUSTED_PROXIES`), then `auth.Authenticate` resolves the session from the `rigel_session` cookie or an `Authorization: Bearer` token on every request.
2. `/api/*` routes behind `auth.RequireUser` return 401 when anonymous and 403 `csrf` when a cookie-authenticated POST/PUT/PATCH/DELETE lacks `X-Rigel-Client`. Behind `auth.RequireInitialized`, a user who has not finished the first-login wizard gets 403 `onboarding_required` (only `/api/me`, logout, `/api/me/email*` and `/api/me/onboarding` are open to them). `/api/admin/*` is behind `auth.RequireAdmin` (404 for non-admins). A `token` session (a person's script, made in Settings -> API tokens) only passes `auth.TokenScope`'s list (`tokenAllowed` in `handlers_imports.go`); a `runner` session (the sync runner's, made by an admin) only reaches `/api/runner/*`, which answers 404 to every other kind; anything else is 403 `token_scope`.
3. `/api/books/{bookID}/*` goes through `bookAccess`, which loads the caller's membership (`ledger.Access`); a non-member gets 404, never 403.
4. Handlers decode DTOs, call `ledger.Service` or `identity.Service`, and map `*ledger.Error` to 404/403/409/422 and `*auth.ThrottledError` to 429 (+ `Retry-After`), with `{"error":{"code","message","fields"}}`.
5. `/livez` (process up, never touches the DB) and `/readyz` (pings the DB, 503 when down) are the kubelet probes.
6. Every other GET renders the SPA shell with the user's language and theme and a CSP nonce.

### Ledger rules (internal/ledger)

- Postings are signed (debit > 0). `amount` is in the posting's commodity; `base_amount` is in the book's base currency. A foreign line takes the client's `base_amount` or is converted via `RateOn` (direct, inverse, or crossed through USD, newest price on or before the date).
- A residue of at most one base minor unit is booked to the `fx_gain_loss` account; anything larger is `unbalanced`.
- Exchange rates: `internal/rates` stores one USD->X snapshot per day for every ISO currency (source `er-api` or `fawaz`); `RateOn` crosses through USD. A `manual` rate wins on its date. Never persist a rate as float: providers are decoded with `json.Number`.
- Commodities are `currency` (ISO seed), `security` (`XNAS:AAPL`, quoted in `quote_currency`, futures carry `contract_size`) or `points` (`MILES:EVA`, never priced). For the last two, `amount` is units and `base_amount` is cost: a buy with `unit_cost` is priced via the quote rate; units leaving an asset account go at weighted-average cost (`costBasis`, excluding the transaction being edited); anything else must state its cost (`cost_required`).
- `validCurrency` means ISO money only (book base, display currency, rate quote); `validCommodity` is anything an account can hold. The commodity cache is dropped on `CreateCommodity`.
- `prices` obeys the lock date too: a rate on or before the lock date of any book using either side is frozen (trigger `prices_lock`).
- Go validates first for friendly field errors; the triggers in `000001_init.up.sql` enforce the same rules for any writer. A trigger's `CONSTRAINT` name becomes the API error code (see `ledger.translate`).
- No stored balances: `Balances` sums postings and rolls up the account tree in Go.
- Statements (`internal/ledger/reports.go`): `BalanceSheet`, `IncomeStatement`, `CashFlow`, computed per request, never persisted; values in the base, then translated at the report date (`reportCtx.out`). Revaluation uses `RateDetail` (the rate plus its date and path) so every report lists `rates_used`. Keep assets - liabilities - equity at zero by deriving the unrealised line, not by summing it. `Tags`/`Tag` (tagreport.go) sum expense postings of tagged transactions; `Rebase` (rebase.go) changes a book's base, re-translating every posting (dry run first; refuses on gaps or a lock date).
- New books are seeded from `personalTemplate` in `template.go`; account names are i18n keys (`account.template.<key>`) until renamed.
- Imports (`imports.go`, migration `000005`): sources stage `import_rows`; `propose` matches each (duplicate -> clears -> transfer -> rule) and `AcceptRow` is the only way a row becomes a transaction. Balance rows become `balance_assertions`; `Drifts` compares the newest one per account with the books. Windows and rules: `docs/ARCHITECTURE.md` "The import core".

### Frontend (web/src)

A SolidJS SPA mounted into the Go shell's `<div id="app">`. Vite builds one content-hashed JS and CSS file (`main-<hash>.js`, `main-<hash>.css`) plus `manifest.json` into `web/static/dist` (embedded in the binary); `internal/response/assets.go` reads the manifest to link them, `/static/dist` is served `immutable`, the shell `no-cache`. API responses carry `X-App-Build` (the bundle's name), and a tab running an older bundle shows a reload banner. Never link a dist file by a fixed name. Follow the `frontend-ui` skill (`.claude/skills/frontend-ui/SKILL.md`) for any UI change.

```
api/         client.ts (fetch + X-Rigel-Client + ApiError), types.ts (mirrors routes/dto.go)
stores/      session (me, currencies, live language/theme), book (book, accounts, names, paths, roles)
components/  ui/ -- shadcn-style kit (tokens only, cva variants, Kobalte where a11y is hard)
             AppShell, AccountCombobox, Money/MoneyInput, TransactionSheet (simple + split entry)
pages/       Login, Register, RequestAccess, Invite, VerifyLink, Welcome (wizard), SetupMFA, Onboarding (new book),
             Overview (balances), Transactions, Accounts, Reports (3 statements), Imports (review queue), Connections, BookSettings, UserSettings, admin/ (tabs)
i18n/        en.json, zh.json (Traditional), es.json -- same keys; account names under account.template.*
lib/         money.ts (decimal strings via js-big-decimal), dates.ts, csv.ts (statement parsing), seal.ts (sealing to the runner), cn.ts
```

Routes: signed out `/login`, `/register`, `/request-access`, `/invite/:token`, `/verify?token=`; `/welcome` (first-login wizard); `/onboarding` (new book), `/settings`, `/connections`, `/admin/:tab`, `/b/:bookId/{,transactions,accounts,reports/:tab,imports,settings}`; `/` redirects to the default book.

## Configuration

Copy `.env.example` to `.env`. Real environment variables always win over `.env`.

| Variable | Purpose |
|---|---|
| `APP_ENV` | `development` (no Secure cookie, template reload) or `production` |
| `APP_PORT` | HTTP listen port |
| `SESSION_TTL` | Sliding login lifetime (default `720h`) |
| `DATABASE_URL` | Full postgres URL; wins over the `PG_*` parts (hcloud sets this) |
| `PG_HOST/PORT/USER/PASS/APP_DBNAME` | PostgreSQL connection parts |
| `TEST_DATABASE_URL` | Server where tests may create/drop databases; `REQUIRE_DB_TESTS=1` makes a missing one fatal |
| `APP_ORIGIN` | Public base URL that mail links point at (`https://ledger.chenantunez.com`); development defaults to `http://localhost:<port>` |
| `APP_ENCRYPTION_KEY` | 32 bytes base64 (`openssl rand -base64 32`); seals the SMTP password (and TOTP seeds). Required in production; development uses a public dev key |
| `TRUSTED_PROXIES` | CIDRs whose `X-Forwarded-For` is believed (default pod network + loopback) |
| `RATES_ENABLED` | Daily exchange-rate fetch (default `true`): USD vs every currency from open.er-api, fawazahmed0 as fallback and backfill; `false` = manual rates only |
| `ADMIN_USERNAME` / `ADMIN_INITIAL_PASSWORD` / `ADMIN_EMAIL` | Bootstrap admin, created only while no admin exists; must change the password in the wizard |
| `MAIL_DRIVER`, `SMTP_HOST/PORT/SECURITY/USER/PASS`, `MAIL_FROM(_NAME)` | Seed the mail settings on the first start only; afterwards the Administration page owns them. `MAIL_DRIVER=log` prints mails (links, codes) to the log |

## Build Tags

- `//go:build !prod` -- Swagger UI at `/swagger/`
- `//go:build prod` -- no Swagger UI

`make build/dev` passes `-tags dev`; `make build/prod` passes `-tags prod`.

## Key Conventions

- API endpoints live under `/api`; book data under `/api/books/{bookID}/...`. Check the role with `Access.require` inside the service, not in handlers.
- Every write goes through `db.Store.WithTx(ctx, userID, ...)`, which sets `app.current_user` for the audit trigger. Pass 0 only for CLI/system changes.
- Schema or query change: follow the `db-change` skill (`.claude/skills/db-change/SKILL.md`) -- edit `000001_init` in place until the first deploy, run `make sqlc`, add a DB test for any trigger.
- Money: `NUMERIC` in Postgres, `shopspring/decimal` in Go, strings in JSON -- never floats anywhere. Dates are `YYYY-MM-DD` (`routes.Date`).
- UI text, including account names, lives in the frontend i18n files keyed by stable codes; the database stores no translations. API error codes are translated as `error.<code>`, per-field codes as `field.<code>`, sign-in events as `event.<name>`. The one server-side exception is mail: `internal/mail/templates/{en,zh,es}/*.tmpl`, one file per message per language.
- Two-factor sign-in: sessions carry `aal` (1 password/link, 2 second factor or passkey). `auth.Manager.RequireMFA` confines aal-1 sessions to enrolment while `system_settings.mfa_required`; login answers a challenge instead of a session when the user has a factor. Factors live in `mfa_factors` (email, TOTP sealed by `secretbox`), `webauthn_credentials`, `mfa_recovery_codes`; short-lived ceremony state in `auth_challenges`. Code in `internal/identity/{mfa,passkey}.go`; the frontend's WebAuthn JSON glue is `web/src/lib/webauthn.ts`. Passkeys bind to `APP_ORIGIN`'s host.
- Sign-in and account flows go through `identity.Service`, which records every outcome in `auth_events` (also the throttle's source). New anonymous or code-checking endpoints call `auth.Manager.Check` first and record failures with `Failure: true`.
- Sync and imports: design in `docs/ARCHITECTURE.md` ("Data sources and sync"). Taiwan bank, card, 集保 and e-invoice connectors come from [all-set-tw](https://github.com/TedLin1993/all-set-tw) (MIT) via a Node runner; Shioaji and Firstrade via a Python runner. Each person links institutions under Connections: the browser seals the credentials to the runner's X25519 key (`web/src/lib/seal.ts` = `internal/sealing`), the app stores ciphertext it cannot open, and the runner claims jobs over `/api/runner/*` with a `runner` token (`internal/connections`). Never add a code path that decrypts, logs or returns a sealed blob. `integrations/fake-runner` is the reference runner for development.
- Deployment target: the Helm chart lives in the hcloud repo (`k3s/helm/rigel-ledger/`); this repo only builds the image. See `docs/ARCHITECTURE.md#deployment`.

## CI and releases

Gitea (`git.chenantunez.com`, private) is the primary remote and push-mirrors every commit and tag to the public GitHub repo. Gitea runs only `.gitea/workflows/`, GitHub runs only `.github/workflows/`, and **the two sets must stay behaviourally identical — change one, change the other in the same commit.**

| Trigger | `ci` (frontend build, `sqlc diff`, `make audit` against PostgreSQL 18, pre-commit hooks) | `image` | `release` (GoReleaser) |
|---|---|---|---|
| any push / PR | yes | — | — |
| push to `main` | yes | `:sha-<12>`, `:latest` | — |
| tag `vX.Y.Z` | yes | `:vX.Y.Z`, `:sha-<12>` | binaries (linux/darwin × amd64/arm64) + checksums |

- Images: `git.chenantunez.com/cwchen-twn/rigel-ledger` (Gitea) and `ghcr.io/cwchen-twn/rigel-ledger` (GitHub), built from the same `Dockerfile`, linux/amd64 only. The image carries `rigel-ledger` (entrypoint) and `rigel-ledger-cli`.
- Releases are cut by hand: `git tag vX.Y.Z && git push origin vX.Y.Z` on Gitea; the mirror carries the tag to GitHub. One `.goreleaser.yaml` serves both; `GORELEASER_FORCE_TOKEN` in each workflow picks the forge.
- The version shown in logs comes from `-X main.version` (both `cmd/*/main.go`); a non-empty `APP_VERSION` env var overrides it.
- Gitea secrets on this repo: `REGISTRY_USER`, `REGISTRY_TOKEN` (package rw), `RELEASE_TOKEN` (repo write; mapped to `GITEA_TOKEN`, since Gitea forbids secret names starting `GITEA_`).
- Gitea runner traps are inherited from hcloud (`hcloud/.gitea/CLAUDE.md`): checkout and the GoReleaser API use the in-cluster Service `http://gitea-http.gitea.svc.cluster.local:3000`, `setup-go` runs with `cache: false`, and docker needs the buildx plugin.
- Toolchain pins: Go in `go.mod` + Dockerfile build stage; Bun in `web/package.json` `packageManager` + Dockerfile web stage; sqlc in the Makefile + both CI files (`SQLC_VERSION`). Renovate (hcloud's self-hosted bot, config in `renovate.json`) groups each pair so they move together.
- How CI gets PostgreSQL 18 is the one intended difference between forges: GitHub uses a `services:` container; Gitea installs it inside the job from PGDG, because hcloud's runner puts jobs on docker's default bridge (no service DNS). Both use `localhost:5432`.

## Deploying

**No deploys to hcloud before v1.0.0** (the owner's call, 2026-09-24). Tags and releases are fine; production stays on what it runs, and features are checked on `make run/live` / a local server. Do not open hcloud chart bumps until then.

## Committing

- Use the `commit-style` skill (`.claude/skills/commit-style/SKILL.md`) for every commit message and PR description.
- Commit on a branch: pre-commit runs `no-commit-to-branch main`, `make audit` and gitleaks. Fix failures; never `--no-verify`.
