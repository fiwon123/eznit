include .env

export

CONN:="postgresql://$(DB_USER):$(DB_PWD)@localhost:$(DB_PORT)/$(DB_NAME)?sslmode=disable"
FOLDER:=./db/migrations

## Builds
.PHONY:

## Migrations
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
