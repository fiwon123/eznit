package update

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/fiwon123/eznit/internal/config"
	"github.com/minio/selfupdate"
)

// AssetName is the name the release actually publishes: since v0.24.0 (#242)
// one archive per platform carries every member, so the prefix is the packer's
// FAMILY and never a per-family name.
func TestAssetName(t *testing.T) {
	tests := []struct {
		name    string
		goos    string
		goarch  string
		version string
		want    string
		wantErr bool
	}{
		{"linux amd64", "linux", "amd64", "v1.2.3", "eznit_v1.2.3_linux_amd64.tar.gz", false},
		{"darwin arm64", "darwin", "arm64", "v1.2.3", "eznit_v1.2.3_darwin_arm64.tar.gz", false},
		{"windows arm64", "windows", "arm64", "v1.2.3", "eznit_v1.2.3_windows_arm64.zip", false},
		{"untagged version", "linux", "amd64", "1.2.3", "", true},
		{"unsupported os", "plan9", "amd64", "v1.2.3", "", true},
		{"unsupported arch", "linux", "386", "v1.2.3", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := AssetName(tt.goos, tt.goarch, tt.version)

			if (err != nil) != tt.wantErr {
				t.Fatalf("AssetName() error = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("AssetName() = %q, want %q", got, tt.want)
			}
		})
	}
}

// The archive prefix is the packer's FAMILY. Reading it from the Makefile is
// what makes a rename fail here instead of in a user's update: the last time
// the two drifted, every install asked for a file the release no longer had.
func TestAssetNameMatchesThePacker(t *testing.T) {
	makefile, err := os.ReadFile(filepath.Join("..", "..", "Makefile"))
	if err != nil {
		t.Fatalf("read the Makefile: %v", err)
	}

	family := makeVariable(t, makefile, "FAMILY")

	// FAMILY is written as $(APP_NAME); resolve that one reference, and refuse
	// anything more complicated rather than guessing at make syntax.
	if strings.HasPrefix(family, "$(") {
		name := strings.TrimSuffix(strings.TrimPrefix(family, "$("), ")")
		family = makeVariable(t, makefile, name)
	}

	if family != ArchiveFamily {
		t.Errorf("the packer's FAMILY is %q but the updater asks for %q", family, ArchiveFamily)
	}
}

// makeVariable reads one `NAME := value` from a Makefile.
func makeVariable(t *testing.T, makefile []byte, name string) string {
	t.Helper()

	pattern := regexp.MustCompile(`(?m)^` + regexp.QuoteMeta(name) + ` := (\S+)\s*$`)
	match := pattern.FindSubmatch(makefile)
	if match == nil {
		t.Fatalf("the Makefile names no %s", name)
	}

	return string(match[1])
}

func TestParseChecksum(t *testing.T) {
	body := []byte("deadbeef  other.tar.gz\n" +
		"e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855 *target.tar.gz\n")

	got, err := parseChecksum(body, "target.tar.gz")
	if err != nil {
		t.Fatalf("parseChecksum() error = %v", err)
	}
	if hex.EncodeToString(got) != "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855" {
		t.Errorf("checksum = %x", got)
	}

	if _, err := parseChecksum(body, "missing.tar.gz"); err == nil {
		t.Error("parseChecksum(missing) error = nil, want an error")
	}
}

func tarGz(t *testing.T, name string, content []byte) []byte {
	t.Helper()

	var buffer bytes.Buffer
	gzipWriter := gzip.NewWriter(&buffer)
	writer := tar.NewWriter(gzipWriter)

	if err := writer.WriteHeader(&tar.Header{Name: "folder/" + name, Mode: 0o755, Size: int64(len(content))}); err != nil {
		t.Fatalf("header: %v", err)
	}
	if _, err := writer.Write(content); err != nil {
		t.Fatalf("write: %v", err)
	}

	writer.Close()
	gzipWriter.Close()

	return buffer.Bytes()
}

func zipFile(t *testing.T, name string, content []byte) []byte {
	t.Helper()

	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)

	entry, err := writer.Create("folder/" + name)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := entry.Write(content); err != nil {
		t.Fatalf("write: %v", err)
	}

	writer.Close()

	return buffer.Bytes()
}

type releaseServer struct {
	server   *httptest.Server
	version  string
	asset    string
	archive  []byte
	checksum string

	// The last install, captured by the fake apply.
	applied []byte
	options selfupdate.Options
}

func newReleaseServer(t *testing.T, version, asset string, archive []byte) *releaseServer {
	t.Helper()

	release := &releaseServer{version: version, asset: asset, archive: archive}

	sum := sha256.Sum256(archive)
	release.checksum = hex.EncodeToString(sum[:])

	release.server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/latest":
			_, _ = fmt.Fprintf(w, `{"tag_name":%q}`, release.version)
		case r.URL.Path == "/download/"+release.version+"/checksums.txt":
			_, _ = fmt.Fprintf(w, "%s  %s\n", release.checksum, release.asset)
		case r.URL.Path == "/download/"+release.version+"/"+release.asset:
			_, _ = w.Write(release.archive)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(release.server.Close)

	t.Setenv(config.Env, filepath.Join(t.TempDir(), "config.json"))

	applyUpdate = func(reader io.Reader, options selfupdate.Options) error {
		release.applied, _ = io.ReadAll(reader)
		release.options = options
		return nil
	}
	t.Cleanup(func() { applyUpdate = selfupdate.Apply })

	return release
}

func (r *releaseServer) updater(t *testing.T, member string, goos, goarch string) *Updater {
	t.Helper()

	return &Updater{
		Member:     member,
		BaseURL:    r.server.URL + "/download",
		LatestURL:  r.server.URL + "/latest",
		GOOS:       goos,
		GOARCH:     goarch,
		Executable: filepath.Join(t.TempDir(), member),
		HTTP:       r.server.Client(),
	}
}

func TestInstallAndRestore(t *testing.T) {
	version := "v1.2.3"
	asset, _ := AssetName("linux", "amd64", version)

	release := newReleaseServer(t, version, asset, tarGz(t, MemberGUI, []byte("new-binary")))
	updater := release.updater(t, MemberGUI, "linux", "amd64")

	if err := updater.Install("v1.0.0", version); err != nil {
		t.Fatalf("Install() error = %v", err)
	}

	if string(release.applied) != "new-binary" {
		t.Errorf("installed %q, want the GUI member", release.applied)
	}
	if release.options.OldSavePath != updater.Executable+".old" {
		t.Errorf("OldSavePath = %q, want the backup beside the binary", release.options.OldSavePath)
	}

	previous, ok := updater.Previous()
	if !ok || previous != "v1.0.0" {
		t.Errorf("Previous() = %q, %v, want v1.0.0", previous, ok)
	}

	// The fake apply does not write the backup; write it, then restore.
	if err := os.WriteFile(updater.Executable+".old", []byte("old-binary"), 0o755); err != nil {
		t.Fatalf("write backup: %v", err)
	}

	release.applied = nil
	if err := updater.Restore(); err != nil {
		t.Fatalf("Restore() error = %v", err)
	}
	if string(release.applied) != "old-binary" {
		t.Errorf("restored %q, want the previous binary", release.applied)
	}
	if _, ok := updater.Previous(); ok {
		t.Error("Previous() still set after a restore")
	}
}

func TestInstallRefusesAChecksumMismatch(t *testing.T) {
	version := "v1.2.3"
	asset, _ := AssetName("linux", "amd64", version)

	release := newReleaseServer(t, version, asset, tarGz(t, MemberCLI, []byte("new-binary")))
	release.checksum = strings.Repeat("0", 64)
	updater := release.updater(t, MemberCLI, "linux", "amd64")

	if err := updater.Install("v1.0.0", version); err == nil {
		t.Fatal("Install() error = nil, want the checksum refusal")
	}
}

func TestInstallRefusesAMissingMember(t *testing.T) {
	version := "v1.2.3"
	asset, _ := AssetName("linux", "amd64", version)

	release := newReleaseServer(t, version, asset, tarGz(t, MemberCLI, []byte("cli")))
	updater := release.updater(t, MemberGUI, "linux", "amd64")

	if err := updater.Install("v1.0.0", version); err == nil {
		t.Fatal("Install() error = nil, want the missing member refused")
	}
}

func TestInstallWindowsZip(t *testing.T) {
	version := "v1.2.3"
	asset, _ := AssetName("windows", "amd64", version)

	release := newReleaseServer(t, version, asset, zipFile(t, MemberGUI+".exe", []byte("exe")))
	updater := release.updater(t, MemberGUI, "windows", "amd64")

	if err := updater.Install("v1.0.0", version); err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	if string(release.applied) != "exe" {
		t.Errorf("installed %q, want the exe member", release.applied)
	}
}

func TestLatest(t *testing.T) {
	release := newReleaseServer(t, "v9.9.9", "unused", nil)
	updater := release.updater(t, MemberCLI, "linux", "amd64")

	tag, err := updater.Latest()
	if err != nil {
		t.Fatalf("Latest() error = %v", err)
	}
	if tag != "v9.9.9" {
		t.Errorf("Latest() = %q, want v9.9.9", tag)
	}
}

func TestLatestRefusesPlainHTTP(t *testing.T) {
	updater := &Updater{LatestURL: "http://example.test/latest", HTTP: http.DefaultClient}

	if _, err := updater.Latest(); err == nil {
		t.Fatal("Latest() error = nil, want the scheme refused")
	}
}

func TestReleaseURLRefusesPlainHTTP(t *testing.T) {
	updater := &Updater{BaseURL: "http://example.test/download", HTTP: http.DefaultClient}

	if _, err := updater.releaseURL("v1.2.3", "checksums.txt"); err == nil {
		t.Fatal("releaseURL() error = nil, want the scheme refused")
	}
}
