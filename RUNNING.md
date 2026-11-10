# Running a release bundle

This folder is a **server bundle**: the API, the window, the migrations and the
Compose services that give it a database. It is not a source checkout — there is
no `go.mod`, no `web/` and no image to build.

## What works from here

The database and the migration job are published images plus the SQL in
`db/migrations/`, so they start with no source:

```sh
cp secrets/db_password.txt.example secrets/db_password.txt   # then edit it
cp .env.example .env                                         # then edit it
docker compose up db migrate
```

Then run the API on the host, pointed at the published port:

```sh
DB_HOST=localhost DB_PORT=5432 ./eznit-server
```

`eznit-server` embeds the web UI, so <http://localhost:4000> serves it.

## What does not work from here

`docker compose up` (the whole file) builds the `api` and `web` services from
source: `Dockerfile.api` copies `go.mod`, `web/` and `cmd/`, none of which this
folder carries. Those two services belong to the source checkout — use
`db migrate` plus the prebuilt binaries instead, as above.

The `dev` and `web-dev` services (the developer loop) are not part of a bundle.

## The desktop app

The native window is not in this bundle: a server has no display. It is built
from source today with `make build_gui` (cgo + GL/X11 headers) and will ship
with the CLI archive in #214. Use it on the machine you work from, pointed at
this server:

```sh
GUI_API_URL=http://<server>:4000 ./eznit-gui
```

## Migrating by hand

The `migrate` service is `migrate/migrate` with `db/migrations/` mounted. If you
prefer the CLI:

```sh
migrate -path db/migrations \
  -database "postgres://USER:PASSWORD@localhost:5432/DB?sslmode=disable" up
```

## What this bundle does not prove

A green build says the binaries were produced. It does not say the schema
matches the code that queries it — run `db migrate` and exercise the routes
before relying on a deploy.
