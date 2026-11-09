# Configuration

eznit is configured through environment variables and two local files. The API
loads `.env.local` and `.env` through `godotenv` (`.env.local` overrides `.env`);
Compose sets the in-network values itself.

## API environment

| Variable | Default | Meaning |
| --- | --- | --- |
| `DB_HOST`, `DB_PORT`, `DB_USER`, `DB_NAME` | — | PostgreSQL connection |
| `DB_PASSWORD_FILE` | — | path to a file holding the password — **preferred** |
| `DB_PWD` | — | the password inline, for a throwaway local database |
| `API_UPLOADS` | (unset) | the upload root (where file bytes live); set it in `.env` — the example uses `./uploads`, and there is no built-in fallback |
| `API_LOGS` | `./logs` | where the API writes logs (rotating files, `lumberjack`) |
| `API_WEB_DIST` | `web/dist` | where to find a built UI, when one exists |
| `API_SERVE_WEB` | `true` | serve the UI from this process; the compose `api` and `dev` services set `false` |
| `API_PORT` | `4000` | the bind port |

> **`API_PORT` only moves the bind port.** It does not change what the CLI or the
> docs expect (`:4000`), and the compose file also uses it for the published host
> port. It is not a way to put the service behind a proxy without also changing the
> proxy target.

The password is a **Docker file secret** in the local stack:
`secrets/db_password.txt` is mounted at `/run/secrets/db_password`, and
`DB_PASSWORD_FILE` points at it. Prefer the file over `DB_PWD`; the latter puts
the password in the process environment.

## CLI environment

See [CLI Reference](https://github.com/fiwon123/eznit/wiki/CLI-Reference#cli-environment-variables) for `CLI_API_HOST`,
`CLI_API_PORT`, `CLI_DOWNLOADS`, `CLI_LOGS`, `EZNIT_CONFIG`, `EZNIT_REPOS` and
`EZNIT_GITHUB_API`.

## GUI environment

| Variable | Default | Meaning |
| --- | --- | --- |
| `GUI_API_URL` | — | proxy `/v1` to a running API instead of running one |
| `GUI_PORT` | `8765` | the window's own port |
| `GUI_LOGS` | `./logs/` | GUI log folder |

With `GUI_API_URL` set, the GUI is a window in front of an existing API. With no
`GUI_API_URL` and the `DB_*` variables set, it runs the API in its own process, so
one file is the whole product. With neither, the window opens and every `/v1`
call answers `503` naming the variable to set.

## Ports

| Role | Product (`make up`) | Dev loop (`make dev-up`) |
| --- | --- | --- |
| Frontend | `web` (nginx) — **5173** | `web-dev` (Vite) — **5173** |
| Backend | `api` — **4000** | `dev` (`make run_api`) — **4000** |
| Database | `db` — **5432** | `db` — **5432** |

The two entry points publish the same ports and are alternatives; each refuses to
start while the other runs and names the command that clears the way.

## Files and secrets

| File | Committed? | Holds |
| --- | --- | --- |
| `.env` | no (gitignored) | local configuration; `make setup` creates it from `.env.example` |
| `.env.local` | no (gitignored) | optional overrides; read after `.env` |
| `.env.example` | **yes** | placeholders only — safe to read |
| `secrets/db_password.txt` | no (gitignored) | the database password, as a Docker file secret |
| `secrets/db_password.txt.example` | **yes** | placeholder |

`.env`, `.env.local` and `secrets/db_password.txt` must be mode `600`. They are
gitignored, so this cannot be fixed by a commit — set it on each checkout
(`make setup` creates the first and the third with that mode already set):

```sh
chmod 600 .env .env.local secrets/db_password.txt
```

None of these is ever committed, logged or printed. The logger records a token
**fingerprint**, never a token.

## Example `.env`

The committed `.env.example` is the template; a minimal local file is:

```sh
API_PORT="4000"
API_UPLOADS="./uploads"
API_LOGS="./logs"

DB_HOST="db"
DB_PORT="5432"
DB_USER="root"
DB_NAME="eznit"

API_SERVE_WEB="true"
```

Compose overrides `DB_HOST`/`DB_PORT` for the in-network services (`db:5432`); the
dev sandbox deliberately does not import the host's `.env`, so a host-side
`DB_HOST` cannot point it at a database that does not exist on the Compose network.
