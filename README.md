<div align="center">

# eznit

Self-hosted file storage. A Go API and a CLI client for uploading, listing and
retrieving your own files, backed by PostgreSQL.

[![License: MIT](https://img.shields.io/github/license/fiwon123/eznit)](LICENSE)
[![Go version](https://img.shields.io/github/go-mod/go-version/fiwon123/eznit?logo=go&logoColor=white)](go.mod)
[![CI (Go)](https://github.com/fiwon123/eznit/actions/workflows/ci.go.yml/badge.svg)](https://github.com/fiwon123/eznit/actions/workflows/ci.go.yml)

</div>

## Table of contents

- [About](#about)
- [Architecture](ARCHITECTURE.md)
- [How to use](#how-to-use)
  - [Run the stack](#run-the-stack)
  - [Build the CLI](#build-the-cli)
  - [Commands](#commands)
- [Web UI](#web-ui)
- [Security status](#security-status)
- [How to develop](#how-to-develop)
  - [Layout](#layout)
  - [Migrations](#migrations)
  - [Checking your change](#checking-your-change)
- [Troubleshooting](#troubleshooting)
- [License](#license)

## About

Eznit keeps a file on storage you control, indexes it in PostgreSQL, and gets it
back out with one command. There is no browser and no account with a third
party — the API and the CLI are the whole product.

Two entry points:

- **API server** (`cmd/api`) — serves the HTTP API the CLI talks to
- **CLI tool** (`cmd/cli`) — signup, login, logout, upload, download, list,
  update, delete

> **Pre-1.0 and unhardened.** Read [Security status](#security-status) before
> exposing this anywhere but `localhost`.

**Supported platforms:** Linux, macOS and Windows. The CLI and the API are pure
Go; the development flow below runs through the dev sandbox, so it behaves the
same on all three — see [How to develop](#how-to-develop).

## How to use

### Run the stack

Two local files come first — the configuration and the database secret. Both
are gitignored, and `make setup` creates them from the committed examples:

```sh
make setup
```

Without `make`, copy `.env.example` to `.env` and
`secrets/db_password.txt.example` to `secrets/db_password.txt` by hand. The
database password is a Docker file secret read through `DB_PASSWORD_FILE`; it
is never an environment variable.

Then one command brings up the product:

```sh
make up             # or: docker compose up
```

That does five things, in order:

1. builds two images: `web/Dockerfile` (Node builds the UI, nginx serves it) and
   `Dockerfile.api` (Node builds the UI, Go embeds it in the binary);
2. starts PostgreSQL and waits until it is healthy;
3. runs the `migrate` service once (`migrate up` is idempotent);
4. starts the API on `http://localhost:4000` — API only, because the compose
   service sets `API_SERVE_WEB=false`;
5. starts the web UI on `http://localhost:5173`. It proxies `/v1/*` to the API,
   so the browser still talks to one origin and the API needs no CORS headers.

| Role | Where | Port |
| --- | --- | --- |
| Frontend (the app) | `web` service, nginx | **5173** |
| Backend (the API) | `api` service | **4000** |
| Database | `db` service | 5432 |

`http://localhost:4000` still serves the UI inside the *released binary* (it
embeds it); the compose stack keeps the two tiers apart so each port means one
service.

Uploaded bytes live on the `api-data` volume, so they survive
`docker compose down` (use `docker compose down -v` to remove them too).

> In a **release bundle** the source is not there, so there is no image to
> build. Start the database with `docker compose up db migrate` and run the
> prebuilt `build/api/server` (Windows: `build\api\server.exe`) from the
> archive; it serves the UI embedded at build time. Plain `docker compose up`
> is the source-checkout path — in a bundle it would try to build an image the
> archive does not contain. `make dev-up` and the `dev` sandbox are not part of
> the bundle either.

### Build the CLI

```sh
make build        # both binaries into ./build (and the web UI, which they embed)
./build/cli/eznit --help
```

Node is needed here even for the Go binaries: `make build_api` and
`make build_gui` build the web UI first, because both binaries carry a copy of
it (see [Web UI](#web-ui)). `make check` does not — the Go gate stays Node-free.

### Commands

| Command | What it does |
| --- | --- |
| `eznit signup` | Create an account |
| `eznit login` | Start a session; the token is stored in `~/.eznit/config.json` (mode 0600) |
| `eznit list` | List your own files (`--me` is an alias for the same list) |
| `eznit upload` | Upload a file |
| `eznit download` | Download one of your own files by id |
| `eznit delete` | Delete a file |
| `eznit logout` | End the session |

Every command except `signup` and `login` needs a session: the API requires
the token, and the CLI sends the one `login` stored.

```sh
eznit signup
eznit login
eznit upload
eznit list
eznit download
```

#### Templates

A template is a folder structure — agent instructions, prompts, editor config —
that can be published and applied to another project:

| Command | What it does |
| --- | --- |
| `eznit template publish <folder>` | Publish a folder as a template; `--command <id>=<path>` pulls a command in |
| `eznit template list` | The catalogue: the public templates and your own |
| `eznit template show <id>` | One template, its files and the variables it asks for |
| `eznit template apply <id>` | Write a template into the current folder |
| `eznit template init` | Write a starter template to publish |
| `eznit command add <file>` | Keep a script in your command library (`--replace` updates it) |
| `eznit command list` | List your command library |
| `eznit command show <name>` | Show one command |
| `eznit command remove <name>` | Remove a command; a command a template pulls in is refused |

A template holds text files and binary files. Text travels as it is; a binary
file — an image, a compiled helper, anything with a byte that is not valid
UTF-8 — is sent as base64 and stored on disk by the server, under the upload
root. The CLI detects the kind per file, keeps the executable bit, and reports
the files it left out. Limits: 256 KiB per text file and 4 MiB of text per
template, 10 MiB per binary file and 32 MiB per template counting both.

A **command** is a file kept once in your library and pulled into templates by
id, so one copy is updated once:

```sh
eznit command add scripts/fmt.sh --name fmt
eznit template publish . --command <command-id>=scripts/fmt.sh
```

`eznit template apply` writes the command at its target path. A command that a
template pulls in cannot be removed until the template stops referencing it.

#### Git hosts

`eznit` can list the repositories a GitHub token can reach and clone one. The
token is a **personal access token**, and it stays on your machine: it is kept
in `~/.eznit/config.json` (mode 0600) next to the session token, and the eznit
API never sees it.

| Command | What it does |
| --- | --- |
| `eznit repo auth` | Store a GitHub token (prompt by default, `--token`, `--token-file`); `--remove` forgets it |
| `eznit repo list` | List the repositories the token can reach |
| `eznit repo clone <owner/name> [--dir <path>]` | Clone into `<dir>/github/<owner>/<repo>` (default `~/eznit`) |

```sh
eznit repo auth                 # paste a token at the prompt
eznit repo list
eznit repo clone fiwon123/eznit
```

The clone runs `git` directly, with the token supplied through the child
process's environment (`http.extraheader`), so it is never in the command
line, never in the URL, and never written into the clone's `.git/config`. The
remote stays the plain HTTPS URL. `EZNIT_REPOS` sets the clone root when
`--dir` is not given, and `EZNIT_GITHUB_API` points at a GitHub Enterprise API.
`eznit logout` removes the session token and leaves the host token in place;
`eznit repo auth --remove` removes that one.

## Web UI

The web UI lives in `web/`. It reaches the browser two ways:

- **from disk** — the API serves `web/dist` when it holds a build, so a
  production server runs the UI from the same origin as the API, and no CORS
  is needed. Rebuilding the UI is enough; the API does not need a restart.
- **from the binary** — the API and the GUI carry the UI embedded at build
  time (`web/embed.go`), so a released binary serves it with no `web/dist`
  next to it. `make build_api`, `make build_gui` and the compose image
  (`Dockerfile.api`) build the UI first for that reason.

A `web/dist` with no `index.html` — a fresh clone, where the folder holds only
the committed placeholder — counts as "no UI": the API serves the API only.

| Command | What it does |
| --- | --- |
| `make web_install` | Install the web dependencies (`npm ci`) |
| `make web_dev` | Run the Vite dev server on `http://localhost:5173`; `/v1` is proxied to `http://localhost:4000` |
| `make web_build` | Build the static assets into `web/dist` |
| `make web_check` | Lint, format check, typecheck and build the web UI (fails on any error) |
| `make dev-up` | Start the dev loop: UI `:5173` (Vite, hot reload), API `:4000` |
| `make up` | Build and run the product stack: UI on `:5173`, API on `:4000` |

The quickest way to see it needs no Node on the host — the images build the UI:

```sh
make setup
make up                 # UI http://localhost:5173 — API http://localhost:4000
```

For the development loop, with hot reload — one command, no host Go or Node:

```sh
make setup
make dev-up             # UI http://localhost:5173 + API http://localhost:4000
```

Both ports answer when it returns: the API runs inside the sandbox against the
source (`make run_api`), and the UI is Vite in the `web-dev` service, proxying
`/v1` to it. Editing a `.tsx` file reloads the browser; a `.go` file needs the
API restarted (`make dev-down && make dev-up`, or `make run_api` again inside
`make dev-shell`).

Host tools still work: `make run_api` and `make web_dev` are the same two
processes outside containers.

Sign up or log in through the CLI or the API, put the session token in
`localStorage` as `eznit.token` in the browser, or use the login page.

Node is pinned in `mise.toml` (the CI reads it via `node-version-file`).

### The GUI binary

`make build_gui` produces one executable that serves the UI it carries and
opens a browser on it (`GUI_PORT`, default `8765`). The pages call `/v1` on the
same origin, so the GUI answers those calls one of two ways:

- **`GUI_API_URL` names a running API** — `/v1` is proxied there. This is the
  launcher mode: the GUI needs no database configuration of its own, and
  several windows can point at one server.
- **otherwise the `DB_*` variables describe a database** — the API runs inside
  the GUI process on the same listener, so one file is the whole product. The
  GUI reads `API_UPLOADS` and `API_LOGS` like the API does, and `.env.local` /
  `.env` if they are there.

With neither, the window still opens and every `/v1` call answers `503` with
the variable to set. A GUI that cannot reach the API or the database it was
configured with says so the same way, instead of leaving the page with the
static file server's 404.

```sh
# one file, with the database configuration from .env
./build/eznit-gui

# or: a window in front of an API that is already running
GUI_API_URL=http://localhost:4000 ./build/eznit-gui
```

## Security status

This is a self-hosted tool, and it is not hardened. What is true today:

- **The coverage is partial.** The files domain, the CLI and the path helper
  have tests; the users, sessions, middleware and platform packages have
  none. `make check` compiles and vets everything. Treat a green run as
  "it builds, and the tested packages passed" — not "it works".
- Files are stored unencrypted on the server's disk.
- There is no rate limiting on `signup` or `login`.
- Passwords are hashed; session tokens are random and stored in the database
  with an expiry. The only `golang.org/x/crypto` package this repository links
  is `bcrypt` — the module's SSH and S/MIME advisories do not apply to the code
  that is actually compiled. CI now runs a reachable-vulnerability scan
  (`govulncheck`), and `make vuln` runs the same scan locally. It fails only
  when this code reaches a vulnerable function: a module-level advisory in a
  package nothing calls — such as the unmaintained `openpgp` package in
  `x/crypto`, which has no fix — is printed by the tool but does not fail the
  gate. `make check` stays offline and does not scan.
- A git-host token is stored only in the CLI's own config file (mode 0600).
  It is never sent to the eznit API, never logged, never put in a URL, and
  never written into a clone's `.git/config`. It is passed to `git` through
  the child process's environment, which the same user's other processes can
  read — the same exposure as any credential on a shared machine.

**Do not put this on a public interface as it stands.** It is fine on
`localhost`, or behind a VPN you control.

### Local secret files

`.env`, `.env.local` and `secrets/db_password.txt` must be mode `600` — they
hold credentials, and the default umask makes them readable by every user on
the machine. They are gitignored, so this cannot be fixed by a commit; set it
on each checkout (`make setup` creates the first and the third with that mode
already set):

```sh
chmod 600 .env .env.local secrets/db_password.txt
```

## How to develop

```sh
make check     # gofmt, vet, test, build — the local verification gate
make vuln      # scan for reachable dependency vulnerabilities (needs network)
make run_api   # run the API against the configured database
make run_cli   # run the CLI
```

`make check` needs no database, no container and no secret file: it compiles
the API and the CLI, it does not run them. That is deliberate — the gate has to
work on a fresh clone. It fails loudly if the Go toolchain is missing rather
than skipping, and it lists the packages with no test files, because `go test`
prints `[no test files]` and still exits 0.

### Dev sandbox

The commands above need a Go toolchain on the host. The sandbox removes that
requirement: it is a container with Go **1.27.3** — the version in `go.mod`,
the same one CI uses — and the workspace is bind-mounted into it, so the files
you edit are the same files. The frontend has the same arrangement in
`web-dev`: the Node from `mise.toml`, running Vite against the same bind mount.

```sh
make dev-up     # the loop: UI :5173 (Vite, hot reload) + API :4000
make dev-shell  # a shell inside the sandbox
make dev-down   # stop it (keeps the volumes)
```

`make dev-up` starts four things: PostgreSQL, the migrations, the sandbox with
`make run_api` in it, and the Vite dev server. Both HTTP ports answer when it
returns, and an edit on the host is visible in the other one immediately — the
Go process recompiles on its next start (there is no file watcher: restart the
API after a Go change), and Vite reloads the browser on a web change.

| Role | Where | Port |
| --- | --- | --- |
| Frontend (the app) | `web-dev` service, Vite | **5173** |
| Backend (the API) | `dev` service, `make run_api` | **4000** |

| What you want | Commands | Ports |
| --- | --- | --- |
| the product, in containers | `make up` | UI `:5173`, API `:4000` |
| the dev loop, against the source | `make dev-up` | UI `:5173`, API `:4000` |
| the same loop with host toolchains | `make run_api` + `make web_dev` | API `:4000`, UI `:5173` |

The two entry points are alternatives: `make up` and `make dev-up` publish the
same two ports, so each refuses to start while the other one holds them, and
names the command that clears the way (`make docker_down` or `make dev-down`)
instead of failing on `bind: address already in use`.

`Ctrl-C` stops the log stream and leaves the loop running — `make dev-down` is
what stops it. The marker file `.dev/sandbox-start` (gitignored) is how the
entrypoint knows to start the API rather than sleep; `dev-up` creates it and
`dev-down` removes it, so a bare `docker compose up dev db migrate` is still a
quiet sandbox with a shell.

Inside the sandbox the configuration comes from the container, not from `.env`:
compose sets `DB_HOST=db` and `DB_PORT=5432` because that is the in-network
address, and the Makefile deliberately does not import the host's `.env` there.
The database password stays a file secret the container reads from
`/run/secrets/db_password`.

The sandbox waits for the stack: compose starts PostgreSQL, waits for it to
be healthy, runs the migrations, and only then starts the sandbox. A
failed migration keeps the sandbox down rather than starting it against an
unmigrated database.

Everything also works without Make:

```sh
mkdir -p .dev && touch .dev/sandbox-start      # makes the sandbox run the API
docker compose up dev web-dev db migrate       # the whole loop
python scripts/dev-up.py dev-up                # native Windows: same thing
```

| Platform | How to run the dev flow |
| --- | --- |
| Linux, macOS | `make dev-up` |
| WSL2 | `make dev-up` (the host is Linux) |
| Windows (native) | `python scripts/dev-up.py dev-up`, or `docker compose up dev web-dev db migrate` after creating the marker |

The `dev` and `web-dev` services are behind the `dev` Compose profile: plain
`docker compose up` is the **product** (see [Run the stack](#run-the-stack)),
and naming one of them explicitly enables the profile. `make dev-up` does
exactly that, and creates the `.dev/sandbox-start` marker that tells the
sandbox entrypoint to start the API instead of sleeping for a shell.

The sandbox shares the Compose network with PostgreSQL, so the API started
inside it (`make run_api`) reaches the database at `db:5432` with the stock
`.env` — no `DB_HOST` override, and no password on the command line: the
container reads the same file secret the API container reads.

`make dev-opencode` runs the AI coding agent inside the sandbox, with the
host's `~/.config/gh`, `~/.config/opencode` and `~/.gitconfig` mounted
read-only. It needs a **Linux** opencode binary: on macOS and native Windows
the target refuses with a clear message and the agent stays on the host (use
WSL2, where the host binary *is* Linux). The container has no Docker socket —
the agent inside it cannot manage containers, on purpose.

### Layout

```
cmd/             the three entrypoints: api, cli, gui
internal/domain/ one package per resource: users, sessions, files, templates
                 handler -> service -> repository, plus dto.go
internal/platform/
                 middleware (session guard), sql (connection), webui (SPA handler)
pkg/             logger, errors, http/json helpers, path helpers, types
db/migrations/   golang-migrate SQL, one up/down pair per change
web/             React + TypeScript UI; embeds into the api/gui binaries
```

The layering is strict: a handler decodes and writes, a service holds the
rules, a repository is the only place that speaks SQL.

[ARCHITECTURE.md](ARCHITECTURE.md) is the full map: the request path, the data
model, where file bytes live, how the UI reaches the browser, and where a
change belongs.

### Migrations

`db/migrations/` is append-only. Never edit a migration that has run
somewhere — add a new numbered up/down pair.

```sh
make migrate_up
make migrate_down      # one step
make migrate_down_all
```

The migration targets read `DB_USER`, `DB_HOST` and `DB_NAME` from `.env` or
`.env.local`; if they are missing, they say so instead of failing with an empty
connection string.

### Checking your change

1. `make check` — the gate above
2. If you touched the API, run the stack and exercise the route: the gate
   compiles, it does not call anything
3. If the change is user-visible in the CLI, read the output — a CLI has no
   screenshot, and its output is the artifact

### Releasing

A release is a `v*` tag on `main`. `.github/workflows/release.yml` refuses a tag
that does not point at a commit on `main`, builds the archives with
`make build VERSION=<tag>` — the same target a developer runs — and publishes
them with `gh release create`:

| Asset | Contents |
| --- | --- |
| `eznit_cli_<version>_windows.zip` | `eznit.exe`, `ez.exe`, `add_to_path.ps1`, README, LICENSE |
| `eznit_cli_<version>_linux.tar.gz` | `eznit`, `ez`, `add_to_path.sh`, README, LICENSE |
| `eznit_server_<version>_windows.zip` | the API server for Windows, the migrations, the Compose files and the example config |
| `eznit_server_<version>_linux.tar.gz` | the API server for Linux, the migrations, the Compose files and the example config |
| `checksums.txt` | SHA-256 of every archive |

The workflow can also be run by hand (`workflow_dispatch`): that run builds the
same archives and keeps them on the run, and publishes nothing. Only a tag
publishes.

## Troubleshooting

**`ERROR: DB_USER, DB_HOST and DB_NAME are not set`**

The migration targets need a database connection. Copy `.env.example` to
`.env`, fill it in, and try again. `make check` and `make build` do not need it.
`make setup` creates the file for you.

**`make check` passes, but nothing seems tested**

That is correct and expected — there are no test files yet. `make check`
prints the list of packages it did not cover. Coverage is a tracked gap, not an
oversight in the gate.

**The API starts and immediately exits**

Check the log for the connection error. The API reads `DB_PASSWORD_FILE` first
and falls back to `DB_PASSWORD`; Compose sets the former from the Docker
secret. If you are running the API by hand, set one of them.

**Permission denied reading a secret file**

The file is mode `600` and owned by another user. See
[Local secret files](#local-secret-files).

**`set: pipefail: invalid option name` (or `bad interpreter`) in the sandbox**

The scripts run through the bind mount, so bash parses your working tree's
bytes. A checkout made before the `.gitattributes` rule was added keeps CRLF
endings, and `git status` stays clean while it does. Re-check the files out
with LF endings:

```sh
git rm --cached -r . >/dev/null && git reset --hard
```

**`docker compose up` fails on a missing secret**

The database secret is a file secret, so Compose refuses to start without it.
`make setup` creates it from the committed example.

**`EACCES: permission denied, mkdir '/home/devuser/.cache/opencode'`, or `mkdir /go/pkg/mod/cache: permission denied` in the sandbox**

The sandbox's caches are Docker volumes, and Docker creates a mount destination
the image does not contain **as root** — while the container runs as your UID.
The image now creates those directories itself, so a sandbox built from the
current `Dockerfile` is fine.

A sandbox created **before** that fix keeps its old, root-owned volumes:
Docker does not re-initialise a volume that already exists. Recreate them
once:

```sh
make dev-reset     # docker compose down --volumes
make dev-up
```

`make dev-reset` also drops the `postgres-data` and `api-data` volumes — the
database and the uploaded files — which is why it is not called `reset`. The
migrations run again on the next `make dev-up`.

**`bind: address already in use` on port 4000 or 5173**

The product and the dev loop publish the same two ports, and they are
alternatives. Both entry points now check before they start anything and say
which one is running, instead of failing halfway with Docker's message:

```sh
make docker_down      # stop the product, then: make dev-up
make dev-down         # stop the dev loop, then: make up
```

When the port belongs to something that is not this stack (a `make run_api` in
another terminal, a second checkout), move one of them:

```sh
HOST_PORT=4001 make dev-up                     # the sandbox API
WEB_PORT=5174 make dev-up                      # the dev UI (HMR follows it)
API_PORT=4001 docker compose up                # the product API
WEB_PORT=5174 docker compose up                # the product UI
```

**The UI loads on `:5173` but every request fails**

The Vite dev server proxies `/v1` to the API, so an API that is not running
looks like a broken UI: the page renders and the data does not. Check the API
first — `curl -s -o /dev/null -w '%{http_code}\n' localhost:4000/v1/healthcheck`
— then the sandbox log for a compile error (`make dev-log`). With
`make dev-up` the log stream is attached, so the error is usually on screen.

## License

MIT — see [LICENSE](LICENSE).
