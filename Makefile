include .env .env.local
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

.PHONY: migrate_up migrate_down migrate_create migrate_force

migrate_create:
	@if [ -z "$(VAL)" ]; then\
	    echo "VAL is required. Usage: make create VAL=NEW_NAME"; exit 1;\
	fi
	@echo "Running migrate create $(VAL)"
	migrate create -ext sql -dir $(FOLDER) -seq $(VAL)

migrate_up:
	@if [ -z "$(VAL)" ]; then \
	    echo "Running just migrate up"; \
	    migrate -path "$(FOLDER)" -database "$(CONN)" up; \
	else \
	    echo "Running migrate up $(VAL)"; \
	    migrate -path "$(FOLDER)" -database "$(CONN)" up "$(VAL)"; \
	fi

migrate_down_all:
	@echo "Running just migrate down"
	migrate -path "$(FOLDER)" -database "$(CONN)" down

migrate_down: VAL=1
migrate_down:
	@echo "Running migrate down $(VAL)"
	migrate -path "$(FOLDER)" -database "$(CONN)" down "$(VAL)"

migrate_force:
	@if [ -z "$(VAL)" ]; then\
	    echo "VAL is required. Usage: make force VAL=YOUR_STEP"; exit 1;\
	fi
	@echo "Running migrate force $(VAL)"
	migrate -path $(FOLDER) -database "$(CONN)" force $(VAL)

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
.PHONY: run_api run_cli

run_api:
	go run $(API_FOLDER) $(filter-out $@,$(MAKECMDGOALS))

run_cli:
	go run $(CLI_FOLDER) $(filter-out $@,$(MAKECMDGOALS))

# args
%:
	@:
