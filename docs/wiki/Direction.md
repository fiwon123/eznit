# 🧭 Direction

This page is the one place a promise is allowed. Everything below is **planned,
not implemented** unless it says otherwise, and the issue is the source of
truth for whether it has landed.

The principle behind the list: one file or one command per step, and the offline
paths need no account. A setup tool that requires a server to hand a file to the
machine next to it is solving the wrong problem.

## 📦 Offline template bundles

**Not implemented** — [#184](https://github.com/fiwon123/eznit/issues/184).

Today a template moves between machines only through a running instance. The
plan is a single-file bundle: `eznit template export <id> -o starter.eznit`, and
`eznit template apply --from-file starter.eznit` on the other machine. The same
limits, the same path rules, no server and no session.

## 🐦 Crower packs

**Not implemented** — [#185](https://github.com/fiwon123/eznit/issues/185).

[crower](https://github.com/fiwon123/crower) keeps shell aliases and multi-step
commands in shareable TOML packs; eznit keeps scripts in a command library that
templates pull in. They hold the same thing. The plan is
`eznit command import|export --crower pack.toml`, so a workflow is written once
and used by both.

## 📱 Device-to-device over cthrone

**Not implemented, no issue yet** — the most work of anything here.

[cthrone](https://github.com/fiwon123/cthrone) pairs two devices with no account
and no server. The plan is to send a template bundle over it, so a second
machine (a laptop, a tablet, a machine on the same network) receives a setup
without either side running eznit's API.

## 🐚 More shells and OSes

- **Shell starter kinds** — `eznit template init --kind bash|zsh|fish|powershell`
  — not implemented ([#186](https://github.com/fiwon123/eznit/issues/186)).
  Templates can carry shell files today; only the built-in starter is missing.
- **macOS archives** — `darwin/amd64` and `darwin/arm64` CLI and server bundles —
  not implemented ([#187](https://github.com/fiwon123/eznit/issues/187)).
- **GitLab and Bitbucket** clone providers next to GitHub — not implemented
  ([#188](https://github.com/fiwon123/eznit/issues/188)).

## 🧱 What already works without any of this

- `eznit template init` writes a starter with no server and no account.
- `eznit template publish` / `apply` move a folder of text and binary files
  between machines through your instance.
- The command library keeps a script once and pulls it into templates by id.
- A GitHub token on your machine lists and clones repositories; the token never
  reaches the API.

If a feature on this page would change how you use eznit today, open the issue
and say so — the plan is a direction, not a schedule.
