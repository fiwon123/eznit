include .env

export

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

WINDOWS_BIN := $(BUILD_DIR)/$(APP_NAME).exe
SHORT_WINDOWS_BIN := $(BUILD_DIR)/$(APP_NAME_SHORT).exe
WINDOWS_ZIP := $(BUILD_DIR)/$(APP_NAME)_$(VERSION)_windows.zip
WINDOWS_SCRIPT :=  $(SCRIPTS_DIR)/$(SCRIPT_NAME).bat

LINUX_BIN := $(BUILD_DIR)/$(APP_NAME)
SHORT_LINUX_BIN := $(BUILD_DIR)/$(APP_NAME_SHORT)
LINUX_TAR := $(BUILD_DIR)/$(APP_NAME)_$(VERSION)_linux.tar.gz
LINUX_SCRIPT :=  $(SCRIPTS_DIR)/$(SCRIPT_NAME).sh

## Builds
.PHONY: build_all
.PHONY: build_api_windows build_api_linux build_zip_linux build_zip_windows build_clean release
.PHONY: build_cli_windows build_cli_linux build_zip_linux build_zip_windows build_clean release

# Create build folder if missing
$(BUILD_DIR):
		mkdir -p $(BUILD_DIR)

build_all: release

build_api_windows: $(BUILD_DIR)
	    GOOS=windows GOARCH=amd64 go build -ldflags "-X $(INJECT_VERSION)=$(VERSION)" -o $(WINDOWS_BIN) $(API_FOLDER)
		cp $(WINDOWS_BIN) $(SHORT_WINDOWS_BIN)

build_api_linux: $(BUILD_DIR)
		GOOS=linux GOARCH=amd64 go build -ldflags "-X $(INJECT_VERSION)=$(VERSION)" -o $(LINUX_BIN) $(API_FOLDER)
		cp $(LINUX_BIN) $(SHORT_LINUX_BIN)

build_cli_windows: $(BUILD_DIR)
	    GOOS=windows GOARCH=amd64 go build -ldflags "-X $(INJECT_VERSION)=$(VERSION)" -o $(WINDOWS_BIN) $(CLI_FOLDER)
		cp $(WINDOWS_BIN) $(SHORT_WINDOWS_BIN)

build_cli_linux: $(BUILD_DIR)
		GOOS=linux GOARCH=amd64 go build -ldflags "-X $(INJECT_VERSION)=$(VERSION)" -o $(LINUX_BIN) $(CLI_FOLDER)
		cp $(LINUX_BIN) $(SHORT_LINUX_BIN)

build_zip_windows: windows
		zip -j $(WINDOWS_ZIP) $(WINDOWS_BIN) $(SHORT_WINDOWS_BIN) $(WINDOWS_SCRIPT) README.md LICENSE LICENSE-APACHE NOTICE

build_zip_linux: linux
		tar -czvf $(LINUX_TAR) \
		          -C $(BUILD_DIR) $(notdir $(LINUX_BIN)) $(notdir $(SHORT_LINUX_BIN)) \
		          -C ../$(SCRIPTS_DIR) $(notdir $(LINUX_SCRIPT)) ../README.md ../LICENSE ../LICENSE-APACHE ../NOTICE

build_release: zip_windows zip_linux
		@echo "Release ready: $(VERSION)"

build_clean:
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

migrate_up: VAL := 1
migrate_up:
	@echo "Running migrate up $(VAL)"
	migrate -path $(FOLDER) -database $(CONN) up $(VAL)

migrate_down: VAL := 1
migrate_down:
	@echo "Running migrate down $(VAL)"
	migrate -path $(FOLDER) -database $(CONN) down $(VAL)

migrate_force:
	@if [ -z "$(VAL)" ]; then\
	    echo "VAL is required. Usage: make force VAL=YOUR_STEP"; exit 1;\
	fi
	@echo "Running migrate force $(VAL)"
	migrate -path $(FOLDER) -database "$(CONN)" force $(VAL)
