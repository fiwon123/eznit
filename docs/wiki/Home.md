# 🏠 eznit

**Your project's setup, in one place you host.** Templates, scripts and files you
pull onto any machine — self-hosted, no subscription, no third party.

- **One place for the files a project needs.** Agent instructions, editor rules,
  skills, scripts, any text or binary file.
- **Publish once, apply anywhere.** A folder becomes a template; the next
  machine gets `eznit template apply <id>`.
- **With or without AI.** Starters for opencode, Codex, Copilot, Cursor and
  Windsurf; plain shell and editor configuration travels too.
- **Yours.** Bytes on your disk, your PostgreSQL, no third-party account.

> **Pre-1.0 and unhardened.** Read the security status in the
> [README](https://github.com/fiwon123/eznit#-security-status) before exposing
> this beyond `localhost`.

## 🚀 Start here

| Step | Command | What happens |
| --- | --- | --- |
| 1. Run it | `make setup && make up` | Docker builds and starts the API, the UI, PostgreSQL and the migrations |
| 2. Write a starter | `eznit template init --kind opencode --target ./demo` | Files appear in `./demo`; no server, no account |
| 3. Create an account | `eznit signup` | The first account on the instance is its admin |
| 4. Publish it | `eznit template publish ./demo --public` | The folder is in the shared catalogue |
| 5. Apply it elsewhere | `eznit template apply <id> --target ../other-project` | The files are written; the variables are asked here |
| 6. Take it offline | `eznit template export <id> -o setup.eznit` | One file; `apply --from-file` needs no server or account |

The full walkthrough, including the release-bundle and no-Make paths, is on
[Getting Started](https://github.com/fiwon123/eznit/wiki/Getting-Started).

## 🧭 I want to…

| I want to… | Read |
| --- | --- |
| try it in five minutes | [Getting Started](https://github.com/fiwon123/eznit/wiki/Getting-Started) |
| publish and apply a template | [Templates and skills](https://github.com/fiwon123/eznit/wiki/Templates-and-Skills) |
| look up a command or flag | [CLI Reference](https://github.com/fiwon123/eznit/wiki/CLI-Reference) |
| configure every variable and secret | [Configuration](https://github.com/fiwon123/eznit/wiki/Configuration) |
| know what runs on which OS or shell | [Platforms and shells](https://github.com/fiwon123/eznit/wiki/Platforms-and-Shells) |
| see what is planned | [Direction](https://github.com/fiwon123/eznit/wiki/Direction) |
| develop or cut a release | [Development](https://github.com/fiwon123/eznit/wiki/Development) · [Releases](https://github.com/fiwon123/eznit/wiki/Releases) |
| fix an error | [Troubleshooting](https://github.com/fiwon123/eznit/wiki/Troubleshooting) |

## 🔗 Also useful

- [README](https://github.com/fiwon123/eznit#readme) — the short project overview
- [ARCHITECTURE](https://github.com/fiwon123/eznit/blob/dev/ARCHITECTURE.md) —
  the request path, the data model, where the bytes live, where a change belongs
- [Roadmap board](https://github.com/users/fiwon123/projects/12) — planned work;
  the issues are the source of truth
- [Releases](https://github.com/fiwon123/eznit/releases) — downloadable archives
- [Contributing](https://github.com/fiwon123/eznit/blob/dev/CONTRIBUTING.md)

## 📖 About these pages

The wiki's source lives in the repository at `docs/wiki/`, so it is reviewed like
code and synced with `make wiki_push`. Edit a page there through a pull request
rather than only in the wiki UI, or the next sync will overwrite it.
