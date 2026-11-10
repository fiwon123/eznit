package main

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"debug/buildinfo"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// archiveOptions says what every archive in a release directory must contain.
type archiveOptions struct {
	// App is the binary name the release ships, without an extension.
	App string
	// Short is the optional alias shipped beside it.
	Short string
	// GUI requires App+"-gui" in every archive.
	GUI bool
	// Commit, when set, must match every binary's vcs.revision, and the build
	// must not report vcs.modified.
	Commit string
	// Files are the non-binary files every archive must carry.
	Files []string
}

// archiveEntry is one member of an archive, read into memory. Releases are
// small, and a verifier that streams has to re-open the archive per check.
type archiveEntry struct {
	name string
	data []byte
	mode fs.FileMode
}

// archSuffixed matches a binary that names its target in the file name. The
// platform is already in the archive name, and the instructions tell the user
// to run the plain name, so a suffixed copy is unreachable.
var archSuffixed = regexp.MustCompile(`(?i)(linux|windows|darwin)[-_ .](amd64|arm64|386)`)

// verifyArchives checks every archive in dir and reports all of their
// problems at once. An empty directory is an error: a check that inspected
// nothing cannot report success.
func verifyArchives(dir string, opts archiveOptions) error {
	paths, err := filepath.Glob(filepath.Join(dir, "*"))
	if err != nil {
		return err
	}

	var archives []string
	for _, path := range paths {
		base := filepath.Base(path)
		if strings.HasSuffix(base, ".tar.gz") || strings.HasSuffix(base, ".zip") {
			archives = append(archives, path)
		}
	}

	if len(archives) == 0 {
		return fmt.Errorf("%s holds no .tar.gz or .zip archive — nothing was verified, so this is not a pass", dir)
	}

	sort.Strings(archives)

	var problems []string

	for _, path := range archives {
		for _, problem := range verifyArchive(path, opts) {
			problems = append(problems, problem)
		}
	}

	if len(problems) > 0 {
		return fmt.Errorf("%d problem(s) in %d archive(s):\n  %s", len(problems), len(archives), strings.Join(problems, "\n  "))
	}

	for _, path := range archives {
		fmt.Printf("ok   %s\n", filepath.Base(path))
	}

	return nil
}

// verifyArchive returns every problem found in one archive, so a fixed archive
// can be re-checked without rediscovering the others one at a time.
func verifyArchive(path string, opts archiveOptions) []string {
	base := filepath.Base(path)

	var folder, ext string

	switch {
	case strings.HasSuffix(base, ".tar.gz"):
		folder, ext = strings.TrimSuffix(base, ".tar.gz"), ".tar.gz"
	case strings.HasSuffix(base, ".zip"):
		folder, ext = strings.TrimSuffix(base, ".zip"), ".zip"
	default:
		return nil
	}

	entries, err := readArchive(path, ext)
	if err != nil {
		return []string{fmt.Sprintf("%s: %v", base, err)}
	}

	var problems []string

	report := func(format string, args ...any) {
		problems = append(problems, fmt.Sprintf("%s: %s", base, fmt.Sprintf(format, args...)))
	}

	// One top-level folder, named after the archive.
	tops := map[string]bool{}

	for _, entry := range entries {
		name := strings.TrimSuffix(entry.name, "/")
		if name == "" {
			continue
		}

		if unsafeMember(name) {
			report("carries the unsafe member %q — extraction could write outside the target", name)
			continue
		}

		tops[strings.SplitN(name, "/", 2)[0]] = true
	}

	if len(tops) != 1 || !tops[folder] {
		report("top-level entries are %v, want exactly one folder %q", sortedKeys(tops), folder)
	}

	files := map[string][]byte{}
	modes := map[string]fs.FileMode{}
	for _, entry := range entries {
		files[entry.name] = entry.data
		modes[entry.name] = entry.mode
	}

	windows := strings.Contains(base, "_windows_")
	darwin := strings.Contains(base, "_darwin_")

	darwinCPU, darwinNamed := DarwinMachineForArchive(base)
	if darwin && !darwinNamed {
		report("is a darwin archive whose name does not say amd64 or arm64, so its binary cannot be checked")
	}

	// The binaries the project promises, at the folder's root.
	var binaries []string

	binaries = append(binaries, opts.App)
	if opts.Short != "" {
		binaries = append(binaries, opts.Short)
	}
	if opts.GUI {
		binaries = append(binaries, opts.App+"-gui")
	}

	machine, _ := MachineForArchive(base)

	for _, name := range binaries {
		member := folder + "/" + name
		if windows {
			member += ".exe"
		}

		data, ok := files[member]
		if !ok {
			report("does not carry %s", strings.TrimPrefix(member, folder+"/"))
			continue
		}

		// A binary that is not executable is a download that answers
		// "Permission denied". The mode is in the archive, so it is a fact
		// about the artifact, not about how the extractor behaved.
		if !windows && modes[member]&0o111 == 0 {
			report("%s is not executable in the archive (mode %v)", strings.TrimPrefix(member, folder+"/"), modes[member])
		}

		if windows {
			info, err := Inspect(bytes.NewReader(data))
			if err != nil {
				report("%s: %v", filepath.Base(member), err)
				continue
			}

			if err := info.VerifyMachine(machine); err != nil {
				report("%s: %v", filepath.Base(member), err)
			}

			if name == opts.App+"-gui" {
				if err := info.VerifyGUI(); err != nil {
					report("%s: %v", filepath.Base(member), err)
				}
			} else if err := info.VerifyConsole(); err != nil {
				report("%s: %v", filepath.Base(member), err)
			}
		}

		if darwin && darwinNamed {
			cpu, err := InspectMachO(data)
			if err != nil {
				report("%s: %v", filepath.Base(member), err)
			} else if cpu != darwinCPU {
				report("%s: Mach-O CPU is %v, want %v", filepath.Base(member), cpu, darwinCPU)
			}
		}

		if opts.Commit != "" {
			for _, problem := range revisionProblems(filepath.Base(member), data, opts.Commit) {
				report("%s", problem)
			}
		}
	}

	// An arch-suffixed binary anywhere is unreachable by the documented name.
	for name := range files {
		parts := strings.Split(name, "/")
		if len(parts) != 2 {
			continue
		}
		if archSuffixed.MatchString(parts[1]) {
			report("carries the arch-suffixed binary %q — the platform is in the archive name, so the documented command will not find it", parts[1])
		}
	}

	for _, file := range opts.Files {
		if _, ok := files[folder+"/"+file]; !ok {
			report("does not carry %s", file)
		}
	}

	return problems
}

// revisionProblems reports every provenance mismatch in one binary.
func revisionProblems(name string, data []byte, commit string) []string {
	info, err := buildinfo.Read(bytes.NewReader(data))
	if err != nil {
		return []string{fmt.Sprintf("%s: no build information to verify: %v", name, err)}
	}

	settings := map[string]string{}
	for _, setting := range info.Settings {
		settings[setting.Key] = setting.Value
	}

	var problems []string

	if got := settings["vcs.revision"]; got != commit {
		problems = append(problems, fmt.Sprintf("%s was built from %q, want %q", name, got, commit))
	}
	if settings["vcs.modified"] == "true" {
		problems = append(problems, fmt.Sprintf("%s was built from a modified tree (vcs.modified=true) — a release ships a committed tree", name))
	}

	return problems
}

// readArchive reads every regular file in a .zip or .tar.gz into memory.
func readArchive(path, ext string) ([]archiveEntry, error) {
	if ext == ".zip" {
		reader, err := zip.OpenReader(path)
		if err != nil {
			return nil, err
		}
		defer reader.Close()

		var entries []archiveEntry
		for _, file := range reader.File {
			if file.FileInfo().IsDir() {
				entries = append(entries, archiveEntry{name: file.Name + "/", mode: file.Mode()})
				continue
			}

			handle, err := file.Open()
			if err != nil {
				return nil, err
			}

			data, err := io.ReadAll(handle)
			handle.Close()
			if err != nil {
				return nil, err
			}

			entries = append(entries, archiveEntry{name: file.Name, data: data, mode: file.Mode()})
		}

		return entries, nil
	}

	handle, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer handle.Close()

	gzipReader, err := gzip.NewReader(handle)
	if err != nil {
		return nil, err
	}
	defer gzipReader.Close()

	var entries []archiveEntry

	reader := tar.NewReader(gzipReader)
	for {
		header, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}

		if header.Typeflag == tar.TypeDir {
			entries = append(entries, archiveEntry{name: header.Name + "/", mode: header.FileInfo().Mode()})
			continue
		}
		if header.Typeflag != tar.TypeReg {
			continue
		}

		data, err := io.ReadAll(reader)
		if err != nil {
			return nil, err
		}

		entries = append(entries, archiveEntry{name: header.Name, data: data, mode: header.FileInfo().Mode()})
	}

	return entries, nil
}

// unsafeMember reports an absolute path, a `..` component or a Windows drive
// prefix: the three ways an archive writes outside the directory it was
// extracted into.
func unsafeMember(name string) bool {
	if strings.HasPrefix(name, "/") || strings.HasPrefix(name, `\`) {
		return true
	}
	if len(name) >= 2 && name[1] == ':' && ((name[0] >= 'a' && name[0] <= 'z') || (name[0] >= 'A' && name[0] <= 'Z')) {
		return true
	}
	for _, part := range strings.Split(name, "/") {
		if part == ".." {
			return true
		}
	}

	return false
}

func sortedKeys(values map[string]bool) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}

	sort.Strings(keys)

	return keys
}
