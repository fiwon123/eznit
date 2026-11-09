# winres — Windows resources for a Go binary

Go has no flag for an application icon, a manifest or a version block. The only
hook is a `.syso` COFF object sitting beside the package being built. This tool
writes one, on any host — no Windows toolchain, no `windres`, standard library
only.

```bash
go run ./internal/tools/winres \
  -icon assets/logo.png \
  -out ./rsrc_windows_amd64.syso \
  -arch amd64 -version v1.2.3 \
  -name mytool -description "a small tool"
```

`-arch` accepts `amd64`, `arm64` and `386`; the file name must be
`rsrc_windows_<arch>.syso` for Go to include it in that target. `-app-id`
defaults to `com.example.<name>` — set it to the identifier your project uses.
`-company` defaults to `-name`.

## Verify the built binary, do not trust the build flags

```bash
go run ./internal/tools/winres -verify-console -require-resources build/mytool.exe
go run ./internal/tools/winres -verify build/mytool-gui.exe
```

`-verify-console` requires subsystem 3 (the binary keeps its console) and, with
`-require-resources`, the four resource types. `-verify` requires subsystem 2
plus the resources, for a Fyne window built with `-H windowsgui`.

A payload is only counted when it is reachable the way the loader looks for it —
type, then name, then language. A tree that stops one level short still parses
and Windows refuses the image ("This app can't run on your PC"), which is why
the check walks all three levels.

## Copying it in

Copy this directory to `internal/tools/winres` in the project. It is a single
`main` package with no module-path imports, so nothing needs rewriting, and
`make release-bin` invokes it from `templates/components/makefile.component-go`.

Run its tests with `go test ./internal/tools/winres` — they build a synthetic
icon and assert the COFF object, the resource tree, the manifest and the version
block without needing Windows.

## Verify a release directory

```bash
go run ./internal/tools/winres -verify-archives build/dist \
  -app mytool -short mt -gui -commit "$(git rev-parse HEAD)"
```

This reads every `.tar.gz`/`.zip` in the directory and fails on:

- an empty directory (a check that inspected nothing is not a pass);
- a missing, extra or misnamed top-level folder;
- a missing binary or file (`-app`, `-short`, `-gui`, `-files`);
- an absolute path, a `..` member or a Windows drive prefix;
- an arch-suffixed binary such as `mytool_linux_amd64`, which the documented
  `mytool` command would never find;
- for a Windows archive: a wrong machine, a console window binary, or a missing
  icon/manifest/version in the window;
- with `-commit`: a binary whose `vcs.revision` differs, or one built from a
  modified tree.

`release-bin`/`release-gui-bin` end by calling it (`make verify-release`), and
the release workflows pass the tagged commit, so a release that fails this is
never uploaded.
