# 💻 CLI Reference

The CLI talks to the eznit API and keeps its own state in a local config file. It
is one binary, `eznit`, with an alias `ez`.

```sh
eznit --help
eznit -v            # the build's version
eznit --debug <cmd> # debug-level logs
```

**The core commands are interactive** — they prompt for what they need rather
than take flags. Subcommands take flags, and several of them (`template
publish`/`show`/`apply`/`init`, `repo auth`) also prompt for values.

Jump to: [Templates](#-templates) · [Commands library](#-commands-library) ·
[Git hosts](#-git-hosts) · [Files and account](#-files-and-account) ·
[The config file](#-the-config-file)

## 🗂️ Files and account

| Command | Prompts / flags | What it does |
| --- | --- | --- |
| `eznit signup` | email, password, confirm | Create an account |
| `eznit login` | email, password | Start a session; stores the token in `~/.eznit/config.json` (mode 0600) |
| `eznit logout` | — | End the session (removes the token; leaves a git-host token in place) |
| `eznit upload` | file path | Upload a file |
| `eznit upload --update <id>` | file path | Replace the contents of one of your files |
| `eznit download` | file id, destination folder | Download one of your own files |
| `eznit list` | — | List your own files (`--me` selects the same list) |
| `eznit delete` | file id | Delete a file |
| `eznit share` | file id, confirmation | Make a file public to the instance |
| `eznit unshare` | file id | Make a public file private again (`eznit share --off` is the same) |

Every command that reads or writes your own rows needs a session: the API requires
the token, and the CLI sends the one `login` stored. The exceptions are `signup`,
`login`, and the public template catalogue (`template list`, `template show`, and
`template apply` of a public template). Shared files are readable anonymously
through the API and the web UI, but `eznit download` always needs a session.

A typical first pass:

```sh
eznit signup
eznit login
eznit upload      # file path: ./notes.txt
eznit list        # note the id
eznit download    # id file: <id>  / destination folder path: ./out
```

## 🧩 Templates

A template is a folder structure — agent instructions, prompts, editor config —
that can be published and applied to another project. Text files travel as they
are; binary files are sent as base64 and stored on disk by the server.

| Command | Flags | What it does |
| --- | --- | --- |
| `eznit template publish [folder]` | `--public`, `--commands <id>=<path>` (repeatable/comma-separated) | Publish a folder as a template; `--commands` pulls a library command in |
| `eznit template list` | `--kind <terminal>`, `--mine` | The catalogue: public templates and your own; `--mine` shows only yours |
| `eznit template show [id]` | — | One template, its files and its variables |
| `eznit template apply [id]` | `--target <dir>`, `--force` | Write a template into a folder |
| `eznit template apply --from-file <bundle>` | `--target <dir>`, `--force` | Write a bundle written by `template export`; no server, no session |
| `eznit template export [id]` | `-o <file>` (default `template.eznit`), `--force` | Write a template to one file |
| `eznit template init` | `--kind <terminal>`, `--target <dir>`, `--force` | Write a starter template locally (no server) |

**Offline bundles.** `template export` writes the whole template — files,
binary bytes and variable prompts — into one JSON file (`format: 1`). A command
the template pulls in is written at its target path, so the bundle is a
snapshot to apply, not a publish request. `apply --from-file` checks the same
path rules and limits as the server and asks the variables on the machine that
writes the files. A private template needs a session to export; a public one
does not.

`template init --kind` accepts `opencode`, `codex`, `copilot`, `cursor`,
`windsurf` or `generic` (default `generic`).

**Limits** (enforced by the server): 256 KiB per text file and 4 MiB of text per
template; 10 MiB per binary file and 32 MiB per template counting both.

## 📑 Commands library

A command is a file kept once in your library and pulled into templates by id, so
one copy is updated once.

| Command | Flags | What it does |
| --- | --- | --- |
| `eznit command add <file>` | `--name <name>`, `--not-executable`, `--replace` | Keep a script in your library |
| `eznit command list` | — | List your library |
| `eznit command show <name-or-id>` | — | Show one command |
| `eznit command remove <name-or-id>` | — | Remove a command; refused while a template still pulls it in |

```sh
eznit command add scripts/fmt.sh --name fmt
eznit template publish . --commands <command-id>=scripts/fmt.sh
```

A name is unique per user, not globally.

## 🐙 Git hosts

`eznit` can list the repositories a GitHub token can reach and clone one. The
token is a **personal access token** and it stays on your machine: it is kept in
`~/.eznit/config.json` next to the session token, and the eznit API never sees it.

| Command | Flags | What it does |
| --- | --- | --- |
| `eznit repo auth` | `--provider github`, `--token`, `--token-file`, `--remove` | Store a token (prompt by default); `--remove` forgets it |
| `eznit repo list` | `--provider github` | List the repositories the token can reach |
| `eznit repo clone <owner/name>` | `--provider github`, `--dir <path>` | Clone into `<dir>/github/<owner>/<repo>` |

The clone runs `git` directly, with the token supplied through the child process's
environment (`http.extraheader`), so it is never in the command line, never in the
URL, and never written into the clone's `.git/config`. The remote stays the plain
HTTPS URL. The residual exposure is that child's environment, which the same
user's other processes can read (`/proc/<pid>/environ`) — the cost of keeping the
token out of a file git owns. `eznit logout` removes the session token and leaves the host token in
place; `eznit repo auth --remove` removes that one.

## 🧾 The config file

`~/.eznit/config.json` (mode `0600`) holds the session token (`token`) and, if you
stored one, the git-host token (`github_token`). The two are written separately and
neither erases the other. `EZNIT_CONFIG` points the file elsewhere (a test or a
profile).

## 🌱 CLI environment variables

| Variable | Default | Meaning |
| --- | --- | --- |
| `CLI_API_HOST` | `http://localhost` | API host |
| `CLI_API_PORT` | `4000` | API port |
| `CLI_DOWNLOADS` | `./downloads/` | where `download` writes when you give no folder |
| `CLI_LOGS` | `./logs/` | CLI log folder |
| `EZNIT_CONFIG` | `~/.eznit/config.json` | the config file |
| `EZNIT_REPOS` | `~/eznit` | clone root when `repo clone` has no `--dir` |
| `EZNIT_GITHUB_API` | `https://api.github.com` | GitHub Enterprise API base |

The CLI also loads `.env.local` and `.env` from the working directory, so a
development checkout can point it at a non-default API with the same files the
server reads.
