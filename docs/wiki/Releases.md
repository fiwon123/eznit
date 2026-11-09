# Releases

eznit ships downloadable archives, published by GitHub Actions when a `v*` tag is
pushed. This page describes how a release is cut and what the archives contain.

## How a release is cut

The flow is `dev` → `main` → tag:

1. Merge the milestone's work into `dev` through pull requests.
2. Open a release pull request from `dev` to `main` and merge it with a **merge
   commit** (not a squash), so `dev` stays an ancestor of `main` and no
   back-merge is needed.
3. Tag the **merge commit on `main`**:

   ```sh
   git fetch origin main
   git tag -a v<version> "$(git rev-parse origin/main)" -m "Release v<version>"
   git push origin v<version>
   ```

`.github/workflows/release.yml` refuses a tag that does not point at a commit on
`main`, builds the archives with `make release VERSION=<tag>` — the same target a
developer runs — and publishes them with `gh release create`.

The release notes are built from the pull requests merged into `dev`, grouped by
label, so they read as a changelog rather than a raw commit list.

## The workflow's gates

- **guard** — the tag is on `main`, and the version/commit are resolved once.
- **build** — the archives plus `checksums.txt`, built with read-only permissions.
- **smoke** — on `windows-latest`, the Windows CLI archive is unpacked and `eznit
  -v` and `ez -v` are actually run. A binary that cannot start fails the release
  instead of shipping.
- **publish** — runs only on a tag push; needs all three above. It re-verifies the
  checksums, composes the notes, and creates the GitHub Release.

`workflow_dispatch` runs the same build and smoke and keeps the archives on the
run, and **publishes nothing** — the way to exercise the pipeline without a tag. A
green dry run does not prove the publish step; only a real tag does.

## What the archives contain

`make release` stages one folder per archive and compresses **the folder**, so a
download extracts into a folder named after it instead of scattering files into the
directory you ran the command in. `PLATFORMS` defaults to `windows/amd64`,
`windows/arm64`, `linux/amd64` and `linux/arm64`; the platform is in the archive
name, never in the binary name.

| Asset | Contents |
| --- | --- |
| `eznit-cli_<version>_<os>_<arch>.zip` (Windows) / `.tar.gz` (Linux) | `eznit`, `ez`, `add_to_path.ps1` or `add_to_path.sh`, README, LICENSE |
| `eznit-server_<version>_<os>_<arch>.zip` / `.tar.gz` | `eznit-server`, `eznit-gui`, `db/migrations/`, `scripts/run_migrate.sh`, `docker-compose.yml`, the example configuration and secret files, `RUNNING.md`, README, LICENSE |
| `checksums.txt` | SHA-256 of every archive |

Each archive is checked after it is written: exactly one top-level folder, named
like the archive, and no `..` member. `make release` then runs
`make verify-release`, which reads the finished archives — the names inside, the PE
of every Windows binary, and (when `PROVENANCE_COMMIT` is set, as the workflow
does) the revision and `vcs.modified=false`. Every Windows binary also carries an
icon, a per-monitor DPI manifest and a version block, generated from
`assets/logo.png` before the build and removed afterwards.

## Verify a download

```sh
sha256sum -c checksums.txt          # macOS: shasum -a 256 -c checksums.txt
```

## Running a download

A release archive is a **server bundle**, not a source checkout: there is no
`go.mod`, no `web/`, and no image to build. The database and migration job are
published images plus the SQL in `db/migrations/`; run the prebuilt server against
them. The bundle's `RUNNING.md` has the exact steps and says what the bundle does
not prove.
