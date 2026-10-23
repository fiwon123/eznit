# Architecture

eznit is a self-hosted file store: one PostgreSQL database, one HTTP API, a CLI
that speaks to it, and a web UI the API serves. This document is the map — what
runs where, how a request travels, and where a change belongs. For the commands
that drive it, see [README.md](README.md).

## The pieces

| Piece | Entry point | What it is | Listens |
| --- | --- | --- | --- |
| API | `cmd/api` | The HTTP API, and the web UI in front of it | `:4000` |
| CLI | `cmd/cli` → `internal/cli` | `kong` command line client, one file per command | — |
| GUI | `cmd/gui` | Opens a browser on the bundled UI, with the API in-process or proxied | `:8765` |
| Web UI | `web/src` | React + TypeScript + Vite, built to `web/dist` | — |

The API and the GUI both carry the built UI inside the binary (`web/embed.go`);
the API also serves a `web/dist` folder when one exists. That is why one origin
serves both the pages and `/v1`, and why the API needs no CORS headers.

The API's router is built by `internal/platform/api`, and both binaries use it:
the API serves it directly, and the GUI either mounts it on its own listener
(when the database is configured) or proxies `/v1` to `GUI_API_URL`. One router,
so the two cannot offer different APIs.

## Repository map

```
cmd/api/            API entrypoint: flags, logging, database, router, UI, shutdown
cmd/cli/            CLI entrypoint: the version variable, then internal/cli
internal/cli/       CLI commands, the API client and the token store
cmd/gui/            GUI entrypoint: the UI, the API (or a proxy), a browser
internal/domain/    one package per resource — users, sessions, files, templates
                    handler.go -> service.go -> repository.go, plus dto.go
internal/platform/  cross-domain infrastructure
                    api/         the /v1 router, built once for both binaries
                    middleware/  session guard (Authorization: Bearer <token>)
                    sql/         database connection and the DSN from the environment
                    webui/       the SPA handler both binaries serve the UI with
pkg/                shared code with no project rules in it
                    errors/   AppError: a message and an HTTP status
                    helper/   JSON envelopes, path safety, token generation
                    logger/   zap through log/slog, rotating files (lumberjack)
                    types/    Envelope[T], the response wrapper
web/                the React UI; embed.go embeds the build into the binaries
db/migrations/      golang-migrate SQL, one .up/.down pair per change
scripts/            migrations runner, dev sandbox helpers, path setup
Dockerfile          the dev sandbox image (Go 1.24.1, gh, zip)
docker-compose.yml  db, migrate, and the optional dev sandbox (api is commented)
```

`internal/` is where the project rules live; `pkg/` is reusable and knows
nothing about files, users or templates. The split is enforced by review, not by
the compiler — an import of `internal/domain/files` from `pkg/` is a smell.

## The request path

A CLI command, end to end:

1. **`cmd/cli`** builds an HTTP request from `CLI_API_HOST` and `CLI_API_PORT`
   (default `http://localhost:4000`) and attaches the session token from
   `~/.eznit/config.json` as `Authorization: Bearer <token>`.
2. **`cmd/api`** routes it: `chi` in `main` sends `/v1/...` to the router the
   domain registered with `RegisterRoutes`.
3. **`internal/platform/middleware`** runs first on a guarded route: it asks the
   sessions service whether the token is valid, and puts the caller's user id in
   the request context. No token, no handler.
4. **`internal/domain/*/handler.go`** decodes the request, checks its shape, and
   calls the service. It writes the answer as `types.Envelope[T]`.
5. **`internal/domain/*/service.go`** holds the rules: who may read what, how a
   version number advances, where a path is allowed to point. This is the layer
   a test can pin down without a database.
6. **`internal/domain/*/repository.go`** is the only place that speaks SQL
   (`sqlx`). Its interface is what the service is tested against; its `_sql.go`
   implementation is exercised against a real PostgreSQL.
7. **PostgreSQL** holds the index; **the disk** holds the bytes.

### Where the bytes live

A file's content is not in the database. It is written under the upload root
(`API_UPLOADS`, `./uploads` by default) as:

```
<upload-root>/<user-id>/<file-id>/<version>_<filename>
```

The `files` row is the index of that path — `name`, `ext`, `content_type` and
`version` are what the API serves, the path is what a download opens. Every
version writes a new file into the file's own directory, the `files` row moves
to it, and `files_history` keeps a row for each version written. A superseded
version's bytes stay on disk because that row points at them; deleting the
file removes the whole directory.

Template contents are the exception: they live in `TEXT` columns in PostgreSQL,
not on disk.

## Data model

| Table | Holds |
| --- | --- |
| `users` | id, email, password hash, created_at, updated_at |
| `files` | id, user_id, name, ext, content_type, path, version, timestamps |
| `files_history` | file_id, path, version — one row per stored version: upload writes the first, an update adds the next, and earlier rows are never rewritten |
| `sessions` | token (primary key), user_id, is_active, created_at, expires_at |
| `templates` | id, user_id, name, slug, description, kind, version, is_public |
| `template_files` | template_id, path, content, is_executable — unique per (template, path) |
| `template_variables` | template_id, var_key, prompt, default_value, required, sort_order |

### Migrations

`db/migrations/` is append-only history: a numbered `.up.sql`/`.down.sql` pair
per change, applied by `golang-migrate`. Never edit a migration that has run
somewhere — add the next pair instead. Constraints belong in the schema where
they can be expressed (`UNIQUE`, `FOREIGN KEY`), not only in Go, because a
constraint in Postgres cannot be forgotten by a new code path.

## The web UI

`web/src` is a single-page app: the browser owns the routes, so `/templates/abc`
has no file of its own and must answer with `index.html`. `webui.Handler` does
that: anything under `/v1` goes to the API untouched, a path that names a file
is served as itself, and every other path gets `index.html`.

The build reaches the process two ways:

- **from disk** — a built `web/dist`, preferred by the API. Rebuilding the UI is
  enough; the server does not need a restart.
- **from the binary** — embedded at compile time, preferred by the GUI. This is
  what makes a released binary self-contained.

A `web/dist` holding only the committed placeholder has no UI in it: `HasIndex`
asks for `index.html`, so the API falls back to the embedded copy, or to serving
the API alone. See [README.md](README.md#web-ui) for the authoring commands.

## Configuration and secrets

The API reads its environment through `godotenv` (`.env.local` overrides `.env`):

| Variable | Meaning |
| --- | --- |
| `DB_HOST`, `DB_PORT`, `DB_USER`, `DB_NAME` | PostgreSQL connection |
| `DB_PASSWORD_FILE` | path to a file holding the password — **preferred** |
| `DB_PWD` | the password inline, for a throwaway local database |
| `API_UPLOADS` | the upload root |
| `API_LOGS` | where the API writes logs (`./logs` by default) |
| `API_WEB_DIST` | where to find a built UI (`web/dist` by default) |

The password is a Docker secret in the local stack: `secrets/db_password.txt` is
mounted at `/run/secrets/db_password`, and `DB_PASSWORD_FILE` points at it. The
real file is gitignored; `secrets/db_password.txt.example` holds a placeholder.
`.env` is gitignored for the same reason. Neither is ever committed, logged, or
printed — the logger writes a token fingerprint, never a token.

The CLI reads `CLI_API_HOST`, `CLI_API_PORT`, `CLI_DOWNLOADS` and `CLI_LOGS`,
and stores its session in `~/.eznit/config.json` (mode `0600`).

The GUI reads `GUI_API_URL` (proxy `/v1` to a running API instead of running
one), `GUI_PORT` (`8765` by default) and `GUI_LOGS`, plus the API's database and
upload variables for the in-process mode. With neither `GUI_API_URL` nor
`DB_NAME`, the UI opens and `/v1` answers `503` with the variable to set.

## Gates

| Command | Covers | Needs |
| --- | --- | --- |
| `make check` | gofmt, `go vet`, `go test`, `go build` | Go only |
| `make web_check` | oxlint, oxfmt, `tsc --noEmit`, `vite build` | Node (`mise install`) |
| `make build` | both binaries (and the UI they embed) | Go + Node |

Both gates fail rather than skip when their toolchain is missing. Neither runs
the product: no database, no HTTP request, no browser. `make check` prints the
packages that have no test files, because `go test` exits 0 on those — a green
run means "the packages with tests passed", not coverage.

CI runs both gates on release pull requests (`dev` → `main`) and on manual
dispatch only. Feature pull requests are gated locally.

## Where a change goes

| You are adding… | Start at |
| --- | --- |
| an endpoint | `internal/domain/<resource>/handler.go` (+ `dto.go`) |
| a rule ("only the owner may…") | the same package's `service.go`, with a test |
| a query or a column read | `repository_sql.go`; name the columns, never `SELECT *` |
| a table or a constraint | a new `db/migrations/NNNNNN_*.up.sql`/`.down.sql` pair |
| a CLI command | `internal/cli/<command>.go`, wired in the `CLI` struct in `internal/cli/main.go` |
| a page | `web/src/pages/`, routed in `web/src/App.tsx` |
| shared plumbing | `pkg/` (no domain rules) or `internal/platform/` (infrastructure) |

## Known deviations

These are the places where the code does not match the shape above. They are
tracked, not forgotten:

- **`API_PORT` is not read.** The API listens on `:4000`, hardcoded; the
  variable exists in `.env.example` and matters only to the commented `api`
  service in `docker-compose.yml`.
- **Several packages have no tests.** `internal/domain/users`,
  `internal/platform/middleware`, `pkg/errors`, `pkg/logger` and `pkg/types`
  are compiled and vetted, not exercised. `internal/domain/sessions` has a
  schema test (its reads match its struct and the migrations), not a
  behaviour test.
- **The file catalogue is not owner-scoped.** `GET /v1/files` returns every
  upload in the instance to any logged-in caller; `GET /v1/files/me` is the
  scoped one. See "Security status" in [README.md](README.md#security-status).
