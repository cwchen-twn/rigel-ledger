include .env

APP_VERSION := $(shell git describe --exact-match --tags HEAD 2>/dev/null || git rev-parse --short HEAD)
DB_DSN := $(PG_USER):$(PG_PASS)@$(PG_HOST):$(PG_PORT)/$(APP_DBNAME)?sslmode=disable

DATE := $(shell date -u +%Y%m%d)
BUILD_DIR := /tmp/bin

.PHONY: help
help:
	@echo 'Usage:'
	@sed -n 's/^##//p' $(MAKEFILE_LIST) | column -t -s ':' |  sed -e 's/^/ /'

##init: Initialize the project, including go modules and uv packages
.PHONY: init
init:
	@if [ ! -f go.mod ]; then go mod init github.com/cwc1222/rigelledger; else echo "Project exists, skipping go mod init"; fi
	@go mod tidy
	@go mod verify
	@go mod download
	@command -v uv >/dev/null 2>&1 || curl -LsSf https://astral.sh/uv/install.sh | sh
	@uv sync --dev
	@.venv/bin/pre-commit install

##updatedep: Update dependencies, including go modules and uv packages
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

##installvsext: Install vscode extensions
.PHONY: installvsext
installvsext:
	@if [ -f .vscode/extensions.json ]; then \
		jq -r '.recommendations[]' .vscode/extensions.json | while read ext; do \
			if [ -n "$$ext" ]; then \
				echo "Installing extension: $$ext"; \
				code --install-extension "$$ext"; \
			fi; \
		done; \
	else \
		echo "No .vscode/extensions.json file found"; \
	fi

##upgradeable: Check for upgradeable dependencies
.PHONY: upgradeable
upgradeable:
	@go run github.com/oligot/go-mod-upgrade@latest

##tidy: Tidy go modules and format code
.PHONY: tidy
tidy:
	go mod tidy -v
	go fmt ./...

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

##build/dev: Build the application for development
.PHONY: build/dev
build/dev:
	@CGO_ENABLED=0 go build -tags "dev" -gcflags=all="-N -l" -o=$(BUILD_DIR)/rigelledger ./cmd/rigelledger
	@echo "Development build finished"
	du -sh $(BUILD_DIR)/rigelledger

##build/prod: Build the application for production
.PHONY: build/prod
build/prod:
	@echo "Building production version... $(APP_VERSION)"
	@CGO_ENABLED=0 GOOS=linux go build -tags "prod" -a -installsuffix cgo \
		-ldflags "-s -w -extldflags '-static'" \
		-o=$(BUILD_DIR)/rigelledger.$(APP_VERSION) ./cmd/rigelledger
	@echo "Production build finished"
	@du -sh $(BUILD_DIR)/rigelledger.$(APP_VERSION)

##build/compare: Compare development vs production build sizes
.PHONY: build/compare
build/compare:
	@echo "Building development version..."
	@make build/dev > /dev/null 2>&1
	@echo "Building production version... $(APP_VERSION)"
	@make build/prod > /dev/null 2>&1
	@echo "\nSize comparison:"
	@echo "Development build, with default go build flags and swagger enabled:"
	@du -sh $(BUILD_DIR)/rigelledger
	@echo "Minimized build for production, including build tags and static linking:"
	@du -sh $(BUILD_DIR)/rigelledger.$(APP_VERSION)

##checkbuilt: Analyze the built binary using go-size-analyzer
.PHONY: checkbuilt
checkbuilt:
	@if [ -f $(BUILD_DIR)/rigelledger ]; then \
		go run github.com/Zxilly/go-size-analyzer/cmd/gsa@latest $(BUILD_DIR)/rigelledger; \
	else \
		echo "Error: $(BUILD_DIR)/rigelledger not found, please build the development binary first"; \
		echo "To build the development binary, run: make build/dev"; \
		exit 1; \
	fi

##checkbuilt/prod: Analyze the built binary using go-size-analyzer
.PHONY: checkbuilt/prod
checkbuilt/prod:
	@if [ -f $(BUILD_DIR)/rigelledger.$(APP_VERSION) ]; then \
		go run github.com/Zxilly/go-size-analyzer/cmd/gsa@latest $(BUILD_DIR)/rigelledger.$(APP_VERSION); \
	else \
		echo "Error: $(BUILD_DIR)/rigelledger.$(APP_VERSION) not found, please build the production binary first"; \
		echo "To build the production binary, run: make build/prod"; \
		exit 1; \
	fi

##run: Build and run the application for development
.PHONY: run
run: build/dev
	$(BUILD_DIR)/rigelledger

##run/live: Run with live reload using Air for development
.PHONY: run/live
run/live:
	go run github.com/air-verse/air@latest \
		--build.cmd "make build/dev" --build.bin "$(BUILD_DIR)/rigelledger" --build.delay "100" \
		--build.exclude_dir "" \
		--build.include_ext "go, tpl, tmpl, html, css, scss, js, ts, sql, jpeg, jpg, gif, png, bmp, svg, webp, ico" \
		--misc.clean_on_exit "true"

##run/livedebug: Run with live reload using Air, and debug with delve for development
.PHONY: run/livedebug
run/livedebug:
	go run github.com/air-verse/air@latest \
		--build.cmd "make build/dev" --build.bin "$(BUILD_DIR)/rigelledger" --build.delay "100" \
		--build.exclude_dir "" \
		--build.include_ext "go, tpl, tmpl, html, css, scss, js, ts, sql, jpeg, jpg, gif, png, bmp, svg, webp, ico" \
		--build.full_bin "dlv exec $(BUILD_DIR)/rigelledger --listen=127.0.0.1:2345 --headless=true --api-version=2 --accept-multiclient --continue --log -- " \
		--misc.clean_on_exit "true"

##swag: Generate swagger documentation
.PHONY: swag
swag:
	go run github.com/swaggo/swag/cmd/swag@latest init -g internal/routes/router.go -o ./api

##migrations/new: create a new database migration (e.g. $ make migrations/new name=init_database)
.PHONY: migrations/new
migrations/new:
	@if [ -z "$(name)" ]; then \
		echo "Error: Please specify a name. Usage: make migrations/new name=migration_name"; \
		exit 1; \
	fi
	go run -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest create -seq -ext=.sql -dir=./migrations $(name)

##migrations/version: show the current migration version
.PHONY: migrations/version
migrations/version:
	go run -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest \
		-path ./migrations \
		-database "postgres://$(DB_DSN)" \
		version

##migrations/up: apply all pending migrations
.PHONY: migrations/up
migrations/up:
	go run -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest \
		-path ./migrations \
		-database "postgres://$(DB_DSN)" \
		up

##migrations/down: revert the last migration
.PHONY: migrations/down
migrations/down:
	go run -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest \
		-path ./migrations \
		-database "postgres://$(DB_DSN)" \
		down

##migrations/goto: go to a specific migration (e.g. $ make migrations/goto version=1)
.PHONY: migrations/goto
migrations/goto:
	@if [ -z "$(version)" ]; then \
		echo "Error: Please specify a version. Usage: make migrations/goto version=1"; \
		exit 1; \
	fi
	go run -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest \
		-path ./migrations \
		-database "postgres://$(DB_DSN)" \
		goto $(version)
