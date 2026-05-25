# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

RigelLedger is a personal finance management web application built with Go (backend) and server-rendered HTML templates (frontend). It supports double-entry bookkeeping with multi-currency and multilingual (English/Chinese) ledger types.

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
cmd/rigelledger/    # HTTP server entry point
cmd/cli/            # Admin CLI (user creation)
internal/
  application.go    # App bootstrap: config → DB → HTTP server
  config.go         # Viper-based environment config
  postgres.go       # DB connection + embedded migration runner
  auth/             # JWT middleware and token helpers
  models/           # Domain models + all SQL queries
  response/         # JSON and HTML template renderers
  routes/           # Route definitions and HTTP handlers
web/
  efs.go            # Embeds static/ and templates/ into the binary
  static/           # Vendored JS/CSS libraries
  templates/        # Go html/template files
migrations/         # golang-migrate SQL files (numbered pairs)
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

Queries use `sqlx`. Financial values use `shopspring/decimal` throughout — never `float64`.

### Frontend

Templates use Go's `html/template` with a base layout (`web/templates/partials/base.tmpl`). Pages use Alpine.js for interactivity and Bootstrap 5.3 for styling. DataTables handles server-side pagination via `POST /{username}/api/transactions`. All vendored libraries are embedded into the binary via `web/efs.go`.

## Configuration

Copy `.env.example` to `.env`. Key variables:

| Variable | Purpose |
|---|---|
| `APP_ENV` | `development` or `production` |
| `JWT_SECRET` | HS256 signing key |
| `PG_HOST/PORT/USER/PASS/APP_DBNAME` | PostgreSQL connection |
| `APP_PORT` | HTTP listen port |

## Build Tags

- `//go:build dev` — enables Swagger UI at `/swagger/`
- `//go:build prod` — disables Swagger UI

`make build/dev` passes `-tags dev`; `make build/prod` passes `-tags prod`.

## Key Conventions

- New API endpoints follow the pattern `/{username}/api/<resource>` and require JWT middleware.
- PostgreSQL mutations call `SET LOCAL app.current_user` before writing (audit trail via DB config).
- Bilingual ledger type names are stored as a single concatenated string split by a delimiter — see `migrations/000003_Insert_Ledger_Types.up.sql` for the pattern.
- Pre-commit hooks run `make audit` and gitleaks — fix audit failures before committing.
