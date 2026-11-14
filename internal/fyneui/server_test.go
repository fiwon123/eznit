//go:build fyne

package fyneui

import (
	"context"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/fiwon123/eznit/internal/config"
	"github.com/fiwon123/eznit/internal/serverctl"
)

// newTestServerView builds the Server tab against a test app. Nothing is
// started: these tests cover what the form reads and writes.
func newTestServerView(t *testing.T, dir string) *serverView {
	t.Helper()

	return newTestServerViewIn(t, dir, filepath.Join(t.TempDir(), "config.json"))
}

// newTestServerViewIn builds the panel against a named config file, so a test
// can read what was kept for the next start.
func newTestServerViewIn(t *testing.T, dir, store string) *serverView {
	t.Helper()

	// The panel stores the chosen folder; a test must not touch the real one.
	t.Setenv(config.Env, store)

	app := test.NewApp()
	t.Cleanup(app.Quit)

	window := app.NewWindow("test")

	view := newServerView(&ui{
		app:     app,
		window:  window,
		updater: &appUpdater{version: "test"},
	}, dir)

	// Run wires the panel back into the window; a test that leaves it out would
	// not see the address the Server tab's configuration names.
	view.ui.server = view

	// Lay the tab out as the window would: a test that only builds it cannot
	// see what a person would (or would not) be able to reach.
	window.SetContent(view.content())

	return view
}

// bundleDir writes a bundle into a temp folder and returns it: a
// configuration, and the server executable the tab checks for before it offers
// Start. The executable is a placeholder — these tests read and write the
// configuration, they never run the server.
func bundleDir(t *testing.T, content string) string {
	t.Helper()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, serverctl.EnvFileName), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(dir, serverctl.BinaryName()), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	return dir
}

func TestServerViewShowsTheConfiguredValues(t *testing.T) {
	dir := bundleDir(t, "# test\nAPI_PORT=\"4321\"\nDB_HOST=\"127.0.0.1\"\n")

	view := newTestServerView(t, dir)

	if got := view.fields["API_PORT"].Text; got != "4321" {
		t.Errorf("API_PORT = %q, want %q", got, "4321")
	}
	if got := view.fields["DB_HOST"].Text; got != "127.0.0.1" {
		t.Errorf("DB_HOST = %q, want %q", got, "127.0.0.1")
	}
	if got := view.status.Text; got != "Stopped" {
		t.Errorf("status = %q, want %q", got, "Stopped")
	}
	if view.start.Disabled() {
		t.Error("Start is disabled while the server is stopped")
	}
	if !view.stop.Disabled() {
		t.Error("Stop is enabled while the server is stopped")
	}
}

// A port that is not a port must be refused before anything is written.
func TestServerViewRefusesAnInvalidPort(t *testing.T) {
	dir := bundleDir(t, "API_PORT=\"4321\"\n")

	view := newTestServerView(t, dir)
	view.fields["API_PORT"].SetText("not-a-port")
	view.saveConfig()

	if !strings.Contains(view.note.Text, "1 and 65535") {
		t.Errorf("note = %q, want it to explain the port range", view.note.Text)
	}

	file, err := serverctl.LoadEnv(filepath.Join(dir, serverctl.EnvFileName))
	if err != nil {
		t.Fatal(err)
	}
	if got := file.Value("API_PORT", ""); got != "4321" {
		t.Errorf("API_PORT on disk = %q, want the old value %q", got, "4321")
	}
}

// The password goes to the secret file, and the .env is pointed at it: the
// value itself must not appear in the .env.
func TestServerViewSavesThePasswordIntoTheSecretFile(t *testing.T) {
	dir := bundleDir(t, "API_PORT=\"4321\"\nDB_PWD=\"throwaway\"\n")

	view := newTestServerView(t, dir)
	view.password.SetText("hunter2")
	view.saveConfig()

	if !strings.Contains(view.note.Text, "Saved") {
		t.Errorf("note = %q, want it to say the configuration was saved", view.note.Text)
	}

	data, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(serverctl.SecretPath)))
	if err != nil {
		t.Fatalf("read the secret: %v", err)
	}
	if got := strings.TrimSpace(string(data)); got != "hunter2" {
		t.Errorf("the secret file holds %q, want the password", got)
	}

	info, err := os.Stat(filepath.Join(dir, filepath.FromSlash(serverctl.SecretPath)))
	if err != nil {
		t.Fatal(err)
	}
	if mode := info.Mode().Perm(); mode != 0o600 {
		t.Errorf("secret mode = %v, want 0600", mode)
	}

	env, err := serverctl.LoadEnv(filepath.Join(dir, serverctl.EnvFileName))
	if err != nil {
		t.Fatal(err)
	}
	if got := env.Value("DB_PASSWORD_FILE", ""); got != "./"+serverctl.SecretPath {
		t.Errorf("DB_PASSWORD_FILE = %q, want %q", got, "./"+serverctl.SecretPath)
	}
	if _, ok := env.Get("DB_PWD"); ok {
		t.Error("DB_PWD is still set, so it would win over the secret file")
	}

	saved, err := os.ReadFile(filepath.Join(dir, serverctl.EnvFileName))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(saved), "hunter2") {
		t.Error("the password was written into the .env as well")
	}

	if view.password.Text != "" {
		t.Error("the password entry still holds the value after a save")
	}
	if !strings.Contains(view.secret.Text, "set in") {
		t.Errorf("the secret line = %q, want it to report the file is set", view.secret.Text)
	}
}

// A folder with only the example offers to create the .env, and creating it
// produces a file that points at a local database.
func TestServerViewCreatesTheEnvFromTheExample(t *testing.T) {
	dir := t.TempDir()
	example := "API_PORT=\"4000\"\nDB_HOST=\"db\"\n"
	if err := os.WriteFile(filepath.Join(dir, serverctl.EnvExampleName), []byte(example), 0o644); err != nil {
		t.Fatal(err)
	}

	view := newTestServerView(t, dir)

	if !view.create.Visible() {
		t.Error("the create button is hidden although there is no .env")
	}
	if !strings.Contains(view.note.Text, "no .env") {
		t.Errorf("note = %q, want it to mention the missing .env", view.note.Text)
	}

	view.createEnv()

	if view.create.Visible() {
		t.Error("the create button is still visible after the .env was created")
	}

	env, err := serverctl.LoadEnv(filepath.Join(dir, serverctl.EnvFileName))
	if err != nil {
		t.Fatalf("the .env was not created: %v", err)
	}
	if got := env.Value("DB_HOST", ""); got != "127.0.0.1" {
		t.Errorf("DB_HOST = %q, want %q for a server on this machine", got, "127.0.0.1")
	}
	if got := view.fields["API_PORT"].Text; got != "4000" {
		t.Errorf("the form shows API_PORT %q after creating the .env, want %q", got, "4000")
	}
}

// Saving with no .env must say what to do instead of writing one silently. A
// folder with the example can make one; a folder with neither is not a bundle.
func TestServerViewRefusesToSaveWithoutAnEnv(t *testing.T) {
	t.Run("a folder that is not a bundle", func(t *testing.T) {
		view := newTestServerView(t, t.TempDir())

		view.saveConfig()

		if !strings.Contains(view.note.Text, "not a server bundle") {
			t.Errorf("note = %q, want it to say the folder is not a bundle", view.note.Text)
		}

		if _, err := os.Stat(filepath.Join(view.folderPath(), serverctl.EnvFileName)); err == nil {
			t.Error("a .env was written although there was nothing to copy")
		}
	})

	t.Run("a folder with the example", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, serverctl.EnvExampleName), []byte("API_PORT=\"4000\"\n"), 0o644); err != nil {
			t.Fatal(err)
		}

		view := newTestServerView(t, dir)

		view.saveConfig()

		if !strings.Contains(view.note.Text, "create one") {
			t.Errorf("note = %q, want it to point at creating the .env", view.note.Text)
		}
	})
}

// A folder that is not a bundle says so before anything is pressed: Start is
// off, Create is not offered, and the note names what is missing and where a
// bundle comes from.
func TestServerViewSaysWhenTheFolderIsNotABundle(t *testing.T) {
	view := newTestServerView(t, t.TempDir())

	if !view.start.Disabled() {
		t.Error("Start is enabled in a folder with no server binary")
	}

	if view.create.Visible() {
		t.Error("Create .env is offered although there is no example to copy")
	}

	for _, want := range []string{"not a server bundle", serverctl.BinaryName(), "release archive", "Files tab"} {
		if !strings.Contains(view.note.Text, want) {
			t.Errorf("note = %q, want it to mention %q", view.note.Text, want)
		}
	}
}

// The two halves of a bundle are judged separately: a configured folder with no
// binary cannot start, and a folder with only the example can still make a
// configuration.
func TestServerViewJudgesTheTwoHalvesOfABundle(t *testing.T) {
	t.Run("a configuration without a server", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, serverctl.EnvFileName), []byte("API_PORT=\"4000\"\n"), 0o600); err != nil {
			t.Fatal(err)
		}

		view := newTestServerView(t, dir)

		if !view.start.Disabled() {
			t.Error("Start is enabled with no server binary in the folder")
		}

		if !strings.Contains(view.note.Text, serverctl.BinaryName()) {
			t.Errorf("note = %q, want it to name the missing binary", view.note.Text)
		}
	})

	t.Run("an example without a server", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, serverctl.EnvExampleName), []byte("API_PORT=\"4000\"\n"), 0o644); err != nil {
			t.Fatal(err)
		}

		view := newTestServerView(t, dir)

		if !view.create.Visible() {
			t.Error("Create .env is hidden although the example is there")
		}

		if !view.start.Disabled() {
			t.Error("Start is enabled with no server binary in the folder")
		}

		if !strings.Contains(view.note.Text, "Start brings up the database") {
			t.Errorf("note = %q, want it to say Start brings the database up", view.note.Text)
		}

		if !strings.Contains(view.note.Text, "Start also needs") {
			t.Errorf("note = %q, want it to say Start needs the binary", view.note.Text)
		}
	})
}

// The API prints one JSON record per line (pkg/logger.NewJson). The grid must
// show what a person reads, not the record's keys.
func TestServerViewFormatsTheServerLog(t *testing.T) {
	view := newTestServerView(t, bundleDir(t, "API_PORT=\"4321\"\n"))

	line := `{"level":"info","ts":"2026-11-12T14:03:05Z","msg":"Server Running...","caller":"main.go:110"}` + "\n"
	if _, err := view.server().Write([]byte(line)); err != nil {
		t.Fatalf("write the log: %v", err)
	}

	view.refresh()

	row := view.log.RowText(0)
	if !strings.Contains(row, "INFO") || !strings.Contains(row, "Server Running...") {
		t.Errorf("row 0 = %q, want a level column and the message", row)
	}
	if strings.Contains(row, `"msg"`) || strings.Contains(row, `"level"`) {
		t.Errorf("row 0 = %q, want the JSON keys gone", row)
	}
	if !strings.Contains(row, "caller=main.go:110") {
		t.Errorf("row 0 = %q, want the field as key=value", row)
	}
}

// A new line appends a row: the grid must not be rebuilt from scratch, which
// is what made a chatty server stall the window.
func TestServerViewAppendsNewLines(t *testing.T) {
	view := newTestServerView(t, bundleDir(t, "API_PORT=\"4321\"\n"))

	for _, message := range []string{"first", "second"} {
		if _, err := view.server().Write([]byte("INFO\t" + message + "\n")); err != nil {
			t.Fatal(err)
		}
	}
	view.refresh()

	if rows := len(view.log.Rows); rows != 2 {
		t.Fatalf("the grid holds %d rows, want 2", rows)
	}
	if got := view.log.RowText(1); !strings.Contains(got, "second") {
		t.Errorf("row 1 = %q, want the second line", got)
	}
}

// The supervisor keeps only the newest lines; the grid drops the same rows
// from the top, so it keeps showing the buffer rather than a stale prefix.
func TestServerViewFollowsATrimmedLog(t *testing.T) {
	view := newTestServerView(t, bundleDir(t, "API_PORT=\"4321\"\n"))

	const total = 505
	for line := 1; line <= total; line++ {
		if _, err := view.server().Write([]byte("INFO\tline " + strconv.Itoa(line) + "\n")); err != nil {
			t.Fatal(err)
		}
	}
	view.refresh()

	held := len(view.server().Output())
	if rows := len(view.log.Rows); rows != held {
		t.Fatalf("the grid holds %d rows, want the %d the supervisor kept", rows, held)
	}

	// The oldest line the buffer still holds is the one it kept: total - held.
	if got := view.log.RowText(0); !strings.Contains(got, "line "+strconv.Itoa(total-held+1)) {
		t.Errorf("row 0 = %q, want the oldest line still buffered", got)
	}
	if got := view.log.RowText(held - 1); !strings.Contains(got, "line "+strconv.Itoa(total)) {
		t.Errorf("last row = %q, want the newest line", got)
	}
}

// Clearing forgets the grid, and the next line starts a new one.
func TestServerViewClearsTheLog(t *testing.T) {
	view := newTestServerView(t, bundleDir(t, "API_PORT=\"4321\"\n"))

	for _, message := range []string{"first", "second"} {
		if _, err := view.server().Write([]byte("INFO\t" + message + "\n")); err != nil {
			t.Fatal(err)
		}
	}
	view.refresh()
	view.clearLog()

	if got := view.log.Text(); got != "" {
		t.Errorf("the grid still holds %q after a clear", got)
	}

	if _, err := view.server().Write([]byte("INFO\tafter the clear\n")); err != nil {
		t.Fatal(err)
	}
	view.refresh()

	if rows := len(view.log.Rows); rows != 1 {
		t.Fatalf("the grid holds %d rows, want the one new line", rows)
	}
	if got := view.log.RowText(0); !strings.Contains(got, "after the clear") {
		t.Errorf("row 0 = %q, want the new line", got)
	}
}

// The log has no selection of its own, so copying is the button. It copies
// what is on screen, formatted.
func TestServerViewCopiesTheLog(t *testing.T) {
	view := newTestServerView(t, bundleDir(t, "API_PORT=\"4321\"\n"))
	view.ui.clip = test.NewClipboard()

	if _, err := view.server().Write([]byte("INFO\tsomething happened\n")); err != nil {
		t.Fatal(err)
	}
	view.refresh()
	view.copyLog()

	copied := view.ui.clip.Content()
	if !strings.Contains(copied, "something happened") {
		t.Errorf("clipboard = %q, want the log line", copied)
	}
	if !strings.Contains(view.note.Text, "Copied") {
		t.Errorf("note = %q, want confirmation of the copy", view.note.Text)
	}
}

// Following is on by default and turns itself off when the reader scrolls
// away from the end, so new output cannot yank the view back.
func TestServerViewStopsFollowingWhenTheReaderScrollsUp(t *testing.T) {
	view := newTestServerView(t, bundleDir(t, "API_PORT=\"4321\"\n"))

	for range 200 {
		if _, err := view.server().Write([]byte("INFO\tline\n")); err != nil {
			t.Fatal(err)
		}
	}

	size := fyne.NewSize(800, 600)
	view.ui.window.Resize(size)
	view.ui.window.Content().Resize(size)
	view.refresh()

	if !view.follow.Checked {
		t.Fatal("following is off before the reader scrolled")
	}

	view.followOutput(fyne.NewPos(0, 0))

	if view.follow.Checked {
		t.Error("following is still on after the reader scrolled to the top")
	}
}

// The resolved folder is the flag, then the stored choice, then the folder the
// window was installed in.
func TestServerFromPrefersTheFlagThenTheStore(t *testing.T) {
	t.Setenv(config.Env, filepath.Join(t.TempDir(), "config.json"))

	flagged := t.TempDir()
	if got := serverFrom(flagged); got != flagged {
		t.Errorf("serverFrom(flag) = %q, want the flag %q", got, flagged)
	}

	if got := serverFrom(""); got != defaultServerDir() {
		t.Errorf("serverFrom(\"\") = %q, want the installed folder %q", got, defaultServerDir())
	}

	stored := t.TempDir()
	if err := config.Set("server_dir", stored); err != nil {
		t.Fatal(err)
	}

	if got := serverFrom(""); got != stored {
		t.Errorf("serverFrom(\"\") = %q, want the stored folder %q", got, stored)
	}
}

// Building the tab must not record the default as if it were a choice: that is
// what made the folder impossible to get back to.
func TestServerViewDoesNotStoreTheDefaultFolder(t *testing.T) {
	store := filepath.Join(t.TempDir(), "config.json")

	newTestServerViewIn(t, t.TempDir(), store)

	if _, err := os.Stat(store); err == nil {
		t.Error("building the tab wrote the config file, so the default became a choice")
	}
}

// A folder a person picks is remembered for the next start.
func TestServerViewRemembersAChosenFolder(t *testing.T) {
	store := filepath.Join(t.TempDir(), "config.json")
	view := newTestServerViewIn(t, t.TempDir(), store)

	chosen := t.TempDir()
	if !view.useChosenDir(chosen) {
		t.Fatal("the folder change was refused while the server is stopped")
	}

	if got := view.folderPath(); got != chosen {
		t.Errorf("the panel drives %q, want %q", got, chosen)
	}
	if got := view.folder.Text; got != chosen {
		t.Errorf("the header shows %q, want %q", got, chosen)
	}

	stored, err := config.Value("server_dir")
	if err != nil || stored != chosen {
		t.Errorf("stored server_dir = %q (%v), want %q", stored, err, chosen)
	}
}

// Resetting forgets the stored choice and points the panel at the folder the
// window was installed in, so a moved binary picks its own folder next launch.
func TestServerViewResetsToTheInstalledFolder(t *testing.T) {
	store := filepath.Join(t.TempDir(), "config.json")
	view := newTestServerViewIn(t, t.TempDir(), store)

	view.useChosenDir(t.TempDir())
	view.applyReset()

	if got := view.folderPath(); got != defaultServerDir() {
		t.Errorf("the panel drives %q, want the installed folder %q", got, defaultServerDir())
	}
	if got := view.folder.Text; got != defaultServerDir() {
		t.Errorf("the header shows %q, want %q", got, defaultServerDir())
	}

	if _, err := config.Value("server_dir"); err == nil {
		t.Error("server_dir is still stored after a reset, so the default did not stick")
	}
}

// A running server belongs to the folder it was started from; swapping folders
// would leave the process with nobody holding it.
func TestServerViewRefusesAFolderChangeWhileRunning(t *testing.T) {
	dir := bundleDir(t, "API_PORT=\"4321\"\n")
	view := newTestServerViewIn(t, dir, filepath.Join(t.TempDir(), "config.json"))

	startFakeServer(t, view)

	other := t.TempDir()
	if view.useChosenDir(other) {
		t.Error("a folder change was accepted while the server runs")
	}
	if got := view.folderPath(); got != dir {
		t.Errorf("the panel drives %q, want the folder the server was started from", got)
	}
	if !strings.Contains(view.note.Text, "Stop the server first") {
		t.Errorf("note = %q, want it to say the server has to stop first", view.note.Text)
	}
	if !view.choose.Disabled() || !view.reset.Disabled() {
		t.Error("the folder controls are still enabled while the server runs")
	}
}

// startFakeServer puts a sleeping eznit-server in the panel's folder and starts
// it, so a test can exercise the rules that only hold while it runs.
func startFakeServer(t *testing.T, view *serverView) {
	t.Helper()

	fakeServerBundle(t, view)

	if err := view.server().Start(); err != nil {
		t.Fatalf("start the fake server: %v", err)
	}

	t.Cleanup(func() { _ = view.server().Stop(5 * time.Second) })

	view.refresh()
}

// fakeServerBundle writes the fake server and a free API port into the panel's
// folder, so a test can start it through the panel's own Start.
func fakeServerBundle(t *testing.T, view *serverView) {
	t.Helper()

	if runtime.GOOS == "windows" {
		t.Skip("the fake server is a shell script; Windows builds cannot run it")
	}

	dir := view.folderPath()

	binary := filepath.Join(dir, serverctl.ServerBinaryName)
	if err := os.WriteFile(binary, []byte("#!/bin/sh\nexec sleep 30\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	// The start checks the port is free before it runs anything, so the .env
	// must name one nothing is using.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	port := strconv.Itoa(listener.Addr().(*net.TCPAddr).Port)
	listener.Close()

	env := "API_PORT=\"" + port + "\"\n"
	if err := os.WriteFile(filepath.Join(dir, serverctl.EnvFileName), []byte(env), 0o600); err != nil {
		t.Fatal(err)
	}

	view.loadConfig()
}

// The bundle folder is typed as well as picked: Use commits a path that
// exists, stores it for the next start, and reloads the tab from there.
func TestServerFolderIsTyped(t *testing.T) {
	view := newTestServerView(t, bundleDir(t, "API_PORT=\"4000\"\n"))

	target := bundleDir(t, "API_PORT=\"4100\"\n")

	view.folder.SetText(target)
	view.useTypedFolder()

	if got := view.folderPath(); got != target {
		t.Errorf("the panel drives %q, want the typed folder %q", got, target)
	}

	stored, err := config.Value("server_dir")
	if err != nil || stored != target {
		t.Errorf("stored server_dir = %q (%v), want %q", stored, err, target)
	}

	if got := view.fields["API_PORT"].Text; got != "4100" {
		t.Errorf("API_PORT = %q, want the value from the new folder", got)
	}

	if !strings.Contains(view.note.Text, "Using the folder") {
		t.Errorf("note = %q, want it to confirm the folder", view.note.Text)
	}
}

// A typed path is checked before anything is changed: a file and a missing
// folder are refused with the reason, and the tab keeps the folder it had.
func TestServerFolderTypedPathIsValidated(t *testing.T) {
	dir := bundleDir(t, "API_PORT=\"4000\"\n")
	view := newTestServerView(t, dir)

	view.folder.SetText(filepath.Join(dir, serverctl.EnvFileName))
	view.useTypedFolder()

	if got := view.folderPath(); got != dir {
		t.Errorf("the panel drives %q, want the folder unchanged", got)
	}

	if !strings.Contains(view.note.Text, "not a folder") {
		t.Errorf("note = %q, want it to say the path is a file", view.note.Text)
	}

	view.folder.SetText(filepath.Join(t.TempDir(), "missing"))
	view.useTypedFolder()

	if got := view.folderPath(); got != dir {
		t.Errorf("the panel drives %q, want the folder unchanged", got)
	}

	if !strings.Contains(view.note.Text, "could not be opened") {
		t.Errorf("note = %q, want it to say the folder is missing", view.note.Text)
	}
}

// A running server owns its folder: a typed path is refused like a picked one.
func TestServerFolderTypedChangeIsRefusedWhileRunning(t *testing.T) {
	dir := bundleDir(t, "API_PORT=\"4321\"\n")
	view := newTestServerViewIn(t, dir, filepath.Join(t.TempDir(), "config.json"))

	startFakeServer(t, view)

	view.folder.SetText(t.TempDir())
	view.useTypedFolder()

	if got := view.folderPath(); got != dir {
		t.Errorf("the panel drives %q, want the folder the server runs from", got)
	}

	if !strings.Contains(view.note.Text, "Stop the server first") {
		t.Errorf("note = %q, want it to say the server has to stop first", view.note.Text)
	}
}

// The configuration pane shows what a save would change, field by field, in the
// form's order, and says when the database password would be set.
func TestConfigurationShowsPendingChanges(t *testing.T) {
	dir := bundleDir(t, "API_PORT=\"4000\"\nDB_HOST=\"127.0.0.1\"\n")
	view := newTestServerView(t, dir)

	if strings.Contains(view.secret.Text, "Unsaved") {
		t.Fatalf("the clean form reports changes: %q", view.secret.Text)
	}

	view.fields["API_PORT"].SetText("5000")
	view.fields["DB_HOST"].SetText("")
	view.password.SetText("hunter2")

	for _, want := range []string{
		"Unsaved changes",
		"API_PORT 4000 → 5000",
		"DB_HOST 127.0.0.1 → (empty)",
		"database password will be set",
	} {
		if !strings.Contains(view.secret.Text, want) {
			t.Errorf("line = %q, want %q", view.secret.Text, want)
		}
	}

	if view.revert.Disabled() {
		t.Error("Revert is disabled with pending changes")
	}
}

// Revert puts the form back to the file: the fields, the password and the line.
func TestConfigurationRevertDiscardsChanges(t *testing.T) {
	dir := bundleDir(t, "API_PORT=\"4000\"\n")
	view := newTestServerView(t, dir)

	view.fields["API_PORT"].SetText("5000")
	view.password.SetText("hunter2")
	view.revertChanges()

	if got := view.fields["API_PORT"].Text; got != "4000" {
		t.Errorf("API_PORT = %q, want the file's value", got)
	}

	if view.password.Text != "" {
		t.Error("the password field still holds a value")
	}

	if strings.Contains(view.secret.Text, "Unsaved") {
		t.Errorf("line = %q, want the clean state", view.secret.Text)
	}

	if !view.revert.Disabled() {
		t.Error("Revert is enabled with nothing to discard")
	}

	if !strings.Contains(view.note.Text, "Changes discarded") {
		t.Errorf("note = %q, want it to confirm the discard", view.note.Text)
	}
}

// Saving an unchanged form writes nothing and says so: the line already says
// there is nothing to change.
func TestConfigurationSavesNothingWhenUnchanged(t *testing.T) {
	dir := bundleDir(t, "API_PORT=\"4000\"\n")
	view := newTestServerView(t, dir)

	before, err := os.ReadFile(filepath.Join(dir, serverctl.EnvFileName))
	if err != nil {
		t.Fatal(err)
	}

	view.saveConfig()

	after, err := os.ReadFile(filepath.Join(dir, serverctl.EnvFileName))
	if err != nil {
		t.Fatal(err)
	}

	if string(before) != string(after) {
		t.Error("the file was rewritten although nothing changed")
	}

	if !strings.Contains(view.note.Text, "nothing to save") {
		t.Errorf("note = %q, want it to say there is nothing to save", view.note.Text)
	}
}

// A pending edit is marked on the field itself: a tinted input and a warning
// caption, cleared when the value matches the file again or when Revert runs.
func TestConfigurationMarksChangedFields(t *testing.T) {
	dir := bundleDir(t, "API_PORT=\"4000\"\nDB_HOST=\"127.0.0.1\"\n")
	view := newTestServerView(t, dir)

	if _, tinted := view.fieldOverrides["API_PORT"].Theme.(changedTheme); tinted {
		t.Fatal("an untouched field starts tinted")
	}

	view.fields["API_PORT"].SetText("5000")

	if _, tinted := view.fieldOverrides["API_PORT"].Theme.(changedTheme); !tinted {
		t.Error("the changed field is not tinted")
	}

	if got := view.fieldCaptions["API_PORT"].Importance; got != widget.WarningImportance {
		t.Errorf("caption importance = %v, want the warning colour", got)
	}

	if _, tinted := view.fieldOverrides["DB_HOST"].Theme.(changedTheme); tinted {
		t.Error("an untouched field was tinted as well")
	}

	view.fields["API_PORT"].SetText("4000")

	if _, tinted := view.fieldOverrides["API_PORT"].Theme.(changedTheme); tinted {
		t.Error("the field is still tinted after being typed back to the file's value")
	}

	view.fields["API_PORT"].SetText("5000")
	view.password.SetText("hunter2")

	if _, tinted := view.passwordOverride.Theme.(changedTheme); !tinted {
		t.Error("a typed password is not tinted")
	}

	view.revertChanges()

	for key, override := range view.fieldOverrides {
		if _, tinted := override.Theme.(changedTheme); tinted {
			t.Errorf("%s is still tinted after a revert", key)
		}
	}

	if _, tinted := view.passwordOverride.Theme.(changedTheme); tinted {
		t.Error("the password is still tinted after a revert")
	}
}

// An exit must not replace the address: the line beside the error still says
// which server stopped.
func TestServerHealthKeepsTheAddressAfterAnExit(t *testing.T) {
	dir := bundleDir(t, "API_PORT=\"4321\"\n")
	view := newTestServerViewIn(t, dir, filepath.Join(t.TempDir(), "config.json"))

	startFakeServer(t, view)

	if err := view.server().Stop(5 * time.Second); err != nil {
		t.Fatalf("stop the fake server: %v", err)
	}

	view.refresh()

	if view.localURL == "" {
		t.Fatal("the tab has no local address to keep")
	}

	if !strings.Contains(view.health.Text, view.localURL) {
		t.Errorf("health = %q, want the address %q to survive the exit", view.health.Text, view.localURL)
	}

	if !strings.Contains(view.health.Text, "last exit") {
		t.Errorf("health = %q, want the exit reason as well", view.health.Text)
	}
}

// The snapshot is the selectable copy of the log, and reading it does not
// disturb the grid that follows the server.
func TestOutputSnapshotIsSelectable(t *testing.T) {
	view := newTestServerView(t, bundleDir(t, "API_PORT=\"4321\"\n"))

	for _, message := range []string{"first", "second"} {
		if _, err := view.server().Write([]byte("INFO\t" + message + "\n")); err != nil {
			t.Fatal(err)
		}
	}

	view.refresh()
	rows := len(view.log.Rows)

	snapshot := view.outputSnapshot()

	if !snapshot.Selectable {
		t.Error("the snapshot cannot be selected")
	}

	if snapshot.Text != view.log.Text() {
		t.Error("the snapshot is not the log the grid shows")
	}

	if !strings.Contains(snapshot.Text, "second") {
		t.Errorf("snapshot = %q, want the last line", snapshot.Text)
	}

	if len(view.log.Rows) != rows {
		t.Error("making the snapshot changed the live grid")
	}
}

// newLogView is a Server tab that has printed the given lines and been laid
// out: a grid that was never laid out has no cell size, so every pointer
// position would map to cell (0,0).
func newLogView(t *testing.T, messages ...string) *serverView {
	t.Helper()

	view := newTestServerView(t, bundleDir(t, "API_PORT=\"4321\"\n"))

	for _, message := range messages {
		if _, err := view.server().Write([]byte("INFO\t" + message + "\n")); err != nil {
			t.Fatal(err)
		}
	}
	view.refresh()

	size := fyne.NewSize(980, 640)
	view.ui.window.Resize(size)
	view.ui.window.Content().Resize(size)

	return view
}

// cellPoint is the middle of a cell, which is where a pointer has to be for
// its position to map back to that cell and not to the edge before it.
func cellPoint(view *serverView, at cell) fyne.Position {
	topLeft := view.log.PositionForCursorLocation(at.row, at.col)
	width := view.log.PositionForCursorLocation(0, 1).X
	height := view.log.PositionForCursorLocation(1, 0).Y

	return fyne.NewPos(topLeft.X+width/2, topLeft.Y+height/2)
}

// dragOver presses at one cell and drags to another, in the order the driver
// sends the events.
func dragOver(view *serverView, from, to cell) {
	view.grid.MouseDown(&desktop.MouseEvent{
		PointEvent: fyne.PointEvent{Position: cellPoint(view, from)},
		Button:     desktop.MouseButtonPrimary,
	})
	view.grid.Dragged(&fyne.DragEvent{
		PointEvent: fyne.PointEvent{Position: cellPoint(view, to)},
	})
	view.grid.MouseUp(&desktop.MouseEvent{})
}

// A press and a drag mark the cells under the pointer, and what they mark is
// exactly what gets copied (#308).
func TestServerViewSelectsWithADrag(t *testing.T) {
	view := newLogView(t, "first", "second", "third")

	// Row 1 whole, and the first five characters of row 2.
	dragOver(view, cell{1, 0}, cell{2, 4})

	if view.sel == nil {
		t.Fatal("the drag selected nothing")
	}
	if view.sel.start != (cell{row: 1, col: 0}) || view.sel.end != (cell{row: 2, col: 4}) {
		t.Errorf("selection = %+v, want rows 1..2 from column 0 to 4", *view.sel)
	}

	want := view.log.RowText(1) + "\n" + string([]rune(view.log.RowText(2))[:5])
	if got := view.selectionText(); got != want {
		t.Errorf("selected text = %q, want %q", got, want)
	}
	if !strings.Contains(view.selectionText(), "second") {
		t.Errorf("selected text = %q, want the second line in it", view.selectionText())
	}
	if view.copySelection.Disabled() {
		t.Error("Copy selection is disabled with a selection")
	}
}

// The button copies the selection, and the whole-buffer button stays whole.
func TestServerViewCopiesJustTheSelection(t *testing.T) {
	view := newLogView(t, "first", "second", "third")
	view.ui.clip = test.NewClipboard()

	line := view.log.RowText(0)
	dragOver(view, cell{0, 0}, cell{0, len([]rune(line)) - 1})
	view.copySelected()

	if got := view.ui.clip.Content(); got != line {
		t.Errorf("clipboard = %q, want the selected line %q", got, line)
	}
	if !strings.Contains(view.note.Text, "1 line(s)") {
		t.Errorf("note = %q, want the copy confirmed with its line count", view.note.Text)
	}

	view.copyLog()

	if got := view.ui.clip.Content(); !strings.Contains(got, "first") || !strings.Contains(got, "third") {
		t.Errorf("clipboard after Copy log = %q, want the whole buffer", got)
	}
}

// Selected cells keep their level's colour and gain the theme's selection
// behind them; cells outside the selection keep no background at all.
func TestServerViewPaintsTheSelection(t *testing.T) {
	for name, th := range map[string]fyne.Theme{"light": theme.LightTheme(), "dark": theme.DarkTheme()} {
		t.Run(name, func(t *testing.T) {
			view := newLogView(t, "first", "second", "third")
			view.ui.app.Settings().SetTheme(th)

			dragOver(view, cell{1, 0}, cell{1, 2})

			selected := view.log.Rows[1].Cells[0].Style
			if selected == nil || selected.BackgroundColor() == nil {
				t.Fatal("the selected cell has no background")
			}
			if got, want := selected.BackgroundColor(), th.Color(theme.ColorNameSelection, view.ui.app.Settings().ThemeVariant()); got != want {
				t.Errorf("selection background = %v, want the theme's %v", got, want)
			}

			level := logStyle(view.ui.app.Settings().ThemeVariant(), view.parsed[1])
			if got, want := selected.TextColor(), level.FGColor; got != want {
				t.Errorf("selected text colour = %v, want the level's %v", got, want)
			}
			if got, want := selected.Style().Bold, level.TextStyle.Bold; got != want {
				t.Errorf("selected bold = %v, want the level's %v", got, want)
			}

			if outside := view.log.Rows[0].Cells[0].Style; outside != nil && outside.BackgroundColor() != nil {
				t.Errorf("an unselected cell has a background: %v", outside.BackgroundColor())
			}
			if after := view.log.Rows[1].Cells[3].Style; after != nil && after.BackgroundColor() != nil {
				t.Errorf("a cell past the selection has a background: %v", after.BackgroundColor())
			}
		})
	}
}

// A click on its own clears: a drag is what makes a range, and a stale
// selection would be copied by the next press of the button.
func TestServerViewClickClearsTheSelection(t *testing.T) {
	view := newLogView(t, "first", "second", "third")

	dragOver(view, cell{0, 0}, cell{2, 4})
	if view.sel == nil {
		t.Fatal("the drag selected nothing")
	}

	view.grid.MouseDown(&desktop.MouseEvent{
		PointEvent: fyne.PointEvent{Position: cellPoint(view, cell{1, 1})},
		Button:     desktop.MouseButtonPrimary,
	})
	view.grid.MouseUp(&desktop.MouseEvent{})

	if view.sel != nil {
		t.Errorf("a click left the selection %+v", *view.sel)
	}
	if !view.copySelection.Disabled() {
		t.Error("Copy selection is enabled with nothing selected")
	}
	if style := view.log.Rows[0].Cells[0].Style; style != nil {
		t.Errorf("the cleared selection is still painted: %+v", style)
	}
}

// The buffer drops its oldest lines as the server prints: the selection has to
// stay on the same text, and go when that text does.
func TestServerViewSelectionKeepsUpWithATrimmedLog(t *testing.T) {
	view := newLogView(t)

	// One write carries many lines: the supervisor notifies once, so the test
	// does not pay for a window refresh per line.
	writeLines := func(prefix string, count int) {
		t.Helper()

		var bulk strings.Builder
		for line := 1; line <= count; line++ {
			bulk.WriteString("INFO\t" + prefix + " " + strconv.Itoa(line) + "\n")
		}

		if _, err := view.server().Write([]byte(bulk.String())); err != nil {
			t.Fatal(err)
		}
	}

	// Far more than the buffer keeps, so it is certainly full and has certainly
	// trimmed: the cap is the supervisor's business, not this test's.
	writeLines("line", 1000)
	view.refresh()

	held := len(view.server().Output())
	if held >= 1000 {
		t.Fatalf("the supervisor kept all 1000 lines — nothing was trimmed")
	}
	if held < 20 {
		t.Fatalf("the supervisor kept only %d lines — too few to select inside", held)
	}
	if rows := len(view.log.Rows); rows != held {
		t.Fatalf("the grid holds %d rows, want the %d the supervisor keeps", rows, held)
	}

	dragOver(view, cell{10, 0}, cell{10, 70})
	if view.sel == nil || view.sel.start.row != 10 {
		t.Fatalf("the drag selected %+v, want row 10", view.sel)
	}
	marked := view.selectionText()

	writeLines("newer", 3)
	view.refresh()

	if view.sel == nil {
		t.Fatal("trimming three lines dropped the selection")
	}
	if got := view.sel.start.row; got != 7 {
		t.Errorf("the selection starts at row %d, want 7 after three lines left the top", got)
	}
	if got := view.selectionText(); got != marked {
		t.Errorf("the selection now reads %q, want the same text %q", got, marked)
	}

	// Enough new lines to evict the marked ones: there is nothing left to copy,
	// and copying the rows that took their place would be a lie.
	writeLines("later", held)
	view.refresh()

	if view.sel != nil {
		t.Errorf("the selection survived the lines it covered: %+v", *view.sel)
	}
	if !view.copySelection.Disabled() {
		t.Error("Copy selection is enabled after its text left the buffer")
	}
}

// Marking lines turns following off: output that arrives must not pull the
// view away from what is being selected.
func TestServerViewSelectingStopsFollowing(t *testing.T) {
	view := newLogView(t, "first", "second")

	if !view.follow.Checked {
		t.Fatal("following is off before anything was selected")
	}

	dragOver(view, cell{0, 0}, cell{0, 3})

	if view.follow.Checked {
		t.Error("following is still on with a selection")
	}
}

// The grid implements no mouse interface, so the driver has to find the
// wrapper around it. This is the assumption the whole feature rests on.
func TestLogGridTakesTheDragFromTheDriver(t *testing.T) {
	view := newLogView(t, "first", "second", "third")

	view.grid.MouseDown(&desktop.MouseEvent{
		PointEvent: fyne.PointEvent{Position: cellPoint(view, cell{0, 0})},
		Button:     desktop.MouseButtonPrimary,
	})

	at := fyne.CurrentApp().Driver().AbsolutePositionForObject(view.grid)
	test.Drag(view.ui.window.Canvas(), at.Add(cellPoint(view, cell{2, 2})), 0, 0)

	if view.sel == nil {
		t.Fatal("the drag did not reach the log grid")
	}
	if got := view.sel.end.row; got != 2 {
		t.Errorf("the drag ended on row %d, want 2", got)
	}
}

// A drag that no press started must not select from a stale anchor: the
// wrapper only marks from the cell the press landed on.
func TestLogGridIgnoresADragWithoutAPress(t *testing.T) {
	view := newLogView(t, "first", "second")

	view.grid.Dragged(&fyne.DragEvent{
		PointEvent: fyne.PointEvent{Position: cellPoint(view, cell{1, 3})},
	})

	if view.sel != nil {
		t.Errorf("a drag with no press selected %+v", *view.sel)
	}
}

// The span of one row a selection covers: rows outside it are not covered, and
// the start and end rows are partial.
func TestSelectionSpan(t *testing.T) {
	tests := []struct {
		name string
		sel  *selection
		row  int
		want rowRange
	}{
		{"no selection", nil, 0, rowRange{}},
		{"a row above", &selection{start: cell{1, 2}, end: cell{3, 4}}, 0, rowRange{}},
		{"a row below", &selection{start: cell{1, 2}, end: cell{3, 4}}, 4, rowRange{}},
		{"one row", &selection{start: cell{1, 2}, end: cell{1, 4}}, 1, rowRange{from: 2, to: 5, ok: true}},
		{"the first of several", &selection{start: cell{1, 2}, end: cell{3, 4}}, 1, rowRange{from: 2, to: -1, ok: true}},
		{"one in the middle", &selection{start: cell{1, 2}, end: cell{3, 4}}, 2, rowRange{from: 0, to: -1, ok: true}},
		{"the last of several", &selection{start: cell{1, 2}, end: cell{3, 4}}, 3, rowRange{from: 0, to: 5, ok: true}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.sel.span(tt.row); got != tt.want {
				t.Errorf("span(%d) = %+v, want %+v", tt.row, got, tt.want)
			}
		})
	}
}

// Clamping keeps a selection inside the buffer: the lines above it are gone, so
// it starts at the top of what is left, and a drag below the last line takes
// that line's end.
func TestSelectionClampedToTheBuffer(t *testing.T) {
	tests := []struct {
		name string
		sel  *selection
		rows int
		want *selection
	}{
		{"no selection", nil, 5, nil},
		{"an empty grid", &selection{start: cell{0, 0}, end: cell{0, 3}}, 0, nil},
		{"inside the buffer", &selection{start: cell{1, 2}, end: cell{3, 4}}, 5, &selection{start: cell{1, 2}, end: cell{3, 4}}},
		{"the first line left", &selection{start: cell{-2, 5}, end: cell{3, 4}}, 5, &selection{start: cell{0, 0}, end: cell{3, 4}}},
		{"every line left", &selection{start: cell{-4, 1}, end: cell{-1, 4}}, 5, nil},
		{"past the last line", &selection{start: cell{1, 2}, end: cell{9, 4}}, 5, &selection{start: cell{1, 2}, end: cell{4, endOfLine}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.sel.clamped(tt.rows)

			if (got == nil) != (tt.want == nil) {
				t.Fatalf("clamped(%d) = %+v, want %+v", tt.rows, got, tt.want)
			}
			if got != nil && *got != *tt.want {
				t.Errorf("clamped(%d) = %+v, want %+v", tt.rows, *got, *tt.want)
			}
		})
	}
}

// A rune's column and its index in a row are not the same number once a tab or
// a wide rune is padded: rowCellRunes has to read the row exactly as Fyne does.
func TestRowCellRunesMatchTheRowsOwnText(t *testing.T) {
	for _, text := range []string{"no tabs at all", "\tindented", "a\tb\tc", "wide 世界 text", "tab\tand 世界"} {
		t.Run(text, func(t *testing.T) {
			grid := widget.NewTextGrid()
			grid.SetText(text)

			row := grid.Rows[0]
			runes := rowCellRunes(row, grid.TabWidth)

			var joined strings.Builder
			for _, at := range runes {
				joined.WriteRune(at.r)
			}

			if got, want := joined.String(), grid.RowText(0); got != want {
				t.Errorf("rowCellRunes read %q, want the row's own text %q", got, want)
			}

			if len(runes) == len(row.Cells) && strings.ContainsAny(text, "\t") {
				t.Errorf("row %q has %d cells and %d runes — the padding was not skipped", text, len(row.Cells), len(runes))
			}
		})
	}
}

// Copying reads through that padding: a selection that covers a tab's cells and
// the text after them copies the text, not the padding spaces.
func TestSelectionTextReadsThroughTabPadding(t *testing.T) {
	grid := widget.NewTextGrid()
	grid.SetText("a\tb")

	view := &serverView{log: grid, parsed: make([]serverctl.LogLine, len(grid.Rows))}

	// The whole row: the text the grid shows, padding and all.
	view.sel = &selection{start: cell{0, 0}, end: cell{0, len(grid.Rows[0].Cells) - 1}}
	if got, want := view.selectionText(), grid.RowText(0); got != want {
		t.Errorf("whole row copied %q, want %q", got, want)
	}

	// From the tab's cell to the end: the tab and the character after it.
	view.sel = &selection{start: cell{0, 1}, end: cell{0, len(grid.Rows[0].Cells) - 1}}
	if got, want := view.selectionText(), "\tb"; got != want {
		t.Errorf("selection copied %q, want %q", got, want)
	}

	// A single cell after the padding: the rune it holds, not the padding.
	view.sel = &selection{start: cell{0, len(grid.Rows[0].Cells) - 1}, end: cell{0, len(grid.Rows[0].Cells) - 1}}
	if got, want := view.selectionText(), "b"; got != want {
		t.Errorf("the last cell copied %q, want %q", got, want)
	}

	// A column past the end of the row still copies the row's tail rather than
	// slicing past it.
	view.sel = &selection{start: cell{0, 1}, end: cell{0, endOfLine}}
	if got, want := view.selectionText(), "\tb"; got != want {
		t.Errorf("selection past the end copied %q, want %q", got, want)
	}
}

// A pointer that leaves the grid still means what a person meant: below the last
// line takes the rest of the log, left of the first column takes the line from
// its start, and neither panics.
func TestSelectionHandlesAPointerOutsideTheGrid(t *testing.T) {
	view := newLogView(t, "first", "second")

	press := func(at cell) {
		view.grid.MouseDown(&desktop.MouseEvent{
			PointEvent: fyne.PointEvent{Position: cellPoint(view, at)},
			Button:     desktop.MouseButtonPrimary,
		})
	}
	dragTo := func(point fyne.Position) {
		view.grid.Dragged(&fyne.DragEvent{PointEvent: fyne.PointEvent{Position: point}})
		view.grid.MouseUp(&desktop.MouseEvent{})
	}

	// Below the last line: everything down to the end of the log.
	press(cell{0, 0})
	below := cellPoint(view, cell{1, 0})
	height := view.log.PositionForCursorLocation(1, 0).Y
	dragTo(fyne.NewPos(below.X, below.Y+3*height))

	want := view.log.RowText(0) + "\n" + view.log.RowText(1)
	if got := view.selectionText(); got != want {
		t.Errorf("dragging below the log copied %q, want %q", got, want)
	}

	// Left of the first column: the line from its first character.
	press(cell{1, 10})
	dragTo(fyne.NewPos(-40, cellPoint(view, cell{1, 0}).Y))

	got := view.selectionText()
	if got == "" || !strings.HasPrefix(view.log.RowText(1), got) {
		t.Errorf("dragging past the left edge copied %q, want a prefix of %q", got, view.log.RowText(1))
	}
}

// A burst of output larger than the buffer, arriving after a clear, must not
// panic in the trim path: the grid can hold a row the parsed log does not.
func TestServerViewTakesABurstAfterAClear(t *testing.T) {
	view := newLogView(t, "first")
	view.clearLog()

	var burst strings.Builder
	for line := 1; line <= 600; line++ {
		burst.WriteString("INFO\tline " + strconv.Itoa(line) + "\n")
	}
	if _, err := view.server().Write([]byte(burst.String())); err != nil {
		t.Fatal(err)
	}

	view.refresh()

	rows := len(view.log.Rows)
	if rows == 0 {
		t.Fatal("the grid is empty after a burst")
	}
	if got := view.log.RowText(rows - 1); !strings.Contains(got, "line 600") {
		t.Errorf("the last row = %q, want the newest line", got)
	}
}

// The database step comes first, and a successful one starts the API and says
// what it did (#311).
func TestServerViewAfterPrepareStartsTheServer(t *testing.T) {
	view := newTestServerView(t, bundleDir(t, "API_PORT=\"4000\"\n"))
	fakeServerBundle(t, view)

	view.afterPrepare(serverctl.Result{Started: true, Migrated: true}, nil, &databaseProgress{})

	t.Cleanup(func() { _ = view.server().Stop(5 * time.Second) })

	if !view.running() {
		t.Fatal("the API was not started after the database step succeeded")
	}
	if !strings.Contains(view.note.Text, "Started the database") {
		t.Errorf("note = %q, want it to name the database that was started", view.note.Text)
	}
	if !strings.Contains(view.note.Text, "migrations") {
		t.Errorf("note = %q, want it to name the migrations", view.note.Text)
	}
}

// A database that was already running still gets its migrations applied, and
// the note says so rather than claiming a start.
func TestServerViewAfterPrepareOnlyMigrated(t *testing.T) {
	view := newTestServerView(t, bundleDir(t, "API_PORT=\"4000\"\n"))
	fakeServerBundle(t, view)

	view.afterPrepare(serverctl.Result{Migrated: true}, nil, &databaseProgress{})

	t.Cleanup(func() { _ = view.server().Stop(5 * time.Second) })

	if !strings.Contains(view.note.Text, "already running") {
		t.Errorf("note = %q, want it to say the database was already up", view.note.Text)
	}
	if !view.running() {
		t.Error("the API was not started after the migrations were applied")
	}
}

// No database and no way to start one: no API process, and a note naming the
// missing piece. A window that says "Running" over pq errors is the bug.
func TestServerViewAfterPrepareRefusesWithoutADatabase(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{"no compose file", serverctl.ErrNoCompose, "docker-compose.yml"},
		{"no runtime", serverctl.ErrNoRuntime, "Docker or Podman"},
		{"no password file", serverctl.ErrNoPassword, "Configuration"},
		{"never answered", serverctl.ErrNoAnswer, "press Start again"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			view := newTestServerView(t, bundleDir(t, "API_PORT=\"4000\"\n"))
			fakeServerBundle(t, view)

			view.afterPrepare(serverctl.Result{}, tt.err, &databaseProgress{})

			if view.running() {
				t.Fatal("the API was started with no database to read")
			}
			if view.start.Disabled() {
				t.Error("Start is disabled after a database step that can be retried")
			}
			if !strings.Contains(view.note.Text, tt.want) {
				t.Errorf("note = %q, want it to mention %q", view.note.Text, tt.want)
			}
		})
	}
}

// A runtime that failed is reported with its own output, and the API stays down.
func TestServerViewAfterPrepareReportsAFailedStep(t *testing.T) {
	view := newTestServerView(t, bundleDir(t, "API_PORT=\"4000\"\n"))
	fakeServerBundle(t, view)

	view.afterPrepare(serverctl.Result{}, errors.New("docker compose up -d db: exit status 1"), &databaseProgress{})

	if view.running() {
		t.Fatal("the API was started after the database step failed")
	}
	if !strings.Contains(view.note.Text, "could not be prepared") {
		t.Errorf("note = %q, want the failure named", view.note.Text)
	}
}

// The field is wired to the real step: a folder whose database address is dead
// and that carries no compose file is refused, without a process.
func TestServerViewPrepareIsTheRealStep(t *testing.T) {
	dir := bundleDir(t, "API_PORT=\"4000\"\nDB_HOST=\"127.0.0.1\"\nDB_PORT=\"1\"\n")
	view := newTestServerView(t, dir)

	_, err := view.prepare(context.Background(), dir, io.Discard)

	if !errors.Is(err, serverctl.ErrNoCompose) {
		t.Errorf("prepare() error = %v, want %v", err, serverctl.ErrNoCompose)
	}
}

// While the database step runs, Start and Stop are dead and the status says
// what is happening: a second press must not start a second server.
func TestServerViewDisablesStartWhilePreparing(t *testing.T) {
	view := newTestServerView(t, bundleDir(t, "API_PORT=\"4000\"\n"))
	fakeServerBundle(t, view)

	view.preparing = true
	view.refresh()

	if !view.start.Disabled() || !view.stop.Disabled() {
		t.Error("Start or Stop is still enabled while the database is being prepared")
	}
	if got := view.status.Text; got != "Preparing the database…" {
		t.Errorf("status = %q, want the database step named", got)
	}
	if !view.choose.Disabled() || !view.reset.Disabled() {
		t.Error("the folder is editable while the database step runs in it")
	}
}

// A folder change is refused while the database step is working in it.
func TestServerViewRefusesAFolderChangeWhilePreparing(t *testing.T) {
	view := newTestServerView(t, bundleDir(t, "API_PORT=\"4000\"\n"))

	view.preparing = true

	if view.canChangeFolder() {
		t.Error("a folder change was accepted while the database step runs")
	}
	if !strings.Contains(view.note.Text, "Wait for the database step") {
		t.Errorf("note = %q, want it to say the step has to finish", view.note.Text)
	}
}

// The modal keeps the tail of what the runtime said, which is what the failure
// dialog shows.
func TestDatabaseProgressKeepsTheTail(t *testing.T) {
	view := newTestServerView(t, bundleDir(t, "API_PORT=\"4000\"\n"))

	progress := view.showDatabaseProgress(func() {})
	defer progress.Close()

	for line := 1; line <= 300; line++ {
		if _, err := progress.Write([]byte("line " + strconv.Itoa(line) + "\n")); err != nil {
			t.Fatal(err)
		}
	}

	output := progress.Output()

	if !strings.Contains(output, "line 300") {
		t.Errorf("the output does not end with the newest line: %q", output)
	}
	if strings.Contains(output, "line 1\n") {
		t.Error("the output kept every line; the tail is bounded")
	}
}

// Start runs the database step through its seam, in the folder the tab is
// driving, with Start and Stop dead while it works.
func TestServerViewStartRunsTheDatabaseStep(t *testing.T) {
	dir := bundleDir(t, "API_PORT=\"4000\"\n")
	view := newTestServerView(t, dir)
	fakeServerBundle(t, view)

	called := make(chan string, 1)
	release := make(chan struct{})

	view.prepare = func(_ context.Context, folder string, _ io.Writer) (serverctl.Result, error) {
		called <- folder
		<-release

		return serverctl.Result{}, serverctl.ErrNoCompose
	}

	view.startServer()

	select {
	case folder := <-called:
		if folder != view.folderPath() {
			t.Errorf("the step ran for %q, want the panel's folder %q", folder, view.folderPath())
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Start never ran the database step")
	}

	// The step is still inside its call: the buttons must already be dead, so a
	// second press cannot start a second server.
	if !view.start.Disabled() || !view.stop.Disabled() {
		t.Error("Start or Stop is enabled while the database step runs")
	}

	close(release)

	// The driver runs fyne.Do inline on this goroutine, so the step hands back
	// before it ends; the test must not quit the app while that is running.
	deadline := time.Now().Add(5 * time.Second)
	for view.preparing && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if view.preparing {
		t.Fatal("the panel is still preparing after the step returned")
	}
	if !strings.Contains(view.note.Text, "docker-compose.yml") {
		t.Errorf("note = %q, want the refusal reported", view.note.Text)
	}
	if view.running() {
		t.Error("the API was started although the step refused")
	}
}

// A port the API cannot bind is refused before the database is touched: the
// containers are not worth starting for a server that cannot listen.
func TestServerViewStartRefusesABusyPort(t *testing.T) {
	view := newTestServerView(t, bundleDir(t, "API_PORT=\"4000\"\n"))
	fakeServerBundle(t, view)

	// Take the port the bundle names, the way another server would.
	port := view.fields["API_PORT"].Text
	listener, err := net.Listen("tcp", "127.0.0.1:"+port)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	prepared := false
	view.prepare = func(context.Context, string, io.Writer) (serverctl.Result, error) {
		prepared = true

		return serverctl.Result{}, nil
	}

	view.startServer()

	if prepared {
		t.Error("the database step ran for a server that cannot bind its port")
	}
	if !strings.Contains(view.note.Text, "already in use") {
		t.Errorf("note = %q, want the busy port named", view.note.Text)
	}
	if view.preparing {
		t.Error("the panel is stuck in the preparing state after refusing the port")
	}
}

// Cancelling the step is reported as a cancellation, not as a runtime failure,
// and Start comes back.
func TestServerViewReportsACancelledStep(t *testing.T) {
	view := newTestServerView(t, bundleDir(t, "API_PORT=\"4000\"\n"))
	fakeServerBundle(t, view)

	view.afterPrepare(serverctl.Result{}, context.Canceled, &databaseProgress{})

	if view.running() {
		t.Fatal("the API was started after the step was cancelled")
	}
	if !strings.Contains(view.note.Text, "cancelled") {
		t.Errorf("note = %q, want the cancellation named", view.note.Text)
	}
	if view.start.Disabled() {
		t.Error("Start is disabled after a cancellation")
	}
}
