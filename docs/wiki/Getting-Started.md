# Getting Started

This page runs eznit on your machine and puts one file in, then back out.

## What you need

- [Docker](https://docs.docker.com/get-docker/) with Compose — the quickest path,
  and the only thing the product stack needs.
- Or, for the host-toolchain path: Go (the version in `go.mod`) and Node (the pin
  in `mise.toml`). See [Development](https://github.com/fiwon123/eznit/wiki/Development) if you want the full loop.

You do **not** need Go or Node to run the product stack; the images build the UI.

## Run the stack

Two local files come first — the configuration and the database secret. Both are
gitignored, and `make setup` creates them from the committed examples:

```sh
make setup
```

Without `make`, copy `.env.example` to `.env` and
`secrets/db_password.txt.example` to `secrets/db_password.txt` by hand, then
`chmod 600 .env secrets/db_password.txt` (see [Configuration](https://github.com/fiwon123/eznit/wiki/Configuration)) —
`cp` under a normal umask leaves them world-readable. The database password is a
Docker file secret read through `DB_PASSWORD_FILE`; it is never an environment
variable.

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
5. starts the web UI on `http://localhost:5173`. It proxies `/v1/*` to the API, so
   the browser talks to one origin and the API needs no CORS headers.

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

## Your first file

The CLI talks to the API on `:4000`. Build it with Go on the host (or use a release
archive — see [Releases](https://github.com/fiwon123/eznit/wiki/Releases)):

```sh
make build_cli                  # needs Go; writes build/cli/linux/eznit and build/cli/windows/eznit.exe
./build/cli/linux/eznit --help
```

The core commands are interactive: they prompt for what they need.

```sh
./build/cli/linux/eznit signup     # Enter Email / Password / Confirm
./build/cli/linux/eznit login      # stores a session token in ~/.eznit/config.json (mode 0600)
./build/cli/linux/eznit upload     # file path: ./notes.txt
./build/cli/linux/eznit list       # the files you own
./build/cli/linux/eznit download   # id file: <id from list>  / destination folder path:
./build/cli/linux/eznit delete     # id file: <id>
./build/cli/linux/eznit logout     # removes the session token
```

`signup` and `login` are the only unauthenticated identity commands; everything
else that touches your own rows needs the session `login` stored. Full flags and
the templates/commands/repos commands are in [CLI Reference](https://github.com/fiwon123/eznit/wiki/CLI-Reference).

The web UI is at `http://localhost:5173`. Sign up or log in there, or paste the
token from `eznit login` into `localStorage` as `eznit.token`.

## From a release bundle (no source)

A release archive is **not** a source checkout: there is no `go.mod`, no `web/`,
and no image to build. Start the database and migrations from the published
images, then run the prebuilt server:

```sh
cp secrets/db_password.txt.example secrets/db_password.txt   # then edit it
cp .env.example .env                                         # then edit it
docker compose up db migrate

DB_HOST=localhost DB_PORT=5432 ./eznit-server               # serves the embedded UI on :4000
```

Set the database password for the host-run server too: `DB_PWD`, or
`DB_PASSWORD_FILE` pointing at `secrets/db_password.txt`. A .env without a
password would otherwise connect with an empty one.

Plain `docker compose up` is the source-checkout path — in a bundle it would try
to build an image the archive does not contain. `eznit-gui` opens a window on the
UI (`:8765`); with `GUI_API_URL` it proxies to a running API, otherwise it runs
one in-process when the `DB_*` variables are set. The bundle's `RUNNING.md` says
the same, and what it does not prove.

## Without Make

```sh
cp .env.example .env
cp secrets/db_password.txt.example secrets/db_password.txt
docker compose up
```

The dev loop (source, live reload) is a different entry point — see
[Development](https://github.com/fiwon123/eznit/wiki/Development).

## Next

- [Configuration](https://github.com/fiwon123/eznit/wiki/Configuration) — every variable, secret and port
- [CLI Reference](https://github.com/fiwon123/eznit/wiki/CLI-Reference) — the full command surface
- [Troubleshooting](https://github.com/fiwon123/eznit/wiki/Troubleshooting) — if a port is busy or a secret is missing
