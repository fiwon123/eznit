//go:build fyne

package fyneui

import (
	"fmt"
	"os"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/fiwon123/eznit/internal/apiclient"
	"github.com/fiwon123/eznit/internal/config"
)

// localModeKey remembers whether the window talks to this machine. A window
// that has never chosen is local: the Server tab is on the same machine, and
// "this machine" is the answer that makes the two tabs agree.
const localModeKey = "api_local"

// localSource names the address a checked bar uses, and is what the source line
// says after connecting to it.
const localSource = "the local server"

// addressBar is the one place the API address is chosen. Files and Templates
// each build one; every bar reads ui.address and ui.local, and ui.syncAddressBars
// keeps them in step after a change.
type addressBar struct {
	ui *ui

	local   *widget.Check
	entry   *widget.Entry
	source  *widget.Label
	test    *widget.Button
	connect *widget.Button
	signOut *widget.Button

	// report writes the bar's messages where the owning tab shows status.
	report func(text string, kind statusKind)

	// afterConnect runs after a successful switch; the Templates tab reloads
	// its catalogue from the new server.
	afterConnect func()

	// syncing suppresses the checkbox callback while sync sets the state.
	syncing bool
}

// newAddressBar builds the bar. withSignOut is the Files tab's: the session
// belongs to that tab, so only it offers to end one.
func newAddressBar(u *ui, report func(text string, kind statusKind), withSignOut bool) *addressBar {
	bar := &addressBar{ui: u, report: report}

	bar.local = widget.NewCheck("Use this machine", bar.toggleLocal)
	bar.entry = widget.NewEntry()
	bar.entry.SetPlaceHolder("http://127.0.0.1:4000")
	bar.entry.OnSubmitted = func(string) { bar.connectTyped() }
	bar.source = muted("")

	bar.test = widget.NewButton("Test", bar.testTyped)
	bar.connect = widget.NewButton("Connect", bar.connectTyped)

	if withSignOut {
		bar.signOut = widget.NewButtonWithIcon("Sign out", theme.LogoutIcon(), u.signOut)
		bar.signOut.Importance = widget.LowImportance
	}

	bar.sync()

	return bar
}

// content lays the bar out: the local choice and the source on their own line,
// the address and its actions below.
func (b *addressBar) content() fyne.CanvasObject {
	actions := []fyne.CanvasObject{b.test, b.connect}

	if b.signOut != nil {
		actions = append(actions, widget.NewSeparator(), b.signOut)
	}

	line := container.NewBorder(nil, nil, muted("Server"), container.NewHBox(actions...), b.entry)
	head := container.NewBorder(nil, nil, b.local, b.source, nil)

	return container.NewPadded(container.NewVBox(head, line))
}

// sync reads the current mode and address into the widgets.
func (b *addressBar) sync() {
	b.syncing = true
	defer func() { b.syncing = false }()

	b.local.SetChecked(b.ui.local)

	url := b.ui.address.url
	if b.ui.local {
		url = b.ui.localAddress()
	}

	b.entry.SetText(url)

	if b.ui.local {
		b.entry.Disable()
	} else {
		b.entry.Enable()
	}

	b.source.SetText("(" + b.ui.address.source + ")")
}

// target is the address the bar means right now: the local one when the box is
// checked, the typed one otherwise.
func (b *addressBar) target() string {
	if b.ui.local {
		return b.ui.localAddress()
	}

	return b.entry.Text
}

// toggleLocal remembers the choice. It does not connect: an address is a
// deliberate press, so a wrong folder cannot take the session by surprise.
func (b *addressBar) toggleLocal(checked bool) {
	if b.syncing {
		return
	}

	if err := b.ui.setLocal(checked); err != nil {
		b.message("The choice could not be saved: "+err.Error(), statusError)
		return
	}

	b.ui.syncAddressBars()

	if checked {
		b.message("This machine: "+b.ui.localAddress()+". Press Connect to switch to it.", statusIdle)
		return
	}

	b.message("Type the server address, then Connect.", statusIdle)
}

// testTyped asks the typed instance whether it answers, without saving it.
func (b *addressBar) testTyped() {
	address, err := apiclient.ValidateBaseURL(b.target())
	if err != nil {
		b.message(err.Error(), statusError)

		return
	}

	b.message("Asking "+address+"…", statusPending)

	go func() {
		started := time.Now()
		err := apiclient.New(address).Health()
		took := time.Since(started)

		fyne.Do(func() {
			if err != nil {
				b.message(fmt.Sprintf("No answer from %s: %s", address, messageOf(err)), statusError)
				return
			}

			b.message(fmt.Sprintf("%s answered in %s.", address, took.Round(time.Millisecond)), statusOK)
		})
	}()
}

// connectTyped switches every tab to the address the bar means. The rules are
// the ones a session needs: the same host keeps the token, a different one asks
// before signing out.
func (b *addressBar) connectTyped() {
	typed, err := apiclient.ValidateBaseURL(b.target())
	if err != nil {
		b.message(err.Error(), statusError)

		return
	}

	source := "the address you entered"
	if b.ui.local {
		source = localSource
	}

	choice := apiChoice{url: typed, source: source}

	if b.ui.client.Token != "" && !apiclient.SameHost(b.ui.client.BaseURL, typed) {
		dialog.ShowConfirm("Sign out of the current server?",
			"A session belongs to the server that issued it.\n\nChanging to "+typed+
				" signs you out of "+b.ui.client.BaseURL+". Continue?",
			func(confirmed bool) {
				if confirmed {
					b.apply(choice)
				}
			}, b.ui.window)

		return
	}

	b.apply(choice)
}

// apply performs the switch and reports whatever went wrong.
func (b *addressBar) apply(choice apiChoice) {
	signedOut, err := b.ui.connect(choice)
	if err != nil {
		b.message(err.Error(), statusError)

		return
	}

	if b.afterConnect != nil {
		b.afterConnect()
	}

	if !signedOut {
		b.sync()
	}
}

func (b *addressBar) message(text string, kind statusKind) {
	if b.report != nil {
		b.report(text, kind)
	}
}

// localMode decides the startup mode: an explicit flag or environment address
// is not "this machine", and a remembered choice is the user's. The default is
// local.
func localMode(flagValue string) bool {
	if strings.TrimSpace(flagValue) != "" || strings.TrimSpace(os.Getenv("GUI_API_URL")) != "" {
		return false
	}

	if stored, err := config.Value(localModeKey); err == nil {
		return stored != "false"
	}

	return true
}
