# 🧰 Configuration

eznit is configured through environment variables and two local files. The API
loads `.env.local` and `.env` through `godotenv` (`.env.local` overrides `.env`);
Compose sets the in-network values itself.

## 🔌 API environment

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

## 💻 CLI environment

See [CLI Reference](https://github.com/fiwon123/eznit/wiki/CLI-Reference#-cli-environment-variables) for `CLI_API_HOST`,
`CLI_API_PORT`, `CLI_DOWNLOADS`, `CLI_LOGS`, `EZNIT_CONFIG`, `EZNIT_REPOS` and
`EZNIT_GITHUB_API`.

## 🪟 Desktop app environment

The window is a client: it talks to an eznit API and runs no server.

| Variable | Default | Meaning |
| --- | --- | --- |
| `GUI_API_URL` | `http://localhost:4000` | the API to talk to; it wins over the address saved in the app, so a wrapper script keeps working after a login |
| `EZNIT_CONFIG` | `~/.eznit/config.json` | the shared store: the server address (`api_url`) and the session token |

The server address is also a field on the sign-in screen and is stored in the
config file, so the window and the CLI use the same file. The Files tab's header
always names the address in use and where it came from — the `-api` flag,
`GUI_API_URL`, the saved address, or the default — and the **Files** header
offering **This machine** (the port the Server tab's configuration names) or any
other address, with a health check before it is saved. A session belongs to the
server that issued it: changing hosts signs you out, changing ports on one host
does not. There is no `GUI_PORT`: the window has no HTTP server of its own.

## 🔒 Auth rate limiting

`signup` and `login` are throttled per IP and per submitted address; over the
limit answers `429` with a `Retry-After` header. The limiter is in-process — the
API and the GUI each enforce their own — so it is a per-instance guard, not a
distributed one.

| Variable | Default | Meaning |
| --- | --- | --- |
| `RATE_LIMIT_ENABLED` | `true` | turn the throttle on or off |
| `RATE_LIMIT_WINDOW` | `15m` | the window every limit is counted over |
| `RATE_LIMIT_LOGIN_IP` | `20` | login attempts per IP per window |
| `RATE_LIMIT_LOGIN_EMAIL` | `10` | login attempts per address per window |
| `RATE_LIMIT_SIGNUP_IP` | `10` | signups per IP per window |
| `RATE_LIMIT_SIGNUP_EMAIL` | `5` | signups per address per window |
| `TRUST_PROXY` | `false` | trust `X-Real-IP` / `X-Forwarded-For` for the client IP |

`TRUST_PROXY` is off by default, in code and in the compose services. Enable it
only when the API is reached **solely** through a proxy you control that sets
`X-Real-IP` — otherwise a direct client can forge the header and dodge the per-IP
limit (the per-address limit still applies, because the address is in the request
body). The compose product publishes `:4000` directly, so to use the forwarded
address safely there, bind that port to loopback first
(`127.0.0.1:4000:4000`). With `TRUST_PROXY` off behind the web tier, every web
request shares the proxy's address for the per-IP dimension.

A limit of `0` (or less) disables that one dimension; `RATE_LIMIT_ENABLED=false`
turns the throttle off entirely. The standalone GUI reads the same variables, so
it applies the same limits to its in-process API.

## 🌐 Ports

| Role | Product (`make up`) | Dev loop (`make dev-up`) |
| --- | --- | --- |
| Frontend | `web` (nginx) — **5173** | `web-dev` (Vite) — **5173** |
| Backend | `api` — **4000** | `dev` (`make run_api`) — **4000** |
| Database | `db` — **5432** | `db` — **5432** |

The two entry points publish the same ports and are alternatives; each refuses to
start while the other runs and names the command that clears the way.

## 🔐 Files and secrets

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

## 📝 Example `.env`

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
