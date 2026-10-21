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

**Supported platforms:** Linux and macOS. Windows works through WSL2; the CLI
and the API are pure Go, but the development flow below assumes a POSIX shell.

## How to use

### Run the stack

Docker Compose brings up PostgreSQL, runs the migrations, and starts the API:

```sh
make docker_up_all
```

The database password comes from `secrets/db_password.txt`, which Compose
mounts as a Docker secret and the API reads through `DB_PASSWORD_FILE`. Create
it from the example before the first run:

```sh
cp secrets/db_password.txt.example secrets/db_password.txt
chmod 600 secrets/db_password.txt
```

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
| `eznit list` | List files |
| `eznit upload` | Upload a file |
| `eznit download` | Download a file by id |
| `eznit delete` | Delete a file |
| `eznit logout` | End the session |

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
| `download` requires no token, and the CLI never sends one | [#4](https://github.com/fiwon123/eznit/issues/4) |
| `list` returns every user's file metadata, unauthenticated | [#5](https://github.com/fiwon123/eznit/issues/5) |
| The stored filename comes from the client | [#5](https://github.com/fiwon123/eznit/issues/5) |

Also true today:

- **There are no automated tests.** `make check` compiles and vets; it executes
  nothing. Treat a green run as "it builds", not "it works".
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
on each checkout:

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

## License

MIT — see [LICENSE](LICENSE).
