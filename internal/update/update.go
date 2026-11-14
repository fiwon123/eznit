// Package update installs an eznit release: the newest, a named one, or the
// previous one, from the same GitHub releases the archives are published to.
//
// The rules are the ones a self-updater has to have:
//
//   - checksums.txt is fetched before the archive, and a missing or malformed
//     entry fails the install rather than skipping the check;
//   - only HTTPS is followed, including redirects;
//   - the archive and the extracted binary are size-capped;
//   - the binary that is installed is named by the caller, because an archive
//     carries more than one (the CLI must not install the window, and the
//     window must not install the CLI).
package update

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/minio/selfupdate"

	"github.com/fiwon123/eznit/internal/config"
)

// The members a release archive carries, and the family prefix of the archive
// itself.
const (
	// MemberCLI and MemberGUI are the executables inside the one archive a
	// release publishes per platform. The client and the server are the same
	// program, so there is no separate server member to update.
	MemberCLI = "eznit"
	MemberGUI = "eznit-gui"

	// ArchiveFamily prefixes every release archive:
	// <ArchiveFamily>_<version>_<os>_<arch>. Since v0.24.0 (#242) one archive
	// per platform carries every member, so this is a constant and not a
	// parameter — the parameter that used to be here went stale when the
	// archives were merged, and every update after that asked for a file the
	// release no longer published. The Makefile's FAMILY is the same value, and
	// a test reads it.
	ArchiveFamily = "eznit"
)

const (
	// LatestReleaseURL names the newest published release.
	LatestReleaseURL = "https://api.github.com/repos/fiwon123/eznit/releases/latest"
	// ReleaseBaseURL is where the archives are published.
	ReleaseBaseURL = "https://github.com/fiwon123/eznit/releases/download"

	// The caps stop a corrupt or hostile response from exhausting the machine.
	maxDownloadBytes = 128 << 20
	maxBinaryBytes   = 512 << 20
	maxChecksumBytes = 1 << 20
)

// versionPattern is the tag shape the release workflow accepts.
var versionPattern = regexp.MustCompile(`^v[0-9]+\.[0-9]+\.[0-9]+$`)

// applyUpdate is selfupdate.Apply behind a variable so a test can observe the
// bytes an install would write without replacing the test binary.
var applyUpdate = selfupdate.Apply

// Updater downloads, verifies and applies one release.
type Updater struct {
	// Member is the executable installed from the archive.
	Member string

	BaseURL   string
	LatestURL string
	GOOS      string
	GOARCH    string

	// Executable is the path an install replaces, recorded when the Updater is
	// built. It must be read before an install and kept: after the swap the
	// running image's own path follows its inode into the backup, so a caller
	// that restarts itself has to use this path and not os.Executable().
	Executable string

	HTTP *http.Client
}

// New builds an Updater for one executable of the release archive.
func New(member string) *Updater {
	executable, _ := os.Executable()

	return &Updater{
		Member:     member,
		BaseURL:    ReleaseBaseURL,
		LatestURL:  LatestReleaseURL,
		GOOS:       runtime.GOOS,
		GOARCH:     runtime.GOARCH,
		Executable: executable,
		HTTP: &http.Client{
			Timeout:       60 * time.Second,
			CheckRedirect: httpsOnlyRedirect,
		},
	}
}

// AssetName names the release archive for a platform. The platform and the
// architecture belong in the filename, and the archive is a tar.gz everywhere
// except Windows. It is the shape the packer writes (<ArchiveFamily>_<version>
// _<os>_<arch>) — an updater that asks for anything else is broken.
func AssetName(goos string, goarch string, version string) (string, error) {
	if !versionPattern.MatchString(version) {
		return "", fmt.Errorf("invalid release version: %q", version)
	}

	if goarch != "amd64" && goarch != "arm64" {
		return "", fmt.Errorf("no release build for %s/%s", goos, goarch)
	}

	switch goos {
	case "windows":
		return fmt.Sprintf("%s_%s_%s_%s.zip", ArchiveFamily, version, goos, goarch), nil
	case "linux", "darwin":
		return fmt.Sprintf("%s_%s_%s_%s.tar.gz", ArchiveFamily, version, goos, goarch), nil
	default:
		return "", fmt.Errorf("no release build for %s/%s", goos, goarch)
	}
}

// Latest returns the newest published tag.
func (u *Updater) Latest() (string, error) {
	if err := u.requireHTTPS(u.LatestURL); err != nil {
		return "", err
	}

	response, err := u.HTTP.Get(u.LatestURL)
	if err != nil {
		return "", fmt.Errorf("check for a new release: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("check for a new release: %s", response.Status)
	}

	var release struct {
		TagName string `json:"tag_name"`
	}

	if err := json.NewDecoder(response.Body).Decode(&release); err != nil {
		return "", fmt.Errorf("read the latest release: %w", err)
	}

	if release.TagName == "" {
		return "", fmt.Errorf("read the latest release: no tag in the response")
	}

	return release.TagName, nil
}

// Install downloads, verifies and applies one release, keeping the binary it
// replaces so Restore can put it back.
//
// current is recorded so the window can say what it came from; the updater
// does not read the running version itself.
func (u *Updater) Install(current string, version string) error {
	asset, err := AssetName(u.GOOS, u.GOARCH, version)
	if err != nil {
		return err
	}

	binary, err := u.fetchBinary(version, asset)
	if err != nil {
		return err
	}

	backup := u.Executable + ".old"
	options := selfupdate.Options{}
	if u.Executable != "" {
		options.OldSavePath = backup
	}

	if err := applyUpdate(bytes.NewReader(binary), options); err != nil {
		return fmt.Errorf("apply %s: %w", version, err)
	}

	if current != "" && u.Executable != "" {
		_ = config.Set("update_previous_version", current)
		_ = config.Set("update_previous_path", backup)
	}

	return nil
}

// Previous reports the version installed before the current one, if the
// installation recorded one.
func (u *Updater) Previous() (string, bool) {
	version, err := config.Value("update_previous_version")
	if err != nil || version == "" {
		return "", false
	}

	return version, true
}

// Restore puts the saved previous binary back. It is a local file operation:
// no network, and it works when the server is down.
func (u *Updater) Restore() error {
	path, err := config.Value("update_previous_path")
	if err != nil {
		return fmt.Errorf("no previous version is recorded")
	}

	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("read the previous binary: %w", err)
	}
	defer file.Close()

	if err := applyUpdate(file, selfupdate.Options{}); err != nil {
		return fmt.Errorf("restore the previous binary: %w", err)
	}

	_ = config.Remove("update_previous_version")
	_ = config.Remove("update_previous_path")

	return nil
}

// fetchBinary downloads the asset, verifies it against the release's
// checksums.txt and returns the executable inside it.
func (u *Updater) fetchBinary(version string, asset string) ([]byte, error) {
	expected, err := u.fetchChecksum(version, asset)
	if err != nil {
		return nil, err
	}

	url, err := u.releaseURL(version, asset)
	if err != nil {
		return nil, err
	}

	body, err := u.download(url, maxDownloadBytes)
	if err != nil {
		return nil, err
	}

	actual := sha256.Sum256(body)
	if !bytes.Equal(actual[:], expected) {
		return nil, fmt.Errorf("checksum mismatch for %s", asset)
	}

	return binaryFromArchive(asset, body, u.Member)
}

func (u *Updater) releaseURL(version string, file string) (string, error) {
	url := u.BaseURL + "/" + version + "/" + file
	if err := u.requireHTTPS(url); err != nil {
		return "", err
	}

	return url, nil
}

func (u *Updater) requireHTTPS(url string) error {
	if !strings.HasPrefix(url, "https://") {
		return fmt.Errorf("refusing a release URL that is not HTTPS: %s", url)
	}

	return nil
}

func (u *Updater) download(url string, limit int64) ([]byte, error) {
	response, err := u.HTTP.Get(url)
	if err != nil {
		return nil, fmt.Errorf("download %s: %w", url, err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download %s: %s", url, response.Status)
	}

	body, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", url, err)
	}

	if int64(len(body)) > limit {
		return nil, fmt.Errorf("read %s: larger than %d bytes", url, limit)
	}

	return body, nil
}

// fetchChecksum reads the SHA-256 of one asset from checksums.txt. A missing or
// malformed entry is an error: skipping the check would silently accept a
// tampered archive.
func (u *Updater) fetchChecksum(version string, asset string) ([]byte, error) {
	url, err := u.releaseURL(version, "checksums.txt")
	if err != nil {
		return nil, err
	}

	body, err := u.download(url, maxChecksumBytes)
	if err != nil {
		return nil, err
	}

	return parseChecksum(body, asset)
}

// parseChecksum reads one `sha256sum` line. The name may carry the "*" that
// sha256sum writes in binary mode.
func parseChecksum(body []byte, asset string) ([]byte, error) {
	for _, line := range strings.Split(string(body), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}

		if strings.TrimPrefix(fields[1], "*") != asset {
			continue
		}

		sum, err := hex.DecodeString(fields[0])
		if err != nil || len(sum) != sha256.Size {
			return nil, fmt.Errorf("checksums.txt has a malformed hash for %s", asset)
		}

		return sum, nil
	}

	return nil, fmt.Errorf("checksums.txt has no entry for %s", asset)
}

func binaryFromArchive(asset string, body []byte, member string) ([]byte, error) {
	if strings.HasSuffix(asset, ".zip") {
		return binaryFromZip(body, member+".exe")
	}

	if strings.HasSuffix(asset, ".tar.gz") {
		return binaryFromTarGz(body, member)
	}

	return nil, fmt.Errorf("unsupported archive %q", asset)
}

func binaryFromZip(body []byte, name string) ([]byte, error) {
	reader, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		return nil, fmt.Errorf("read zip: %w", err)
	}

	for _, file := range reader.File {
		if filepath.Base(file.Name) != name {
			continue
		}

		opened, err := file.Open()
		if err != nil {
			return nil, fmt.Errorf("open %s: %w", file.Name, err)
		}
		defer opened.Close()

		return readBinary(opened, name)
	}

	return nil, fmt.Errorf("zip carries no %s", name)
}

func binaryFromTarGz(body []byte, name string) ([]byte, error) {
	gzipReader, err := gzip.NewReader(bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("read gzip: %w", err)
	}
	defer gzipReader.Close()

	archive := tar.NewReader(gzipReader)

	for {
		header, err := archive.Next()
		if err == io.EOF {
			break
		}

		if err != nil {
			return nil, fmt.Errorf("read tar: %w", err)
		}

		if header.Typeflag != tar.TypeReg || filepath.Base(header.Name) != name {
			continue
		}

		return readBinary(archive, name)
	}

	return nil, fmt.Errorf("archive carries no %s", name)
}

// readBinary refuses an empty executable: selfupdate would write a zero-byte
// file over the running binary.
func readBinary(reader io.Reader, name string) ([]byte, error) {
	binary, err := io.ReadAll(io.LimitReader(reader, maxBinaryBytes))
	if err != nil {
		return nil, err
	}

	if len(binary) == 0 {
		return nil, fmt.Errorf("%s is empty", name)
	}

	return binary, nil
}

// httpsOnlyRedirect refuses a redirect to plaintext and keeps the default
// ten-hop limit a custom CheckRedirect would otherwise remove.
func httpsOnlyRedirect(request *http.Request, via []*http.Request) error {
	if len(via) >= 10 {
		return fmt.Errorf("stopped after %d redirects", len(via))
	}

	if request.URL.Scheme != "https" {
		return fmt.Errorf("refusing a redirect to %s", request.URL.Scheme)
	}

	return nil
}
