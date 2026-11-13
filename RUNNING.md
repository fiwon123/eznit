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

`eznit-gui` is in this bundle. Its **Server** tab runs `eznit-server` from this
folder as a child process, shows its log, and edits the `.env` and the database
password the server reads:

```sh
./eznit-gui        # then: Server tab → Start
```

Run it from **this** folder: the Server tab looks for `eznit-server` beside the
window, so a window copied somewhere else (or started from another folder) has
no server to run and will say so. **Choose folder…** helps only when the folder
it is pointed at is another bundle.

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
  started — keeps its own text. The grid itself cannot be selected, so **Copy
  log** puts the whole buffer on the clipboard and **Select…** opens a frozen
  snapshot in a dialog where a line or a range can be dragged and copied.
  **Follow output** stays on until you scroll away from the end; **Jump to
  end** goes back. The line next to the status always names the address,
  including after an exit (`Stopped at http://… — last exit: …`).
- **Stop** asks the server to shut down, which drains in-flight requests for up
  to 10 seconds. On Windows a stop is a kill: Windows cannot deliver an
  interrupt to another process.
- Closing the window stops a server this window started, after asking.
- **Open in browser** goes to `http://127.0.0.1:<API_PORT>`.
- The configuration panel writes `.env` (mode `0600`) and
  `secrets/db_password.txt` (mode `0600`). The password is never displayed, never
  logged, and never copied into `~/.eznit/config.json`. Its action row is the
  state of an edit: the password state when the form matches the file, and
  **`Unsaved changes: API_PORT 4000 → 5000; database password will be set.`**
  when it does not. A field that differs from the file is tinted and its caption
  takes the warning colour, so the mark is on the field as well as in the line.
  **Revert** reads the file again and discards the edit; **Save configuration**
  answers "nothing to save" when nothing changed.

The window is also a client: the **Files** tab signs in to one eznit API and
always shows which. Its header is an address bar, and the **Templates** tab has
the same one:

- **Use this machine** is the default and is remembered: checked, the field
  shows the server the Server tab would run, on the port that folder's
  configuration names, and is shut; unchecked, the field takes any address.
- **Connect** validates the address and switches to it, remembering it.
- **Test** asks the address `/v1/healthcheck` without saving it.
- The source line under the checkbox names where the address came from: this
  machine, `-api`, `GUI_API_URL`, or an address you typed.

The Server tab's folder is an editable field too: type a bundle folder and press
**Use** (or Enter), or pick one with **Browse…**. **Reset** returns to the folder
the window was installed in, after asking.

The address is resolved as `-api`, then `GUI_API_URL`, then the address saved in
`~/.eznit/config.json`, then `http://localhost:4000`. A session belongs to the
server that issued it: changing to a different host asks first, then signs you
out; a port change on one host keeps the session.

The **Templates** tab reads that server's catalogue: filter by kind or down to
your own templates, then select a row. The table is Name, Kind, Version and
Files; the selected template's summary adds its visibility, owner and
description, and three tabs — **Files**, **Variables**, **Commands**, each
labelled with its count — hold the contents, every line selectable. Two actions
sit beside the summary:

- **Save as bundle…** writes the `.eznit` file that `eznit template apply
  --from-file` reads; it is the same file `template export` produces, from the
  same code.
- **Apply…** writes the template into a folder you pick. It checks the folder
  first and lists any file it would overwrite before asking anything, then asks
  the template's variables (and any `{{PLACEHOLDER}}` its files use without
  declaring), refuses to write while a required answer is empty, and names every
  path it wrote — selectable, with **Copy paths**. The rules are shared with the
  CLI, so the two cannot disagree about what applying means.

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
