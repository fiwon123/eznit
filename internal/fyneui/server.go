//go:build fyne

package fyneui

import (
	"errors"
	"fmt"
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

// serverView is the Local server tab: run the eznit-server from a bundle
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

	built fyne.CanvasObject

	folder *widget.Label
	status *widget.Label
	health *widget.Label
	dot    *canvas.Circle
	start  *widget.Button
	stop   *widget.Button
	open   *widget.Button

	log    *widget.TextGrid
	scroll *container.Scroll
	follow *widget.Check

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

	password *widget.Entry
	secret   *widget.Label
	note     *widget.Label
	create   *widget.Button
}

// newServerView returns the panel for the folder it is given.
func newServerView(u *ui, dir string) *serverView {
	view := &serverView{
		ui:     u,
		fields: map[string]*widget.Entry{},
	}

	view.useDir(dir)

	return view
}

// useDir points the panel at another bundle folder.
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
	v.rendered = 0
	v.parsed = nil
	v.consumed = 0
	if v.log != nil {
		v.log.SetText("")
	}

	if err := config.Set("server_dir", dir); err != nil {
		v.setNote("The folder could not be saved for the next start: " + err.Error())
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

	v.health = widget.NewLabel("")
	v.health.Selectable = true
	// Wrapped rather than truncated: a label's minimum size is its whole text,
	// so a long error would otherwise raise the window's minimum width.
	v.health.Wrapping = fyne.TextWrapWord

	v.dot = canvas.NewCircle(statusColour(v.ui.app.Settings().ThemeVariant(), statusIdle))

	// A shape in an HBox has no minimum of its own; a grid-wrap cell gives the
	// dot its 10 px.
	dotBox := container.NewGridWrap(fyne.NewSize(10, 10), v.dot)

	v.start = widget.NewButtonWithIcon("Start", theme.MediaPlayIcon(), v.startServer)
	v.start.Importance = widget.HighImportance
	v.stop = widget.NewButtonWithIcon("Stop", theme.MediaStopIcon(), v.stopServer)
	v.stop.Importance = widget.DangerImportance
	v.open = widget.NewButtonWithIcon("Open in browser", theme.ComputerIcon(), v.openBrowser)

	v.folder = widget.NewLabel(v.folderPath())
	v.folder.Wrapping = fyne.TextWrapWord
	v.folder.Selectable = true

	choose := widget.NewButtonWithIcon("Choose folder…", theme.FolderOpenIcon(), v.chooseFolder)

	// One compact header rather than a card: the card left the panes 121 px
	// tall at 980x640 (#235), which is why the log could not be read and the
	// form had to scroll.
	stateRow := container.NewBorder(nil, nil,
		container.NewHBox(dotBox, v.status),
		container.NewHBox(v.start, v.stop, v.open),
		v.health,
	)
	folderRow := container.NewBorder(nil, nil, widget.NewLabel("Folder"), choose, v.folder)

	v.note = widget.NewLabel("")
	v.note.Wrapping = fyne.TextWrapWord
	v.note.Selectable = true
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

	v.scroll = container.NewScroll(v.log)
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
	clear := widget.NewButtonWithIcon("Clear", theme.DeleteIcon(), v.clearLog)

	actions := container.NewBorder(nil, nil, v.follow, container.NewHBox(jump, copyLog, clear))
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

	v.secret = widget.NewLabel("")
	v.secret.Wrapping = fyne.TextWrapWord
	v.secret.Selectable = true

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
		formRow{caption: "Password", input: v.password},
	)

	v.save = widget.NewButtonWithIcon("Save configuration", theme.DocumentSaveIcon(), v.saveConfig)
	v.save.Importance = widget.HighImportance
	v.create = widget.NewButtonWithIcon("Create .env from the example", theme.DocumentCreateIcon(), v.createEnv)
	v.create.Hide()

	forms := container.NewGridWithColumns(2, serverForm, databaseForm)
	content := container.NewPadded(forms)

	v.configScroll = container.NewScroll(content)
	// The pane may not be smaller than the form: a window that cannot show the
	// fields is better refused than scrolled through, because a Fyne entry
	// keeps a scroll of its own and eats the wheel over it.
	v.configScroll.SetMinSize(fyne.NewSize(440, content.MinSize().Height))

	actions := container.NewBorder(nil, nil, nil, container.NewHBox(v.create, v.save), v.secret)

	v.configScrollPane = container.NewBorder(nil, container.NewPadded(actions), nil, nil, v.configScroll)
}

// configurationPane is the form with its pinned action row.
func (v *serverView) configurationPane() fyne.CanvasObject {
	return v.configScrollPane
}

// formRow is one caption and its value.
type formRow struct {
	caption string
	input   fyne.CanvasObject
}

// formRows lays captions and values out as a form, with every caption
// selectable.
func formRows(rows ...formRow) fyne.CanvasObject {
	objects := make([]fyne.CanvasObject, 0, len(rows)*2)

	for _, row := range rows {
		caption := widget.NewLabel(row.caption)
		caption.Alignment = fyne.TextAlignTrailing
		caption.Selectable = true

		objects = append(objects, caption, row.input)
	}

	return container.New(layout.NewFormLayout(), objects...)
}

// field is one .env key in the form, remembered so a load and a save can walk
// the same set. Enter in a field saves the form.
func (v *serverView) field(key, caption string) formRow {
	entry := widget.NewEntry()
	entry.OnSubmitted = func(string) { v.saveConfig() }
	v.fields[key] = entry

	return formRow{caption: caption, input: entry}
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

		return
	}

	v.note.Show()
}

// refresh redraws the status, the dot, the buttons and the log from the
// supervisor.
func (v *serverView) refresh() {
	if v.status == nil || v.log == nil {
		return
	}

	state := v.server().State()
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
		health = "Last exit: " + state.Exit.Error()
		kind = statusError
	}

	v.status.SetText(status)
	v.health.SetText(health)
	v.dot.FillColor = statusColour(v.ui.app.Settings().ThemeVariant(), kind)
	v.dot.Refresh()

	if state.Running {
		v.start.Disable()
		v.stop.Enable()
	} else {
		v.start.Enable()
		v.stop.Disable()
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

	v.log.Rows = append([]widget.TextGridRow(nil), v.log.Rows[count:]...)
	v.parsed = append([]serverctl.LogLine(nil), v.parsed[count:]...)
	v.rendered -= count
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

// copyLog puts the whole log on the clipboard. The grid has no selection of its
// own, so this is how a person gets the output out of the window.
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

// clearLog forgets the output and the grid that shows it.
func (v *serverView) clearLog() {
	v.server().ClearOutput()

	v.rendered = 0
	v.parsed = nil
	v.consumed = v.server().Written()
	v.log.SetText("")
	v.setNote("Log cleared. The server's own log file still has the history.")
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

func (v *serverView) startServer() {
	v.setNote("")

	v.mu.Lock()
	v.answered, v.answerErr = false, nil
	v.mu.Unlock()

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

func (v *serverView) chooseFolder() {
	dialog.ShowFolderOpen(func(uri fyne.ListableURI, err error) {
		if err != nil {
			v.setNote(err.Error())
			return
		}
		if uri == nil {
			return
		}

		v.useDir(uri.Path())
		v.folder.SetText(uri.Path())
		v.loadConfig()
		v.refresh()
	}, v.ui.window)
}

// loadConfig fills the form and the secret state from the folder. It never
// creates anything: a folder with no .env shows the button that does.
func (v *serverView) loadConfig() {
	dir := v.folderPath()

	file, err := serverctl.LoadEnv(filepath.Join(dir, serverctl.EnvFileName))
	v.env = file

	switch {
	case errors.Is(err, os.ErrNotExist):
		v.create.Show()
		v.setNote("There is no .env here yet — create it from the example, then set the database password.")
	case err != nil:
		v.create.Hide()
		v.setNote(err.Error())
	default:
		v.create.Hide()
		v.setNote("")
	}

	for key, entry := range v.fields {
		value := ""
		if file != nil {
			value = file.Value(key, "")
		}
		entry.SetText(value)
	}

	// The password is reported as a state, never shown: the file is the only
	// place the value lives.
	if exists, mode, err := serverctl.SecretState(dir); err != nil {
		v.secret.SetText("Database password: " + err.Error())
	} else if exists {
		v.secret.SetText(fmt.Sprintf("Database password: set in %s (mode %v)", serverctl.SecretPath, mode))
	} else {
		v.secret.SetText("Database password: not set yet")
	}

	v.password.SetText("")

	// Reading the .env here keeps the address off the refresh path, which runs
	// on every line the server prints.
	v.localURL = v.server().LocalURL()
}

func (v *serverView) createEnv() {
	if _, err := serverctl.LoadOrCreate(v.folderPath()); err != nil {
		v.setNote(err.Error())
		return
	}

	v.loadConfig()
	v.setNote(".env created for a local server. Set the database password, then start.")
}

func (v *serverView) saveConfig() {
	if v.env == nil {
		v.setNote("There is no .env to save — create one from the example first.")
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
