include .env

export

SERVER_NAME := server

APP_NAME := eznit
APP_NAME_SHORT := ez
BUILD_DIR := build
SCRIPTS_DIR := scripts
SCRIPT_NAME := add_to_path
INJECT_VERSION:= main.Version

API_FOLDER := ./cmd/api
CLI_FOLDER := ./cmd/cli/

# Detect last tag and increment patch
VERSION := $(shell git describe --tags --always)

# WINDOWS
WINDOWS_BUILD:=$(BUILD_DIR)/windows
WINDOWS_CLI:=$(WINDOWS_BUILD)/cli

WINDOWS_BIN := $(WINDOWS_CLI)/$(APP_NAME).exe
SHORT_WINDOWS_BIN := $(WINDOWS_CLI)/$(APP_NAME_SHORT).exe
WINDOWS_ZIP := $(WINDOWS_CLI)/$(APP_NAME)_$(VERSION)_windows.zip

WINDOWS_SERVER:=$(WINDOWS_BUILD)/api
WINDOWS_SERVER_BIN := $(WINDOWS_SERVER)/$(SERVER_NAME).exe
WINDOWS_SERVER_ZIP := $(WINDOWS_SERVER)/$(SERVER_NAME)_$(VERSION)_windows.zip

WINDOWS_SCRIPT :=  $(SCRIPTS_DIR)/$(SCRIPT_NAME).bat

# LINUX
LINUX_BUILD:=$(BUILD_DIR)/linux
LINUX_CLI:=$(LINUX_BUILD)/cli

LINUX_BIN := $(LINUX_CLI)/$(APP_NAME)
SHORT_LINUX_BIN := $(LINUX_CLI)/$(APP_NAME_SHORT)
LINUX_TAR := $(LINUX_CLI)/$(APP_NAME)_$(VERSION)_linux.tar.gz

LINUX_SERVER:=$(LINUX_BUILD)/api
LINUX_SERVER_BIN := $(LINUX_SERVER)/$(SERVER_NAME)
LINUX_SERVER_TAR := $(LINUX_SERVER)/$(SERVER_NAME)_$(VERSION)_linux.tar.gz

LINUX_SCRIPT :=  $(SCRIPTS_DIR)/$(SCRIPT_NAME).sh

## Builds
.PHONY: clean
.PHONY: build_all build_windows build_linux
.PHONY: zip_linux zip_windows

# Create build folder if missing
$(BUILD_DIR):
		mkdir -p $(BUILD_DIR)

build_all: zip_windows zip_linux
	@echo "Release ready: $(VERSION)"

build_linux: $(BUILD_DIR)
	GOOS=linux GOARCH=amd64 go build -ldflags "-X $(INJECT_VERSION)=$(VERSION)" -o $(LINUX_SERVER_BIN) $(API_FOLDER)

	GOOS=linux GOARCH=amd64 go build -ldflags "-X $(INJECT_VERSION)=$(VERSION)" -o $(LINUX_BIN) $(CLI_FOLDER)
	cp $(LINUX_BIN) $(SHORT_LINUX_BIN)

build_windows: $(BUILD_DIR)
	GOOS=windows GOARCH=amd64 go build -ldflags "-X $(INJECT_VERSION)=$(VERSION)" -o $(WINDOWS_SERVER_BIN) $(API_FOLDER)

	GOOS=windows GOARCH=amd64 go build -ldflags "-X $(INJECT_VERSION)=$(VERSION)" -o $(WINDOWS_BIN) $(CLI_FOLDER)
	cp $(WINDOWS_BIN) $(SHORT_WINDOWS_BIN)

zip_windows: build_windows
	zip -j $(WINDOWS_ZIP) $(WINDOWS_BIN) $(SHORT_WINDOWS_BIN) $(WINDOWS_SCRIPT) README.md LICENSE
	zip -j $(WINDOWS_SERVER_ZIP) $(WINDOWS_SERVER_BIN) README.md LICENSE

zip_linux: build_linux
	tar -czvf $(LINUX_TAR) \
	          -C $(LINUX_CLI) $(notdir $(LINUX_BIN)) $(notdir $(SHORT_LINUX_BIN)) \
	          -C ../../../$(SCRIPTS_DIR) $(notdir $(LINUX_SCRIPT)) ../README.md ../LICENSE

	tar -czvf $(LINUX_SERVER_TAR) \
	          -C $(LINUX_SERVER) $(notdir $(LINUX_SERVER_BIN)) \
	          -C ../../../ ./README.md ./LICENSE

clean:
	rm -rf $(BUILD_DIR)

## Migrations
CONN := "postgresql://$(DB_USER):$(DB_PWD)@localhost:$(DB_PORT)/$(DB_NAME)?sslmode=disable"
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
