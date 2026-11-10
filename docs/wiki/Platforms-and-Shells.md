# 🖥️ Platforms and shells

What runs where, and what a template carries between them. The short version:
the released binaries cover Linux and Windows on amd64 and arm64; a template is
plain files, so the shell or editor it configures is your choice.

## 📦 Released archives

| Platform | CLI | Server bundle | Verified how |
| --- | --- | --- | --- |
| 🐧 Linux amd64 | `eznit-cli_<tag>_linux_amd64.tar.gz` | `eznit-server_<tag>_linux_amd64.tar.gz` | built |
| 🐧 Linux arm64 | `…_linux_arm64.tar.gz` | `…_linux_arm64.tar.gz` | built, never executed on ARM |
| 🪟 Windows amd64 | `…_windows_amd64.zip` | `…_windows_amd64.zip` | CLI started in CI |
| 🪟 Windows arm64 | `…_windows_arm64.zip` | `…_windows_arm64.zip` | built, never executed on ARM |
| 🍎 macOS amd64 | `…_darwin_amd64.tar.gz` | `…_darwin_amd64.tar.gz` | Mach-O checked, never executed |
| 🍎 macOS arm64 | `…_darwin_arm64.tar.gz` | `…_darwin_arm64.tar.gz` | Mach-O checked, never executed |

Every archive has `checksums.txt` on the release page. The Windows smoke test
unpacks the CLI, runs `eznit -v` and `ez -v`, and checks the version — that is
what "started in CI" means. The server bundle is built, not started.

**macOS** is cross-compiled on Linux and published for amd64 and arm64. The
release verifier reads each archive and checks the binary is a Mach-O
executable for the CPU the archive name claims; no gate starts one on a Mac.

## 🐳 The server

The product stack is Linux containers: `db`, `migrate`, `api`, `web`. On Windows
and macOS that means Docker Desktop running Linux containers. The stack is not
started by any CI job, on any OS, so "it runs there" rests on the Docker images,
not on a gate.

The server binary also runs directly on the host (`./eznit-server`), with
PostgreSQL reachable through `DB_*`. That is the release-bundle path in
[Getting Started](https://github.com/fiwon123/eznit/wiki/Getting-Started); it is
tested on Linux only.

`eznit-gui` is the native desktop app (Fyne) and ships in the CLI archive. It
talks to an API: set the server address in the app or `GUI_API_URL`. Building it
needs cgo and the platform's GL/X11 headers (`make build_gui`); at run time it
needs the platform's GL libraries on Linux.

## 🐚 Shells

eznit does not detect or edit your shell. A template is a folder of files, so
whatever you put in it is written exactly where you asked:

| Shell | Where you would keep the snippet |
| --- | --- |
| bash | `shell/bashrc.sh`, sourced from `~/.bashrc` |
| zsh | `shell/zshrc.sh`, sourced from `~/.zshrc` |
| fish | `shell/config.fish`, linked or copied into `~/.config/fish/` |
| PowerShell | `shell/profile.ps1`, dot-sourced from `$PROFILE` |

`eznit template init --kind` writes the shell starter too:

| Kind | Writes |
| --- | --- |
| `bash` | `shell/bashrc.sh` |
| `zsh` | `shell/zshrc.sh` |
| `fish` | `shell/config.fish` |
| `powershell` | `shell/profile.ps1` |

Each is a commented snippet — the project identity in a header, an example of
how to source it, and an example alias or function. It does nothing until you
uncomment it, which is deliberate: a tool that edits `~/.bashrc` unasked is a
tool people stop running. A shell starter writes only its shell file, not
`AGENTS.md`.

Applying a template **never** edits a file in your home directory unless you
point `--target` there, and it refuses to overwrite an existing file without
`--force`.

## ↔️ What survives a round trip

- **Paths** are stored relative with forward slashes and written with the local
  separator, so a template published on Linux lands as `shell\profile.ps1` on
  Windows.
- **Text** is UTF-8. Line endings are written as published: the tool does not
  normalise CRLF.
- **The executable bit** is stored per file and applied as mode `0755` on Unix.
  Windows has no execute bit — call the script through its interpreter or use
  `.cmd`/`.ps1`.
- **Binary files** are copied byte for byte; they are stored on the server's
  disk, not in PostgreSQL.
- **Symlinks** are skipped at publish time: a template is files, not a tree of
  links.

## 🧭 Not proven

- The macOS archives are built, never executed on a Mac.
- The ARM64 binaries are built, never executed on ARM hardware.
- The Windows server bundle is built, never started.
- The browser UI has no automated test on any platform.

See the README's
[What is not proven](https://github.com/fiwon123/eznit#-what-is-not-proven) for
the full list, and [Direction](https://github.com/fiwon123/eznit/wiki/Direction)
for what is planned.
