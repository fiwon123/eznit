//go:build fyne

package fyneui

import (
	"path/filepath"
	"strings"
	"testing"

	"fyne.io/fyne/v2/test"

	"github.com/fiwon123/eznit/internal/apiclient"
	"github.com/fiwon123/eznit/internal/config"
)

// newTestFilesView builds the Files tab for an address. Nothing is contacted:
// these tests cover what the tab says and what it allows.
func newTestFilesView(t *testing.T, address apiChoice) *filesView {
	t.Helper()

	t.Setenv(config.Env, filepath.Join(t.TempDir(), "config.json"))

	app := test.NewApp()
	t.Cleanup(app.Quit)

	u := &ui{
		app:     app,
		window:  app.NewWindow("test"),
		address: address,
		client:  apiclient.New(address.url),
	}

	u.files = newFilesView(u)
	u.window.SetContent(u.files.content())

	return u.files
}

// The tab says which server it talks to, and why that address: an address that
// came from an environment variable must not look like one a person saved.
func TestFilesHeaderNamesTheServerAndItsSource(t *testing.T) {
	view := newTestFilesView(t, apiChoice{url: "http://127.0.0.1:4321", source: "the local server"})

	if got := view.address.Text; got != "http://127.0.0.1:4321" {
		t.Errorf("address = %q, want the resolved URL", got)
	}
	if !view.address.Selectable {
		t.Error("the address is not selectable, so it cannot be copied")
	}
	if !view.source.Selectable {
		t.Error("the source is not selectable")
	}
	if got := view.source.Text; !strings.Contains(got, "the local server") {
		t.Errorf("source = %q, want where the address came from", got)
	}
}

// Nothing that acts on the server may work before it has answered once: a
// wrong address must not look like an empty library.
func TestFilesActionsWaitForTheServer(t *testing.T) {
	view := newTestFilesView(t, apiChoice{url: "http://127.0.0.1:4321", source: "the default"})

	view.files = []apiclient.File{{ID: "1", Name: "notes.txt", Version: 1}}
	view.selected = 0

	view.ready = false
	view.updateActions()

	if !view.upload.Disabled() || !view.download.Disabled() || !view.remove.Disabled() || !view.share.Disabled() {
		t.Error("an action is enabled before the server has answered")
	}

	view.ready = true
	view.updateActions()

	if view.upload.Disabled() || view.download.Disabled() || view.remove.Disabled() || view.share.Disabled() {
		t.Error("an action is still disabled after a successful listing")
	}
}

// A server that does not answer is named in the message, and the actions go
// back to off.
func TestTakeOfflineNamesTheAddress(t *testing.T) {
	view := newTestFilesView(t, apiChoice{url: "https://eznit.example.com", source: "the saved address"})

	view.ready = true
	view.takeOffline(&apiclient.APIError{Status: 401, Message: "no session"})

	if !strings.Contains(view.status.Text, "https://eznit.example.com") {
		t.Errorf("status = %q, want the address that failed", view.status.Text)
	}
	if !strings.Contains(view.status.Text, "Change") {
		t.Errorf("status = %q, want it to point at Change", view.status.Text)
	}
	if view.ready {
		t.Error("the view is still ready after a failed load")
	}
}
