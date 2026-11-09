# Development

How to run eznit from source, the sandbox that removes the host-toolchain
requirement, and the gates a change must pass.

## The gates

```sh
make check     # gofmt, vet, test, build — the local verification gate
make vuln      # reachable-dependency vulnerability scan (needs network)
make web_check # oxlint, oxfmt, tsc --noEmit, vite build — the web gate
```

`make check` needs no database, no container and no secret file: it compiles every
Go module (`api`, `cli`, `gui`, `winres`), it does not run them. That is deliberate — the gate has to work on
a fresh clone. It fails loudly if the Go toolchain is missing rather than skipping,
and it lists the packages with no test files, because `go test` prints
`[no test files]` and still exits 0. **A green `make check` means "the packages
with tests passed", not coverage** — name the packages when you report a result.

`make web_check` needs Node (the `mise.toml` pin) and does not run the product.

Both feature gates run locally: CI runs on release pull requests (`dev` → `main`)
and on manual dispatch only.

## Make targets

| Target | What it does |
| --- | --- |
| `make setup` | create `.env` and the database secret from their examples (idempotent) |
| `make build` | the local binaries into `./build` (and the UI they embed) |
| `make release` | one-folder archives for every platform into `./build/dist` |
| `make run_api` | run the API against the configured database |
| `make run_cli` | run the CLI |
| `make migrate_up` / `migrate_down` / `migrate_down_all` | apply or roll back migrations |
| `make up` | build and run the product: UI `:5173`, API `:4000`, db, migrations |
| `make dev-up` / `dev-down` / `dev-restart` | the source dev loop |
| `make web_install` / `web_build` / `web_dev` / `web_check` | the web UI |
| `make screenshots` | capture the README screenshots (needs the app running) |

`make build_api` and `make build_gui` build the web UI first, because both binaries
carry a copy of it. `make check` stays Go-only; the web has its own gate.

## The dev sandbox

The commands above need a Go toolchain on the host. The sandbox removes that
requirement: a container with Go **1.27.3** — the version in `go.mod`, the same one
CI uses — with the workspace bind-mounted, so the files you edit are the same
files. The frontend has the same arrangement in `web-dev`: the Node from
`mise.toml`, running Vite against the same bind mount.

```sh
make dev-up     # the loop: UI :5173 (Vite, hot reload) + API :4000
make dev-shell  # a shell inside the sandbox
make dev-down   # stop it (keeps the volumes)
```

`make dev-up` starts four things: PostgreSQL, the migrations, the sandbox with
`make run_api` in it, and the Vite dev server. Both HTTP ports answer when it
returns, and an edit on the host is visible in the other one immediately — the Go
process recompiles on its next start (there is no file watcher: restart the API
after a Go change), and Vite reloads the browser on a web change.

`dev-up` does not rebuild an image, so a start is a start. Compose still builds the
images the first time they are needed. An edit to a `Dockerfile`,
`web/Dockerfile.dev`, or a script one of them `COPY`s is the one change the loop
cannot pick up by itself — `make dev-build` rebuilds both images, and
`make dev-restart` is `dev-down` followed by `dev-up`.

| Role | Where | Port |
| --- | --- | --- |
| Frontend (the app) | `web-dev` service, Vite | **5173** |
| Backend (the API) | `dev` service, `make run_api` | **4000** |

| What you want | Commands | Ports |
| --- | --- | --- |
| the product, in containers | `make up` | UI `:5173`, API `:4000` |
| the dev loop, against the source | `make dev-up` | UI `:5173`, API `:4000` |
| the same loop with host toolchains | `make run_api` + `make web_dev` | API `:4000`, UI `:5173` |

`Ctrl-C` stops the log stream and leaves the loop running — `make dev-down` stops
it. The marker file `.dev/sandbox-start` (gitignored) is how the entrypoint knows
to start the API rather than sleep; `dev-up` creates it and `dev-down` removes it,
so a bare `docker compose up dev db migrate` is still a quiet sandbox with a shell.

Inside the sandbox the configuration comes from the container, not from `.env`:
compose sets `DB_HOST=db` and `DB_PORT=5432`, and the Makefile deliberately does
not import the host's `.env` there. The database password stays a file secret the
container reads from `/run/secrets/db_password`.

Everything also works without Make:

```sh
mkdir -p .dev && touch .dev/sandbox-start   # makes the sandbox run the API
docker compose up dev web-dev db migrate     # the whole loop
python scripts/dev-up.py dev-up              # native Windows: same thing
```

| Platform | How to run the dev flow |
| --- | --- |
| Linux, macOS | `make dev-up` |
| WSL2 | `make dev-up` (the host is Linux) |
| Windows (native) | `python scripts/dev-up.py dev-up`, or the compose command above |

`make dev-opencode` runs the AI coding agent inside the sandbox, with the host's
`~/.config/gh`, `~/.config/opencode` and `~/.gitconfig` mounted read-only. It needs
a **Linux** opencode binary: on macOS and native Windows the target refuses and the
agent stays on the host (use WSL2). The container has no Docker socket — the agent
inside it cannot manage containers, on purpose.

## Migrations

`db/migrations/` is append-only history: a numbered `.up.sql`/`.down.sql` pair per
change, applied by `golang-migrate`. **Never edit a migration that has run
somewhere** — add the next pair instead. Constraints belong in the schema where
they can be expressed (`UNIQUE`, `FOREIGN KEY`), not only in Go, because a
constraint in Postgres cannot be forgotten by a new code path.

```sh
make migrate_up
make migrate_down      # one step
make migrate_down_all
```

The migration targets read `DB_USER`, `DB_HOST` and `DB_NAME` from `.env` or
`.env.local`; if they are missing, they say so instead of failing with an empty
connection string.

## Checking your change

1. `make check` — the gate above, and `make web_check` for the UI.
2. If you touched the API, run the stack and exercise the route: the gate
   compiles, it does not call anything.
3. If the change is user-visible in the CLI, read the output — a CLI has no
   screenshot, and its output is the artifact.

## Where a change belongs

[ARCHITECTURE.md](https://github.com/fiwon123/eznit/blob/dev/ARCHITECTURE.md) has
the full map. The short version:

```
cmd/             the entrypoints: api, cli, gui
internal/domain/ one package per resource: users, sessions, files, templates
                 handler -> service -> repository, plus dto.go
internal/platform/ middleware (session guard), sql (connection), webui (SPA), openapi
pkg/             logger, errors, http/json helpers, path helpers, types
db/migrations/   golang-migrate SQL, one up/down pair per change
web/             React + TypeScript UI; embeds into the api/gui binaries
```

The layering is strict: a handler decodes and writes, a service holds the rules, a
repository is the only place that speaks SQL.
