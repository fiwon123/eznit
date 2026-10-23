# eznit — AI Agent Instructions

## Project Objective

Self-hosted file storage: a Go API and a CLI client for uploading, listing and
retrieving your own files, backed by PostgreSQL.

### Problem

Storing a file somewhere you control should not require a subscription or a
third-party account, and it should not require a web browser to get the file
back out.

### Goal

An API and a CLI that cover the whole loop — sign up, log in, upload, list,
download, update, delete — on a Compose stack a single developer can run.

### Main Technology

- Go, `net/http` + `chi`, `sqlx` + `lib/pq` over PostgreSQL
- `golang-migrate` for schema changes
- `kong` for the CLI, `zap` + `log/slog` for logging, `godotenv` for config
- Docker Compose for the dev sandbox and the local stack (`dev`, `db`,
  `migrate`; the `api` service is kept as a commented block)

### Core Architecture

```
cmd/api/         API entrypoint: flags, logger, router, graceful shutdown
cmd/cli/         CLI entrypoint and one file per command
web/             React + TypeScript + Vite web UI; built to web/dist and served by the API
internal/domain/ one package per resource (users, sessions, files):
                 handler -> service -> repository, plus dto.go
internal/platform/
                 middleware (session guard), sql (connection)
pkg/             logger, errors, http/json helpers, path helpers, types
db/migrations/   golang-migrate SQL, one up/down pair per change
secrets/         Docker secret files (the real one is gitignored)
```

`ARCHITECTURE.md` is the full map: the request path, the data model, where the
file bytes live, how the UI reaches the browser, and where a change belongs.

## Key Conventions

- **Assignee**: `ASSIGNEE` is `fiwon123` — `workflow.md` tells agents to read
  `ASSIGNEE` from here before assigning issues and PRs
- **Two-tier branch model**: `main` (release only) ← `dev` (integration) ←
  feature branches
- **Issue-first workflow**: No code without an issue — open/organize, create
  branch, build, test, PR
- **Conventional commits**: `feat:`, `fix:`, `refactor:`, `docs:`, `test:`,
  `chore:`, `ci:`
- **Layering is strict**: a handler decodes and writes, a service holds the
  rules, a repository is the only place that speaks SQL
- **Migrations are append-only**: never edit an applied migration; add a new
  numbered pair. A migration that has run anywhere is history
- **Model assignments**: `opencode.json` is the source of truth — it assigns
  `opencode/big-pickle` to `build`, `opencode/mimo-v2.6-flash-free` to `plan`,
  and these to the subagents. Verify a new id with `opencode models` before
  assigning it: a catalog entry on models.dev does not guarantee the installed
  build offers it, and an unresolvable id fails at spawn rather than at config
  load.

## Runtime Environment

The AI agent (opencode) runs at the repository root. There are two modes, and
they are not equivalent — say which one a claim was made in.

### Host mode (the default)

The agent runs directly on the host, against the tools installed on the
machine. This is what `make check` and `git` operate on.

### Sandbox mode (optional)

`make dev-opencode` runs the agent inside the dev container, where the
toolchain is pinned to Go 1.24.1 — the version in `go.mod`, the same one CI
uses. The workspace is bind-mounted at `/workspace`; edits made inside are the
same files on the host.

The sandbox shares the Compose network with the `db` service, so the API
started inside it (`make run_api`) reaches the database at `db:5432` with the
stock `.env` — no `.env.local` override needed, and no password on the command
line: the container reads the same file secret (`/run/secrets/db_password`) the
API container reads.

The container has **no Docker socket**: the agent inside it cannot manage
containers, and that is deliberate. `docker` commands fail there, and a claim
about the sandbox made from inside it cannot be about the host's Docker.

The in-container agent needs a **Linux** opencode binary (the container is
Linux). On macOS and native Windows, `make dev-opencode` refuses with a clear
message and the agent stays on the host; use WSL2, where the host binary *is*
Linux.

### What the agent CAN do

- Run Go commands (`go build`, `go test`, `go vet`, `go run`, `gofmt`)
- Run `make` targets
- Run git and GitHub CLI commands
- Read and edit any file in the repository except the secret files listed below
- Start and use the dev sandbox (`make dev-up`, `make dev-shell`) from the host

### What the agent CANNOT do

- Access `.env`, `.env.*`, `.envrc`, `secrets/db_password.txt`, or any other
  secret file without explicit permission
- Run commands that dump environment values (`printenv`, `env`)
- Run `docker` from **inside** the sandbox (no socket is mounted)
- Push directly to `dev` or `main`
- Merge a pull request without explicit instruction

## Development Commands

```bash
make check     # gofmt, vet, test, build — the local verification gate
make run_api   # run the API against the configured database
make run_cli   # run the CLI
make build     # build both binaries into ./build
make web_check # lint, format check, typecheck and build the web UI
make web_build # build the web UI into web/dist
make web_dev   # run the Vite dev server
```

`make check` needs no database, no container and no secret file: it compiles
the API and the CLI, it does not run them. It fails loudly if the Go toolchain
is missing rather than skipping, and it lists the packages that have no test
files, because `go test` prints `[no test files]` and still exits 0.

### Dev sandbox

```bash
make setup      # one-time: create .env and the database secret from their examples
make dev-up     # start the sandbox + the local stack (db, migrate)
make dev-shell  # a shell inside the sandbox
make dev-down   # stop it (keeps the volumes)
```

`docker compose up` does the same thing without Make, and
`python scripts/dev-up.py dev-up` is the native-Windows path. The sandbox needs
the local files `make setup` creates — the database secret is a file secret, and
Compose refuses to start without it.

The image creates the directories the compose file mounts into, owned by the
runtime user: Docker creates a mount destination the image does not contain as
**root**, and the container runs as `${HOST_UID}`, so a sandbox whose image
omits them cannot write its own caches — the agent dies with
`EACCES: permission denied, mkdir '/home/devuser/.cache/opencode'` and the
build fails with `mkdir /go/pkg/mod/cache: permission denied`. A sandbox
created before that fix keeps its old root-owned volumes — Docker does not
re-initialise a volume that already exists — so it needs `make dev-reset` once,
which also drops the local database.

A claim about the sandbox is only as good as the run behind it. The stack was
run on a machine with Docker for this change: PostgreSQL came up healthy, the
migrations exited 0, `make check` completed inside the container, and the
agent answered from the project's own `opencode.json`. Anything that needs a
live PostgreSQL is still **unverified** until you run it yourself — the point
is that the sandbox can now be run at all, which it could not before.

## Known gaps — read this before trusting a green run

These are tracked issues, not surprises for a reviewer to find:

| Gap | Issue | Why it matters |
| --- | --- | --- |
| The users, sessions and middleware packages have no tests | — | `make check` compiles and vets them; it executes nothing |
| The list of every upload is one catalogue for all users | — | A token is required, but any user sees every other user's file names |

**The files domain, the CLI and the path helper have tests; the rest of the
repository does not.** `go test ./...` prints `[no test files]` for the
packages without one, and still exits 0. A green `Test` step in CI means
"the packages with tests passed" — name the packages when you report a
result, because a run that executed nothing is not coverage.

## Git Workflow

- Branch from `dev`, never from `main`
- Never push directly to `dev` or `main`
- Branch naming: `<type>/<issue-number>-<slug>` (e.g., `fix/4-download-auth`)
- Branch types: `feat/`, `fix/`, `refactor/`, `docs/`, `test/`, `chore/`, `ci/`
- Use conventional commits
- Do NOT merge pull requests unless explicitly instructed
- Do NOT create release PRs or merge to `main` — the user must ask
- Always return to `dev` after completing a merge
- Every change is tracked on GitHub: **issue → branch → PR → merge**

### Branch Strategy

```text
main          ← release merges
  └── dev     ← integration branch (all feature branches merge here)
        ├── fix/4-download-auth
        └── chore/2-opencode-agent-layer
```

### Branch gate

`.github/workflows/branch-gate.yml` fails a pull request whose branch does not
follow the model above, so the author finds out before a reviewer does. It is a
**guard rail, not a lock**: an administrator can override a failed required
check, and a check nobody blocks on is noise. Branch protection (below) is the
control; the gate is the feedback.

### Repository settings (configure by hand)

These live in the repository settings, not in a file, so nothing in this repo
can set them for you.

**Branch protection** (Settings → Branches → Rules):

- `dev`: require a pull request, and require the **Branch gate** check. Do
  *not* require the CI check here — CI runs on release pull requests only, so
  a required check that never runs leaves every pull request waiting forever.
- `main`: require a pull request, and require both **CI (Go)** and **Branch
  gate**. Do not allow deletions: `dev` is the head of the release pull
  request, so deleting it with `--delete-branch` breaks every future pull
  request's base.

**Other settings worth turning on:**

- Secret scanning and push protection (Settings → Code security). A pushed
  token stays in the history even after it is deleted — and this repository
  handles database credentials and session tokens directly.
- Tag protection for `v*`, so a published release tag cannot be moved.

### Milestones

- Every issue MUST be assigned to a milestone before work begins
- Use `gh issue edit <number> --milestone "<milestone-name>"`

### Seed data

`make docker_up` runs the migrations against the local database; it does not
seed anything. (`make docker_up_all` additionally starts the dev sandbox.) The
first `signup` through the CLI or the API creates the first user. There is no
admin account and no way to promote one from the CLI yet.

## Agents

Agent definitions live in `opencode.json` and `.opencode/agents/`:

- **build** (primary): Full development work with all tools enabled.
- **plan** (primary): Analysis and planning without making changes.
- **backend** (subagent): Backend work — Go, SQL, migrations. Its edit scope is
  `cmd/api/**`, `internal/**`, `db/**` and `pkg/**`; it does not touch
  `cmd/cli/**`.
- **reviewer** (subagent): Reviews code for bugs, security, and regressions.
- **visual** (subagent): Reviews the web UI for layout, accessibility,
  console errors and regressions; can capture screenshots against the local
  stack.

Each agent's instructions live in exactly one place: the body of
`.opencode/agents/<name>.md`. Model assignments, permissions, and mode live in
`opencode.json`. Do not duplicate the prompt into `opencode.json` — two prompt
sources for one agent drift apart, and nothing states which one wins.

## Skills

- `github-workflow` (`.opencode/skills/github-workflow/SKILL.md`) is the
  command cookbook for branches, issues and PRs. `workflow.md` remains the
  policy authority.

## Session Memory (4-tier)

- At the START of every new chat: read `.opencode/memory/global.md` then
  `.opencode/memory/current.md` and resume from "Next action" — this is how
  long-running work survives between sessions. If the state references an
  issue/branch/PR, continue that work unless the user says otherwise.
- At meaningful milestones (issue created, branch renamed, code done, tests
  run, PR opened, merged) update `global.md`; update `current.md` after each
  step and at session end rotate the oldest archive out so only 3 remain.
- Durable, reusable lessons go to `lessons.md`. Keep sizes bounded:
  `global.md` ≤ ~200 lines, `lessons.md` ≤ ~100 lines, `current.md` ≤ ~100
  lines, archives ≤ ~50 lines.
- The whole `.opencode/memory/` directory is gitignored and must never be
  committed. Full rules: `.opencode/instructions/memory.md`.

## Environment and Secret-File Rules

- Never access `.env`, `.env.*`, `.envrc`, `secrets/db_password.txt`, or any
  other environment/secret file without explicit permission.
- Never open, read, print, summarize, quote, or send the contents of such a
  file. `.env.example` and `secrets/db_password.txt.example` are the
  exceptions: they hold placeholders and are safe to read.
- Read those with the **Read tool**, not with `cat`. Bash denies cover every
  `.env*` path and the `secrets/` directory on purpose.
- Never run commands that reveal environment values — `printenv`, `env`,
  `docker compose exec SERVICE env`, `docker compose run SERVICE env`,
  `docker compose config` (it renders the *interpolated* values).
- Never display, repeat, log, store, or include secret values in responses,
  code changes, or commits. This includes session tokens: log a fingerprint,
  never the token. `helper.Fingerprint` exists for this.
- To confirm a variable is set, check for presence, never print it: run
  `test -n "$VAR" && echo set`.

### Enforcement and its limits

`opencode.json` enforces the rules above at the tool level: the top-level
`permission` block denies `read`/`edit` of `*.env`, `*.env.*`, `*.envrc` and
`secrets/*` while allowing the `.example` files, and denies `bash` commands
that match those paths or dump the environment. Every agent inherits it.

Treat it as a backstop, not as the permission. The rules match **command
strings and file paths**, not intent:

- an allowed interpreter can still reach a file: `go run ./x -f .env` matches no
  deny pattern;
- a repo-wide `grep -rn SECRET .` ignores `.gitignore`; prefer the Grep tool,
  which is gitignore-aware;
- rules are evaluated last-match-wins, with agent rules beating global ones, so
  a broad allow placed after a narrow deny reinstates the leak;
- a `bash` pattern matches the whole command string and one command can name
  several files, so there is no way to allow `.env.example` while denying `.env`
  in the same namespace. That is why the example file is carved out of
  `read`/`edit` — which match one path per call — but not out of `bash`.

If the user explicitly permits reading environment configuration: read only
non-secret variables, never reveal values whose names contain `KEY`, `TOKEN`,
`SECRET`, `PASSWORD`, `PASS`, `CREDENTIAL`, `AUTH`, `PRIVATE`, `CERT`,
`COOKIE`, `WEBHOOK`, `DATABASE_URL` or `CONNECTION_STRING`, and prefer checking
that a variable exists over printing it.

### Local permissions

`.env`, `.env.local` and `secrets/db_password.txt` must be mode `600`. They are
gitignored, so this cannot be enforced by a commit — set it on each checkout:

```sh
chmod 600 .env .env.local secrets/db_password.txt
```

### Layering and migrations

- Keep handler → service → repository. A handler that queries the database, or
  a repository that returns an HTTP status, is a layering break worth a review
  comment.
- Never edit a migration that has run somewhere. Add a new numbered up/down
  pair; `db/migrations/` is append-only history.
- Constraints belong in the schema where possible, not only in Go: a `UNIQUE`
  that exists in Postgres cannot be forgotten by a new code path.

### Rules for Agents

All workflow rules live in `.opencode/instructions/workflow.md`. This project
demonstrates the pattern other projects should adopt:

- branch strategy and naming conventions;
- CI strategy — `.github/workflows/ci.go.yml` runs on release PRs (`dev` →
  `main`) and manual dispatch, not on feature-branch PRs, which are gated by
  local `make check`;
- milestone and issue conventions;
- development commands;
- environment and secret-file rules.

## CI triggers (read this before claiming a change is verified)

CI here runs on release PRs (`dev` → `main`) and manual dispatch — never on a
feature-branch push. Two things can make a run verify nothing:

- a `paths:` filter the change does not touch means the workflow does not
  trigger at all; and
- a job gated behind a label or `if:` comes back `skipped`, not `passed`.

**Skipped is not passed.** A run that never happened, or whose jobs were all
gated out, is not coverage. Do not describe a change as "CI passed" unless a
run actually executed the job that covers it; if it did not, say the change is
unverified and test it locally.
