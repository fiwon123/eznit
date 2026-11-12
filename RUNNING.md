# Running a release bundle

This folder is a **release archive**: the client (`eznit`, `ez`), the API
(`eznit-server`), the window (`eznit-gui`), the migrations and the Compose
services that give the API a database. It is not a source checkout — there is no
`go.mod`, no `web/` and no image to build.

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

`eznit-gui` is in this bundle — the same window the CLI archive carries. Its
**Server** tab runs `eznit-server` from this folder as a child process, shows
its log, and edits the `.env` and the database password the server reads:

```sh
./eznit-gui        # then: Server tab → Start
```

- **Start** checks the API port and reads the `.env` before it runs anything. A
  folder with no `.env` gets one created from `.env.example`, pointed at
  `127.0.0.1` and at this bundle's secret file.
- **Choose folder…** points the tab at another bundle and remembers the choice
  in `~/.eznit/config.json`. **Reset to default** asks first, then forgets the
  choice and returns to the folder this window was installed in — so a moved
  bundle picks its own folder again. Both are refused while the server runs: the
  process belongs to the folder it was started from.
- The tab has two views: **Output** is the server's own log, and
  **Configuration** is the form that writes `.env` and the password file. Start
  switches to Output, because that is where the answer is.
- The log is one row per line of output: time, level, message, then any
  structured fields, with the level coloured (warnings and errors bold). A line
  that is not a log record — a panic, a plain message from a tool the server
  started — keeps its own text. **Copy log** puts the whole log on the
  clipboard. **Follow output** stays on until you scroll away from the end;
  **Jump to end** goes back.
- **Stop** asks the server to shut down, which drains in-flight requests for up
  to 10 seconds. On Windows a stop is a kill: Windows cannot deliver an
  interrupt to another process.
- Closing the window stops a server this window started, after asking.
- **Open in browser** goes to `http://127.0.0.1:<API_PORT>`.
- The configuration panel writes `.env` (mode `0600`) and
  `secrets/db_password.txt` (mode `0600`). The password is never displayed, never
  logged, and never copied into `~/.eznit/config.json`.

The window is also a client: the **Files** tab signs in to one eznit API and
always shows which. Its header names the address and where it came from, a dot
reports whether that server answered, and **Change…** offers two choices:

- **This machine** — the server the Server tab would run, on the port that
  folder's configuration names, so the two tabs cannot disagree.
- **Another server** — any `http://` or `https://` address, validated and
  testable with **Test** (a `/v1/healthcheck` call) before it is saved.

The address is resolved as `-api`, then `GUI_API_URL`, then the address saved in
`~/.eznit/config.json`, then `http://localhost:4000`. A session belongs to the
server that issued it: changing to a different host asks first, then signs you
out; a port change on one host keeps the session.

The **Templates** tab reads that server's catalogue: filter by kind or down to
your own templates, then select one to see its files, the variables it asks for
and the commands it pulls in — every line selectable. **Save as bundle…** writes
the `.eznit` file that `eznit template apply --from-file` reads; it is the same
file `template export` produces, from the same code.

```sh
GUI_API_URL=http://<server>:4000 ./eznit-gui
```

The window is a GUI binary: it needs `libGL.so.1` and `libwayland-client.so.0`
(on Debian/Ubuntu: `libgl1 libwayland-client0`, plus a working GL driver) and a
glibc of at least **2.34** (Ubuntu 22.04, Debian 12) — the Linux windows are
linked inside a Debian 12 image for that reason. The server binary has no such
requirement: it is `CGO_ENABLED=0` and runs wherever a Go binary runs. There is
no headless mode; `--help` and `--version` exit before the toolkit starts, which
is what lets CI smoke the download.

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
