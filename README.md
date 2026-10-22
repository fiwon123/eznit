# eznit

Self-hosted file storage. A Go API and a CLI client for uploading, listing and
retrieving your own files, backed by PostgreSQL.

## Table of contents

- [About](#about)
- [How to use](#how-to-use)
  - [Run the stack](#run-the-stack)
  - [Build the CLI](#build-the-cli)
  - [Commands](#commands)
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

The database password comes from `secrets/db_password.txt`, which Compose
mounts as a Docker secret and the API reads through `DB_PASSWORD_FILE`. It is
never an environment variable.

Then bring up the database and run the migrations:

```sh
make docker_up      # or: docker compose up db migrate
```

The API runs from source, not from a container: `make run_api` on the host, or
inside the dev sandbox (see [How to develop](#how-to-develop)). The host CLI
talks to it on `http://localhost:4000`.

> The `api` service in `docker-compose.yml` is kept as a commented block: it
> runs the **prebuilt** binary from `./build`, which is the path a release
> bundle takes. Uncomment it when that is what you want.
>
> In a **release bundle** the same file ships without the sandbox's image
> source, so start the database with `docker compose up db migrate` — the
> `dev` service is the development sandbox, and it is not part of the bundle.

### Build the CLI

```sh
make build        # both binaries into ./build
./build/cli/eznit --help
```

### Commands

| Command | What it does |
| --- | --- |
| `eznit signup` | Create an account |
| `eznit login` | Start a session; the token is stored in `~/.eznit/config.json` (mode 0600) |
| `eznit list` | List the files uploaded in this instance (add `--me` for your own) |
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

## Security status

This is a self-hosted tool, and it is not hardened. The known gaps are tracked
as issues rather than left for you to discover:

| Gap | Issue |
| --- | --- |
| The catalogue lists every user's file names to any user with a session | — |

Also true today:

- **The coverage is partial.** The files domain, the CLI and the path helper
  have tests; the users, sessions, middleware and platform packages have
  none. `make check` compiles and vets everything. Treat a green run as
  "it builds, and the tested packages passed" — not "it works".
- Files are stored unencrypted on the server's disk.
- There is no rate limiting on `signup` or `login`.
- Passwords are hashed; session tokens are random and stored in the database
  with an expiry.

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
requirement: it is a container with Go **1.24.1** — the version in `go.mod`,
the same one CI uses — and the workspace is bind-mounted into it, so the files
you edit are the same files.

```sh
make dev-up     # start the sandbox + PostgreSQL + the migrations
make dev-shell  # a shell inside the sandbox
make dev-down   # stop it (keeps the volumes)
```

The sandbox waits for the stack: compose starts PostgreSQL, waits for it to
be healthy, runs the migrations, and only then starts the sandbox. A
failed migration keeps the sandbox down rather than starting it against an
unmigrated database.

Everything also works without Make:

```sh
docker compose up                              # the sandbox and the stack
python scripts/dev-up.py dev-up                # native Windows
```

| Platform | How to run the dev flow |
| --- | --- |
| Linux, macOS | `make dev-up`, or `docker compose up` |
| WSL2 | `make dev-up`, or `docker compose up` (the host is Linux) |
| Windows (native) | `python scripts/dev-up.py dev-up`, or `docker compose up` |

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
cmd/api/         API entrypoint
cmd/cli/         CLI entrypoint, one file per command
internal/domain/ one package per resource: users, sessions, files
                 handler -> service -> repository, plus dto.go
internal/platform/
                 middleware (session guard), sql (connection)
pkg/             logger, errors, http/json helpers, path helpers, types
db/migrations/   golang-migrate SQL, one up/down pair per change
```

The layering is strict: a handler decodes and writes, a service holds the
rules, a repository is the only place that speaks SQL.

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

`make dev-reset` also drops the `postgres-data` volume — that is why it is not
called `reset` — so the migrations run again on the next `make dev-up`.

**`bind: address already in use` on port 4000**

The sandbox publishes 4000 so the host CLI can reach the API inside it. If the
API runs on the host instead, publish a different host port:
`HOST_PORT=4001 docker compose up`.

## License

MIT — see [LICENSE](LICENSE).
