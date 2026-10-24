# Architecture

eznit is a self-hosted file store: one PostgreSQL database, one HTTP API, a CLI
that speaks to it, and a web UI the API serves. This document is the map — what
runs where, how a request travels, and where a change belongs. For the commands
that drive it, see [README.md](README.md).

## The pieces

| Piece | Entry point | What it is | Listens |
| --- | --- | --- | --- |
| API | `cmd/api` | The HTTP API, and the web UI in front of it | `:4000` |
| Web tier | `web/Dockerfile` + `web/nginx.conf` | The built UI behind nginx, proxying `/v1` to the API (compose only) | `:5173` |
| CLI | `cmd/cli` → `internal/cli` | `kong` command line client, one file per command | — |
| GUI | `cmd/gui` | Opens a browser on the bundled UI, with the API in-process or proxied | `:8765` |
| Web UI | `web/src` | React + TypeScript + Vite, built to `web/dist` | — |

The API and the GUI both carry the built UI inside the binary (`web/embed.go`);
the API also serves a `web/dist` folder when one exists. That is why one origin
serves both the pages and `/v1`, and why the API needs no CORS headers.

The compose stack splits that in two: the `web` service serves the same built
UI on `:5173` and proxies `/v1` to the `api` service, so the browser still sees
one origin and still needs no CORS. The `api` service sets `API_SERVE_WEB=false`
so `:4000` stays the API only; the released binary keeps the embedded UI and
needs neither the proxy nor the second port.

The developer loop is the same split with a different pair of processes:
`make dev-up` runs the API from the source inside the `dev` sandbox (Go 1.27.3)
and the UI as Vite in `web-dev` (Node 24.22.1), both over the bind mount. The
same two ports, the same `/v1` proxy, the same `API_SERVE_WEB=false` — so a bug
that is caused by the two tiers is reproducible in either mode. Neither
container holds the other's toolchain, and neither needs one on the host.

The API's router is built by `internal/platform/api`, and both binaries use it:
the API serves it directly, and the GUI either mounts it on its own listener
(when the database is configured) or proxies `/v1` to `GUI_API_URL`. One router,
so the two cannot offer different APIs.

## Repository map

```
cmd/api/            API entrypoint: flags, logging, database, router, UI, shutdown
cmd/cli/            CLI entrypoint: the version variable, then internal/cli
internal/cli/       CLI commands, the API client, the local config/token store,
                    and the git-host commands
cmd/gui/            GUI entrypoint: the UI, the API (or a proxy), a browser
internal/domain/    one package per resource — users, sessions, files, templates
                    handler.go -> service.go -> repository.go, plus dto.go
internal/gitprovider/  git-host clients (GitHub today) behind one interface;
                    holds no credential — the token is an argument
internal/platform/  cross-domain infrastructure
                    api/         the /v1 router, built once for both binaries
                    openapi/     the OpenAPI document, its JSON twin, and Swagger UI at /docs
                    middleware/  session guard (Authorization: Bearer <token>)
                    sql/         database connection and the DSN from the environment
                    webui/       the SPA handler both binaries serve the UI with
pkg/                shared code with no project rules in it
                    errors/   AppError: a message and an HTTP status
                    helper/   JSON envelopes, path safety, token generation
                    logger/   zap through log/slog, rotating files (lumberjack)
                    types/    Envelope[T], the response wrapper
web/                the React UI; embed.go embeds the build into the binaries
                    Dockerfile   the web tier image (Node builds, nginx serves)
                    Dockerfile.dev  the Vite dev server, the `web-dev` service
                    nginx.conf   SPA fallback and the /v1 proxy
db/migrations/      golang-migrate SQL, one .up/.down pair per change
scripts/            migrations runner, dev sandbox helpers, path setup
Dockerfile          the dev sandbox image (Go 1.27.3, gh, zip)
Dockerfile.api      the application image: Node builds the UI, Go embeds it
docker-compose.yml  the product (db, migrate, api, web) and the developer loop (profile: dev: dev, web-dev)
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

A text file of a template lives in its `content` column: templates are
configuration, small and searchable. A **binary** file does not — a `TEXT`
column refuses a byte that is not UTF-8, and the driver would mangle it — so it
is written under the same upload root:

```
<upload-root>/templates/<template-id>/<file-id>/<filename>
```

The `template_files` row holds `kind = 'binary'`, the size and `storage_path`
(the location relative to the upload root, with forward slashes so a row reads
the same on every machine), and its `content` is empty. A `CHECK` constraint
keeps the two shapes apart. Publishing sends a binary file as
`content_base64`; the service decodes it and writes it before the rows, and
removes the bytes again if the write fails, so a template is never half
stored. An update removes the bytes of the version it replaced, and deleting a
template removes its directory.

The limits are enforced in the service, not only the client: 256 KiB per text
file and 4 MiB of text per template, 10 MiB per binary file and 32 MiB per
template counting both.

A **command** is a file a user keeps once and pulls into templates — a linter
wrapper, a release helper — and it uses the same two shapes. A text command
keeps its content in its column; a binary one is written at:

```
<upload-root>/commands/<command-id>/<name>
```

and its row holds the path. A name is unique per user, not globally. The
`template_commands` row says which command a template pulls in and where in
the project it goes (`target_path`, unique per template); the command's bytes
are read through the reference, so updating a command updates every template
that pulls it in. A command a template references cannot be deleted while the
reference exists: `ON DELETE RESTRICT` keeps a published template from quietly
losing a file. A template may only reference commands of its own owner, and
the service checks it before the reference is stored.

## The API reference

`internal/platform/openapi/` holds the contract and the UI over it:

- `openapi.yaml` is the document: OpenAPI 3.1, every `/v1` operation, the
  response envelope, the multipart bodies and the bearer scheme. It is
  committed, reviewed like code, and embedded in the binary.
- `openapi.go` serves it — `GET /v1/openapi.yaml`, the same document as JSON at
  `GET /v1/openapi.json`, and Swagger UI at `/docs`.
- `index.html` is the UI page. Its JavaScript and stylesheet come from
  `github.com/swaggo/files/v2`, which embeds the Swagger UI distribution in the
  binary, so nothing on `/docs` reaches the network: the reference works on a
  host with no internet access.

The routes are public on purpose — a client author reads the contract before
they have a token. They expose the route table and the schemas, never a row, a
file or a token. `internal/platform/webui` passes `/docs` to the API, because
the SPA fallback would otherwise answer it with the app.

The document is the source of truth, not a by-product of comments. Three tests
keep it honest:

- `internal/platform/api/openapi_test.go` walks the router with `chi.Walk` and
  fails when an operation is in one and not the other, in either direction;
- the same file compares each DTO schema with its struct's json tags, and the
  `FileRecord` schema with the field names the storage model actually sends;
- `internal/platform/openapi/openapi_test.go` resolves every `$ref`, checks
  each operation has an id, a summary, a success response and its path
  parameters, and asserts the UI page references no network URL.

Adding an endpoint means adding it to `openapi.yaml` in the same pull request;
the gate fails otherwise.

## Data model

| Table | Holds |
| --- | --- |
| `users` | id, email, password hash, created_at, updated_at |
| `files` | id, user_id, name, ext, content_type, path, version, timestamps |
| `files_history` | file_id, path, version — one row per stored version: upload writes the first, an update adds the next, and earlier rows are never rewritten |
| `sessions` | token (primary key), user_id, is_active, created_at, expires_at |
| `templates` | id, user_id, name, slug, description, kind, version, is_public |
| `template_files` | template_id, path, kind (`text`/`binary`), size, content, storage_path, is_executable — unique per (template, path) |
| `template_variables` | template_id, var_key, prompt, default_value, required, sort_order |
| `commands` | id, user_id, name, kind (`text`/`binary`), content, size, storage_path, is_executable — the name is unique per user |
| `template_commands` | template_id, command_id, target_path — the commands a template pulls in; `ON DELETE RESTRICT` on the command, `CASCADE` on the template |

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
and stores its session in `~/.eznit/config.json` (mode `0600`). `EZNIT_CONFIG`
points the config file elsewhere (a test or a profile), and the file keeps
every key it holds: the session token and the git-host token are written
separately and neither erases the other.

A git-host credential is **the CLI's, not the server's**. `eznit repo auth`
stores a GitHub personal access token under `github_token` in the same file;
the eznit API has no route that reads it and never sees it. `eznit repo clone`
asks the host for the repository (one `GET /repos/{owner}/{repo}`) and runs
`git clone` itself, into `<root>/<provider>/<owner>/<name>` where `<root>` is
`--dir`, `EZNIT_REPOS`, or `~/eznit`. The token reaches git through the child's
environment — `GIT_CONFIG_COUNT=1`, `GIT_CONFIG_KEY_0=http.extraheader`,
`GIT_CONFIG_VALUE_0=AUTHORIZATION: basic …` — never through `argv` and never
through the URL, so it is absent from the process list, from the clone's
`.git/config`, and from the CLI's logs. `GIT_TERMINAL_PROMPT=0` makes a
credential failure an error instead of a prompt. `EZNIT_GITHUB_API` overrides
the API address for GitHub Enterprise.

The residual exposure is the child's environment, which the same user's
processes can read (`/proc/<pid>/environ`). That is the cost of not putting
the token in a file git owns, and it is stated rather than hidden.

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
| the documentation of an endpoint | `internal/platform/openapi/openapi.yaml` — the drift gate fails until it matches |
| a rule ("only the owner may…") | the same package's `service.go`, with a test |
| a query or a column read | `repository_sql.go`; name the columns, never `SELECT *` |
| a table or a constraint | a new `db/migrations/NNNNNN_*.up.sql`/`.down.sql` pair |
| a CLI command | `internal/cli/<command>.go`, wired in the `CLI` struct in `internal/cli/main.go` |
| a git host | an implementation of `gitprovider.Provider`, plus a case in `providerFor` |
| a page | `web/src/pages/`, routed in `web/src/App.tsx` |
| shared plumbing | `pkg/` (no domain rules) or `internal/platform/` (infrastructure) |

## Known deviations

These are the places where the code does not match the shape above. They are
tracked, not forgotten:

- **`API_PORT` is read by the API but only changes the bind port.** It defaults
  to `4000` (what the CLI, the GUI and the docs expect), and the compose file
  also uses it for the published host port. It is not a way to move the service
  behind a proxy without changing the proxy target.
- **Several packages have no tests.** `internal/domain/users`,
  `internal/platform/middleware`, `pkg/errors`, `pkg/logger` and `pkg/types`
  are compiled and vetted, not exercised. `internal/domain/sessions` has a
  schema test (its reads match its struct and the migrations), not a
  behaviour test.
