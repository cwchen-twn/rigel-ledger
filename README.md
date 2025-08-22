# rigelledger

<!-- TOC -->
- [Getting started](#getting-started)
- [References](#references)
    - [Project Layout](#project-layout)
    - [Dependencies](#dependencies)
    - [Others](#others)
<!-- /TOC -->


## Getting started

```bash
# sync the python uv for the precommit
uv sync
pre-commit install

# Ensure the deps are installed
go mod tidy

# Use any of the followings to run the server
go run ./cmd/rigelledger
make run
make run/live

# See all other commands
make help
```

## References

### Project Layout

- [github.com/golang-standards/project-layout](https://github.com/golang-standards/project-layout)
- [github.com/evrone/go-clean-template](https://github.com/evrone/go-clean-template)
- [github.com/eminetto/post-sqlc](https://github.com/eminetto/post-sqlc)
- [autostrada.dev](https://autostrada.dev/)

### Dependencies
- [github.com/go-chi/chi](https://github.com/go-chi/chi)
- [github.com/jmoiron/sqlx](https://github.com/jmoiron/sqlx)
- [github.com/lib/pq](https://github.com/lib/pq)
- [github.com/golang-migrate/migrate](https://github.com/golang-migrate/migrate)
- [github.com/swaggo/swag](https://github.com/swaggo/swag)

### Others
- [github.com/pre-commit/pre-commit-hooks](https://github.com/pre-commit/pre-commit-hooks)
