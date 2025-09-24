# rigelledger

<!-- TOC -->
- [Getting started](#getting-started)
- [References](#references)
    - [Project Layout](#project-layout)
    - [Dependencies](#dependencies)
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
- [github.com/lib/pq](https://github.com/lib/pq)
- [github.com/golang-jwt/jwt](https://github.com/golang-jwt/jwt)
- [github.com/shopspring/decimal](https://github.com/shopspring/decimal)

#### Frontend

- [github.com/alpinejs/alpine](https://github.com/alpinejs/alpine)
- [getbootstrap.com](https://getbootstrap.com/docs/5.3/getting-started/introduction/)
- [icons.getbootstrap.com](https://icons.getbootstrap.com/)
- [github.com/orchidjs/tom-select](https://github.com/orchidjs/tom-select/)
- [github.com/sweetalert2/sweetalert2](https://github.com/sweetalert2/sweetalert2/)
- [github.com/iamkun/dayjs](https://github.com/iamkun/dayjs/)
- [github.com/royNiladri/js-big-decimal](https://github.com/royNiladri/js-big-decimal/)
- [github.com/DataTables/DataTables](https://github.com/DataTables/DataTables)

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
