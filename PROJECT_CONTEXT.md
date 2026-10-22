# Project Context

Fill this in first. Every other file in the agent layer reads from here, so an
empty field here means a placeholder somewhere else.

## Identity

- **Name**: eznit
- **One line**: Self-hosted file storage — a Go API, a CLI client and a web UI
  for uploading, listing and retrieving your own files, backed by PostgreSQL.
- **Repository**: https://github.com/fiwon123/eznit
- **License**: MIT (see `LICENSE`)
- **Status**: pre-1.0. The API, the CLI and the web UI work end to end
  against a local Compose stack. There are **no automated tests yet**, and
  several routes are known-insecure — see "Known gaps" in `AGENTS.md` before
  deploying this anywhere but a laptop.

## Stack

| Layer | Choice |
| --- | --- |
| Language | Go (version pinned in `go.mod`) |
| HTTP | `net/http` + `go-chi/chi` |
| Database | PostgreSQL, `sqlx` + `lib/pq` |
| Migrations | `golang-migrate/migrate` (SQL files in `db/migrations/`) |
| CLI | `alecthomas/kong` |
| Web UI | React + TypeScript + Vite, served by the API from `web/dist` |
| Logging | `go.uber.org/zap` via `log/slog`, rotating files with `lumberjack` |
| Config | `joho/godotenv` |
| Containers | Docker Compose — the dev sandbox (`dev`) and the local stack (`db`, `migrate`; `api` is kept as a commented block) |

## Architecture

```
cmd/api/         API entrypoint: flags, logger, router, graceful shutdown
cmd/cli/         CLI entrypoint and one file per command
web/             React + TypeScript + Vite web UI; built to web/dist and served by the API
internal/domain/ One package per resource: users, sessions, files
                 Each has handler -> service -> repository, plus dto.go
internal/platform/
                 middleware (session guard), sql (connection)
pkg/             Shared: logger, errors, http/json helpers, path helpers, types
db/migrations/   golang-migrate SQL, one up/down pair per change
secrets/         Docker secret files (the real one is gitignored)
```

The layering is strict and worth keeping: a handler decodes the request and
writes the response, a service holds the rules, a repository is the only place
that speaks SQL. A handler that calls the database directly, or a repository
that returns an HTTP status, is a layering break.

## Data model

- `users` — id, email, password (hashed), name, timestamps
- `files` — id, user_id, name, ext, content_type, path, version
- `files_history` — previous versions of a file
- `sessions` — id, user_id, token, expires_at, is_active

File bytes live on disk under the upload root, laid out as
`<upload-root>/<user-id>/<file-id>/<version>_<filename>`. The `files` row is the
index; the path column is what `download` opens.

## Commands

| Command | What it is for |
| --- | --- |
| `make check` | The local verification gate: gofmt, vet, test, build |
| `make run_api` | Run the API against the configured database |
| `make run_cli` | Run the CLI |
| `make build` | Build both binaries into `./build` |
| `make web_check` | Lint, format check, typecheck and build the web UI |
| `make web_build` | Build the web UI into `web/dist` |
| `make web_dev` | Run the Vite dev server |
| `make setup` | One-time: create `.env` and the database secret from their examples |
| `make docker_up` | Bring up db + the migrations |
| `make docker_up_all` | Bring up db + the migrations + the dev sandbox |
| `make dev-up` | Start the dev sandbox (pinned Go toolchain) with the stack |

## Environment

The API reads `DB_USER`, `DB_PASSWORD`, `DB_HOST`, `DB_PORT`, `DB_NAME`
(optional `UPLOAD_FOLDER`), and prefers `DB_PASSWORD_FILE` — a path to a file
holding the password — over the inline variable. Compose passes the Docker
secret `secrets/db_password.txt` through that variable.

`.env` and `.env.local` are for local development. Both are gitignored.
`.env.example` is tracked and holds placeholders only; it is the one file that
is safe to read.

## Security posture

This is a self-hosted tool and an unhardened one. Read "Known gaps" in
`AGENTS.md` before exposing it beyond localhost. The short version: some routes
do not require a session, files are stored unencrypted on disk, and there is no
rate limiting anywhere.
