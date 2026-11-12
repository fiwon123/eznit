# 🧭 Direction

This page is the one place a promise is allowed. Planned work is marked **not
implemented**, with its issue; what has landed is listed as shipped. The issue
is the source of truth for both.

The principle behind the list: one file or one command per step, and the offline
paths need no account. A setup tool that requires a server to hand a file to the
machine next to it is solving the wrong problem.

## ✅ Shipped

- **Offline template bundles** — `eznit template export <id> -o setup.eznit`
  writes a template and its files to one JSON file, and
  `eznit template apply --from-file setup.eznit` writes it on a machine with no
  server and no account ([#184](https://github.com/fiwon123/eznit/issues/184),
  v0.14.0). The same path rules and limits are checked on read, and values are
  asked locally.
- **Crower packs** — `eznit command export --crower pack.toml` and
  `eznit command import --crower pack.toml` move the command library between
  eznit and [crower](https://github.com/fiwon123/crower)
  ([#185](https://github.com/fiwon123/eznit/issues/185), v0.15.0). A script maps
  to `Exec`; `Steps` win on import, as they do in crower. Aliases, env, shell,
  cwd and timeout are reported and not kept; binary commands cannot be exported.
- **macOS archives** — `darwin/amd64` and `darwin/arm64` archives were published
  and verified as Mach-O, cross-compiled on Linux
  ([#187](https://github.com/fiwon123/eznit/issues/187), v0.17.0). No gate runs
  them on a Mac.
- **Shell starters** — `eznit template init --kind bash|zsh|fish|powershell`
  writes `shell/bashrc.sh`, `shell/zshrc.sh`, `shell/config.fish` or
  `shell/profile.ps1`: commented snippets you source yourself
  ([#186](https://github.com/fiwon123/eznit/issues/186), v0.16.0). Templates can
  still carry any shell file, starter or not.
- `eznit template init` writes a starter with no server and no account.
- The command library keeps a script once and pulls it into templates by id.
- **GitHub, GitLab and Bitbucket** tokens on your machine list and clone
  repositories ([#188](https://github.com/fiwon123/eznit/issues/188), v0.18.0);
  a token never reaches the API.
- A git-host token stays on the CLI machine, one config key per host.
- **The desktop window names its server** — the Files tab shows the address it
  talks to and where that address came from, and **Change…** switches between
  the server this machine runs and any other address, testing it first
  ([#248](https://github.com/fiwon123/eznit/issues/248), v0.25.0). A different
  host signs you out, because a session belongs to the server that issued it.
- **Templates are browsable from the window** — a Templates tab lists the
  catalogue (filtered by kind or to your own), shows one template's files,
  variables and commands, and writes the offline `.eznit` bundle the CLI reads
  ([#260](https://github.com/fiwon123/eznit/issues/260), v0.26.0). Applying a
  template into a folder from the window is still planned.

## 📱 Device-to-device over cthrone — not implemented

No issue yet — the most work of anything here.

[cthrone](https://github.com/fiwon123/cthrone) pairs two devices with no account
and no server. The plan is to send a template bundle over it, so a second
machine (a laptop, a tablet, a machine on the same network) receives a setup
without either side running eznit's API.

If a feature on this page would change how you use eznit today, open the issue
and say so — the plan is a direction, not a schedule.
