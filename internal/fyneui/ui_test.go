//go:build fyne

package fyneui

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/fiwon123/eznit/internal/apiclient"
	"github.com/fiwon123/eznit/internal/config"
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

	fromStore := apiFrom("")
	if fromStore.url != "http://saved.test:4000" || fromStore.source != "the saved address" {
		t.Errorf("apiFrom(\"\") = %+v with a stored address, want the store", fromStore)
	}

	t.Setenv("GUI_API_URL", "")

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
