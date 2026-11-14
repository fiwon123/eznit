# 🚀 Getting Started

Two paths. **Try it** runs the server and puts one file in and out. **Use it on
a project** needs no server at all: `template init` writes a starter into a
folder.

## ✅ What you need

- [Docker](https://docs.docker.com/get-docker/) with Compose — the only
  requirement for the stack.
- Go (the version in `go.mod`) — only if you build the CLI or run from source.
- Or neither: a release archive carries a prebuilt CLI and server.

## 🐳 Try it: run the stack

Two local files come first — the configuration and the database secret. Both are
gitignored, and `make setup` creates them from the committed examples:

```sh
make setup
make up             # or: docker compose up
```

`make up` builds the two images (`web/Dockerfile` for nginx, `Dockerfile.api` for
the API), waits for PostgreSQL to be healthy, runs the `migrate` service once,
then starts the API on `:4000` and the UI on `:5173`.

| Role | Where | Port |
| --- | --- | --- |
| Web UI (nginx) | `web` service | **5173** |
| API | `api` service | **4000** |
| API reference (Swagger UI) | `/docs` on either | — |
| PostgreSQL | `db` service | 5432 |

Without `make`, copy `.env.example` to `.env` and
`secrets/db_password.txt.example` to `secrets/db_password.txt`, then
`chmod 600 .env secrets/db_password.txt`, and run `docker compose up`.

Uploaded bytes live on the `api-data` volume, so they survive
`docker compose down` (`down -v` removes them too).

## 📄 Put one file in and out

The CLI is a separate binary. Build it with `make build_cli` (Go only) or use a
release archive — see [Releases](https://github.com/fiwon123/eznit/wiki/Releases).

```sh
./build/cli/linux/eznit signup     # Enter Email / Password / Confirm
./build/cli/linux/eznit login      # stores a session token in ~/.eznit/config.json (0600)
./build/cli/linux/eznit upload     # file path: ./notes.txt
./build/cli/linux/eznit list       # the files you own
./build/cli/linux/eznit download   # id file: <id>  / destination folder path: ./out
```

The core commands prompt for what they need. `signup` and `login` are the only
unauthenticated identity commands; everything else that touches your own rows
needs the session.

## 🧩 Use it on a project

This path needs no server, no login and no Docker. It writes files into a folder
you name:

```sh
eznit template init --kind opencode --target ./demo
```

`--kind` accepts `opencode`, `codex`, `copilot`, `cursor`, `windsurf` or
`generic`; the starter asks for the project name, description and language and
fills them in. What each kind writes is on
[Templates and skills](https://github.com/fiwon123/eznit/wiki/Templates-and-Skills).

To share that folder and pull it elsewhere, publish it to a running instance:

```sh
eznit template publish ./demo --public    # prompts name, kind, description, defaults
eznit template list                       # note the id
eznit template apply <id> --target ../another-project
```

`apply` asks the variables on the machine that writes the files — a value you
type never travels to the server.

To carry the template to a machine with no server at all, export it to one file:

```sh
eznit template export <id> -o setup.eznit
eznit template apply --from-file setup.eznit --target ../another-project
```

## 📦 From a release bundle (no source)

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
`DB_PASSWORD_FILE` pointing at `secrets/db_password.txt`. `RUNNING.md` in the
bundle says the same, and what it does not prove. Plain `docker compose up` is
the source-checkout path — in a bundle it would try to build an image the
archive does not contain.

The bundle also carries `eznit-gui`. Run it **from inside the extracted folder** —
the Server tab looks for `eznit-server` beside the window. The first run is:

1. **Server → Configuration**: set the database password and **Save** (it writes
   `.env` and `secrets/db_password.txt`). This is the one step Start cannot
   invent: Postgres applies its password when it creates the volume;
2. **Server → Start**: the tab brings up the database and applies the migrations
   through the folder's own Compose services when nothing answers, then starts
   the API. The Output tab shows the log; the status dot turns green;
3. **Files**: **Use this machine** is checked by default, so **Test**, then
   **Connect**, and create the first account — it is the instance's admin.

If you prefer the terminal, `docker compose up -d db migrate` in that folder does
the same thing, and **Start** only runs the migrations.

The window is also a client of a server somewhere else, and that needs no bundle:
type the address into the **Files** header, **Test** it, **Connect**. The Server
tab is not involved.

`RUNNING.md` in the bundle says the same, and what it does not prove.

## 👉 Next

- [Templates and skills](https://github.com/fiwon123/eznit/wiki/Templates-and-Skills) — what to put in a template and how to share it
- [CLI Reference](https://github.com/fiwon123/eznit/wiki/CLI-Reference) — the full command surface
- [Configuration](https://github.com/fiwon123/eznit/wiki/Configuration) — every variable, secret and port
- [Troubleshooting](https://github.com/fiwon123/eznit/wiki/Troubleshooting) — if a port is busy or a secret is missing
