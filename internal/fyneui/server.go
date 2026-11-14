//go:build fyne

package fyneui

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/fiwon123/eznit/internal/config"
	"github.com/fiwon123/eznit/internal/serverctl"
)

// stopTimeout is how long the window waits for the server to drain before it
// gives up and kills it. The API drains for 10s; this is the outer bound.
const stopTimeout = 15 * time.Second

// followTolerance is how far above the end the view may sit before "follow
// output" gives up. A fresh row plus a clamp while the layout catches up moves
// the offset a few pixels; a wheel notch moves it much further.
const followTolerance = 24

// serverView is the Local server tab: run the server from a bundle
// folder, watch what it prints, and edit the .env beside it.
//
// It is a control panel, not a remote client: it drives a process on this
// machine. A server elsewhere is reached from the Files tab, with an address.
type serverView struct {
	ui *ui

	// mu guards the folder, the supervisor and the health answer, which the
	// health ticker reads on its own goroutine.
	mu         sync.Mutex
	dir        string
	supervisor *serverctl.Supervisor
	answered   bool
	answerErr  error

	// bundle is what the folder carries of a server bundle, read when the
	// folder loads: a folder with no binary cannot start, and one with no
	// example cannot make a configuration.
	bundle serverctl.Bundle

	built fyne.CanvasObject

	folder *widget.Entry
	status *widget.Label
	health *widget.Label
	dot    *canvas.Circle
	start  *widget.Button
	stop   *widget.Button
	open   *widget.Button
	use    *widget.Button
	choose *widget.Button
	reset  *widget.Button

	log    *widget.TextGrid
	grid   *logGrid
	scroll *container.Scroll
	follow *widget.Check

	// sel is the range a drag covers in the grid, or nil. It is painted on the
	// grid's own cells rather than in a layer above them (see loggrid.go).
	sel           *selection
	copySelection *widget.Button

	// logMenu is the right-click menu over the output while it is open, so a
	// second right-click replaces it instead of stacking a third.
	logMenu *widget.PopUpMenu

	// preparing is true while the database step runs, before the API exists.
	// The refresh cycle keeps the buttons honest about it, and a folder change
	// is refused for the same reason it is refused while the server runs.
	preparing bool

	// prepare is the database step. It is a field so a test can answer for it:
	// the real one starts containers, and most tests are about the buttons.
	prepare func(ctx context.Context, dir string, out io.Writer) (serverctl.Result, error)

	outputControls *fyne.Container

	// rendered and parsed are the log's high-water mark: the grid holds this
	// many lines already, so a new line appends a row instead of rebuilding
	// the whole buffer on every write. consumed is the total the supervisor
	// had written when the grid was last drawn.
	rendered int
	parsed   []serverctl.LogLine
	consumed int

	// localURL is the address the tab would reach, cached: LocalURL reads the
	// .env, and refresh runs on every line of output.
	localURL string

	// save, configScroll and panes are kept so a test can prove the actions are
	// outside the scrolling pane and the fields fit inside it (#235).
	save             *widget.Button
	configScroll     *container.Scroll
	configScrollPane *fyne.Container
	panes            *container.AppTabs

	env    *serverctl.EnvFile
	fields map[string]*widget.Entry

	// fieldOrder is the order a change is reported in: the form's order, not
	// a map's.
	fieldOrder []string

	// loaded and secretState are what the file said when it was read. The
	// action row compares the form against them, so a person can see what a
	// save would change before pressing it.
	loaded      map[string]string
	secretState string

	// loading suppresses the change line while the form is being filled from
	// the file: every SetText would otherwise report a change against a map
	// that is still being built.
	loading bool

	// The theme around each input and its caption: a field that differs from
	// the file is tinted, so the mark is on the field, not only in the line.
	fieldOverrides map[string]*container.ThemeOverride
	fieldCaptions  map[string]*widget.Label
	dirtyFields    map[string]bool

	password         *widget.Entry
	passwordOverride *container.ThemeOverride
	passwordCaption  *widget.Label
	passwordDirty    bool

	secret *prose
	note   *prose
	create *widget.Button
	revert *widget.Button
}

// newServerView returns the panel for the folder it is given.
func newServerView(u *ui, dir string) *serverView {
	view := &serverView{
		ui:             u,
		fields:         map[string]*widget.Entry{},
		fieldOverrides: map[string]*container.ThemeOverride{},
		fieldCaptions:  map[string]*widget.Label{},
		dirtyFields:    map[string]bool{},
		prepare:        serverctl.Prepare,
	}

	view.useDir(dir)

	return view
}

// useDir points the panel at another bundle folder. It does not remember the
// choice: only a folder a person picked is stored, so the default stays
// distinguishable from a decision and can be returned to.
func (v *serverView) useDir(dir string) {
	supervisor := serverctl.NewSupervisor(dir)
	supervisor.SetNotify(func() { fyne.Do(v.refresh) })

	v.mu.Lock()
	v.dir = dir
	v.supervisor = supervisor
	v.answered = false
	v.answerErr = nil
	v.mu.Unlock()

	// The grid held the old server's output; a new folder is a new log.
	v.forgetSelection()
	v.rendered = 0
	v.parsed = nil
	v.consumed = 0
	if v.log != nil {
		v.log.SetText("")
	}
}

// canChangeFolder refuses a folder change while the server runs: the process
// belongs to the folder it was started from, and swapping the supervisor would
// leave it with nobody holding it. The database step is the same case a moment
// earlier — it is working in that folder on the server's behalf.
func (v *serverView) canChangeFolder() bool {
	if v.preparing {
		v.setNote("Wait for the database step to finish — it is working in the folder the server will run from.")

		return false
	}

	if !v.running() {
		return true
	}

	v.setNote("Stop the server first — it belongs to the folder it was started from, and changing the folder would leave it with nobody watching it.")

	return false
}

// useChosenDir points the panel at a folder a person picked and remembers it
// for the next start.
func (v *serverView) useChosenDir(dir string) bool {
	if !v.canChangeFolder() {
		return false
	}

	v.useDir(dir)
	v.folder.SetText(dir)
	v.loadConfig()
	v.refresh()

	if err := config.Set("server_dir", dir); err != nil {
		v.setNote("The folder changed for this session, but it could not be saved for the next start: " + err.Error())
		return true
	}

	v.setNote("Using the folder " + dir + ".")

	return true
}

// resetDir asks before pointing the panel back at the folder the window was
// installed in, and forgets the stored choice.
func (v *serverView) resetDir() {
	if !v.canChangeFolder() {
		return
	}

	if v.folderPath() == defaultServerDir() {
		v.setNote("The Server tab is already using the folder this window was installed in.")
		return
	}

	body := widget.NewLabel(fmt.Sprintf(
		"Point the Server tab back at the default folder?\n\nThe window was installed in: %s\nIt is using: %s\n\nNo .env and no secret file is changed.",
		defaultServerDir(), v.folderPath()))
	body.Wrapping = fyne.TextWrapWord
	body.Selectable = true

	dialog.ShowCustomConfirm("Reset to the default folder", "Reset", "Cancel", body,
		func(confirmed bool) {
			if confirmed {
				v.applyReset()
			}
		}, v.ui.window)
}

// applyReset forgets the stored folder and points the panel at the default.
func (v *serverView) applyReset() {
	if !v.canChangeFolder() {
		return
	}

	target := defaultServerDir()

	if err := config.Remove("server_dir"); err != nil {
		v.setNote("The stored folder could not be forgotten: " + err.Error())
		return
	}

	v.useDir(target)
	v.folder.SetText(target)
	v.loadConfig()
	v.refresh()

	// A folder with no .env already explains itself in the note; do not cover
	// that up with a confirmation.
	if v.env != nil {
		v.setNote("Using the folder this window was installed in: " + target)
	}
}

// server returns the supervisor for the current folder.
func (v *serverView) server() *serverctl.Supervisor {
	v.mu.Lock()
	defer v.mu.Unlock()

	return v.supervisor
}

// folderPath is the folder the panel currently drives.
func (v *serverView) folderPath() string {
	v.mu.Lock()
	defer v.mu.Unlock()

	return v.dir
}

// answerState is the last health answer, and whether it is about the process
// that is running now.
func (v *serverView) answerState() (bool, error) {
	v.mu.Lock()
	defer v.mu.Unlock()

	return v.answered, v.answerErr
}

// running reports whether the panel's server is up, for the window's close
// hook: a server this window started should not outlive it.
func (v *serverView) running() bool {
	return v.server().State().Running
}

func (v *serverView) content() fyne.CanvasObject {
	if v.built != nil {
		return v.built
	}

	v.status = widget.NewLabel("Stopped")
	v.status.TextStyle = fyne.TextStyle{Bold: true}
	v.status.Selectable = true

	v.health = oneLine("")
	v.dot = canvas.NewCircle(statusColour(v.ui.app.Settings().ThemeVariant(), statusIdle))

	v.start = widget.NewButtonWithIcon("Start", theme.MediaPlayIcon(), v.startServer)
	v.start.Importance = widget.HighImportance
	v.stop = widget.NewButtonWithIcon("Stop", theme.MediaStopIcon(), v.stopServer)
	v.stop.Importance = widget.DangerImportance
	v.open = widget.NewButtonWithIcon("Open in browser", theme.ComputerIcon(), v.openBrowser)

	// The folder is an editable field, not only a picker: the path is often
	// known, and a file dialog starts wherever it last was. Use (or Enter)
	// validates and commits it; Browse… is the picker.
	v.folder = widget.NewEntry()
	v.folder.SetText(v.folderPath())
	v.folder.SetPlaceHolder("the extracted bundle folder")
	v.folder.OnSubmitted = func(string) { v.useTypedFolder() }

	v.use = widget.NewButtonWithIcon("Use", theme.ConfirmIcon(), v.useTypedFolder)
	v.choose = widget.NewButtonWithIcon("Browse…", theme.FolderOpenIcon(), v.chooseFolder)
	v.reset = widget.NewButtonWithIcon("Reset", theme.ViewRefreshIcon(), v.resetDir)

	// One compact header rather than a card: the card left the panes 121 px
	// tall at 980x640 (#235), which is why the log could not be read and the
	// form had to scroll.
	//
	// The health line is the middle of the row, and it is one line that
	// ellipsizes: a wrapped label there would draw over the buttons to its
	// right. The dot and the status share a block so the dot centres on the
	// text rather than on the taller buttons.
	stateRow := container.NewBorder(nil, nil,
		dotWithLabel(v.dot, v.status),
		container.NewHBox(v.start, v.stop, widget.NewSeparator(), v.open),
		v.health,
	)
	folderRow := container.NewBorder(nil, nil, muted("Folder"),
		container.NewHBox(v.use, v.choose, widget.NewSeparator(), v.reset), v.folder)

	v.note = newProse("")
	v.note.Hide()

	header := container.NewPadded(container.NewVBox(stateRow, folderRow, v.note))

	v.buildOutput()
	v.buildConfiguration()

	// The output and the form are two views of the same space, not two halves
	// of it: at 980x640 a split left the form 121 px tall, so its fields
	// scrolled behind a card that looked finished (#235).
	v.panes = container.NewAppTabs(
		container.NewTabItemWithIcon("Output", theme.ListIcon(), v.outputPane()),
		container.NewTabItemWithIcon("Configuration", theme.SettingsIcon(), v.configurationPane()),
	)

	v.built = container.NewBorder(
		container.NewVBox(header, widget.NewSeparator()), nil, nil, nil, v.panes)

	v.loadConfig()
	v.refresh()

	return v.built
}

// buildOutput creates the log grid and its controls. The controls are pinned
// below the grid so they cannot scroll away, and the grid keeps its own
// scroll: hovering a text grid reaches that scroll directly, which is what
// makes the wheel work over the log itself.
func (v *serverView) buildOutput() {
	v.log = widget.NewTextGrid()
	v.grid = newLogGrid(v.log)
	v.grid.onSelect = v.selectCells
	v.grid.onMenu = v.showLogMenu

	v.scroll = container.NewScroll(v.grid)
	v.scroll.SetMinSize(fyne.NewSize(440, 80))
	v.scroll.OnScrolled = v.followOutput

	v.follow = widget.NewCheck("Follow output", func(checked bool) {
		if checked {
			v.scrollToEnd()
		}
	})
	v.follow.SetChecked(true)

	jump := widget.NewButton("Jump to end", v.scrollToEnd)
	copyLog := widget.NewButtonWithIcon("Copy log", theme.ContentCopyIcon(), v.copyLog)
	selectOutput := widget.NewButton("Select…", v.selectOutput)
	clear := widget.NewButtonWithIcon("Clear", theme.DeleteIcon(), v.clearLog)

	// The button is there from the start, disabled: a control that appears
	// with a selection is a feature nobody discovers (Select… already was one).
	v.copySelection = widget.NewButton("Copy selection", v.copySelected)
	v.copySelection.Disable()

	actions := container.NewBorder(nil, nil, v.follow,
		container.NewHBox(jump, copyLog, v.copySelection, selectOutput, clear))
	v.outputControls = container.NewPadded(actions)
}

// outputPane is the grid with its pinned control row.
func (v *serverView) outputPane() fyne.CanvasObject {
	return container.NewBorder(nil, v.outputControls, nil, nil, v.scroll)
}

// buildConfiguration creates the .env form. Its captions are labels rather
// than widget.Form captions so they can be selected and copied like every
// other piece of text on the tab.
func (v *serverView) buildConfiguration() {
	v.password = widget.NewPasswordEntry()
	v.password.SetPlaceHolder("leave empty to keep the current password")
	v.password.OnChanged = func(string) { v.refreshChanges() }

	v.passwordCaption = formCaption("Password")
	v.passwordOverride = container.NewThemeOverride(v.password, v.ui.app.Settings().Theme())

	v.secret = newProse("")

	serverForm := formRows(
		v.field("API_PORT", "API port"),
		v.field("API_SERVE_WEB", "Serve the web UI"),
		v.field("API_UPLOADS", "Uploads folder"),
		v.field("API_LOGS", "Logs folder"),
	)
	databaseForm := formRows(
		v.field("DB_HOST", "Host"),
		v.field("DB_PORT", "Port"),
		v.field("DB_USER", "User"),
		v.field("DB_NAME", "Database"),
		formRow{label: v.passwordCaption, input: v.passwordOverride},
	)

	v.save = widget.NewButtonWithIcon("Save configuration", theme.DocumentSaveIcon(), v.saveConfig)
	v.save.Importance = widget.HighImportance
	v.revert = widget.NewButtonWithIcon("Revert", theme.ContentUndoIcon(), v.revertChanges)
	v.create = widget.NewButtonWithIcon("Create .env from the example", theme.DocumentCreateIcon(), v.createEnv)
	v.create.Hide()

	forms := container.NewGridWithColumns(2, serverForm, databaseForm)
	content := container.NewPadded(forms)

	v.configScroll = container.NewScroll(content)
	// The pane may not be smaller than the form: a window that cannot show the
	// fields is better refused than scrolled through, because a Fyne entry
	// keeps a scroll of its own and eats the wheel over it.
	v.configScroll.SetMinSize(fyne.NewSize(440, content.MinSize().Height))

	// The left of the action row is the state line: the password state when the
	// form matches the file, the pending changes when it does not. Revert sits
	// beside Save so the two halves of an edit are in one place.
	actions := container.NewBorder(nil, nil, nil, container.NewHBox(v.create, v.revert, v.save), v.secret)

	v.configScrollPane = container.NewBorder(nil, container.NewPadded(actions), nil, nil, v.configScroll)
}

// configurationPane is the form with its pinned action row.
func (v *serverView) configurationPane() fyne.CanvasObject {
	return v.configScrollPane
}

// formRow is one caption and its value. The caption is either a string, which
// the form makes into a label, or a label the caller keeps — the configuration
// fields keep theirs so a pending change can colour it.
type formRow struct {
	caption string
	label   *widget.Label
	input   fyne.CanvasObject
}

// formCaption is one caption: right-aligned and selectable like every other
// value on the tab.
func formCaption(text string) *widget.Label {
	label := widget.NewLabel(text)
	label.Alignment = fyne.TextAlignTrailing
	label.Selectable = true

	return label
}

// formRows lays captions and values out as a form.
func formRows(rows ...formRow) fyne.CanvasObject {
	objects := make([]fyne.CanvasObject, 0, len(rows)*2)

	for _, row := range rows {
		label := row.label
		if label == nil {
			label = formCaption(row.caption)
		}

		objects = append(objects, label, row.input)
	}

	return container.New(layout.NewFormLayout(), objects...)
}

// field is one configuration key in the form, remembered so a load and a save
// can walk the same set. Enter in a field saves the form; a change updates the
// pending changes line and the field's own tint.
func (v *serverView) field(key, caption string) formRow {
	entry := widget.NewEntry()
	entry.OnSubmitted = func(string) { v.saveConfig() }
	entry.OnChanged = func(string) { v.refreshChanges() }
	v.fields[key] = entry
	v.fieldOrder = append(v.fieldOrder, key)

	label := formCaption(caption)
	v.fieldCaptions[key] = label

	override := container.NewThemeOverride(entry, v.ui.app.Settings().Theme())
	v.fieldOverrides[key] = override

	return formRow{label: label, input: override}
}

// setNote shows a message under the header. An empty note hides itself, so the
// tab does not reserve a blank line for it.
func (v *serverView) setNote(text string) {
	if v.note == nil {
		return
	}

	v.note.SetText(text)

	if strings.TrimSpace(text) == "" {
		v.note.Hide()
	} else {
		v.note.Show()
	}

	// The note is prose: a longer one is a taller header, and the header's rows
	// were measured before the text arrived.
	relayout(v.built)
}

// refresh redraws the status, the dot, the buttons and the log from the
// supervisor.
func (v *serverView) refresh() {
	if v.status == nil || v.log == nil {
		return
	}

	state := v.server().State()

	if v.preparing {
		// The database step is running: the buttons and the folder stay as they
		// were left, and the status says what is happening.
		v.status.SetText("Preparing the database…")
		v.health.SetText("Starting or checking the database")
		v.dot.FillColor = statusColour(v.ui.app.Settings().ThemeVariant(), statusPending)
		v.dot.Refresh()
		v.start.Disable()
		v.stop.Disable()
		v.setFolderEditable(false)
		v.drawLog()

		return
	}

	answered, answerErr := v.answerState()

	status, health, kind := "Stopped", v.localURL, statusIdle

	switch {
	case state.Running && !answered:
		status = fmt.Sprintf("Running — pid %d", state.PID)
		health = "Checking " + v.localURL + "…"
		kind = statusPending
	case state.Running && answerErr != nil:
		status = fmt.Sprintf("Running — pid %d", state.PID)
		health = "Not answering at " + v.localURL + " yet — see the log"
		kind = statusWarn
	case state.Running:
		status = fmt.Sprintf("Running — pid %d", state.PID)
		health = "Answering at " + v.localURL
		kind = statusOK
	case state.Exit != nil:
		status = "Stopped after an error"
		// The address must survive the error: it was the only place it showed,
		// and a stop is exactly when a person needs to know which server said
		// no.
		health = "Stopped at " + v.localURL + " — last exit: " + state.Exit.Error()
		kind = statusError
	}

	v.status.SetText(status)
	v.health.SetText(health)
	v.dot.FillColor = statusColour(v.ui.app.Settings().ThemeVariant(), kind)
	v.dot.Refresh()

	if state.Running {
		v.start.Disable()
		v.stop.Enable()
		// The running process belongs to this folder; a change is refused
		// rather than applied behind the server's back.
		v.setFolderEditable(false)
	} else {
		// A folder with no server cannot be started, and the note already says
		// why; a button that only produces an error is worse than a disabled
		// one.
		if v.bundle.Binary {
			v.start.Enable()
		} else {
			v.start.Disable()
		}

		v.stop.Disable()
		v.setFolderEditable(true)
	}

	v.drawLog()
}

// drawLog keeps the grid one row per line of output. A new line appends a row
// and styles it; a buffer the supervisor trimmed drops the same rows from the
// top. Only a cleared or replaced buffer rebuilds the grid.
func (v *serverView) drawLog() {
	lines := v.server().Output()
	written := v.server().Written()

	// start is the line number the buffer still begins at.
	start := written - len(lines)

	if start < v.consumed-v.rendered || len(lines) < v.rendered {
		// Cleared, or a buffer that shrank more than trimming can explain: draw
		// it from scratch.
		v.forgetSelection()
		v.rendered = 0
		v.parsed = nil
		v.consumed = start
		v.log.SetText("")
	}

	if start > v.consumed-v.rendered {
		v.dropRows(start - (v.consumed - v.rendered))
	}

	fresh := written - v.consumed
	if fresh <= 0 {
		v.scrollToEnd()

		return
	}
	if fresh > len(lines) {
		fresh = len(lines)
	}

	variant := v.ui.app.Settings().ThemeVariant()
	first := v.rendered
	format := make([]string, 0, fresh)

	for _, raw := range lines[len(lines)-fresh:] {
		line := serverctl.ParseLine(raw)
		v.parsed = append(v.parsed, line)
		format = append(format, serverctl.Format(line))
	}

	if first == 0 {
		v.log.SetText(strings.Join(format, "\n"))
	} else {
		v.log.Append(strings.Join(format, "\n"))
	}

	for index := range format {
		v.log.SetRowStyle(first+index, logStyle(variant, v.parsed[first+index]))
	}

	v.rendered += len(format)
	v.consumed = written
	v.log.Refresh()
	v.scrollToEnd()
}

// dropRows forgets the oldest rows of the grid, which is what the supervisor
// did to its own buffer.
func (v *serverView) dropRows(count int) {
	if count <= 0 {
		return
	}
	if count > len(v.log.Rows) {
		count = len(v.log.Rows)
	}
	// parsed can lag the grid by one row — SetText("") leaves one empty row and
	// no parsed line behind it — and trimming past it would panic.
	if count > len(v.parsed) {
		count = len(v.parsed)
	}
	if count <= 0 {
		return
	}

	v.log.Rows = append([]widget.TextGridRow(nil), v.log.Rows[count:]...)
	v.parsed = append([]serverctl.LogLine(nil), v.parsed[count:]...)
	v.rendered -= count

	// The selection keeps pointing at the same text: the lines left the top, so
	// its rows move up with them, and the styles moved with the rows. It is
	// gone only when the text it covered has left the buffer too.
	if v.sel != nil {
		v.sel = v.sel.shifted(-count).clamped(len(v.log.Rows))
		v.refreshCopySelection()
	}
}

// scrollEnd is the offset that shows the last line.
func (v *serverView) scrollEnd() float32 {
	height := v.scroll.Content.MinSize().Height - v.scroll.Size().Height
	if height < 0 {
		return 0
	}

	return height
}

// scrollToEnd follows the output, unless the reader turned following off.
func (v *serverView) scrollToEnd() {
	if v.follow != nil && !v.follow.Checked {
		return
	}

	// Place the offset at the content's own end. ScrollToOffset clamps on the
	// next refresh, which is why a reader who is one line above the end must
	// not be mistaken for a reader who scrolled away.
	v.scroll.ScrollToOffset(fyne.NewPos(0, v.scrollEnd()))
}

// followOutput unchecks "Follow output" when the reader scrolls away from the
// end. It is the log's own scroll callback, so it sees real wheel and bar
// movement, not the programmatic jump above.
func (v *serverView) followOutput(offset fyne.Position) {
	if v.follow == nil || !v.follow.Checked {
		return
	}

	if offset.Y < v.scrollEnd()-followTolerance {
		v.follow.SetChecked(false)
	}
}

// logMenuItems is the right-click menu over the output: the same actions as the
// buttons beside it, where the pointer already is. The items are built per
// click, so their disabled state is the selection's state now.
func (v *serverView) logMenuItems() []*fyne.MenuItem {
	selected := v.sel != nil

	return []*fyne.MenuItem{
		{Label: "Copy selection", Action: v.copySelected, Disabled: !selected},
		{Label: "Copy log", Action: v.copyLog},
		fyne.NewMenuItemSeparator(),
		{Label: "Clear selection", Action: func() { v.selectCells(nil) }, Disabled: !selected},
	}
}

// showLogMenu opens that menu at a canvas position, replacing one that is
// already open: a menu is where the pointer is, and there is only one pointer.
func (v *serverView) showLogMenu(at fyne.Position) {
	if v.logMenu != nil {
		v.logMenu.Hide()
		v.logMenu = nil
	}

	canvas := v.ui.window.Canvas()
	if canvas == nil {
		return
	}

	v.logMenu = widget.NewPopUpMenu(fyne.NewMenu("", v.logMenuItems()...), canvas)
	v.logMenu.ShowAtPosition(at)
}

// copyLog puts the whole buffer on the clipboard, whether or not a selection
// exists: it is the fallback when the drag missed, and the whole log is what a
// person wants when they have not marked anything.
func (v *serverView) copyLog() {
	text := v.log.Text()

	if strings.TrimSpace(text) == "" {
		v.setNote("There is nothing to copy yet — the server has not printed anything.")
		return
	}

	if !v.ui.copyToClipboard(text) {
		v.setNote("This window has no clipboard available — the output could not be copied.")
		return
	}

	v.setNote(fmt.Sprintf("Copied %d line(s) of output to the clipboard.", len(strings.Split(text, "\n"))))
}

// selectCells takes the range a drag covers. Every selection change goes
// through here, so the repaint and the copy button are decided in one place.
func (v *serverView) selectCells(sel *selection) {
	before := v.sel
	v.sel = sel.clamped(len(v.log.Rows))

	v.paintSelection(before, v.sel)
	v.refreshCopySelection()

	// A person selecting is reading, not following: output that arrives now
	// must not pull the view away from the lines being marked.
	if v.sel != nil && v.follow != nil && v.follow.Checked {
		v.follow.SetChecked(false)
	}
}

// forgetSelection drops the selection, for a grid whose rows are going away.
func (v *serverView) forgetSelection() {
	before := v.sel
	v.sel = nil

	v.paintSelection(before, nil)
	v.refreshCopySelection()
}

// paintSelection repaints the rows a selection change touched: rows it left go
// back to their level's style, rows it now covers gain the selection behind
// them. Rows whose coverage did not change are skipped, so a drag only restyles
// what it moves over.
//
// It ends with a Refresh of the grid because Fyne 2.8's SetStyle does not
// repaint the cell it styles (textGridRow.refreshCell indexes past the row's
// own renderer objects), so a style set on a visible cell stays invisible until
// the grid renders again. That refresh costs what appending a line costs —
// which drawLog already pays per line — because a grid with ScrollNone keeps
// every row it holds.
func (v *serverView) paintSelection(before, after *selection) {
	if v.log == nil {
		return
	}

	first, last := unionRows(before, after)
	if first > last {
		return
	}

	settings := v.ui.app.Settings()
	variant, th := settings.ThemeVariant(), settings.Theme()
	painted := false

	for row := first; row <= last; row++ {
		if row < 0 || row >= len(v.log.Rows) {
			continue
		}

		was, now := before.span(row), after.span(row)
		if was == now {
			continue
		}

		// A cell style sits on top of its row's, so a selection is painted on
		// the cells and cleared from them: repainting the row alone would leave
		// the old background behind.
		line := v.logLineAt(row)
		v.log.SetRowStyle(row, logStyle(variant, line))
		painted = true

		for col, cells := 0, len(v.log.Rows[row].Cells); col < cells; col++ {
			v.log.SetStyle(row, col, nil)
		}

		if !now.ok {
			continue
		}

		cells := len(v.log.Rows[row].Cells)
		from, to := now.from, now.to
		if to < 0 || to > cells {
			to = cells
		}
		if from > to {
			from = to
		}
		if from == to {
			continue
		}

		v.log.SetStyleRange(row, from, row, to-1, selectedLogStyle(th, variant, line))
	}

	if painted {
		v.log.Refresh()
	}
}

// refreshCopySelection keeps the button honest: it copies only what is
// selected, so it is dead until something is.
func (v *serverView) refreshCopySelection() {
	if v.copySelection == nil {
		return
	}

	if v.sel == nil {
		v.copySelection.Disable()
		return
	}

	v.copySelection.Enable()
}

// selectionText is exactly what the drag covers, lines joined with a newline.
// It reads the row's cells rather than its text, because a column is a cell and
// a tab or a wide rune is padded with cells that are not runes: slicing the
// text by column would copy the wrong characters.
func (v *serverView) selectionText() string {
	if v.sel == nil {
		return ""
	}

	lines := make([]string, 0, v.sel.end.row-v.sel.start.row+1)

	for row := v.sel.start.row; row <= v.sel.end.row && row < len(v.log.Rows); row++ {
		if row < 0 {
			continue
		}

		span := v.sel.span(row)
		if !span.ok {
			continue
		}

		var line strings.Builder
		for _, at := range rowCellRunes(v.log.Rows[row], v.log.TabWidth) {
			if at.col < span.from {
				continue
			}
			if span.to >= 0 && at.col >= span.to {
				break
			}

			line.WriteRune(at.r)
		}

		lines = append(lines, line.String())
	}

	return strings.Join(lines, "\n")
}

// copySelected puts just the selected text on the clipboard.
func (v *serverView) copySelected() {
	text := v.selectionText()

	if strings.TrimSpace(text) == "" {
		v.setNote("There is nothing selected to copy — drag over the lines you want.")
		return
	}

	if !v.ui.copyToClipboard(text) {
		v.setNote("This window has no clipboard available — the selection could not be copied.")
		return
	}

	v.setNote(fmt.Sprintf("Copied %d line(s) of the selection to the clipboard.", strings.Count(text, "\n")+1))
}

// logLineAt is the parsed line a row shows, for that line's level colour.
func (v *serverView) logLineAt(row int) serverctl.LogLine {
	if row < 0 || row >= len(v.parsed) {
		return serverctl.LogLine{}
	}

	return v.parsed[row]
}

// clearLog forgets the output and the grid that shows it.
func (v *serverView) clearLog() {
	v.server().ClearOutput()

	v.rendered = 0
	v.parsed = nil
	v.consumed = v.server().Written()
	v.forgetSelection()
	v.log.SetText("")
	v.setNote("Log cleared. The server's own log file still has the history.")
}

// outputSnapshot is the log as one selectable label. The grid cannot be
// selected in Fyne, so the dialog this label goes into is how a person takes a
// line or a range rather than the whole buffer.
func (v *serverView) outputSnapshot() *widget.Label {
	snapshot := widget.NewLabel(v.log.Text())
	snapshot.TextStyle = fyne.TextStyle{Monospace: true}
	snapshot.Wrapping = fyne.TextWrapOff
	snapshot.Selectable = true

	return snapshot
}

// selectOutput opens the snapshot in a dialog: it is frozen, so output arriving
// while it is open cannot move or clear a selection.
func (v *serverView) selectOutput() {
	snapshot := v.outputSnapshot()

	if strings.TrimSpace(snapshot.Text) == "" {
		v.setNote("There is nothing to select yet — the server has not printed anything.")
		return
	}

	scroll := container.NewScroll(snapshot)
	scroll.SetMinSize(fyne.NewSize(640, 360))

	dialog.ShowCustom("Select output", "Close", scroll, v.ui.window)
}

// pollHealth asks the API whether it answers, which the process state alone
// cannot say: a server that is up and failing to reach its database is running
// and useless.
func (v *serverView) pollHealth() {
	for {
		time.Sleep(3 * time.Second)

		err := serverctl.Health(v.server().LocalURL(), 2*time.Second)
		state := v.server().State()

		v.mu.Lock()
		if state.Running {
			v.answered, v.answerErr = true, err
		} else {
			v.answered, v.answerErr = false, nil
		}
		v.mu.Unlock()

		fyne.Do(v.refresh)
	}
}

// databaseTimeout bounds the database step. The first run pulls two images, so
// it is generous; a hung daemon still cannot hold the window for ever, and the
// modal carries Cancel for the rest.
const databaseTimeout = 15 * time.Minute

// startServer is Start, and Start begins with the database: an API with no
// database is a process that reports errors and answers a health check, which
// is the state this step exists to prevent.
func (v *serverView) startServer() {
	v.setNote("")

	if !v.bundle.Binary {
		v.setNote(v.notBundleNote())
		return
	}
	if v.preparing {
		return
	}

	// A port the server cannot bind is refused before anything is started: the
	// database is worth bringing up, but not for a start that cannot listen.
	if err := serverctl.PortFree(v.folderPath()); err != nil {
		v.setNote("Cannot start: " + err.Error())
		return
	}

	v.mu.Lock()
	v.answered, v.answerErr = false, nil
	v.mu.Unlock()

	v.preparing = true
	v.start.Disable()
	v.stop.Disable()
	v.setFolderEditable(false)

	ctx, cancel := context.WithTimeout(context.Background(), databaseTimeout)
	progress := v.showDatabaseProgress(cancel)
	folder := v.folderPath()
	v.refresh()

	// Off the UI goroutine: the first run pulls two images and applies the
	// migrations, and the window has to keep painting while it does.
	go func() {
		result, err := v.prepare(ctx, folder, progress)
		cancel()

		fyne.Do(func() { v.afterPrepare(result, err, progress) })
	}()
}

// afterPrepare starts the API once the database step is done, or explains why
// it will not be started at all.
func (v *serverView) afterPrepare(result serverctl.Result, err error, progress *databaseProgress) {
	progress.Close()
	v.preparing = false

	if err != nil {
		if errors.Is(err, context.Canceled) {
			v.setNote("The database step was cancelled — nothing was started.")
		} else {
			v.noteDatabaseFailure(err, progress.Output())
		}

		v.refresh()

		return
	}

	v.startAPI(databaseNote(result))
}

// startAPI runs the server process and puts its log in front of the person who
// pressed Start. The note is set after the configuration reload, which clears
// it when the folder is fine.
func (v *serverView) startAPI(note string) {
	if err := v.server().Start(); err != nil {
		v.setNote("Cannot start: " + err.Error())
		v.refresh()

		return
	}

	// A first start creates the .env from the example, so the form shows what
	// the server is actually reading.
	v.loadConfig()
	v.refresh()

	// The answer to "did it start" is in the log, so put it in front of the
	// person who just pressed the button.
	v.panes.SelectIndex(0)

	if note != "" {
		v.setNote(note)
	}
}

// databaseNote says what the database step did, for a start that worked.
func databaseNote(result serverctl.Result) string {
	switch {
	case result.Started:
		return "Started the database and applied the migrations."
	case result.Migrated:
		return "The database was already running; applied the migrations."
	default:
		return ""
	}
}

// databaseNoteForFailure turns the database step's error into what a person can
// do about it. Each case is a different fix, which is why the step has sentinel
// errors instead of one message.
func databaseNoteForFailure(err error) string {
	switch {
	case errors.Is(err, serverctl.ErrNoCompose):
		return "No database answers, and this folder has no " + serverctl.ComposeFileName +
			" to start one from — start a PostgreSQL, or point DB_HOST and DB_PORT at one in Configuration."
	case errors.Is(err, serverctl.ErrNoRuntime):
		return "No database answers, and there is no container runtime on PATH — install Docker or Podman, " +
			"or start a PostgreSQL and point DB_HOST and DB_PORT at it."
	case errors.Is(err, serverctl.ErrNoPassword):
		return "No database answers, and this folder has no " + serverctl.SecretPath +
			" — set the database password in Configuration first, then press Start again."
	case errors.Is(err, serverctl.ErrNoAnswer):
		return "The database did not answer after it was started — press Start again in a moment. " +
			"If DB_HOST names a service (such as db), a server run on this machine has to use 127.0.0.1 instead."
	default:
		return "The database could not be prepared: " + err.Error()
	}
}

// noteDatabaseFailure puts the short reason in the note and the runtime's own
// output in a dialog: the container runtime says what went wrong in words that
// are the fix, and they are worth keeping.
func (v *serverView) noteDatabaseFailure(err error, output string) {
	v.setNote(databaseNoteForFailure(err))

	body := err.Error()
	if strings.TrimSpace(output) != "" {
		body += "\n\n" + strings.TrimSpace(output)
	}

	label := widget.NewLabel(body)
	label.TextStyle = fyne.TextStyle{Monospace: true}
	label.Wrapping = fyne.TextWrapWord
	label.Selectable = true

	scroll := container.NewVScroll(label)
	scroll.SetMinSize(fyne.NewSize(640, 320))

	copyButton := widget.NewButtonWithIcon("Copy output", theme.ContentCopyIcon(), func() {
		if !v.ui.copyToClipboard(body) {
			v.setNote("This window has no clipboard available — the output could not be copied.")
		}
	})

	actions := container.NewHBox(layout.NewSpacer(), copyButton)
	content := container.NewBorder(nil, actions, nil, nil, scroll)
	dialog.ShowCustom("The database could not be started", "Close", content, v.ui.window)
}

func (v *serverView) stopServer() {
	v.stop.Disable()
	v.start.Disable()
	v.status.SetText("Stopping…")

	go func() {
		err := v.server().Stop(stopTimeout)

		fyne.Do(func() {
			v.mu.Lock()
			v.answered, v.answerErr = false, nil
			v.mu.Unlock()

			if err != nil {
				v.setNote("The stop was not clean: " + err.Error())
			}
			v.refresh()
		})
	}()
}

func (v *serverView) openBrowser() {
	base, err := url.Parse(v.localURL)
	if err != nil {
		v.setNote(err.Error())

		return
	}

	if err := v.ui.app.OpenURL(base); err != nil {
		v.setNote("The browser did not open: " + err.Error())
	}
}

// useTypedFolder commits the path typed into the folder field. The picker and
// the field end in the same place, so the checks live here rather than there:
// the path has to exist and be a directory, and a running server refuses the
// change with the same explanation a picked folder gets.
func (v *serverView) useTypedFolder() {
	dir := strings.TrimSpace(v.folder.Text)

	if dir == "" {
		v.setNote("Enter the folder of a server bundle, or use Browse… to pick one.")
		return
	}

	info, err := os.Stat(dir)
	if err != nil {
		v.setNote("That folder could not be opened: " + err.Error())
		return
	}

	if !info.IsDir() {
		v.setNote("That is a file, not a folder: " + dir)
		return
	}

	if !v.useChosenDir(dir) {
		return
	}
}

// setFolderEditable turns the folder controls on or off together.
func (v *serverView) setFolderEditable(editable bool) {
	for _, button := range []*widget.Button{v.use, v.choose, v.reset} {
		if button == nil {
			continue
		}

		if editable {
			button.Enable()
		} else {
			button.Disable()
		}
	}

	if v.folder != nil {
		if editable {
			v.folder.Enable()
		} else {
			v.folder.Disable()
		}
	}
}

func (v *serverView) chooseFolder() {
	if !v.canChangeFolder() {
		return
	}

	dialog.ShowFolderOpen(func(uri fyne.ListableURI, err error) {
		if err != nil {
			v.setNote(err.Error())
			return
		}
		if uri == nil {
			return
		}

		v.useChosenDir(uri.Path())
	}, v.ui.window)
}

// loadConfig fills the form and the secret state from the folder. It never
// creates anything: a folder with no .env shows the button that does.
//
// It also inspects the folder first. Both buttons below can fail for a reason
// the folder already states — no server to run, no example to copy — and a
// person should not have to press a button to be told that.
func (v *serverView) loadConfig() {
	dir := v.folderPath()

	v.bundle = serverctl.Inspect(dir)

	file, err := serverctl.LoadEnv(filepath.Join(dir, serverctl.EnvFileName))
	v.env = file

	switch {
	case err == nil:
		v.create.Hide()

		if v.bundle.Binary {
			v.setNote("")
		} else {
			v.setNote(v.notBundleNote())
		}
	case errors.Is(err, os.ErrNotExist):
		if v.bundle.Example {
			v.create.Show()

			note := "There is no .env here yet — create it from the example, then set the database password. " +
				"Start brings up the database from this folder's docker-compose.yml when nothing answers."
			if !v.bundle.Binary {
				note += " Start also needs " + serverctl.BinaryName() + ", which this folder does not have."
			}
			v.setNote(note)
		} else {
			v.create.Hide()
			v.setNote(v.notBundleNote())
		}
	default:
		v.create.Hide()
		v.setNote(err.Error())
	}

	// The form is filled with the change line quiet: every SetText would
	// otherwise be reported as a change against a map still being built.
	v.loading = true
	v.loaded = make(map[string]string, len(v.fields))

	for _, key := range v.fieldOrder {
		entry := v.fields[key]
		value := ""
		if file != nil {
			value = file.Value(key, "")
		}
		entry.SetText(value)
		v.loaded[key] = value
	}
	v.loading = false

	// The password is reported as a state, never shown: the file is the only
	// place the value lives.
	if exists, mode, err := serverctl.SecretState(dir); err != nil {
		v.secretState = "Database password: " + err.Error()
	} else if exists {
		v.secretState = fmt.Sprintf("Database password: set in %s (mode %v)", serverctl.SecretPath, mode)
	} else {
		v.secretState = "Database password: not set yet"
	}

	v.password.SetText("")

	// The state line is the middle of the action row; a longer message wraps
	// and the row has to be measured again.
	v.refreshChanges()

	// Reading the .env here keeps the address off the refresh path, which runs
	// on every line the server prints.
	v.localURL = v.server().LocalURL()
}

// changes is what a save would write: the fields that differ from the file,
// plus the password when one was typed. The order is the form's, so the line
// reads the way the fields are laid out.
func (v *serverView) changes() []string {
	changes := make([]string, 0, len(v.fields)+1)

	for _, key := range v.fieldOrder {
		value := strings.TrimSpace(v.fields[key].Text)
		if value == v.loaded[key] {
			continue
		}

		changes = append(changes, fmt.Sprintf("%s %s → %s", key, emptyAs(v.loaded[key]), emptyAs(value)))
	}

	if strings.TrimSpace(v.password.Text) != "" {
		changes = append(changes, "database password will be set")
	}

	return changes
}

// emptyAs names an empty value in the change line: "→ " alone reads as a bug.
func emptyAs(value string) string {
	if value == "" {
		return "(empty)"
	}

	return value
}

// refreshChanges keeps the action row in step with the form: the password state
// when nothing has changed, the pending changes when something has. Revert is
// offered exactly when there is something to discard, and each changed field is
// tinted so the mark is on the field itself.
func (v *serverView) refreshChanges() {
	if v.loading || v.secret == nil {
		return
	}

	v.markChanges()

	changes := v.changes()

	if len(changes) == 0 {
		v.secret.SetText(v.secretState)

		if v.revert != nil {
			v.revert.Disable()
		}
	} else {
		v.secret.SetText("Unsaved changes: " + strings.Join(changes, "; ") + ".")

		if v.revert != nil {
			v.revert.Enable()
		}
	}

	// A pending-changes line can wrap to more lines than the row was measured
	// for.
	relayout(v.configScrollPane)
}

// markChanges tints the fields that differ from the file and clears the ones
// that do not. Only a field whose state changed is refreshed: a theme override
// re-applies to its whole subtree.
func (v *serverView) markChanges() {
	for _, key := range v.fieldOrder {
		v.markField(key, strings.TrimSpace(v.fields[key].Text) != v.loaded[key])
	}

	dirty := strings.TrimSpace(v.password.Text) != ""
	if v.passwordDirty != dirty {
		v.passwordDirty = dirty
		v.tint(v.passwordOverride, v.passwordCaption, dirty)
	}
}

func (v *serverView) markField(key string, dirty bool) {
	if v.dirtyFields[key] == dirty {
		return
	}

	v.dirtyFields[key] = dirty
	v.tint(v.fieldOverrides[key], v.fieldCaptions[key], dirty)
}

// tint marks one input and its caption, or restores the app theme.
func (v *serverView) tint(override *container.ThemeOverride, caption *widget.Label, dirty bool) {
	if override != nil {
		if dirty {
			override.Theme = changedTheme{Theme: v.ui.app.Settings().Theme()}
		} else {
			override.Theme = v.ui.app.Settings().Theme()
		}

		override.Refresh()
	}

	if caption != nil {
		if dirty {
			caption.Importance = widget.WarningImportance
		} else {
			caption.Importance = widget.MediumImportance
		}

		caption.Refresh()
	}
}

// revertChanges discards the form and reads the file again, including clearing
// a typed password.
func (v *serverView) revertChanges() {
	v.loadConfig()
	v.setNote("Changes discarded — the form shows the file again.")
}

// notBundleNote explains a folder the tab cannot drive: what it is missing,
// what a bundle is, and the two ways forward. It is the sentence a person needs
// instead of a button that fails.
func (v *serverView) notBundleNote() string {
	return fmt.Sprintf(
		"This folder is not a server bundle: it has no %s. A bundle is the extracted release archive — "+
			"run eznit-gui from inside it, or choose that folder with Choose folder…. "+
			"To use a server that already runs, type its address on the Files tab instead.",
		strings.Join(v.bundle.Missing(), " and "))
}

func (v *serverView) createEnv() {
	if !v.bundle.Example {
		v.setNote(v.notBundleNote())
		return
	}

	if _, err := serverctl.LoadOrCreate(v.folderPath()); err != nil {
		v.setNote(err.Error())
		return
	}

	v.loadConfig()
	v.setNote(".env created for a local server. Set the database password, then start.")
}

func (v *serverView) saveConfig() {
	if v.env == nil {
		if v.bundle.Example {
			v.setNote("There is no .env to save — create one from the example first.")
		} else {
			v.setNote("There is no .env and no " + serverctl.EnvExampleName +
				" here — this folder is not a server bundle, so there is nothing to save.")
		}

		return
	}

	// Saving an unchanged form would rewrite the file and say "Saved.", which
	// teaches the wrong thing: the line already says nothing changed.
	if len(v.changes()) == 0 {
		v.setNote("There is nothing to save — no value has changed.")
		return
	}

	port := strings.TrimSpace(v.fields["API_PORT"].Text)
	if number, err := strconv.Atoi(port); err != nil || number <= 0 || number > 65535 {
		v.setNote("The API port must be a whole number between 1 and 65535.")
		return
	}

	for key, entry := range v.fields {
		v.env.Set(key, strings.TrimSpace(entry.Text))
	}

	if password := v.password.Text; password != "" {
		if err := serverctl.WriteSecret(v.folderPath(), password); err != nil {
			v.setNote("The password file could not be written: " + err.Error())
			return
		}

		// The host binary reads the file, not a variable: that is what keeps
		// the password out of the process table.
		v.env.Set("DB_PASSWORD_FILE", "./"+serverctl.SecretPath)
		v.env.Unset("DB_PWD")
	}

	if err := v.env.Save(); err != nil {
		v.setNote("The .env could not be saved: " + err.Error())
		return
	}

	state := v.server().State()
	v.loadConfig()

	note := "Saved."
	if state.Running {
		note += " Restart the server to use the new values."
	}
	v.setNote(note)
}
