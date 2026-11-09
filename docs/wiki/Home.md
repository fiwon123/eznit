# 🏠 eznit

**Self-hosted file storage you control.** A Go API, a CLI client and a web UI for
uploading, listing and retrieving your own files, backed by PostgreSQL.

- **Yours.** File bytes live on your disk, indexed in your own database. No
  subscription, no third-party account.
- **No browser needed.** The CLI puts a file back out with one command; the web UI
  is there when you want it. The API and the CLI are the whole product.
- **One stack.** `make setup && make up` runs the API, the UI, PostgreSQL and the
  migrations on a machine with Docker.

> **Pre-1.0 and unhardened.** Read the security status in the
> [README](https://github.com/fiwon123/eznit#security-status) before exposing this
> beyond `localhost`.

## 🚀 Start here

| Page | What it covers |
| --- | --- |
| [Getting Started](https://github.com/fiwon123/eznit/wiki/Getting-Started) | Run the stack and put your first file in and out |
| [CLI Reference](https://github.com/fiwon123/eznit/wiki/CLI-Reference) | Every command, flag, and the local config file |
| [Configuration](https://github.com/fiwon123/eznit/wiki/Configuration) | Every environment variable, secret and port |
| [Development](https://github.com/fiwon123/eznit/wiki/Development) | The dev loop, sandbox, migrations and the gates |
| [Troubleshooting](https://github.com/fiwon123/eznit/wiki/Troubleshooting) | The errors you are likely to hit, and the fix |
| [Releases](https://github.com/fiwon123/eznit/wiki/Releases) | How a release is cut and what the archives contain |

## 🔗 Also useful

- [README](https://github.com/fiwon123/eznit#readme) — the short project overview
- [ARCHITECTURE](https://github.com/fiwon123/eznit/blob/dev/ARCHITECTURE.md) —
  the request path, the data model, where the bytes live, where a change belongs
- [Roadmap board](https://github.com/users/fiwon123/projects/12) — what is next
  and what is deferred; the issues are the source of truth
- [Releases](https://github.com/fiwon123/eznit/releases) — downloadable archives
- [Contributing](https://github.com/fiwon123/eznit/blob/dev/CONTRIBUTING.md)

## 📖 About these pages

The wiki's source lives in the repository at `docs/wiki/`, so it is reviewed like
code and synced with `make wiki_push`. Edit a page there through a pull request
rather than only in the wiki UI, or the next sync will overwrite it.
