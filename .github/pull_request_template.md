<!--
Keep the sections below — they are read at the moment a claim is written, which
is the only moment it can be checked. Delete this comment block before you open
the PR.
-->

Closes #<issue-number>

## What

<!-- What changed, in one or two sentences, and which files or packages carry it. -->

## Why

<!-- The problem this solves. Link the issue's reasoning rather than restating it. -->

## Verification

<!--
What actually ran, and what it proved. Be specific: a command and its exit
status is evidence; "tests pass" is a claim.

State the gate as it is, not more:

- `make check` (gofmt, vet, test, build) and, for UI changes, `make web_check`.
- `go test` prints `[no test files]` for packages without tests and still exits
  0 — list the packages this change touched that ran nothing. `make check`
  prints them at the end of a green run.
- CI runs on `dev` → `main` PRs only. A feature PR may trigger no run at all,
  and a job gated out comes back `skipped`, not `passed`.
- A skipped job, a run that never happened, or an empty test target is
  **unverified**, not passed.
-->

- [ ] `make check` — exit status:
- [ ] `make web_check` — exit status (or "not touched"):
- [ ] `make build` — exit status:
- Untested packages/areas touched (from the `[no test files]` list):
- Anything else that was run, and what it showed:

## Notes

<!--
- Follow-ups deliberately left out of scope, and the issue that tracks them.
- Known caveats a reviewer should not have to rediscover.
- Anything in the diff that looks wrong but is intentional, and why.
-->
