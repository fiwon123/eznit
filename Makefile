include .env

export

CONN="postgresql://$(DB_USER):$(DB_PWD)@localhost:$(DB_PORT)/$(DB_NAME)?sslmode=disable"
FOLDER=./db/migrations

ifndef VAL
$(error ERROR: You must specify VAL variable)
endif

.PHONY: up down create force

create:
	migrate create -ext sql -dir $(FOLDER) -seq $(VAL)

up:
	migrate -path $(FOLDER) -database $(CONN) up $(VAL)

down:
	migrate -path $(FOLDER) -database $(CONN) down $(VAL)

force:
	migrate -path $(FOLDER) -database "$(CONN)" force $(VAL)
