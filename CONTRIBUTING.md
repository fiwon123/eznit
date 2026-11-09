# Contributing

Thanks for wanting to help with eznit.

## Report an issue

Open an [issue](https://github.com/fiwon123/eznit/issues/new) with:

- **the version** — `eznit -v` (or the About section of the UI)
- **the platform** — OS, and whether it runs from source, a release archive or
  the Compose stack
- **what you expected, and what happened** — include logs if relevant; never
  paste a session token, a database password or the contents of `.env`
- **steps to reproduce** — a minimal sequence that hits it

The API's known gaps are tracked issues, not surprises: read "Known gaps" in
`AGENTS.md` before reporting one of them.

## Discuss

- **Issues** are for bugs and concrete proposals.
- **Discussions** are for questions, ideas not yet a plan, and "would this be
  useful?".

## Help without writing code

Running the stack and reporting exactly what happened — including "it worked" —
is useful. The README's "What is not proven" notes and the known gaps list are
where an unproven claim is waiting for someone to check it.

## Send code

- Open an issue first, so the change and the reason are recorded together.
- Branch from `dev` as `<type>/<issue>-<slug>` (`fix/42-scan-timeout`), and use
  conventional commits (`feat:`, `fix:`, `docs:`, `test:`, `chore:`, `ci:`).
- Run `make check` and, when you touched the UI, `make web_check` before
  pushing. CI runs on release pull requests (`dev` → `main`) only; a feature
  pull request is gated locally.
- One feature per pull request. Say in the body what ran and what did not, and
  do not describe a skipped check as a pass.
- `go test` prints `[no test files]` for the packages without tests and still
  exits 0 — name what ran, not just "tests pass". `make check` lists those
  packages at the end for exactly this reason.
- Follow `AGENTS.md` for the full workflow rules.

## About the AI

This project is developed with an AI agent. The review bar is the same: judge by
tests and evidence, not by who wrote it. Say what was verified and what was not.
