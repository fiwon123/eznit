//go:build fyne

package fyneui

import (
	"path/filepath"
	"strings"
	"testing"

	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"

	"github.com/fiwon123/eznit/internal/apiclient"
	"github.com/fiwon123/eznit/internal/config"
	"github.com/fiwon123/eznit/internal/domain/templates"
	"github.com/fiwon123/eznit/internal/update"
)

// The address is resolved in one order, and the source is reported with it:
// the environment wins over a saved address so the documented GUI_API_URL keeps
// working after a login has stored one.
func TestAPISourceOrder(t *testing.T) {
	t.Setenv(config.Env, filepath.Join(t.TempDir(), "config.json"))
	t.Setenv("GUI_API_URL", "http://from-env.test:4000")

	flagged := apiFrom("http://from-flag.test:4000")
	if flagged.url != "http://from-flag.test:4000" || !strings.Contains(flagged.source, "-api") {
		t.Errorf("apiFrom(flag) = %+v, want the flag and its name", flagged)
	}

	fromEnv := apiFrom("")
	if fromEnv.url != "http://from-env.test:4000" || fromEnv.source != "GUI_API_URL" {
		t.Errorf("apiFrom(\"\") = %+v, want GUI_API_URL", fromEnv)
	}

	if err := config.Set("api_url", "http://saved.test:4000"); err != nil {
		t.Fatal(err)
	}

	// With both set, the environment still wins: that is the order that keeps
	// the documented GUI_API_URL working after a login has stored an address.
	both := apiFrom("")
	if both.url != "http://from-env.test:4000" || both.source != "GUI_API_URL" {
		t.Errorf("apiFrom(\"\") = %+v with both set, want the environment", both)
	}

	t.Setenv("GUI_API_URL", "")

	fromStore := apiFrom("")
	if fromStore.url != "http://saved.test:4000" || fromStore.source != "the saved address" {
		t.Errorf("apiFrom(\"\") = %+v without the environment, want the store", fromStore)
	}

	fromStore = apiFrom("")
	if fromStore.url != "http://saved.test:4000" {
		t.Errorf("apiFrom(\"\") = %+v without the environment, want the store", fromStore)
	}

	if err := config.Remove("api_url"); err != nil {
		t.Fatal(err)
	}

	fallback := apiFrom("")
	if fallback.url != apiclient.DefaultBaseURL || fallback.source != "the default" {
		t.Errorf("apiFrom(\"\") = %+v with nothing set, want the default", fallback)
	}
}

// A session belongs to the instance that issued it: a port change on one host
// keeps it, a different host does not.
func TestUseServerKeepsTheTokenOnOneHostOnly(t *testing.T) {
	t.Setenv(config.Env, filepath.Join(t.TempDir(), "config.json"))

	u := &ui{client: apiclient.New("http://127.0.0.1:4000")}
	u.client.Token = "the-token"

	if err := config.Set("token", "the-token"); err != nil {
		t.Fatal(err)
	}

	signedOut, err := u.useServer(apiChoice{url: "http://127.0.0.1:5000", source: "the address you entered"})
	if err != nil {
		t.Fatal(err)
	}
	if signedOut {
		t.Error("a port change on the same host signed the session out")
	}
	if u.client.Token != "the-token" {
		t.Errorf("token = %q, want it kept", u.client.Token)
	}

	signedOut, err = u.useServer(apiChoice{url: "https://eznit.example.com", source: "the address you entered"})
	if err != nil {
		t.Fatal(err)
	}
	if !signedOut {
		t.Error("a different host kept the session")
	}
	if u.client.Token != "" {
		t.Errorf("token = %q, want it forgotten", u.client.Token)
	}

	if _, err := config.Value("token"); err == nil {
		t.Error("the token is still in the config file")
	}

	stored, err := config.Value("api_url")
	if err != nil || stored != "https://eznit.example.com" {
		t.Errorf("stored api_url = %q (%v), want the new address", stored, err)
	}
}

// "This machine" means the server the Server tab would run, on the port that
// folder's configuration names.
func TestLocalAddressFollowsTheServerTabsPort(t *testing.T) {
	view := newTestServerView(t, bundleDir(t, "API_PORT=\"4321\"\n"))

	if got := view.ui.localAddress(); got != "http://127.0.0.1:4321" {
		t.Errorf("localAddress = %q, want the Server tab's port", got)
	}
}

// newTestUI is the window's tabs without the window: enough of it to sign in
// and out, and to look at what a tab says.
func newTestUI(t *testing.T) *ui {
	t.Helper()

	t.Setenv(config.Env, filepath.Join(t.TempDir(), "config.json"))

	app := test.NewApp()
	t.Cleanup(app.Quit)

	u := &ui{
		app:     app,
		window:  app.NewWindow("test"),
		version: "test",
		clip:    test.NewClipboard(),
		address: apiChoice{url: "http://127.0.0.1:4321", source: "the default"},
		updater: &appUpdater{version: "test", updater: update.New(update.CLIFamily, update.MemberGUI)},
	}
	u.client = apiclient.New(u.address.url)
	u.files = newFilesView(u)
	u.templates = newTemplatesView(u)

	u.tabs = container.NewAppTabs(
		container.NewTabItem("Files", u.files.content()),
		container.NewTabItem("Templates", u.templates.content()),
		container.NewTabItem("About", u.aboutContent()),
	)
	u.window.SetContent(u.tabs)

	return u
}

// A session belongs to the Files tab: that is the tab the login form replaces,
// so that is where signing out belongs. It forgets the token and the catalogue,
// and keeps the address and the host tokens.
func TestSignOutLivesOnTheFilesTab(t *testing.T) {
	u := newTestUI(t)

	if err := config.Set("token", "the-token"); err != nil {
		t.Fatal(err)
	}
	if err := config.Set("api_url", "http://127.0.0.1:4321"); err != nil {
		t.Fatal(err)
	}

	u.client.Token = "the-token"
	u.templates.items = []templates.TemplateData{{Name: "a private starter"}}
	u.templates.loaded = true
	library := u.files.built

	signOut := findButton(u.tabs.Items[0].Content, "Sign out")
	if signOut == nil {
		t.Fatal("the Files tab has no Sign out button")
	}

	test.Tap(signOut)

	if _, err := config.Value("token"); err == nil {
		t.Error("the session token is still in the config file")
	}

	stored, err := config.Value("api_url")
	if err != nil || stored != "http://127.0.0.1:4321" {
		t.Errorf("api_url = %q (%v), want the address kept", stored, err)
	}

	if u.client.Token != "" {
		t.Errorf("client token = %q, want it cleared", u.client.Token)
	}

	if u.templates.loaded || u.templates.items != nil {
		t.Error("the catalogue is still loaded after signing out")
	}

	if u.tabs.Items[0].Content == library {
		t.Error("the Files tab still shows the library")
	}

	if findButton(u.aboutContent(), "Sign out") != nil {
		t.Error("the About tab still offers Sign out")
	}
}
