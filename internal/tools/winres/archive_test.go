package main

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeZip writes a .zip holding name → contents.
func writeZip(t *testing.T, path string, contents map[string]string) {
	t.Helper()

	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)

	for name, body := range contents {
		entry, err := writer.Create(name)
		if err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
		if _, err := entry.Write([]byte(body)); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	if err := writer.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
	if err := os.WriteFile(path, buffer.Bytes(), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// writeTarGz writes a .tar.gz holding name → contents.
func writeTarGz(t *testing.T, path string, contents map[string]string) {
	t.Helper()

	var buffer bytes.Buffer
	gzipWriter := gzip.NewWriter(&buffer)
	writer := tar.NewWriter(gzipWriter)

	for name, body := range contents {
		if err := writer.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(body))}); err != nil {
			t.Fatalf("header %s: %v", name, err)
		}
		if _, err := writer.Write([]byte(body)); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	writer.Close()
	gzipWriter.Close()

	if err := os.WriteFile(path, buffer.Bytes(), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func goodContents(folder string) map[string]string {
	return map[string]string{
		folder + "/":               "",
		folder + "/mytool":         "binary",
		folder + "/README.md":      "readme",
		folder + "/LICENSE":        "license",
		folder + "/mytool.png":     "png",
		folder + "/mytool.desktop": "desktop",
	}
}

func options() archiveOptions {
	return archiveOptions{App: "mytool", Files: []string{"README.md", "LICENSE"}}
}

func TestVerifyArchivesFailsOnAnEmptyDirectory(t *testing.T) {
	if err := verifyArchives(t.TempDir(), options()); err == nil {
		t.Fatal("an empty directory passed; a check that inspects nothing is not a pass")
	}
}

func TestVerifyArchivesAcceptsOneFolder(t *testing.T) {
	dir := t.TempDir()
	folder := "mytool_v1.0.0_linux_amd64"

	writeTarGz(t, filepath.Join(dir, folder+".tar.gz"), goodContents(folder))
	writeZip(t, filepath.Join(dir, folder+".zip"), goodContents(folder))

	if err := verifyArchives(dir, options()); err != nil {
		t.Fatalf("good archives failed: %v", err)
	}
}

func TestVerifyArchivesRejectsAFlatArchive(t *testing.T) {
	dir := t.TempDir()
	folder := "mytool_v1.0.0_linux_amd64"

	contents := map[string]string{"mytool": "binary", "README.md": "r", "LICENSE": "l"}
	writeTarGz(t, filepath.Join(dir, folder+".tar.gz"), contents)

	err := verifyArchives(dir, options())
	if err == nil || !strings.Contains(err.Error(), "top-level entries") {
		t.Fatalf("flat archive was not rejected for its layout: %v", err)
	}
}

func TestVerifyArchivesRejectsAMissingFile(t *testing.T) {
	dir := t.TempDir()
	folder := "mytool_v1.0.0_linux_amd64"

	contents := map[string]string{folder + "/mytool": "binary", folder + "/README.md": "r"}
	writeTarGz(t, filepath.Join(dir, folder+".tar.gz"), contents)

	err := verifyArchives(dir, options())
	if err == nil || !strings.Contains(err.Error(), "LICENSE") {
		t.Fatalf("missing LICENSE was not reported: %v", err)
	}
}

func TestVerifyArchivesRejectsAnUnsafeMember(t *testing.T) {
	dir := t.TempDir()
	folder := "mytool_v1.0.0_linux_amd64"

	contents := goodContents(folder)
	contents[folder+"/../escape"] = "x"
	writeTarGz(t, filepath.Join(dir, folder+".tar.gz"), contents)

	err := verifyArchives(dir, options())
	if err == nil || !strings.Contains(err.Error(), "unsafe member") {
		t.Fatalf("unsafe member was not reported: %v", err)
	}
}

func TestVerifyArchivesRejectsAnArchSuffixedBinary(t *testing.T) {
	dir := t.TempDir()
	folder := "mytool_v1.0.0_linux_amd64"

	contents := goodContents(folder)
	contents[folder+"/mytool_linux_amd64"] = "binary"
	writeTarGz(t, filepath.Join(dir, folder+".tar.gz"), contents)

	err := verifyArchives(dir, options())
	if err == nil || !strings.Contains(err.Error(), "arch-suffixed") {
		t.Fatalf("arch-suffixed binary was not reported: %v", err)
	}
}

func TestVerifyArchivesRequiresTheGUI(t *testing.T) {
	dir := t.TempDir()
	folder := "mytool_v1.0.0_linux_amd64"

	writeTarGz(t, filepath.Join(dir, folder+".tar.gz"), goodContents(folder))

	opts := options()
	opts.GUI = true

	err := verifyArchives(dir, opts)
	if err == nil || !strings.Contains(err.Error(), "mytool-gui") {
		t.Fatalf("missing GUI binary was not reported: %v", err)
	}
}

// The folder name and the archive name must agree, or extraction and the
// documented `cd` step point at different directories.
func TestVerifyArchivesRejectsAMisnamedFolder(t *testing.T) {
	dir := t.TempDir()
	folder := "mytool_v1.0.0_linux_amd64"

	contents := map[string]string{"other_folder/mytool": "binary", "other_folder/README.md": "r", "other_folder/LICENSE": "l"}
	writeTarGz(t, filepath.Join(dir, folder+".tar.gz"), contents)

	err := verifyArchives(dir, options())
	if err == nil || !strings.Contains(err.Error(), "top-level entries") {
		t.Fatalf("misnamed folder was not reported: %v", err)
	}
}
