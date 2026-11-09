// Command winres writes the Windows resource object a Go build links into an
// executable: the application icon, a DPI-aware manifest and the version block
// Explorer shows in a file's Properties. It also verifies a built executable,
// because a build that looks right can still ship the wrong subsystem or no
// resources at all.
//
// Go has no flag for any of this. The only hook is a .syso file sitting beside
// the package being built, so a Windows build has to generate one first:
//
//	go run ./internal/tools/winres -icon assets/logo.png \
//	    -out ./rsrc_windows_amd64.syso -arch amd64 -version v1.2.3 \
//	    -name mytool -description "a small tool"
//
// The object is plain COFF with a .rsrc section; no Windows toolchain is
// involved, so it can be produced on any host.
//
// The tool depends on the standard library only and on no module path, so the
// directory copies into a project unchanged.
//
// It also verifies a release directory: `-verify-archives` reads every
// .tar.gz/.zip, checks the one-folder layout, the names the project promises,
// the PE inside each Windows archive, and the build provenance recorded by
// `debug/buildinfo`. That check lives here because this package already reads
// PE files; a separate tool would duplicate the reader or import this module's
// path.
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
)

func main() {
	icon := flag.String("icon", "", "PNG image to use as the application icon (required when writing)")
	out := flag.String("out", "", "path of the .syso file to write (required when writing)")
	arch := flag.String("arch", "amd64", "target architecture: amd64, arm64 or 386")
	version := flag.String("version", "", "version to record, such as v1.2.3")
	name := flag.String("name", "", "product and file description (required when writing)")
	description := flag.String("description", "", "long description; defaults to -name")
	company := flag.String("company", "", "company name recorded in the version block; defaults to -name")
	appID := flag.String("app-id", "", "manifest identity, such as com.example.mytool; defaults to com.example.<name>")
	verify := flag.String("verify", "", "check a built GUI executable instead of writing one")
	verifyConsole := flag.String("verify-console", "", "check a built console executable instead of writing one")
	requireResources := flag.Bool("require-resources", false, "with -verify-console, also require the icon, manifest and version resources")
	verifyArchivesDir := flag.String("verify-archives", "", "check every release archive in this directory instead of writing one")
	app := flag.String("app", "", "binary name the archives must carry (with -verify-archives)")
	short := flag.String("short", "", "optional alias the archives must carry (with -verify-archives)")
	requireGUI := flag.Bool("gui", false, "require <app>-gui in every archive (with -verify-archives)")
	commit := flag.String("commit", "", "require this vcs.revision in every binary (with -verify-archives; empty skips)")
	files := flag.String("files", "README.md,LICENSE", "comma-separated files every archive must carry (with -verify-archives)")
	flag.Parse()

	if *verifyArchivesDir != "" {
		if *app == "" {
			fmt.Fprintln(os.Stderr, "winres: -app is required with -verify-archives")
			os.Exit(2)
		}

		options := archiveOptions{
			App:    *app,
			Short:  *short,
			GUI:    *requireGUI,
			Commit: *commit,
			Files:  splitList(*files),
		}

		if err := verifyArchives(*verifyArchivesDir, options); err != nil {
			fmt.Fprintln(os.Stderr, "winres:", err)
			os.Exit(1)
		}

		return
	}

	if *verify != "" {
		if err := run(*verify, true, true); err != nil {
			fmt.Fprintln(os.Stderr, "winres:", err)
			os.Exit(1)
		}

		return
	}

	if *verifyConsole != "" {
		if err := run(*verifyConsole, false, *requireResources); err != nil {
			fmt.Fprintln(os.Stderr, "winres:", err)
			os.Exit(1)
		}

		return
	}

	if *icon == "" || *out == "" || *name == "" {
		fmt.Fprintln(os.Stderr, "winres: -icon, -out and -name are required")
		flag.Usage()
		os.Exit(2)
	}

	if *description == "" {
		*description = *name
	}

	if *company == "" {
		*company = *name
	}

	if *appID == "" {
		*appID = "com.example." + *name
	}

	object, err := build(*icon, *appID, *name, *description, *company, *version, *arch)
	if err != nil {
		fmt.Fprintln(os.Stderr, "winres:", err)
		os.Exit(1)
	}

	if err := os.WriteFile(*out, object, 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "winres:", err)
		os.Exit(1)
	}
}

// run verifies one built executable and reports what it found.
func run(path string, gui, requireResources bool) error {
	if err := verifyPath(path, gui, requireResources); err != nil {
		return err
	}

	kind := "console"
	if gui {
		kind = "GUI with icon, manifest and version"
	}

	fmt.Printf("ok   %s (%s)\n", path, kind)

	return nil
}

// build turns the icon and metadata into the bytes of a COFF object.
func build(iconPath, appID, name, description, company, version, arch string) ([]byte, error) {
	machine, err := machineFor(arch)
	if err != nil {
		return nil, err
	}

	source, err := os.Open(iconPath)
	if err != nil {
		return nil, fmt.Errorf("open icon: %w", err)
	}
	defer source.Close()

	images, err := iconImages(source)
	if err != nil {
		return nil, err
	}

	section, relocs, err := resources(
		images,
		manifest(appID, description, version),
		versionInfo(name, description, company, version),
	)
	if err != nil {
		return nil, err
	}

	return coff(machine, section, relocs), nil
}

func machineFor(arch string) (uint16, error) {
	switch arch {
	case "amd64":
		return machineAMD64, nil
	case "arm64":
		return machineARM64, nil
	case "386":
		return machineI386, nil
	default:
		return 0, fmt.Errorf("unsupported architecture %q", arch)
	}
}

// splitList turns a comma-separated flag value into a trimmed list.
func splitList(value string) []string {
	var items []string

	for _, item := range strings.Split(value, ",") {
		if item = strings.TrimSpace(item); item != "" {
			items = append(items, item)
		}
	}

	return items
}
