# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

RigelLedger is a personal and family finance web application built with Go (backend) and SolidJS (frontend): double-entry bookkeeping, multi-currency, IFRS-flavoured reports, statement imports and stock investments. It has never been deployed, so schema and design may change freely.

**Read `docs/ARCHITECTURE.md` before designing anything.** It is the accepted target design (schema reset, books with members, multi-currency with `base_amount`, exchange-rate scheduler, user settings, local-only PDF imports, Firstrade sync, hcloud deployment) and the roadmap P1-P6. The code described below PREDATES it: the current migrations, the 4-grade `ref_ledger_*` chart of accounts, the stored `balance` column and the JWT auth are all slated for replacement. Do not extend them; build toward the target.

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
make audit             # gofmt + go vet + staticcheck + govulncheck + race test
make test              # Run all tests with race detection
make test/cover        # Generate HTML coverage report

# Database
make migrations/new name=<name>   # Create new migration file pair
make migrations/up                # Apply pending migrations
make migrations/down              # Revert last migration
make migrations/goto version=<n>  # Jump to specific migration

# Docs
make swag              # Regenerate Swagger/OpenAPI docs (writes to api/)
```

To run a single test: `go test -run TestName ./internal/auth/`

## Architecture

### Package Structure

```
cmd/rigel-ledger/    # HTTP server entry point
cmd/cli/            # Admin CLI (user creation)
internal/
  application.go    # App bootstrap: config → DB → HTTP server
  config.go         # caarlos0/env config, plus a hand-written .env loader
  postgres.go       # DB connection + embedded migration runner
  auth/             # JWT middleware and token helpers
  models/           # Domain models + all SQL queries
  response/         # JSON and HTML template renderers
  routes/           # Route definitions and HTTP handlers
web/
  efs.go            # Embeds static/ and templates/ into the binary
  src/              # SolidJS app (components, pages, stores, API client)
  static/           # Vite build output (static/dist/, gitignored) and icons
  templates/        # Thin Go html/template shell — renders <div id="app"> only
migrations/         # golang-migrate SQL files (numbered pairs), embedded and run on start
docs/               # ARCHITECTURE.md -- target design and roadmap
api/                # Generated Swagger output (do not edit manually)
```

### Request Flow

1. `internal/routes/router.go` mounts all routes with middleware (CSRF-equivalent, security headers, JWT check).
2. JWT middleware (`internal/auth/jwt.go`) extracts tokens from cookies (`rigel_jwt_access`, `rigel_jwt_refresh`), validates with HS256, and sets user context.
3. Handlers in `internal/routes/handlers.go` call model functions for DB access, then delegate to `internal/response/` for JSON or template rendering.
4. Template renderer (`internal/response/templates.go`) executes templates from the embedded FS with a nonce for CSP.

### Models Layer

All database queries live in `internal/models/`. Each file owns one domain:
- `user.go` — authentication, bcrypt password checks
- `ledger.go` — ledger CRUD, ledger types, currency list
- `journal.go` — journal postings with DataTable-compatible server-side pagination

Queries use `sqlx` over the pgx stdlib driver (the target is pgx + sqlc). Financial values use `shopspring/decimal` throughout — never `float64`.

### Frontend

The frontend is a SolidJS SPA. Go's `html/template` serves only as a thin shell — `web/templates/app.tmpl` renders a single `<div id="app"></div>` that SolidJS mounts into. There are no multi-page server-rendered views; all UI lives in `web/src/`. The Go template layer has no user-facing content and does not need to participate in i18n or any UI logic.

## Configuration

Copy `.env.example` to `.env`. Key variables:

| Variable | Purpose |
|---|---|
| `APP_ENV` | `development` or `production` |
| `JWT_SECRET` | HS256 signing key (goes away with the move to session tokens) |
| `PG_HOST/PORT/USER/PASS/APP_DBNAME` | PostgreSQL connection |
| `APP_PORT` | HTTP listen port |

## Build Tags

- `//go:build dev` — enables Swagger UI at `/swagger/`
- `//go:build prod` — disables Swagger UI

`make build/dev` passes `-tags dev`; `make build/prod` passes `-tags prod`.

## Key Conventions

- Current API endpoints follow `/{username}/api/<resource>` behind JWT middleware; the target is `/api/books/{id}/<resource>` authorised by book membership.
- PostgreSQL mutations call `SET LOCAL app.current_user` before writing (audit trail via DB config).
- UI text, including account-type names, lives in the frontend i18n files (`web/src/i18n/{en,zh,es}.json`) keyed by stable codes; the database stores no translations (since migration 000005).
- Money: `NUMERIC` in Postgres, `shopspring/decimal` in Go, strings in JSON — never floats anywhere.
- Deployment target: the Helm chart lives in the hcloud repo (`k3s/helm/rigel-ledger/`); this repo only builds the image. See `docs/ARCHITECTURE.md#deployment`.

## CI and releases

Gitea (`git.chenantunez.com`, private) is the primary remote and push-mirrors every commit and tag to the public GitHub repo. Gitea runs only `.gitea/workflows/`, GitHub runs only `.github/workflows/`, and **the two sets must stay behaviourally identical — change one, change the other in the same commit.**

| Trigger | `ci` (frontend build, `make audit`, pre-commit hooks) | `image` | `release` (GoReleaser) |
|---|---|---|---|
| any push / PR | yes | — | — |
| push to `main` | yes | `:sha-<12>`, `:latest` | — |
| tag `vX.Y.Z` | yes | `:vX.Y.Z`, `:sha-<12>` | binaries (linux/darwin × amd64/arm64) + checksums |

- Images: `git.chenantunez.com/cwchen-twn/rigel-ledger` (Gitea) and `ghcr.io/cwchen-twn/rigel-ledger` (GitHub), built from the same `Dockerfile`, linux/amd64 only. The image carries `rigel-ledger` (entrypoint) and `rigel-ledger-cli`.
- Releases are cut by hand: `git tag vX.Y.Z && git push origin vX.Y.Z` on Gitea; the mirror carries the tag to GitHub. One `.goreleaser.yaml` serves both; `GORELEASER_FORCE_TOKEN` in each workflow picks the forge.
- The version shown in logs comes from `-X main.version` (both `cmd/*/main.go`); a non-empty `APP_VERSION` env var overrides it.
- Gitea secrets on this repo: `REGISTRY_USER`, `REGISTRY_TOKEN` (package rw), `RELEASE_TOKEN` (repo write; mapped to `GITEA_TOKEN`, since Gitea forbids secret names starting `GITEA_`).
- Gitea runner traps are inherited from hcloud (`hcloud/.gitea/CLAUDE.md`): checkout and the GoReleaser API use the in-cluster Service `http://gitea-http.gitea.svc.cluster.local:3000`, `setup-go` runs with `cache: false`, and docker needs the buildx plugin.
- Toolchain pins: Go in `go.mod` + Dockerfile build stage; Bun in `web/package.json` `packageManager` + Dockerfile web stage. Renovate (hcloud's self-hosted bot, config in `renovate.json`) groups each pair so they move together.

## Committing

- Use the `commit-style` skill (`.claude/skills/commit-style/SKILL.md`) for every commit message and PR description.
- Commit on a branch: pre-commit runs `no-commit-to-branch main`, `make audit` and gitleaks. Fix failures; never `--no-verify`.
