# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

RigelLedger is a personal and family finance web application built with Go (backend) and SolidJS (frontend): double-entry bookkeeping, multi-currency, IFRS-flavoured reports, statement imports and stock investments. It has never been deployed, so schema and design may change freely.

**Read `docs/ARCHITECTURE.md` before designing anything.** It is the accepted target design and the roadmap P1-P6; P1 (schema, sqlc, sessions, the book-scoped API and the SolidJS UI) is implemented.

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
make migrations/new name=<name>   # New migration pair (only after the first deploy)

# Docs
make swag              # Regenerate Swagger/OpenAPI docs (writes to api/)

# Admin CLI (also shipped in the image as rigel-ledger-cli)
go run ./cmd/cli create-user -u alice -e alice@example.com   # password read from stdin
```

To run a single test: `go test -run TestName ./internal/ledger/` (with `TEST_DATABASE_URL` set, e.g. from `.env`).

## Architecture

### Package Structure

```
cmd/rigel-ledger/   # HTTP server entry point
cmd/cli/            # Admin CLI: create-user, reset-password, set-admin
internal/
  application.go    # Bootstrap: config -> migrate -> pgx pool -> router; session sweeper
  config.go         # caarlos0/env config; .env fills only what the environment leaves unset
  db/               # sqlc-generated queries (DO NOT EDIT *.sql.go, models.go, db.go),
                    #   queries/*.sql, store.go (pool, WithTx, Migrate)
  dbtest/           # Throwaway migrated database per test
  ledger/           # Bookkeeping rules: books, accounts, transactions, balances, rates, users
  auth/             # Opaque session tokens (cookie or Bearer), CSRF header check
  routes/           # chi router, handlers, JSON DTOs
  response/         # JSON helpers and the SPA shell template
web/
  efs.go            # Embeds static/ and templates/ into the binary (!dev); efs_dev.go reads disk
  src/              # SolidJS app
  static/           # Vite build output (static/dist/, gitignored) and icons
  templates/        # Thin Go html/template shell -- renders <div id="app"> only
migrations/         # golang-migrate SQL, embedded; 000001_init is the whole schema
docs/               # ARCHITECTURE.md -- target design and roadmap
api/                # Generated Swagger output (do not edit manually)
```

### Request Flow

1. `internal/routes/router.go`: `auth.Authenticate` resolves the session from the `rigel_session` cookie or an `Authorization: Bearer` token on every request.
2. `/api/*` routes behind `auth.RequireUser` return 401 when anonymous and 403 `csrf` when a cookie-authenticated POST/PUT/PATCH/DELETE lacks `X-Rigel-Client`.
3. `/api/books/{bookID}/*` goes through `bookAccess`, which loads the caller's membership (`ledger.Access`); a non-member gets 404, never 403.
4. Handlers decode DTOs, call `ledger.Service`, and map `*ledger.Error` to 404/403/409/422 with `{"error":{"code","message","fields"}}`.
5. `/livez` (process up, never touches the DB) and `/readyz` (pings the DB, 503 when down) are the kubelet probes.
6. Every other GET renders the SPA shell with the user's language and theme and a CSP nonce.

### Ledger rules (internal/ledger)

- Postings are signed (debit > 0). `amount` is in the posting's commodity; `base_amount` is in the book's base currency. A foreign line takes the client's `base_amount` or is converted via `RateOn` (direct, inverse, or crossed through USD, newest price on or before the date).
- A residue of at most one base minor unit is booked to the `fx_gain_loss` account; anything larger is `unbalanced`.
- Commodities are `currency` (ISO seed), `security` (`XNAS:AAPL`, quoted in `quote_currency`, futures carry `contract_size`) or `points` (`MILES:EVA`, never priced). For the last two, `amount` is units and `base_amount` is cost: a buy with `unit_cost` is priced via the quote rate; units leaving an asset account go at weighted-average cost (`costBasis`, excluding the transaction being edited); anything else must state its cost (`cost_required`).
- `validCurrency` means ISO money only (book base, display currency, rate quote); `validCommodity` is anything an account can hold. The commodity cache is dropped on `CreateCommodity`.
- `prices` obeys the lock date too: a rate on or before the lock date of any book using either side is frozen (trigger `prices_lock`).
- Go validates first for friendly field errors; the triggers in `000001_init.up.sql` enforce the same rules for any writer. A trigger's `CONSTRAINT` name becomes the API error code (see `ledger.translate`).
- No stored balances: `Balances` sums postings and rolls up the account tree in Go.
- New books are seeded from `personalTemplate` in `template.go`; account names are i18n keys (`account.template.<key>`) until renamed.

### Frontend (web/src)

A SolidJS SPA mounted into the Go shell's `<div id="app">`; Vite builds one `main.js` and one `main.css` into `web/static/dist` (embedded in the binary). Follow the `frontend-ui` skill (`.claude/skills/frontend-ui/SKILL.md`) for any UI change.

```
api/         client.ts (fetch + X-Rigel-Client + ApiError), types.ts (mirrors routes/dto.go)
stores/      session (me, currencies, live language/theme), book (book, accounts, names, paths, roles)
components/  ui/ -- shadcn-style kit (tokens only, cva variants, Kobalte where a11y is hard)
             AppShell, AccountCombobox, Money/MoneyInput, TransactionSheet (simple + split entry)
pages/       Login, Onboarding, Overview (balances), Transactions, Accounts, BookSettings, UserSettings
i18n/        en.json, zh.json (Traditional), es.json -- same keys; account names under account.template.*
lib/         money.ts (decimal strings via js-big-decimal), dates.ts, cn.ts
```

Routes: `/login`, `/onboarding`, `/settings`, `/b/:bookId/{,transactions,accounts,settings}`; `/` redirects to the default book.

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

## Build Tags

- `//go:build !prod` -- Swagger UI at `/swagger/`
- `//go:build prod` -- no Swagger UI

`make build/dev` passes `-tags dev`; `make build/prod` passes `-tags prod`.

## Key Conventions

- API endpoints live under `/api`; book data under `/api/books/{bookID}/...`. Check the role with `Access.require` inside the service, not in handlers.
- Every write goes through `db.Store.WithTx(ctx, userID, ...)`, which sets `app.current_user` for the audit trigger. Pass 0 only for CLI/system changes.
- Schema or query change: follow the `db-change` skill (`.claude/skills/db-change/SKILL.md`) -- edit `000001_init` in place until the first deploy, run `make sqlc`, add a DB test for any trigger.
- Money: `NUMERIC` in Postgres, `shopspring/decimal` in Go, strings in JSON -- never floats anywhere. Dates are `YYYY-MM-DD` (`routes.Date`).
- UI text, including account names, lives in the frontend i18n files keyed by stable codes; the database stores no translations. API error codes are translated as `error.<code>`, per-field codes as `field.<code>`.
- Sync and imports: design in `docs/ARCHITECTURE.md` ("Data sources and sync"). Taiwan bank, card, 集保 and e-invoice connectors come from [all-set-tw](https://github.com/TedLin1993/all-set-tw) (MIT) via a Node runner; Shioaji and Firstrade via a Python runner. Institution credentials live only in the runners' SOPS secrets, never in the database.
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

## Committing

- Use the `commit-style` skill (`.claude/skills/commit-style/SKILL.md`) for every commit message and PR description.
- Commit on a branch: pre-commit runs `no-commit-to-branch main`, `make audit` and gitleaks. Fix failures; never `--no-verify`.
