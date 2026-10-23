# The local environment files are optional. The leading `-` makes a missing
# file a non-error, so a fresh clone can run `make check`, `make clean` and
# `make build` without a secret file. The targets that genuinely need the
# variables check for themselves (see `require_db_vars` below).
#
# Not inside the sandbox. There the configuration is the container's: compose
# sets DB_HOST/DB_PORT/DB_USER/DB_NAME explicitly (the in-network address) and
# passes the rest through `env_file`, which strips the quotes `.env` is written
# with. Importing the file here *again* overrides all of that with the host
# values — and make keeps the quotes, so `DB_HOST="localhost"` reaches the API's
# DSN as a literal `"localhost"` and the sandbox cannot reach the database.
# DEV_SANDBOX is set by the `dev` service in docker-compose.yml.
ifeq ($(DEV_SANDBOX),1)
else
-include .env .env.local
endif
export

SERVER_NAME := server
CLI_NAME := cli

APP_NAME := eznit
APP_NAME_SHORT := ez
SCRIPT_NAME := add_to_path
INJECT_VERSION:= main.Version

# Host ports for the two entry points. `make up` (the product) and
# `make dev-up` (the developer loop) publish the same pair, so they are
# alternatives: `preflight-ports` is what turns "the other one is already
# running" into a message instead of a Docker bind error. Compose reads the
# same names, so an override here is the override everywhere.
HOST_PORT ?= 4000
WEB_PORT ?= 5173

API_FOLDER := ./cmd/api
CLI_FOLDER := ./cmd/cli
WEB_FOLDER := ./web

DOCKER_COMPOSE := ./docker-compose.yml
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

WINDOWS_SERVER_BIN := $(BUILD_API)/$(SERVER_NAME).exe
WINDOWS_SERVER_ZIP := $(BUILD_API)/$(APP_NAME)_$(SERVER_NAME)_$(VERSION)_windows.zip

## Builds
.PHONY: clean
.PHONY: build build_cli build_api build_gui verify
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

# The API and GUI binaries carry the built web UI (web/embed.go), so building
# either one builds the UI first. `make check` deliberately does not: the
# committed web/dist/.gitkeep keeps the embed directive valid without a build,
# which is what lets the Go gate run on a machine with no Node.
build_api: web_build verify
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags "-X $(INJECT_VERSION)=$(VERSION)" -o $(LINUX_SERVER_BIN) $(API_FOLDER)
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -ldflags "-X $(INJECT_VERSION)=$(VERSION)" -o $(WINDOWS_SERVER_BIN) $(API_FOLDER)

	chmod -R a+r $(BUILD_API)
	chmod a+x $(LINUX_SERVER_BIN)

build_gui: web_build verify
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -ldflags "-X $(INJECT_VERSION)=$(VERSION)" -o $(BUILD_DIR)/gui/eznit-gui.exe ./cmd/gui
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags "-X $(INJECT_VERSION)=$(VERSION)" -o $(BUILD_DIR)/gui/eznit-gui ./cmd/gui
	CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -ldflags "-X $(INJECT_VERSION)=$(VERSION)" -o $(BUILD_DIR)/gui/eznit-gui-darwin ./cmd/gui

	chmod -R a+r $(BUILD_DIR)/gui
	chmod a+x $(BUILD_DIR)/gui/eznit-gui

zip: build_cli build_api build_gui
	zip -j $(WINDOWS_ZIP) $(WINDOWS_BIN) $(SHORT_WINDOWS_BIN) $(WINDOWS_SCRIPT) README.md LICENSE
	zip -r $(WINDOWS_SERVER_ZIP) $(WINDOWS_SERVER_BIN) $(MIGRATIONS) $(DOCKER_COMPOSE) $(SCRIPT_MIGRATION) $(SECRETS_EXAMPLE) $(ENV_EXAMPLE) README.md LICENSE

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

.PHONY: docker_up docker_down docker_up_all docker_down_v up preflight-ports

# Which entry point is starting. Set by the target that depends on
# `preflight-ports`, and only read there.
MODE ?= dev

# `make up` and `make dev-up` publish the same host ports, so they are
# alternatives. This guard runs before either one starts anything, and names the
# command that clears the way — instead of starting half a stack and failing on
# `bind: address already in use`.
#
# A port held by *this* entry point's own containers is not a collision:
# `make dev-up` twice in a row must work.
preflight-ports:
	@services="$$(docker compose --profile dev ps --services --status running 2>/dev/null || true)"; \
	running() { printf '%s\n' "$$services" | grep -qx "$$1"; }; \
	probe() { bash -c "exec 3<>/dev/tcp/127.0.0.1/$$1" 2>/dev/null; }; \
	die() { echo "ERROR: $$1" >&2; shift; for line in "$$@"; do echo "       $$line" >&2; done; exit 1; }; \
	if [ "$(MODE)" = "dev" ]; then \
	    if running api || running web; then \
	        die "the product stack is running (api/web) and already holds the host ports." \
	            "make up and make dev-up are alternatives — stop the product first:" \
	            "    make docker_down"; \
	    fi; \
	    if ! running dev && probe "$(HOST_PORT)"; then \
	        die "host port $(HOST_PORT) is already in use by something that is not this stack." \
	            "Free it, or start the dev loop elsewhere: HOST_PORT=<port> make dev-up"; \
	    fi; \
	    if ! running web-dev && probe "$(WEB_PORT)"; then \
	        die "host port $(WEB_PORT) is already in use by something that is not this stack." \
	            "Free it, or start the dev loop elsewhere: WEB_PORT=<port> make dev-up"; \
	    fi; \
	else \
	    if running dev || running web-dev; then \
	        die "the dev loop is running (dev/web-dev) and already holds the host ports." \
	            "make up and make dev-up are alternatives — stop the dev loop first:" \
	            "    make dev-down"; \
	    fi; \
	fi

up: MODE = product
up: preflight-ports ## Build and run the product: API :4000, web UI :5173, db, migrations
	docker compose up --build db migrate api web

docker_up:
	docker compose up db migrate

docker_up_all:
	docker compose --profile dev up --build dev db migrate

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
#   - The first failing step STOPS the target. The recipe is one shell, so
#     `set -e` is what turns a failed `go vet`, `go test` or `go build` into a
#     non-zero exit; chaining the steps with `;` alone lets the shell return
#     the status of the *last* command, and the notested report at the end
#     always succeeds. That is #61.
#   - After the run it names the packages that have no test files. `go test`
#     prints "[no test files]" for those and still exits 0, so a green run
#     would otherwise look like coverage that does not exist.
#
# The suite is partial: packages without a test file show up in that list.
# Read a green run as "what has tests passed, the rest compiled", not as
# coverage of the whole repository.
# ---------------------------------------------------------------------------
check: ## gofmt, vet, test, build — the local verification gate
	@if ! command -v go >/dev/null 2>&1; then \
		echo "ERROR: go is not installed — the check cannot run, so nothing is verified" >&2; \
		exit 1; \
	fi; \
	set -e; \
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

# ---------------------------------------------------------------------------
# Dependency vulnerability scan
#
# Deliberately NOT part of `make check`: the scan downloads the tool and the
# vulnerability database, and the verification gate has to work offline on a
# fresh clone. CI runs the same command as a step of its own.
#
# Symbol-level (the default) fails only when this code actually reaches a
# vulnerable function. `-scan=module` cannot go green here: GO-2026-5932 on
# x/crypto's openpgp package is "Fixed in: N/A" and the package is not
# imported, while x/crypto itself cannot be dropped (it provides `bcrypt`).
# See the comment in .github/workflows/ci.go.yml.
#
# A missing toolchain FAILS the target, like every other gate in this file.
# ---------------------------------------------------------------------------
.PHONY: vuln
vuln: ## scan for reachable dependency vulnerabilities (needs network)
	@if ! command -v go >/dev/null 2>&1; then \
		echo "ERROR: go is not installed — the scan cannot run, so nothing is verified" >&2; \
		exit 1; \
	fi; \
	go run golang.org/x/vuln/cmd/govulncheck@v1.9.0 ./...

# ---------------------------------------------------------------------------
# The web UI (web/)
#
# A second toolchain, so a second gate. `make check` stays Go-only on purpose:
# the Go gate must not start failing because an npm package moved.
#
# The same rules as the Go gate:
#
#   - A missing toolchain FAILS the target. It does not skip. A gate that
#     cannot fail is worse than no gate: it reads green and proves nothing.
#   - Every step can fail on its own. `oxlint` exits non-zero on a lint error
#     and `oxfmt --check` exits non-zero on a misformatted file, so the gate
#     fails when the code is wrong and not only when it is missing.
#
# What this gate does NOT prove: the web has no test suite, so nothing here
# checks what the pages do in a browser. It proves that the code lints,
# formats, typechecks and builds.
# ---------------------------------------------------------------------------
.PHONY: web_install web_check web_build web_dev

web_install: ## Install the web dependencies from the lockfile (npm ci)
	@if ! command -v npm >/dev/null 2>&1; then \
		echo "ERROR: npm is not installed — mise install puts the pinned Node on PATH (see mise.toml)" >&2; \
		exit 1; \
	fi; \
	(cd $(WEB_FOLDER) && npm ci)

web_check: ## lint, format check, typecheck and build the web UI — the web gate
	@if ! command -v node >/dev/null 2>&1; then \
		echo "ERROR: node is not installed — the check cannot run, so nothing is verified" >&2; \
		echo "       mise install puts the pinned version on PATH (see mise.toml)" >&2; \
		exit 1; \
	fi; \
	if [ ! -d "$(WEB_FOLDER)/node_modules" ]; then \
		echo "[web] node_modules is missing — installing the pinned dependencies..."; \
		(cd $(WEB_FOLDER) && npm ci); \
	fi; \
	cd $(WEB_FOLDER) && \
		npm run lint && \
		npm run format:check && \
		npm run typecheck && \
		npm run build

web_build: ## Build the web UI into web/dist
	@if ! command -v node >/dev/null 2>&1; then \
		echo "ERROR: node is not installed — mise install puts the pinned Node on PATH (see mise.toml)" >&2; \
		exit 1; \
	fi; \
	if [ ! -d "$(WEB_FOLDER)/node_modules" ]; then \
		echo "[web] node_modules is missing — installing the pinned dependencies..."; \
		(cd $(WEB_FOLDER) && npm ci); \
	fi; \
	cd $(WEB_FOLDER) && npm run build

web_dev: ## Run the web UI dev server on http://localhost:5173
	@if ! command -v node >/dev/null 2>&1; then \
		echo "ERROR: node is not installed — mise install puts the pinned Node on PATH (see mise.toml)" >&2; \
		exit 1; \
	fi; \
	cd $(WEB_FOLDER) && npm run dev

# ---------------------------------------------------------------------------
# Dev sandbox (multiplatform)
#
# A pinned toolchain and a shell that behave the same on every machine. The
# sandbox is optional: the project still builds and tests with a plain host Go
# toolchain (see the `check` target above).
#
# Cross-platform note: this Makefile targets POSIX hosts — Linux, macOS and
# WSL2 — where `make` runs recipes under a POSIX shell. Native Windows users
# should use `python scripts/dev-up.py dev-up` (or `scripts/dev-up.ps1`), which
# provides the same targets without a POSIX shell. Everyone can run the product
# with a plain `docker compose up`, which needs no Make at all, and the sandbox
# with `docker compose up dev db migrate`.
# ---------------------------------------------------------------------------

# Extra flags for the opencode invocation. `make opencode-auto` /
# `make dev-opencode-auto` set this to --auto, which auto-approves permissions
# that are not explicitly denied; the secret-file deny rules still apply.
OPENCODE_ARGS ?=

# --- Host identity -----------------------------------------------------------
# Run bind-mounted dev services as the host developer instead of root. Derived
# from the workspace OWNER, not the current process, so invoking make inside an
# already-running bind-mounted container still sends the host identity.
# GNU stat (-c) first, then BSD/macOS stat (-f), then plain `id`. The `id`
# fallback is what makes this work everywhere `make` does.
HOST_UID ?= $(shell stat -c '%u' . 2>/dev/null || stat -f '%u' . 2>/dev/null || id -u)
HOST_GID ?= $(shell stat -c '%g' . 2>/dev/null || stat -f '%g' . 2>/dev/null || id -g)
HOST_HOME ?= $(HOME)
# Exported by name. A bare `unexport` cancels the *default* export of the
# variables defined after it, but it cannot cancel an export that names the
# variable — so naming them here is what keeps the host identity reaching
# compose, whatever the surrounding sections do with export attributes.
# Without a named export the sandbox would start as UID/GID 1000 on a Linux
# host whose uid differs, and the bind mount would end up root-owned.
export HOST_UID HOST_GID HOST_HOME HOST_PORT WEB_PORT

# --- Optional in-container agent ---------------------------------------------
# Starting the sandbox never requires opencode. The agent layer is optional so
# `make dev-up` works on macOS and Windows too, where the host binary cannot
# execute inside the Linux container.
#
# NOTE: not named OPENCODE — the opencode runtime exports an OPENCODE env var
# (=1), which would override a `?=` default through make's env import.
OPENCODE_BIN ?= $(HOME)/.opencode/bin/opencode

# The dev container is Linux, so only an ELF binary can execute inside it. A
# macOS Mach-O or Windows PE install is detected here and skipped, instead of
# being mounted and failing later with "exec format error".
OPENCODE_MAGIC := $(shell head -c 4 "$(OPENCODE_BIN)" 2>/dev/null | od -An -tx1 2>/dev/null | tr -d ' \n')
AGENT_AVAILABLE := $(if $(filter 7f454c46,$(OPENCODE_MAGIC)),yes,no)

# --profile dev is explicit on purpose. The dev targets below name services
# anyway (which enables the profile), but `exec`/`ps` act on an already-running
# container and are easier to reason about when the profile is plainly on.
ifeq ($(AGENT_AVAILABLE),yes)
COMPOSE_UP := docker compose --profile dev -f docker-compose.yml -f docker-compose.agent.yml
else
COMPOSE_UP := docker compose --profile dev
endif

# The marker file is the dev entrypoint's opt-in (scripts/dev-entrypoint.sh):
# with it present the sandbox runs `make run_api` instead of sleeping. It is
# gitignored per-checkout state, created by `dev-up` and removed by `dev-down`.
DEV_START_MARKER := .dev/sandbox-start

help: ## List the available targets
	@grep -E '^[a-zA-Z_-]+:.*## ' $(firstword $(MAKEFILE_LIST)) | sort | awk 'BEGIN{FS=":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'
	@printf '\n  \033[36m%-30s\033[0m %s\n' "docker compose up" "run the product with no host prerequisites (any OS)"
	@printf '  \033[36m%-30s\033[0m %s\n' "docker compose up dev web-dev db migrate" "the dev loop without Make (needs .dev/sandbox-start)"
	@printf '  \033[36m%-30s\033[0m %s\n' "scripts/dev-up.py" "native Windows: python scripts/dev-up.py dev-up"
	@printf '\n  %s\n' "dev-up is the dev loop: UI :5173 (Vite, hot reload) and API :4000, both"
	@printf '  %s\n' "from the source. make up is the product: same ports, built images."

# --- One-time local setup ---------------------------------------------------
# Creates the two local files the stack needs, from the committed examples:
# the configuration (.env) and the database secret (secrets/db_password.txt).
# Both are gitignored, and both must be mode 600 (README.md -> Local secret
# files). Idempotent: an existing file is never overwritten.
setup: ## Create .env and the database secret from their examples (idempotent)
	@if [ -f "$(ENV_EXAMPLE)" ]; then \
	  if [ -f ".env" ]; then \
	    echo "[setup] .env already exists — leaving it alone."; \
	  else \
	    cp "$(ENV_EXAMPLE)" .env && chmod 600 .env && echo "[setup] created .env from $(ENV_EXAMPLE)"; \
	  fi; \
	fi
	@if [ -f "$(SECRETS_EXAMPLE)" ]; then \
	  if [ -f "secrets/db_password.txt" ]; then \
	    echo "[setup] secrets/db_password.txt already exists — leaving it alone."; \
	  else \
	    cp "$(SECRETS_EXAMPLE)" secrets/db_password.txt && chmod 600 secrets/db_password.txt && echo "[setup] created secrets/db_password.txt"; \
	  fi; \
	  echo "[setup] the secret is the example's placeholder password — replace it for anything but localhost."; \
	fi

# --- Preflight ---------------------------------------------------------------
# Refuses to start as root, and pre-creates the read-only mount sources the
# agent overlay needs. Missing mount sources are fatal on Windows and create
# root-owned directories on Linux, so they are created here rather than assumed.
# This target NEVER fails on a missing or non-Linux agent binary: that is what
# `require-agent` is for, and it is depended on only by the agent targets.
preflight: check-identity prepare-agent-mounts
	@if [ "$(AGENT_AVAILABLE)" = "yes" ]; then \
	  echo "[sandbox] in-container agent available — merging docker-compose.agent.yml."; \
	else \
	  echo "[sandbox] in-container agent unavailable (no Linux binary at $(OPENCODE_BIN))." >&2; \
	  echo "[sandbox]   Starting the sandbox without it." >&2; \
	fi

check-identity:
	@test "$(HOST_UID)" != "0" || { echo "ERROR: refusing to run the dev sandbox as UID 0; repair workspace ownership or set HOST_UID to the host user." >&2; exit 1; }

prepare-agent-mounts:
	@if [ "$(AGENT_AVAILABLE)" = "yes" ]; then \
	  mkdir -p "$(HOST_HOME)/.config/opencode" "$(HOST_HOME)/.config/gh" && touch "$(HOST_HOME)/.gitconfig"; fi

# Only the agent-specific targets need the in-container binary.
require-agent:
	@test "$(AGENT_AVAILABLE)" = "yes" || { \
	  echo "ERROR: the in-container AI agent needs a Linux opencode binary at $(OPENCODE_BIN)." >&2; \
	  echo "  A macOS/Windows binary cannot execute inside the Linux dev container." >&2; \
	  echo "  Recommended: use WSL2, where the host binary is Linux." >&2; \
	  echo "  Alternative: run 'opencode' on the host against the running stack." >&2; \
	  exit 1; }

# The host owns the opencode binary; the dev sandbox mounts it read-only
# (`~/.opencode/bin/opencode -> /usr/local/bin/opencode:ro`), so it can only be
# upgraded here, on the host. A *file* bind mount pins the inode, so a running
# sandbox keeps the old binary until it restarts — `opencode-upgrade` restarts
# it for you. Override the install URL if your installer differs.
OPENCODE_INSTALL_URL ?= https://opencode.ai/install

opencode-version: ## Print the host opencode version
	@command -v opencode >/dev/null 2>&1 || { echo "ERROR: opencode not on PATH — install it from $(OPENCODE_INSTALL_URL)" >&2; exit 1; }
	@opencode --version

opencode-upgrade: ## Upgrade host opencode and refresh the dev sandbox
	@command -v opencode >/dev/null 2>&1 || { echo "ERROR: opencode not on PATH — install it from $(OPENCODE_INSTALL_URL)" >&2; exit 1; }
	@echo "opencode before: $$(opencode --version 2>/dev/null || echo unknown)"
	@opencode upgrade </dev/null || { echo "opencode upgrade unavailable — falling back to the install script"; curl -fsSL $(OPENCODE_INSTALL_URL) | bash; }
	@echo "opencode after:  $$(opencode --version 2>/dev/null || echo unknown)"
	@if [ -n "$$(docker compose ps -q dev 2>/dev/null)" ]; then \
	  echo "restarting the dev sandbox so the bind-mounted binary refreshes..."; \
	  docker compose restart dev; \
	else \
	  echo "dev sandbox not running — nothing to refresh"; \
	fi

# --- Sandbox lifecycle -------------------------------------------------------
# HOST_UID/HOST_GID need no explicit export here: docker-compose.yml defaults
# them, and the exported values above supply the host's real identity when make
# is running on the host. Only the GitHub token is resolved at up-time.
.PHONY: dev-up dev-down dev-log dev-build dev-reset

# dev-up is the developer loop, all of it: the API runs inside the sandbox
# against the source, and the UI is the Vite server in `web-dev`, with hot
# reload. Both ports answer when this returns — which is the whole point of the
# target, and what it did not do before.
#
# Two steps, not one, because of the marker file. The entrypoint reads it when
# the container *starts*; a plain `up` leaves an already-running sandbox
# asleep, so `dev` is recreated to make the entrypoint run again.
dev-up: MODE = dev
dev-up: preflight preflight-ports ## Start the dev loop: UI :5173 (Vite, live reload), API :4000
	@printf '\n  dev-up starts the developer loop in containers:\n'
	@printf '    %-14s %s\n' "UI (Vite)" "http://localhost:$(WEB_PORT)   hot reload"
	@printf '    %-14s %s\n' "API" "http://localhost:$(HOST_PORT)"
	@printf '  PostgreSQL and the migrations start first; the UI proxies /v1 to the API.\n'
	@printf '  Ctrl-C stops the log stream — the containers keep running. "make dev-down" stops them.\n\n'
	@mkdir -p .dev && touch $(DEV_START_MARKER)
	@GH_TOKEN=$$(scripts/resolve-gh-token.sh 2>/dev/null) GITHUB_TOKEN=$$(scripts/resolve-gh-token.sh 2>/dev/null) HOST_PORT=$(HOST_PORT) WEB_PORT=$(WEB_PORT) $(COMPOSE_UP) up -d --build --force-recreate dev
	@HOST_PORT=$(HOST_PORT) WEB_PORT=$(WEB_PORT) $(COMPOSE_UP) up -d web-dev
	@$(COMPOSE_UP) logs -f dev web-dev

dev-down: ## Stop the dev loop and the local stack (keeps volumes)
	@rm -f $(DEV_START_MARKER)
	@$(COMPOSE_UP) down

dev-log: ## Tail dev sandbox logs
	@$(COMPOSE_UP) logs -f dev

dev-build: check-identity ## Rebuild the dev image after Dockerfile/dependency changes
	@docker compose build dev

# Deliberately NOT called `reset`: `down --volumes` removes the named volumes,
# and one of them is the PostgreSQL data volume. This wipes the local database
# along with the build caches, so the name says so.
dev-reset: ## Stop everything and wipe the volumes — INCLUDING the local database
	@echo "This removes the local database volume as well as the build caches." >&2
	@rm -f $(DEV_START_MARKER)
	@$(COMPOSE_UP) down --volumes

shell: dev-shell
opencode: dev-opencode
opencode-auto: dev-opencode-auto

dev-shell: preflight ## Open a shell inside the dev sandbox
	@$(COMPOSE_UP) exec dev bash || $(COMPOSE_UP) run --rm dev bash

dev-opencode: preflight require-agent ## Run the AI coding agent inside the dev sandbox
	@if [ -z "$$(docker compose ps -q dev 2>/dev/null)" ]; then echo "[opencode] starting dev stack..."; GH_TOKEN=$$(scripts/resolve-gh-token.sh 2>/dev/null) GITHUB_TOKEN=$$(scripts/resolve-gh-token.sh 2>/dev/null) $(COMPOSE_UP) up -d dev; fi
	@if [ -t 0 ]; then $(COMPOSE_UP) exec -it dev zsh -lc "cd /workspace && opencode $(OPENCODE_ARGS)" || $(COMPOSE_UP) exec -it dev bash -lc "opencode $(OPENCODE_ARGS)"; else $(COMPOSE_UP) exec -T dev zsh -lc "cd /workspace && opencode $(OPENCODE_ARGS)" || $(COMPOSE_UP) exec -T dev bash -lc "opencode $(OPENCODE_ARGS)"; fi

# Same recipe, with --auto: auto-approve permissions that are not explicitly
# denied (every secret-file deny rule still applies).
dev-opencode-auto: OPENCODE_ARGS = --auto
dev-opencode-auto: dev-opencode

# args
%:
	@:
