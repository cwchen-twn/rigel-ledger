include .env

DB_DSN := ${PG_USER}:${PG_PASS}@${PG_HOST}:${PG_PORT}/${APP_DBNAME}?sslmode=disable

.PHONY: help
help:
	@echo 'Usage:'
	@sed -n 's/^##//p' ${MAKEFILE_LIST} | column -t -s ':' |  sed -e 's/^/ /'

##init: Initialize the project
.PHONY: init
init:
	@if [ ! -f go.mod ]; then go mod init github.com/cwc1222/rigelledger; else echo "Project exists, skipping go mod init"; fi
	@go mod tidy
	@go mod verify
	@go mod download
	@command -v uv >/dev/null 2>&1 || curl -LsSf https://astral.sh/uv/install.sh | sh
	@uv sync --dev
	@pre-commit install

##updatedep: Update dependencies
.PHONY: updatedep
updatedep:
	go get -u ./...
	go mod tidy
	uv sync --upgrade-package pre-commit
	pre-commit autoupdate
	pre-commit install

##updatego: Update go version (e.g. $ make updatego version=1.24.6), remember to have $(go env GOPATH) in your PATH
.PHONY: updatego
updatego:
	@if [ -z "$(version)" ]; then \
		echo "Error: Please specify a version. Usage: make updatego version=1.24.6"; \
		exit 1; \
	fi
	go install golang.org/dl/go$(version)@latest
	go$(version) download
	@echo "Go $(version) downloaded successfully in $(shell go env GOPATH)/bin/go$(version). Use 'go$(version)' to run commands with this version."
	cp $(shell go env GOPATH)/bin/go$(version) $(shell go env GOPATH)/bin/go
	@echo "You should add '$(shell go env GOPATH)/bin' to your PATH before '/usr/local/go/bin'"
	go$(version) mod edit -go=$(version)
	go$(version) mod tidy
	go$(version) mod verify
	go$(version) mod download

##audit: Run code quality checks and security audits
.PHONY: audit
audit: test
	go mod tidy -diff
	go mod verify
	test -z "$(shell gofmt -l .)"
	go vet ./...
	go run honnef.co/go/tools/cmd/staticcheck@latest -checks=all,-ST1000,-U1000 ./...
	go run golang.org/x/vuln/cmd/govulncheck@latest ./...

##test: Run all tests with race detection
.PHONY: test
test:
	go test -v -race -buildvcs ./...

##test/cover: Run tests with coverage report
.PHONY: test/cover
test/cover:
	go test -v -race -buildvcs -coverprofile=/tmp/coverage.out ./...
	go tool cover -html=/tmp/coverage.out

##upgradeable: Check for upgradeable dependencies
.PHONY: upgradeable
upgradeable:
	@go run github.com/oligot/go-mod-upgrade@latest

##tidy: Tidy go modules and format code
.PHONY: tidy
tidy:
	go mod tidy -v
	go fmt ./...

##build: Build the application
.PHONY: build
build:
	go build -ldflags "-s -w" -o=/tmp/bin/rigelledger ./cmd/rigelledger

##run: Build and run the application
.PHONY: run
run:
	go build -o=/tmp/bin/rigelledger ./cmd/rigelledger
	/tmp/bin/rigelledger

##run/live: Run with live reload using Air
.PHONY: run/live
run/live:
	go run github.com/cosmtrek/air@v1.43.0 \
		--build.cmd "make build" --build.bin "/tmp/bin/rigelledger" --build.delay "100" \
		--build.exclude_dir "" \
		--build.include_ext "go, tpl, tmpl, html, css, scss, js, ts, sql, jpeg, jpg, gif, png, bmp, svg, webp, ico" \
		--misc.clean_on_exit "true"

##swag: generate swagger documentation
.PHONY: swag
swag:
	go run github.com/swaggo/swag/cmd/swag@latest init -g internal/router.go -o ./api

##migrations/new: create a new database migration (e.g. $ make migrations/new name=init_database)
.PHONY: migrations/new
migrations/new:
	@if [ -z "$(name)" ]; then \
		echo "Error: Please specify a name. Usage: make migrations/new name=migration_name"; \
		exit 1; \
	fi
	go run -tags 'pgx5' github.com/golang-migrate/migrate/v4/cmd/migrate@latest create -seq -ext=.sql -dir=./migrations $(name)

##migrations/version: show the current migration version
.PHONY: migrations/version
migrations/version:
	go run -tags 'pgx5' github.com/golang-migrate/migrate/v4/cmd/migrate@latest \
		-path ./migrations \
		-database "pgx5://${DB_DSN}" \
		version

##migrations/up: apply all pending migrations
.PHONY: migrations/up
migrations/up:
	go run -tags 'pgx5' github.com/golang-migrate/migrate/v4/cmd/migrate@latest \
		-path ./migrations \
		-database "pgx5://${DB_DSN}" \
		up

##migrations/down: revert the last migration
.PHONY: migrations/down
migrations/down:
	go run -tags 'pgx5' github.com/golang-migrate/migrate/v4/cmd/migrate@latest \
		-path ./migrations \
		-database "pgx5://${DB_DSN}" \
		down

##migrations/goto: go to a specific migration (e.g. $ make migrations/goto version=1)
.PHONY: migrations/goto
migrations/goto:
	@if [ -z "$(version)" ]; then \
		echo "Error: Please specify a version. Usage: make migrations/goto version=1"; \
		exit 1; \
	fi
	go run -tags 'pgx5' github.com/golang-migrate/migrate/v4/cmd/migrate@latest \
		-path ./migrations \
		-database "pgx5://${DB_DSN}" \
		goto $(version)
