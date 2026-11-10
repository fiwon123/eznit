//go:build fyne

package fyneui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"fyne.io/fyne/v2/test"

	"github.com/fiwon123/eznit/internal/config"
	"github.com/fiwon123/eznit/internal/serverctl"
)

// newTestServerView builds the Server tab against a test app. Nothing is
// started: these tests cover what the form reads and writes.
func newTestServerView(t *testing.T, dir string) *serverView {
	t.Helper()

	// The panel stores the chosen folder; a test must not touch the real one.
	t.Setenv(config.Env, filepath.Join(t.TempDir(), "config.json"))

	app := test.NewApp()
	t.Cleanup(app.Quit)

	window := app.NewWindow("test")

	view := newServerView(&ui{
		app:     app,
		window:  window,
		updater: &appUpdater{version: "test"},
	}, dir)

	// Lay the tab out as the window would: a test that only builds it cannot
	// see what a person would (or would not) be able to reach.
	window.SetContent(view.content())

	return view
}

// bundleDir writes a .env into a temp folder and returns it.
func bundleDir(t *testing.T, content string) string {
	t.Helper()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, serverctl.EnvFileName), []byte(content), 0o600); err != nil {
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

// Saving with no .env must say what to do instead of writing one silently.
func TestServerViewRefusesToSaveWithoutAnEnv(t *testing.T) {
	view := newTestServerView(t, t.TempDir())

	view.saveConfig()

	if !strings.Contains(view.note.Text, "create one") {
		t.Errorf("note = %q, want it to point at creating the .env", view.note.Text)
	}
}
