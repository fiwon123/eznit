# 🧯 Troubleshooting

The errors you are most likely to hit, and the fix.

## `ERROR: DB_USER, DB_HOST and DB_NAME are not set`

The migration targets need a database connection. Copy `.env.example` to `.env`,
fill it in, and try again. `make check` and `make build` do not need it.
`make setup` creates the file for you.

## `make check` passes, but nothing seems tested

`make check` prints the packages it did not cover at the end of a green run.
That list is short now — `cmd/cli` is the only package with no test file — but a
green `make check` still means "the tested packages passed and the rest
compiled", not "it works". SQL is covered by `make test_integration`, not by
`make check`.

## `template apply` stops without writing anything

One of the target files already exists. `apply` refuses to overwrite without
`--force`, and it checks every file before writing any of them. Move the
conflict, or rerun with `--force`.

## `template publish` skips a file, or the server refuses one

Symlinks are skipped (a template is files, not links), and the limits are 256 KiB
per text file, 4 MiB of text per template, 10 MiB per binary file, 32 MiB in
total and 200 files. The CLI checks them before uploading and names the file.

## `template apply <id>` answers 401

A private template needs a session: run `eznit login`. Public templates apply
without one.

## `eznit-gui` exits with a GL or display error

Fyne needs OpenGL at run time. The released Linux window links exactly two
libraries of its own — `libGL.so.1` and `libwayland-client.so.0` — so install
`libgl1` and `libwayland-client0` (Debian/Ubuntu) plus a working GL driver, and
run it on a machine with a display. It is a desktop window, not a server
component; a headless machine should use the CLI and the server's web UI.

Building it is a different requirement: `make build_gui` needs cgo and the GL/
X11 **headers**; if `pkg-config` cannot find `gl.pc`, the headers are missing.

## `version 'GLIBC_2.38' not found` from `eznit-gui` (v0.23.0 and earlier)

Those releases built the Linux window on a current runner, so the download
required glibc 2.38 — newer than Debian 12 (2.36) and Ubuntu 22.04 (2.35). The
window in **v0.23.1 and later** is linked inside a Debian 12 image and needs
only glibc **2.34**; use the CLI (`eznit`, `ez`) in the meantime, which is
`CGO_ENABLED=0` and unaffected.

## The Server tab says "not a server bundle"

The panel runs the `eznit-server` in the folder it points at, and your CLI
archive has no server binary in it. Download the **server** bundle for your
platform, put the window in that folder (or choose it with **Choose folder…**),
and start it from there.

## The Server tab says the port is in use

Something else already listens on `API_PORT` (4000 by default) — often a server
this window started in another session, or `make run_api`. Stop it, or change
the port in the configuration panel and save.

## The server starts and stops at once

Read the **Output** tab: the API prints why it exited. The usual cause is an
unreachable database — check `DB_HOST`, `DB_PORT` and that
`secrets/db_password.txt` exists (set it on the **Configuration** tab). The window
creates a missing `.env` from `.env.example` and points it at a local database.

## A stop takes a while

That is the drain: the API gives in-flight requests up to 10 seconds to finish.
On Windows the stop is immediate — Windows cannot deliver an interrupt to
another process, so the window kills it.

## `eznit update` refuses the download

The updater fetches the release's `checksums.txt` **before** the archive and
refuses a missing, malformed or mismatched entry. That is deliberate: an
archive that does not match its published hash is not installed. Check that the
release has both the archive and `checksums.txt`, and that the tag is
`vMAJOR.MINOR.PATCH`. Only HTTPS is accepted.

## `Install failed: checksums.txt has no entry`

Versions up to **v0.28.0** asked for an archive name the release stopped
publishing in **v0.24.0** (`eznit-cli_…` instead of `eznit_…`), so neither
`eznit update` nor the window's **Install** could work. The fix ships in
**v0.28.1**. A binary from v0.28.0 or earlier cannot fetch it — the name it
asks for is the bug — so upgrade once by hand: download
`eznit_v0.28.1_<os>_<arch>` from the release page, verify it against
`checksums.txt`, extract it, and run the binary from the extracted folder (or
replace the old one). From v0.28.1 on, the built-in update works again.

## The update installed, but `-v` still shows the old version

The running process keeps the binary it started from. Restart `eznit` (or the
window) to run the new one. `eznit update --revert` puts back the version saved
by the last install.

## The API starts and immediately exits

Check the log for the connection error. The API reads `DB_PASSWORD_FILE` first and
falls back to `DB_PWD`; Compose sets the former from the Docker secret. If you are
running the API by hand, set one of them.

## Permission denied reading a secret file

The file is mode `600` and owned by another user. See
[Configuration](https://github.com/fiwon123/eznit/wiki/Configuration#-files-and-secrets).

## `set: pipefail: invalid option name` (or `bad interpreter`) in the sandbox

The scripts run through the bind mount, so bash parses your working tree's bytes. A
checkout made before the `.gitattributes` rule was added keeps CRLF endings, and
`git status` stays clean while it does. Re-check the files out with LF endings:

```sh
git rm --cached -r . >/dev/null && git reset --hard
```

## `docker compose up` fails on a missing secret

The database secret is a file secret, so Compose refuses to start without it.
`make setup` creates it from the committed example.

## `EACCES: permission denied, mkdir '/home/devuser/.cache/opencode'`, or `mkdir /go/pkg/mod/cache: permission denied` in the sandbox

The sandbox's caches are Docker volumes, and Docker creates a mount destination the
image does not contain **as root** — while the container runs as your UID. The image
now creates those directories itself, so a sandbox built from the current
`Dockerfile` is fine.

A sandbox created **before** that fix keeps its old, root-owned volumes: Docker does
not re-initialise a volume that already exists. Recreate them once:

```sh
make dev-reset     # docker compose down --volumes
make dev-up
```

`make dev-reset` also drops the `postgres-data` and `api-data` volumes — the
database and the uploaded files — which is why it is not called `reset`. The
migrations run again on the next `make dev-up`.

## `bind: address already in use` on port 4000 or 5173

The product and the dev loop publish the same two ports, and they are alternatives.
Both entry points check before they start anything and say which one is running,
instead of failing halfway with Docker's message:

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

## The UI loads on `:5173` but every request fails

The Vite dev server proxies `/v1` to the API, so an API that is not running looks
like a broken UI: the page renders and the data does not. Check the API first —
`curl -s -o /dev/null -w '%{http_code}\n' localhost:4000/v1/healthcheck` — then
the sandbox log for a compile error (`make dev-log`). With `make dev-up` the log
stream is attached, so the error is usually on screen.
