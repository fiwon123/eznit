//go:build fyne

package fyneui

import (
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

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

		if !strings.Contains(view.note.Text, "docker compose up db migrate") {
			t.Errorf("note = %q, want it to name the database that has to be up first", view.note.Text)
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

	if err := view.server().Start(); err != nil {
		t.Fatalf("start the fake server: %v", err)
	}

	t.Cleanup(func() { _ = view.server().Stop(5 * time.Second) })

	view.refresh()
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
