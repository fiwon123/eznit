#!/bin/sh
MIGRATE_DB_PASSWORD="$(cat /run/secrets/db_password)"

exec /migrate -path ./migrations -database "postgres://${DB_USER}:${MIGRATE_DB_PASSWORD}@db:${DB_PORT}/${DB_NAME}?sslmode=disable" up
