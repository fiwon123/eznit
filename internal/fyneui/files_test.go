//go:build fyne

package fyneui

import (
	"path/filepath"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"

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

	if got := view.bar.entry.Text; got != "http://127.0.0.1:4321" {
		t.Errorf("address = %q, want the resolved URL", got)
	}
	if view.bar.entry.Disabled() {
		t.Error("the address field is disabled, so it cannot be typed")
	}
	if !view.bar.source.Selectable {
		t.Error("the source is not selectable")
	}
	if got := view.bar.source.Text; !strings.Contains(got, "the local server") {
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

// The table's cells are selectable — a selectable cell would take the tap that
// selects the row, so each cell carries a tap layer above its label. A tap
// still selects the row, and the selected row is described under the table so
// both copies work from one line.
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

// Every cell is selectable — the header included — and a tap on a cell still
// selects its row. The tap goes to the transparent layer above the label,
// because a selectable label would otherwise swallow it.
func TestFilesTableCellsAreSelectableAndSelectTheRow(t *testing.T) {
	view := newTestFilesView(t, apiChoice{url: "http://127.0.0.1:4321", source: "the default"})

	view.files = []apiclient.File{{ID: "1", Name: "notes.txt", Version: 2}}
	view.ready = true
	view.updateActions()

	cell := view.table.CreateCell()
	view.table.UpdateCell(widget.TableCellID{Row: 1, Col: 0}, cell)

	stack, ok := cell.(*fyne.Container)
	if !ok || len(stack.Objects) != 2 {
		t.Fatalf("a table cell is %T, want a label and a tap layer", cell)
	}

	label, ok := stack.Objects[0].(*widget.Label)
	if !ok {
		t.Fatalf("the cell's first object is %T, want a label", stack.Objects[0])
	}

	if !label.Selectable {
		t.Error("the cell text is not selectable")
	}

	if label.Text != "notes.txt" {
		t.Errorf("cell = %q, want the file name", label.Text)
	}

	tap, ok := stack.Objects[1].(*tapTarget)
	if !ok {
		t.Fatalf("the cell's second object is %T, want a tap layer", stack.Objects[1])
	}

	test.Tap(tap)

	if view.selected != 0 {
		t.Errorf("selected = %d, want 0 after tapping the first row", view.selected)
	}

	header := view.table.CreateCell()
	view.table.UpdateCell(widget.TableCellID{Row: 0, Col: 0}, header)

	headerStack := header.(*fyne.Container)
	headerLabel := headerStack.Objects[0].(*widget.Label)

	if !headerLabel.Selectable || headerLabel.Text != "Name" {
		t.Errorf("header cell = %q (selectable %t), want a selectable caption", headerLabel.Text, headerLabel.Selectable)
	}

	test.Tap(headerStack.Objects[1].(*tapTarget))

	if view.selected != -1 {
		t.Errorf("selected = %d, want the header tap to unselect", view.selected)
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

	u.files.bar.entry.SetText("http://127.0.0.1:5000")
	u.files.bar.connectTyped()

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

	u.files.bar.entry.SetText("not a url")
	u.files.bar.connectTyped()

	if _, err := config.Value("api_url"); err == nil {
		t.Error("a bad address was saved")
	}

	if !strings.Contains(u.files.status.Text, "http://") {
		t.Errorf("status = %q, want the validation message", u.files.status.Text)
	}
}

// The checkbox is the choice: checked, the bar shows this machine's address and
// keeps the field shut; unchecking opens it. Neither saves an address — Connect
// is what switches.
func TestFilesLocalCheckboxFillsTheField(t *testing.T) {
	u := newTestUI(t)

	u.local = true
	u.files.bar.sync()

	if got := u.files.bar.entry.Text; got != u.localAddress() {
		t.Errorf("address = %q, want the Server tab's address %q", got, u.localAddress())
	}

	if !u.files.bar.entry.Disabled() {
		t.Error("the field is open while the bar is set to this machine")
	}

	if _, err := config.Value("api_url"); err == nil {
		t.Error("setting the choice saved an address")
	}

	u.files.bar.local.SetChecked(false)

	if u.files.bar.entry.Disabled() {
		t.Error("unchecking did not open the field")
	}

	stored, err := config.Value(localModeKey)
	if err != nil || stored != "false" {
		t.Errorf("%s = %q (%v), want the manual mode remembered", localModeKey, stored, err)
	}
}
