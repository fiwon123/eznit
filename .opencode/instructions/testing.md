## Testing Requirements

### General

- Tests are mandatory for new features and bug fixes
- Write tests before or alongside implementation
- Mock external services when possible — never call third-party production APIs in tests
- Aim for critical path coverage, not 100% line coverage

### Gates and baselines

A gate is only worth having if it can fail.

- **Do not fix pre-existing failures inside an unrelated PR.** If the suite (or
  `make check`) is already red when you start, your change is not the cause and
  the fix is not the change you set out to make. Say so, and either fix it
  forward on its own issue/branch or record it as a baseline below. Bundling an
  unrelated repair into your PR makes "is my change correct?" unanswerable from
  the diff.
- **Record a known-failing baseline.** When you inherit a red suite, write down
  which tests fail and why (a tracked issue, a TODO, the failing assertion) so
  the next person does not re-investigate the same failures. A baseline turns
  "the suite is red, I don't know why" into "these N are known-red".
- **A gate that cannot fail is worse than no gate.** A test target that collects
  zero tests, a CI job that always skips, or an assertion with no possible
  failure manufactures false confidence: it reads green and proves nothing. If a
  gate reports success without executing anything, treat it as **unverified**,
  not as passed — a skipped run is not a passed run.

### The current state of this suite — read this before claiming coverage

The suite has grown, but it is still **partial**. `go test ./...` prints
`? package [no test files]` for every package without a test file, and exits 0.
That is not a pass: it means nothing was executed there. Only `cmd/cli` (the
kong wiring) is in that state now.

A green `make check` proves:

- the code is `gofmt`-clean,
- `go vet` found nothing,
- the packages with tests passed, and the rest compiled.

It does **not** prove the SQL still matches the schema — that is
`make test_integration` (and the CI (Go) integration step), which applies and
reverses the migrations and drives every repository against a real PostgreSQL.
It also does not exercise HTTP routes end to end, or the browser.

Report what ran and what it covered: "make check exit 0, and the integration
suite ran against eznit_test" is evidence; "tests pass" alone is not.

### Go conventions

- Test file: `<file>_test.go`, next to the code it tests, same package (or
  `<package>_test` when the test is exercising the public surface only)
- Test function: `func Test<Thing>(<t *testing.T>)`
- Use the standard library only — no assertion framework unless the project
  already depends on one
- Table-driven tests for anything with more than one case:

  ```go
  func TestParse(t *testing.T) {
      tests := []struct {
          name    string
          input   string
          want    int
          wantErr bool
      }{
          {"empty", "", 0, true},
          {"one", "1", 1, false},
      }
      for _, tt := range tests {
          t.Run(tt.name, func(t *testing.T) {
              got, err := parse(tt.input)
              if (err != nil) != tt.wantErr {
                  t.Fatalf("parse(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
              }
              if got != tt.want {
                  t.Errorf("parse(%q) = %d, want %d", tt.input, got, tt.want)
              }
          })
      }
  }
  ```

- `t.TempDir()` for anything that touches the filesystem — never write into the
  working tree from a test
- `t.Setenv()` for environment variables (it restores the previous value and
  refuses to run in parallel)
- `httptest.NewServer()` for HTTP handlers; `httptest.NewRecorder()` when you
  only need the response
- Assert on error identity with `errors.Is` / `errors.As`, not on the message
  text — a message is not part of the contract
- `t.Helper()` in test helpers, so a failure points at the caller
- Never let a test reach the network, and never let it depend on wall-clock
  time or a random seed unless it controls both

### Database tests (when the project has a database)

- A test that truncates, drops, or rewrites tables acts on whatever
  `DATABASE_URL` points at. Guard it:
  - before any destructive operation, assert the connected database's *name*
    matches the test database (e.g. `SELECT current_database()` equals
    `<project>_test`) and abort otherwise;
  - never point the destructive suite at a connection string that production also
    uses without that assertion in front of it;
  - prefer a dedicated test database whose name cannot be confused for the real
    one.

This is the difference between a mistyped environment variable failing a run and
it wiping a real database.

This project implements that guard in `internal/platform/dbtest`: every file
carries `//go:build integration`, `Open` refuses a database whose name does not
end in `_test`, and `make test_integration` fails when `TEST_DATABASE_URL` is
unset rather than skipping. The suite applies and reverses every migration and
drives each repository against a real PostgreSQL; CI runs it in the **CI (Go)**
job against a `postgres:18.3-bookworm` service. Keep `make check` database-free.

### Before committing

1. Run `make check` — gofmt, vet, test, build
2. If the change touches a package that has no tests, say so in the PR body
3. Verify no regressions: if the suite was already red, confirm your change did
   not add to it

### Visual checks

The web UI lives in `web/`. When a change affects the UI, use the `visual`
subagent to review layouts, console errors and accessibility against the local
stack (serve `web/dist` via the API or run the Vite dev server). Capture
screenshots of the affected screens in both themes when possible. A page can
look broken while all tests pass — a visual audit catches it.
