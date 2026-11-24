# Task interface for this repo: run `make` (or `make help`) to list targets.
# Tool versions live in mise.toml; this file only runs tasks. GNU Make 3.81 compatible (macOS).
# Contract targets first (makefile skill: init tidy lint test audit build run updatedep upgradeable
# clean release), then this repo's group/action targets.

-include .env

# the pinned tools (mise.toml) come first on PATH, so make, pre-commit and CI run the same versions
export PATH := $(shell mise bin-paths 2>/dev/null | tr '\n' ':')$(PATH)
# A shell that mise activated elsewhere may still export another Go's GOROOT; unset, go finds its own
unexport GOROOT

APP_VERSION := $(shell git describe --exact-match --tags HEAD 2>/dev/null || git rev-parse --short HEAD)
DB_DSN := $(PG_USER):$(PG_PASS)@$(PG_HOST):$(PG_PORT)/$(APP_DBNAME)?sslmode=disable

CURRENT_BRANCH := $(shell git branch --show-current)
BUILD_DIR := /tmp/bin

.DEFAULT_GOAL := help

##help: List the targets
.PHONY: help
help:
	@echo 'Usage: make <target> [name=value ...]'
	@sed -n 's/^##//p' $(MAKEFILE_LIST) | column -t -s ':' | sed -e 's/^/ /'

##init: Install the pinned tools (mise), Go and bun dependencies, and the git hooks (commit -> lint, push -> audit)
.PHONY: init
init:
	mise install
	go mod download
	cd web && bun install --frozen-lockfile
	cd integrations/rigel-sync && bun install --frozen-lockfile
	pre-commit install --hook-type pre-commit --hook-type pre-push

##tidy: Format code and tidy modules (writes files)
.PHONY: tidy
tidy:
	go mod tidy -v
	go fmt ./...
	markdownlint-cli2 --fix

##lint: Static checks, read-only and quick (the pre-commit hook) - file hygiene, markdown, gofmt, vet, staticcheck, sqlc diff
.PHONY: lint
lint: lint/hygiene lint/markdown lint/go lint/sqlc

.PHONY: lint/hygiene lint/markdown lint/go lint/sqlc
# the pre-commit file checks on every file (make-lint and make-audit would recurse; the others only apply to a commit)
lint/hygiene:
	SKIP=make-lint,make-audit,no-commit-to-branch,gitleaks pre-commit run --all-files --show-diff-on-failure
lint/markdown:
	markdownlint-cli2
lint/go:
	@test -z "$$(gofmt -l .)" || { gofmt -l .; echo "run: make tidy"; exit 1; }
	go vet ./...
	staticcheck -checks=all,-ST1000,-U1000 ./...
# internal/db is generated: fail if it no longer matches migrations/ and internal/db/queries/
lint/sqlc:
	sqlc diff

##test: Run all tests with race detection (DB tests need TEST_DATABASE_URL, see .env.example)
.PHONY: test
test:
	TEST_DATABASE_URL="$(TEST_DATABASE_URL)" go test -race -buildvcs ./...

##audit: The gate (pre-push hook and CI) - frontend build, lint, tests, tidy/verify, govulncheck, gitleaks
.PHONY: audit
audit: frontend/build/prod lint test
	go mod tidy -diff
	go mod verify
	govulncheck ./...
	gitleaks git --no-banner --redact .

##build: Build the production binary (alias of build/prod)
.PHONY: build
build: build/prod

##updatedep: Update dependencies -- go modules, web packages (bun) and the pre-commit hooks (writes files)
.PHONY: updatedep
updatedep:
	go get -u ./...
	go mod tidy
	# Every web dependency to its latest release, majors included (bun's
	# built-in npm-check-updates; a bare `ncu` may be NVIDIA Nsight Compute).
	cd web && bun update --latest
	cd web && bun run build:prod
	cd integrations/rigel-sync && bun update --latest && bun run check
	pre-commit autoupdate

##upgrade/go: Upgrade the Go toolchain, e.g. `make upgrade/go version=1.27.4` (mise.toml, go.mod, Dockerfile)
.PHONY: upgrade/go
upgrade/go:
	@test -n "$(version)" || { echo "usage: make upgrade/go version=X.Y.Z (newer ones: make upgradeable)"; exit 1; }
	# mise.toml pins the toolchain for this repo (and installs it); go.mod and
	# the image's build stage follow, so CI and the image build with the same
	# compiler (CLAUDE.md "Toolchain pins").
	mise use go@$(version)
	mise exec -- go mod edit -go=$(version)
	mise exec -- go mod tidy
	mise exec -- go mod verify
	# -i.bak: the same flag works with GNU and BSD (macOS) sed.
	sed -i.bak -E 's|^FROM golang:[0-9.]+-bookworm|FROM golang:$(version)-bookworm|' Dockerfile && rm -f Dockerfile.bak
	@grep -n '^go ' go.mod; grep -n '^FROM golang:' Dockerfile; mise exec -- go version

##upgrade/bun: Upgrade Bun, e.g. `make upgrade/bun version=1.4.5` (mise.toml, package.json packageManager, Dockerfiles, bun.lock)
.PHONY: upgrade/bun
upgrade/bun:
	@test -n "$(version)" || { echo "usage: make upgrade/bun version=X.Y.Z (newer ones: make upgradeable)"; exit 1; }
	mise use bun@$(version)
	# CI and the images read packageManager and the Dockerfiles; Renovate moves
	# them together too.
	sed -i.bak -E 's|"packageManager": "bun@[0-9.]+"|"packageManager": "bun@$(version)"|' web/package.json integrations/rigel-sync/package.json \
		&& rm -f web/package.json.bak integrations/rigel-sync/package.json.bak
	sed -i.bak -E 's|^FROM oven/bun:[0-9.]+|FROM oven/bun:$(version)|' Dockerfile integrations/rigel-sync/Dockerfile \
		&& rm -f Dockerfile.bak integrations/rigel-sync/Dockerfile.bak
	cd web && mise exec -- bun install
	cd web && mise exec -- bun run build:prod
	cd integrations/rigel-sync && mise exec -- bun install
	@grep -n '"packageManager"' web/package.json integrations/rigel-sync/package.json; grep -n '^FROM oven/bun:' Dockerfile integrations/rigel-sync/Dockerfile

##upgradeable: List pinned tools (mise.toml) and Go modules with newer versions
.PHONY: upgradeable
upgradeable:
	mise outdated --bump --no-global
	@go list -m -u -f '{{if and .Update (not .Indirect)}}{{.Path}} {{.Version}} -> {{.Update.Version}}{{end}}' all

##clean: Remove build output
.PHONY: clean
clean:
	rm -rf $(BUILD_DIR)/rigel-ledger* ./web/static/dist ./dist ./tmp

##release: Preview the vX.Y.Z tag, and with confirm=yes tag main and push - make release version=X.Y.Z summary="..."
.PHONY: release
release:
	@test -n "$(version)" || { echo 'usage: make release version=X.Y.Z summary="..." [confirm=yes]'; exit 1; }
	./scripts/release.sh "$(version)" "$(summary)" "$(confirm)"

##rigel-sync/check: Typecheck and test the rigel-sync runner (Node 22.18+ runs its TypeScript directly)
.PHONY: rigel-sync/check
rigel-sync/check:
	cd integrations/rigel-sync && bun install --frozen-lockfile && bun run check

##ci/image: Build rigel-ledger-ci, the image the ci job runs in (ci/Dockerfile - the mise.toml tools and PostgreSQL, no code)
.PHONY: ci/image
ci/image:
	docker build -f ci/Dockerfile \
		--build-arg MISE_VERSION=$$(sed -n 's/^ *MISE_VERSION: "\(.*\)"/\1/p' .gitea/workflows/ci-image.yaml) \
		--secret id=gh_token,env=GH_PUBLIC_TOKEN \
		-t rigel-ledger-ci .

# the lockfile's dependencies, as CI and the image install them (about a second when already there)
.PHONY: frontend/deps
frontend/deps:
	cd web && bun install --frozen-lockfile

##frontend/install: Install frontend dependencies using bun
.PHONY: frontend/install
frontend/install:
	cd web && bun install

##frontend/build/dev: Build frontend for development (with source maps)
.PHONY: frontend/build/dev
frontend/build/dev: frontend/deps
	cd web && bun run build:dev

##frontend/build/prod: Build frontend for production (minified)
.PHONY: frontend/build/prod
frontend/build/prod: frontend/deps
	cd web && bun run build:prod

##test/cover: Run tests with coverage report
.PHONY: test/cover
test/cover:
	TEST_DATABASE_URL="$(TEST_DATABASE_URL)" go test -v -race -buildvcs -coverprofile=/tmp/coverage.out ./...
	go tool cover -html=/tmp/coverage.out

##build/dev: Build the application for development
.PHONY: build/dev
build/dev: frontend/build/dev
	@CGO_ENABLED=0 go build -tags "dev" -gcflags=all="-N -l" -o=$(BUILD_DIR)/rigel-ledger ./cmd/rigel-ledger
	@echo "Development build finished"
	du -sh $(BUILD_DIR)/rigel-ledger

##build/dev/go: Rebuild only the Go binary (used by Air during live reload)
.PHONY: build/dev/go
build/dev/go:
	@CGO_ENABLED=0 go build -tags "dev" -gcflags=all="-N -l" -o=$(BUILD_DIR)/rigel-ledger ./cmd/rigel-ledger
	@echo "Go build finished"

##build/prod: Build the application for production
.PHONY: build/prod
build/prod: frontend/build/prod
	@echo "Building production version... $(APP_VERSION)"
	@CGO_ENABLED=0 GOOS=linux go build -tags "prod" -a -installsuffix cgo \
		-ldflags "-s -w -extldflags '-static' -X main.version=$(APP_VERSION)" \
		-o=$(BUILD_DIR)/rigel-ledger.$(APP_VERSION) ./cmd/rigel-ledger
	@echo "Production build finished"
	@du -sh $(BUILD_DIR)/rigel-ledger.$(APP_VERSION)

##build/compare: Compare development vs production build sizes
.PHONY: build/compare
build/compare:
	@echo "Building development version..."
	@make build/dev > /dev/null 2>&1
	@echo "Building production version... $(APP_VERSION)"
	@make build/prod > /dev/null 2>&1
	@echo "\nSize comparison:"
	@echo "Development build, with default go build flags and swagger enabled:"
	@du -sh $(BUILD_DIR)/rigel-ledger
	@echo "Minimized build for production, including build tags and static linking:"
	@du -sh $(BUILD_DIR)/rigel-ledger.$(APP_VERSION)

##checkbuilt: Analyze the built binary using go-size-analyzer
.PHONY: checkbuilt
checkbuilt:
	@if [ -f $(BUILD_DIR)/rigel-ledger ]; then \
		go run github.com/Zxilly/go-size-analyzer/cmd/gsa@v1.12.0 $(BUILD_DIR)/rigel-ledger; \
	else \
		echo "Error: $(BUILD_DIR)/rigel-ledger not found, please build the development binary first"; \
		echo "To build the development binary, run: make build/dev"; \
		exit 1; \
	fi

##checkbuilt/prod: Analyze the built binary using go-size-analyzer
.PHONY: checkbuilt/prod
checkbuilt/prod:
	@if [ -f $(BUILD_DIR)/rigel-ledger.$(APP_VERSION) ]; then \
		go run github.com/Zxilly/go-size-analyzer/cmd/gsa@v1.12.0 $(BUILD_DIR)/rigel-ledger.$(APP_VERSION); \
	else \
		echo "Error: $(BUILD_DIR)/rigel-ledger.$(APP_VERSION) not found, please build the production binary first"; \
		echo "To build the production binary, run: make build/prod"; \
		exit 1; \
	fi

##run: Build and run the application for development
.PHONY: run
run: build/dev
	$(BUILD_DIR)/rigel-ledger

##run/live: Run with live reload using Air for development
.PHONY: run/live
run/live: frontend/build/dev
	(cd web && bun run build:watch) & VITE_PID=$$!; air -c .air.toml; kill $$VITE_PID 2>/dev/null

##run/livedebug: Run with live reload using Air, and debug with delve for development
.PHONY: run/livedebug
run/livedebug: frontend/build/dev
	(cd web && bun run build:watch) & VITE_PID=$$!; air -c .air.toml \
		--build.full_bin "dlv exec $(BUILD_DIR)/rigel-ledger --listen=127.0.0.1:2345 --headless=true --api-version=2 --accept-multiclient --continue --log -- "; kill $$VITE_PID 2>/dev/null

##sqlc: Regenerate internal/db from migrations/ and internal/db/queries/ (make lint fails if this is stale)
.PHONY: sqlc
sqlc:
	sqlc generate

##db/check-dump dump=<file>: Restore a production backup into a throwaway database, migrate it with this build, check it starts and its books' totals hold (#45)
.PHONY: db/check-dump
db/check-dump:
	@test -n "$(dump)" || (echo 'usage: make db/check-dump dump=<file>' && exit 2)
	@TEST_DATABASE_URL="$(TEST_DATABASE_URL)" ./scripts/check-dump.sh $(dump)

##db/reset: Drop every table in the dev database and re-apply migrations (destroys local data) - make db/reset confirm=yes
.PHONY: db/reset
db/reset:
	@echo "drops every table in $(APP_DBNAME) on $(PG_HOST):$(PG_PORT), then re-applies migrations/"
	@test "$(confirm)" = yes || { echo "rerun with confirm=yes"; exit 1; }
	migrate \
		-path ./migrations \
		-database "postgres://$(DB_DSN)" \
		drop -f
	migrate \
		-path ./migrations \
		-database "postgres://$(DB_DSN)" \
		up

##swag: Generate swagger documentation
.PHONY: swag
swag:
	swag init -g internal/routes/router.go -o ./api
	@# swag leaves swagger.json without a final newline, which the end-of-file
	@# hook then adds, failing the commit once; add it here instead.
	@for f in api/docs.go api/swagger.json api/swagger.yaml; do [ -z "$$(tail -c1 $$f)" ] || echo >> $$f; done

##git/analyze: Analyze the git repository using git-of-theseus
.PHONY: git/analyze
git/analyze:
	@uvx --from git-of-theseus==0.3.4 git-of-theseus-analyze --procs 10 --branch $(CURRENT_BRANCH) --outdir ./git-of-theseus ./
	@uvx --from git-of-theseus==0.3.4 git-of-theseus-line-plot --outfile ./git-of-theseus/line_plot_author.png ./git-of-theseus/authors.json
	@uvx --from git-of-theseus==0.3.4 git-of-theseus-line-plot --normalize --outfile ./git-of-theseus/line_plot_author_n.png ./git-of-theseus/authors.json
	@uvx --from git-of-theseus==0.3.4 git-of-theseus-stack-plot --normalize --outfile ./git-of-theseus/stack_plot_author_n.png ./git-of-theseus/authors.json
	@uvx --from git-of-theseus==0.3.4 git-of-theseus-survival-plot --outfile ./git-of-theseus/survival_plot.png ./git-of-theseus/survival.json
	@uvx --from git-of-theseus==0.3.4 git-of-theseus-line-plot --outfile ./git-of-theseus/line_plot_ext.png ./git-of-theseus/exts.json
	@uvx --from git-of-theseus==0.3.4 git-of-theseus-line-plot --normalize --outfile ./git-of-theseus/line_plot_ext_n.png ./git-of-theseus/exts.json
	@uvx --from git-of-theseus==0.3.4 git-of-theseus-stack-plot --normalize --outfile ./git-of-theseus/stack_plot_ext_n.png ./git-of-theseus/exts.json
	@uvx --from git-of-theseus==0.3.4 git-of-theseus-stack-plot --outfile ./git-of-theseus/stack_plot_ext.png ./git-of-theseus/exts.json
	@uvx --from git-of-theseus==0.3.4 git-of-theseus-line-plot --outfile ./git-of-theseus/line_plot_cohorts.png ./git-of-theseus/cohorts.json
	@uvx --from git-of-theseus==0.3.4 git-of-theseus-line-plot --normalize --outfile ./git-of-theseus/line_plot_cohorts_n.png ./git-of-theseus/cohorts.json
	@uvx --from git-of-theseus==0.3.4 git-of-theseus-stack-plot --normalize --outfile ./git-of-theseus/stack_plot_cohorts_n.png ./git-of-theseus/cohorts.json
	@uvx --from git-of-theseus==0.3.4 git-of-theseus-stack-plot --outfile ./git-of-theseus/stack_plot_cohorts.png ./git-of-theseus/cohorts.json

##migrations/new: create a new database migration (e.g. $ make migrations/new name=init_database)
.PHONY: migrations/new
migrations/new:
	@if [ -z "$(name)" ]; then \
		echo "Error: Please specify a name. Usage: make migrations/new name=migration_name"; \
		exit 1; \
	fi
	migrate create -seq -ext=.sql -dir=./migrations $(name)

##migrations/version: show the current migration version
.PHONY: migrations/version
migrations/version:
	migrate \
		-path ./migrations \
		-database "postgres://$(DB_DSN)" \
		version

##migrations/up: apply all pending migrations
.PHONY: migrations/up
migrations/up:
	migrate \
		-path ./migrations \
		-database "postgres://$(DB_DSN)" \
		up

##migrations/down: revert the last migration (destroys its data) - make migrations/down confirm=yes
.PHONY: migrations/down
migrations/down:
	@echo "reverts the newest migration in $(APP_DBNAME) on $(PG_HOST):$(PG_PORT)"
	@test "$(confirm)" = yes || { echo "rerun with confirm=yes"; exit 1; }
	migrate \
		-path ./migrations \
		-database "postgres://$(DB_DSN)" \
		down 1

##migrations/goto: go to a specific migration (e.g. $ make migrations/goto version=1)
.PHONY: migrations/goto
migrations/goto:
	@if [ -z "$(version)" ]; then \
		echo "Error: Please specify a version. Usage: make migrations/goto version=1"; \
		exit 1; \
	fi
	migrate \
		-path ./migrations \
		-database "postgres://$(DB_DSN)" \
		goto $(version)
