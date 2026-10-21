# The local environment files are optional. The leading `-` makes a missing
# file a non-error, so a fresh clone can run `make check`, `make clean` and
# `make build` without a secret file. The targets that genuinely need the
# variables check for themselves (see `require_db_vars` below).
-include .env .env.local
export

SERVER_NAME := server
CLI_NAME := cli

APP_NAME := eznit
APP_NAME_SHORT := ez
SCRIPT_NAME := add_to_path
INJECT_VERSION:= main.Version

API_FOLDER := ./cmd/api
CLI_FOLDER := ./cmd/cli

DOCKER_COMPOSE := ./docker-compose.yaml
MIGRATIONS := ./db/migrations
SCRIPT_MIGRATION := ./scripts/run_migrate.sh
SECRETS_EXAMPLE := ./secrets/db_password.txt.example
ENV_EXAMPLE := ./.env.example

# Detect last tag and increment patch
VERSION := $(shell git describe --tags --always)

# Scripts
SCRIPTS_DIR := scripts
WINDOWS_SCRIPT :=  $(SCRIPTS_DIR)/$(SCRIPT_NAME).ps1
LINUX_SCRIPT :=  $(SCRIPTS_DIR)/$(SCRIPT_NAME).sh

# Builds
BUILD_DIR := ./build
BUILD_CLI:=$(BUILD_DIR)/cli
BUILD_API:=$(BUILD_DIR)/api

# CLI
WINDOWS_CLI:=$(BUILD_CLI)/windows
WINDOWS_BIN := $(WINDOWS_CLI)/$(APP_NAME).exe
SHORT_WINDOWS_BIN := $(WINDOWS_CLI)/$(APP_NAME_SHORT).exe
WINDOWS_ZIP := $(WINDOWS_CLI)/$(APP_NAME)_$(CLI_NAME)_$(VERSION)_windows.zip

LINUX_CLI:=$(BUILD_CLI)/linux
LINUX_BIN := $(LINUX_CLI)/$(APP_NAME)
SHORT_LINUX_BIN := $(LINUX_CLI)/$(APP_NAME_SHORT)
LINUX_TAR := $(LINUX_CLI)/$(APP_NAME)_$(CLI_NAME)_$(VERSION)_linux.tar.gz

# API
LINUX_SERVER_BIN := $(BUILD_API)/$(SERVER_NAME)
LINUX_SERVER_TAR := $(BUILD_API)/$(APP_NAME)_$(SERVER_NAME)_$(VERSION)_linux.tar.gz

WINDOWS_SERVER_ZIP := $(BUILD_API)/$(APP_NAME)_$(SERVER_NAME)_$(VERSION)_windows.zip

## Builds
.PHONY: clean
.PHONY: build build_cli build_api verify
.PHONY: zip
.PHONY: check

build: clean zip
	@echo "Release ready: $(VERSION)"

# Create build folder if missing
verify:
	mkdir -p $(BUILD_DIR)

build_cli: verify
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -ldflags "-X $(INJECT_VERSION)=$(VERSION)" -o $(WINDOWS_BIN) $(CLI_FOLDER)
	cp $(WINDOWS_BIN) $(SHORT_WINDOWS_BIN)

	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags "-X $(INJECT_VERSION)=$(VERSION)" -o $(LINUX_BIN) $(CLI_FOLDER)
	cp $(LINUX_BIN) $(SHORT_LINUX_BIN)

	chmod -R a+r $(BUILD_CLI)
	chmod a+x $(LINUX_BIN)
	chmod a+x $(SHORT_LINUX_BIN)

build_api: verify
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags "-X $(INJECT_VERSION)=$(VERSION)" -o $(LINUX_SERVER_BIN) $(API_FOLDER)

	chmod -R a+r $(BUILD_API)
	chmod a+x $(LINUX_SERVER_BIN)

zip: build_cli build_api
	zip -j $(WINDOWS_ZIP) $(WINDOWS_BIN) $(SHORT_WINDOWS_BIN) $(WINDOWS_SCRIPT) README.md LICENSE
	zip -r $(WINDOWS_SERVER_ZIP) $(LINUX_SERVER_BIN) $(MIGRATIONS) $(DOCKER_COMPOSE) $(SCRIPT_MIGRATION) $(SECRETS_EXAMPLE) $(ENV_EXAMPLE) README.md LICENSE

	tar -czvf $(LINUX_TAR) \
	          -C $(LINUX_CLI) $(notdir $(LINUX_BIN)) $(notdir $(SHORT_LINUX_BIN)) \
	          -C ../../../$(SCRIPTS_DIR) $(notdir $(LINUX_SCRIPT)) ../README.md ../LICENSE

	tar -czvf $(LINUX_SERVER_TAR) \
	          -C ./ $(LINUX_SERVER_BIN) \
	          -C ./ ./README.md ./LICENSE $(MIGRATIONS) $(DOCKER_COMPOSE) $(SCRIPT_MIGRATION) $(SECRETS_EXAMPLE) $(ENV_EXAMPLE) \

clean:
	rm -rf $(BUILD_DIR)

## Migrations
CONN := "postgresql://$(DB_USER):$(DB_PWD)@$(DB_HOST):$(DB_PORT)/$(DB_NAME)?sslmode=disable"
FOLDER := ./db/migrations

# The connection string carries the database password, and `make` echoes a
# recipe before running it. Every migration recipe below is therefore silenced
# with `@`, so the password never reaches the terminal or a CI log. The
# `migrate` tool still prints what it does.
#
# The environment files are optional (see the `-include` at the top of this
# file), so the targets that need the connection check for it themselves
# instead of failing with an empty DSN.
define require_db_vars
	@if [ -z "$(DB_USER)" ] || [ -z "$(DB_HOST)" ] || [ -z "$(DB_NAME)" ]; then \
	    echo "ERROR: DB_USER, DB_HOST and DB_NAME are not set." >&2; \
	    echo "       Copy .env.example to .env and fill it in, then try again." >&2; \
	    exit 1; \
	fi
endef

.PHONY: migrate_up migrate_down migrate_create migrate_force

migrate_create:
	@if [ -z "$(VAL)" ]; then\
	    echo "VAL is required. Usage: make create VAL=NEW_NAME"; exit 1;\
	fi
	@echo "Running migrate create $(VAL)"
	migrate create -ext sql -dir $(FOLDER) -seq $(VAL)

migrate_up:
	$(require_db_vars)
	@if [ -z "$(VAL)" ]; then \
	    echo "Running just migrate up"; \
	    @migrate -path "$(FOLDER)" -database "$(CONN)" up; \
	else \
	    echo "Running migrate up $(VAL)"; \
	    @migrate -path "$(FOLDER)" -database "$(CONN)" up "$(VAL)"; \
	fi

migrate_down_all:
	$(require_db_vars)
	@echo "Running just migrate down"
	@migrate -path "$(FOLDER)" -database "$(CONN)" down

migrate_down: VAL=1
migrate_down:
	$(require_db_vars)
	@echo "Running migrate down $(VAL)"
	@migrate -path "$(FOLDER)" -database "$(CONN)" down "$(VAL)"

migrate_force:
	$(require_db_vars)
	@if [ -z "$(VAL)" ]; then\
	    echo "VAL is required. Usage: make force VAL=YOUR_STEP"; exit 1;\
	fi
	@echo "Running migrate force $(VAL)"
	@migrate -path $(FOLDER) -database "$(CONN)" force $(VAL)

## Docker
unexport

.PHONY: docker_up docker_down docker_up_all docker_down_v

docker_up:
	docker compose up db migrate

docker_up_all:
	docker compose up

docker_down:
	docker compose down

docker_down_v:
	docker compose down -v

## Development
.PHONY: run_api run_cli run_jq

run_api:
	go run $(API_FOLDER) $(filter-out $@,$(MAKECMDGOALS))

run_cli:
	go run $(CLI_FOLDER) $(filter-out $@,$(MAKECMDGOALS))

run_jq:
	@if [ -z "$(VAL)" ]; then \
	    echo "need to specify filepath using VAL"; \
	    exit 1; \
	fi

	cat $(VAL) | jq -C . | sed 's/\\n/\n/g; s/\\t/\t/g'

# ---------------------------------------------------------------------------
# Local verification gate
#
# gofmt, vet, test, build — in that order, because a formatting failure hides
# everything after it.
#
# It needs no database, no secret file and no container: the API and the CLI
# are compiled, not run. The `-include` at the top of this file is what makes
# that true on a fresh clone.
#
# Two properties this target deliberately has:
#
#   - A missing toolchain FAILS the target. It does not skip. A gate that
#     cannot fail is worse than no gate: it reads green and proves nothing.
#   - After the run it names the packages that have no test files. `go test`
#     prints "[no test files]" for those and still exits 0, so a green run
#     would otherwise look like coverage that does not exist.
#
# This repository has no test file at all yet, so every package shows up in
# that list. That is the state of the suite, not a pass.
# ---------------------------------------------------------------------------
check: ## gofmt, vet, test, build — the local verification gate
	@if ! command -v go >/dev/null 2>&1; then \
		echo "ERROR: go is not installed — the check cannot run, so nothing is verified" >&2; \
		exit 1; \
	fi; \
	out="$$(gofmt -l .)"; \
	if [ -n "$$out" ]; then echo "unformatted files:"; echo "$$out"; exit 1; fi; \
	go vet ./...; \
	go test ./...; \
	go build ./...; \
	notested="$$(go list -f '{{if and (not .TestGoFiles) (not .XTestGoFiles)}}{{.ImportPath}}{{end}}' ./... | grep -v '^$$')"; \
	if [ -n "$$notested" ]; then \
		echo ""; \
		echo "NOTE: these packages have no test files — this run does not cover them:"; \
		echo "$$notested" | sed 's/^/  /'; \
	fi

# args
%:
	@:
