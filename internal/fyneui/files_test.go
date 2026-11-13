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
	if view.address.Disabled() {
		t.Error("the address field is disabled, so it cannot be typed")
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
	view.takeOffline("https://eznit.example.com", &apiclient.APIError{Status: 401, Message: "no session"})

	if !strings.Contains(view.status.Text, "https://eznit.example.com") {
		t.Errorf("status = %q, want the address that failed", view.status.Text)
	}
	if !strings.Contains(view.status.Text, "Connect") {
		t.Errorf("status = %q, want it to point at Connect", view.status.Text)
	}
	if view.ready {
		t.Error("the view is still ready after a failed load")
	}
}

// The table's cells cannot be selected — a selectable cell would take the tap
// that selects the row — so the selected row is described under it, and both
// copies work from that line.
func TestSelectedFileIsDescribedAndCopied(t *testing.T) {
	view := newTestFilesView(t, apiChoice{url: "http://127.0.0.1:4321", source: "the default"})
	view.ui.clip = test.NewClipboard()

	view.files = []apiclient.File{{ID: "9f1a", Name: "notes.txt", Version: 3}}
	view.selected = 0
	view.ready = true
	view.updateActions()

	if !strings.Contains(view.detail.Text, "notes.txt") || !strings.Contains(view.detail.Text, "9f1a") {
		t.Errorf("detail = %q, want the name and the id", view.detail.Text)
	}
	if !view.detail.Selectable {
		t.Error("the detail line is not selectable")
	}

	view.copySelectedName()

	if got := view.ui.clip.Content(); got != "notes.txt" {
		t.Errorf("clipboard = %q, want just the name", got)
	}

	view.copySelectedDetails()

	if got := view.ui.clip.Content(); got != "notes.txt\t3\tprivate\t9f1a" {
		t.Errorf("clipboard = %q, want a tab-separated record", got)
	}
	if !strings.Contains(view.status.Text, "Copied") {
		t.Errorf("status = %q, want confirmation of the copy", view.status.Text)
	}
}

// With no row selected there is nothing to copy, and nothing to describe.
func TestCopyIsOffWithoutASelection(t *testing.T) {
	view := newTestFilesView(t, apiChoice{url: "http://127.0.0.1:4321", source: "the default"})

	view.selected = -1
	view.ready = true
	view.updateActions()

	if !view.copyName.Disabled() || !view.copyDetails.Disabled() {
		t.Error("the copy buttons are enabled with no row selected")
	}
	if !strings.Contains(view.detail.Text, "Select a file") {
		t.Errorf("detail = %q, want it to ask for a selection", view.detail.Text)
	}
}

// The address in the header is the thing a person changes, so it is a field
// with a Connect action: typing a valid address switches the tab and is
// remembered, and the session stays on the same host.
func TestFilesAddressIsTypedAndConnected(t *testing.T) {
	u := newTestUI(t)

	u.client.Token = "the-token"
	if err := config.Set("token", "the-token"); err != nil {
		t.Fatal(err)
	}

	u.files.address.SetText("http://127.0.0.1:5000")
	u.files.connectTyped()

	stored, err := config.Value("api_url")
	if err != nil || stored != "http://127.0.0.1:5000" {
		t.Errorf("api_url = %q (%v), want the typed address", stored, err)
	}

	if u.client.Token != "the-token" {
		t.Errorf("token = %q, want it kept on the same host", u.client.Token)
	}

	if u.address.url != "http://127.0.0.1:5000" {
		t.Errorf("address = %q, want the typed one", u.address.url)
	}
}

// An address the client cannot use is refused where it was typed: nothing is
// saved and the status says what is wrong.
func TestFilesRefusesABadTypedAddress(t *testing.T) {
	u := newTestUI(t)

	u.files.address.SetText("not a url")
	u.files.connectTyped()

	if _, err := config.Value("api_url"); err == nil {
		t.Error("a bad address was saved")
	}

	if !strings.Contains(u.files.status.Text, "http://") {
		t.Errorf("status = %q, want the validation message", u.files.status.Text)
	}
}

// "This machine" fills the field from the Server tab rather than saving
// anything: it is a shortcut for typing, not a connection.
func TestFilesThisMachineFillsTheField(t *testing.T) {
	u := newTestUI(t)

	u.files.useLocalAddress()

	if got := u.files.address.Text; got != u.localAddress() {
		t.Errorf("address = %q, want the Server tab's address %q", got, u.localAddress())
	}

	if _, err := config.Value("api_url"); err == nil {
		t.Error("filling the field saved the address")
	}
}
