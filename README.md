# rigel-ledger

[![Go Report Card](https://goreportcard.com/badge/github.com/cwchen-twn/rigel-ledger)](https://goreportcard.com/report/github.com/cwchen-twn/rigel-ledger)
[![GitHub Release](https://img.shields.io/github/v/release/cwchen-twn/rigel-ledger?logo=github)](https://github.com/cwchen-twn/rigel-ledger/releases)
[![GitHub Release Status](https://github.com/cwchen-twn/rigel-ledger/workflows/Release/badge.svg)](https://github.com/cwchen-twn/rigel-ledger/actions)
[![GitHub License](https://img.shields.io/github/license/cwchen-twn/rigel-ledger?logo=github)](https://github.com/cwchen-twn/rigel-ledger/blob/main/LICENSE)
[![GitHub Repo Size](https://img.shields.io/github/repo-size/cwchen-twn/rigel-ledger?logo=github)](https://github.com/cwchen-twn/rigel-ledger)

Self-hosted double entry bookkeeping accounting system, focusing in personal and family usage. Designing for finance management, income statement, cash flow statement, balance sheet, real estate, and stock/future investment.

The target design (schema, multi-currency, IFRS reports, imports, deployment) is in
[docs/ARCHITECTURE.md](docs/ARCHITECTURE.md); the current code predates it.


<!-- TOC -->
- [Getting started](#getting-started)
- [References](#references)
    - [Project Layout](#project-layout)
    - [Dependencies](#dependencies)
        - [Backend](#backend)
        - [Frontend](#frontend)
    - [Go Tools](#go-tools)
    - [Others](#others)
<!-- /TOC -->


## Getting started

```bash
# Init the project using make
make init

# Create a new user
go run ./cmd/cli -c=create-user -u=juanvaldez -e=juanvaldez@gmail.com -p=juanvaldez123 -f="Juan Valdez" -l="Bernbach"

# Use any of the followings to run the server
#   1. make run
#   2. make run/live
#   3. make run/livedebug
make run

# See all other commands
make help
```

## References

### Project Layout

- [github.com/golang-standards/project-layout](https://github.com/golang-standards/project-layout)
- [github.com/evrone/go-clean-template](https://github.com/evrone/go-clean-template)
- [github.com/eminetto/post-sqlc](https://github.com/eminetto/post-sqlc)
- [github.com/avelino/awesome-go](https://github.com/avelino/awesome-go)
- [autostrada.dev](https://autostrada.dev/)
- [github.com/StarpTech/go-web](https://github.com/StarpTech/go-web)

### Dependencies

#### Backend

- [github.com/go-chi/chi](https://github.com/go-chi/chi)
- [github.com/jmoiron/sqlx](https://github.com/jmoiron/sqlx)
- [github.com/jackc/pgx](https://github.com/jackc/pgx)
- [github.com/golang-jwt/jwt](https://github.com/golang-jwt/jwt)
- [github.com/shopspring/decimal](https://github.com/shopspring/decimal)

#### Frontend

- [solidjs.com](https://www.solidjs.com/) and [@solidjs/router](https://github.com/solidjs/solid-router)
- [getbootstrap.com](https://getbootstrap.com/docs/5.3/getting-started/introduction/)
- [icons.getbootstrap.com](https://icons.getbootstrap.com/)
- [github.com/sweetalert2/sweetalert2](https://github.com/sweetalert2/sweetalert2/)
- [github.com/iamkun/dayjs](https://github.com/iamkun/dayjs/)
- [vite.dev](https://vite.dev/) and [bun.sh](https://bun.sh/) (build tooling)

### Go Tools
- [github.com/golang-migrate/migrate](https://github.com/golang-migrate/migrate)
- [github.com/swaggo/swag](https://github.com/swaggo/swag)
- [github.com/go-delve/delve](https://github.com/go-delve/delve)
- [github.com/air-verse/air](https://github.com/air-verse/air)
- [github.com/oligot/go-mod-upgrade](https://github.com/oligot/go-mod-upgrade)
- [github.com/Zxilly/go-size-analyzer](https://github.com/Zxilly/go-size-analyzer)

### Others
- [www.arhea.net/posts/2023-08-25-golang-debugging-with-air-and-vscode](https://www.arhea.net/posts/2023-08-25-golang-debugging-with-air-and-vscode/)
- [github.com/pre-commit/pre-commit-hooks](https://github.com/pre-commit/pre-commit-hooks)
- [github.com/go-chi/jwtauth](https://github.com/go-chi/jwtauth)
- [shields.io/badges](https://shields.io/badges)
- [simpleicons.org](https://simpleicons.org/)
- [goreportcard.com](https://goreportcard.com/)
- [app.codacy.com](https://app.codacy.com/)
- [goreleaser.com](https://goreleaser.com/customization/)
