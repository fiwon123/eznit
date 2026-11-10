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
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/fiwon123/eznit/internal/config"
	"github.com/fiwon123/eznit/internal/serverctl"
)

// stopTimeout is how long the window waits for the server to drain before it
// gives up and kills it. The API drains for 10s; this is the outer bound.
const stopTimeout = 15 * time.Second

// serverView is the Local server tab: run the eznit-server from a bundle
// folder, watch what it prints, and edit the .env beside it.
//
// It is a control panel, not a remote client: it drives a process on this
// machine. A server elsewhere is reached from the Files tab, with an address.
type serverView struct {
	ui *ui

	// mu guards the folder and the supervisor, which the health ticker reads
	// on its own goroutine.
	mu         sync.Mutex
	dir        string
	supervisor *serverctl.Supervisor

	built fyne.CanvasObject

	folder *widget.Label
	status *widget.Label
	health *widget.Label
	start  *widget.Button
	stop   *widget.Button
	open   *widget.Button

	log    *widget.TextGrid
	scroll *container.Scroll

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
	v.mu.Unlock()

	if err := config.Set("server_dir", dir); err != nil {
		if v.note != nil {
			v.note.SetText("The folder could not be saved for the next start: " + err.Error())
		}
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

// running reports whether the panel's server is up, for the window's close
// hook: a server this window started should not outlive it.
func (v *serverView) running() bool {
	return v.server().State().Running
}

func (v *serverView) content() fyne.CanvasObject {
	if v.built != nil {
		return v.built
	}

	v.folder = widget.NewLabel(v.folderPath())
	v.folder.Wrapping = fyne.TextWrapWord
	v.folder.Selectable = true

	choose := widget.NewButtonWithIcon("Choose folder…", theme.FolderOpenIcon(), v.chooseFolder)

	v.status = widget.NewLabel("Stopped")
	v.health = widget.NewLabel("")
	v.health.Wrapping = fyne.TextWrapWord

	v.start = widget.NewButtonWithIcon("Start", theme.MediaPlayIcon(), v.startServer)
	v.start.Importance = widget.HighImportance
	v.stop = widget.NewButtonWithIcon("Stop", theme.MediaStopIcon(), v.stopServer)
	v.stop.Importance = widget.DangerImportance
	v.open = widget.NewButtonWithIcon("Open in browser", theme.ComputerIcon(), v.openBrowser)
	clear := widget.NewButton("Clear log", v.server().ClearOutput)

	folderRow := container.NewBorder(nil, nil, widget.NewLabel("Folder"), choose, v.folder)
	buttons := container.NewHBox(v.start, v.stop, v.open, clear)

	controls := widget.NewCard("Local server",
		"Runs the eznit-server in this folder, with the .env beside it, and shows what it says.",
		container.NewVBox(folderRow, v.status, v.health, buttons))

	v.log = widget.NewTextGrid()
	v.scroll = container.NewScroll(v.log)
	v.scroll.SetMinSize(fyne.NewSize(440, 140))
	logCard := widget.NewCard("Output", "The server's own log, as it prints it.", v.scroll)

	v.password = widget.NewPasswordEntry()
	v.password.SetPlaceHolder("leave empty to keep the current password")
	v.secret = widget.NewLabel("")
	v.note = widget.NewLabel("")
	v.note.Wrapping = fyne.TextWrapWord

	serverForm := widget.NewForm(
		v.field("API_PORT", "API port"),
		v.field("API_SERVE_WEB", "Serve the web UI"),
		v.field("API_UPLOADS", "Uploads folder"),
		v.field("API_LOGS", "Logs folder"),
	)
	databaseForm := widget.NewForm(
		v.field("DB_HOST", "Host"),
		v.field("DB_PORT", "Port"),
		v.field("DB_USER", "User"),
		v.field("DB_NAME", "Database"),
		widget.NewFormItem("Password", v.password),
	)

	save := widget.NewButtonWithIcon("Save configuration", theme.DocumentSaveIcon(), v.saveConfig)
	save.Importance = widget.HighImportance
	v.create = widget.NewButtonWithIcon("Create .env from the example", theme.DocumentCreateIcon(), v.createEnv)
	v.create.Hide()

	configCard := widget.NewCard("Configuration",
		"The .env this server reads. A change applies the next time it starts.",
		container.NewVBox(
			container.NewGridWithColumns(2, serverForm, databaseForm),
			v.secret,
			container.NewHBox(save, v.create),
			v.note,
		))

	upper := container.NewBorder(controls, nil, nil, nil, logCard)
	split := container.NewVSplit(upper, container.NewVScroll(configCard))
	split.SetOffset(0.58)

	v.built = split

	v.loadConfig()
	v.refresh()

	return v.built
}

// pollHealth is started by the window once it is shown, not by the content
// builder: a view that has only been laid out has nothing to poll.

// field is one .env key in the form, remembered so a load and a save can walk
// the same set.
func (v *serverView) field(key, label string) *widget.FormItem {
	entry := widget.NewEntry()
	v.fields[key] = entry

	return widget.NewFormItem(label, entry)
}

// refresh redraws the status, the buttons and the log from the supervisor.
func (v *serverView) refresh() {
	state := v.server().State()

	switch {
	case state.Running:
		v.status.SetText(fmt.Sprintf("Running — pid %d", state.PID))
	case state.Exit != nil:
		v.status.SetText("Stopped after an error: " + state.Exit.Error())
	default:
		v.status.SetText("Stopped")
	}

	if state.Running {
		v.start.Disable()
		v.stop.Enable()
	} else {
		v.start.Enable()
		v.stop.Disable()
	}

	v.log.SetText(strings.Join(v.server().Output(), "\n"))
	v.scroll.ScrollToBottom()
}

// pollHealth asks the API whether it answers, which the process state alone
// cannot say: a server that is up and failing to reach its database is running
// and useless.
func (v *serverView) pollHealth() {
	for {
		time.Sleep(3 * time.Second)

		base := v.server().LocalURL()
		err := serverctl.Health(base, 2*time.Second)

		fyne.Do(func() {
			if !v.server().State().Running {
				v.health.SetText("")
				return
			}

			if err != nil {
				v.health.SetText("Not answering at " + base + " yet — see the log below.")
				return
			}

			v.health.SetText("Answering at " + base)
		})
	}
}

func (v *serverView) startServer() {
	v.note.SetText("")

	if err := v.server().Start(); err != nil {
		v.status.SetText("Cannot start: " + err.Error())
		return
	}

	// A first start creates the .env from the example, so the form shows what
	// the server is actually reading.
	v.loadConfig()
	v.refresh()
}

func (v *serverView) stopServer() {
	v.stop.Disable()
	v.start.Disable()
	v.status.SetText("Stopping…")

	go func() {
		err := v.server().Stop(stopTimeout)

		fyne.Do(func() {
			if err != nil {
				v.note.SetText("The stop was not clean: " + err.Error())
			}
			v.refresh()
		})
	}()
}

func (v *serverView) openBrowser() {
	base, err := url.Parse(v.server().LocalURL())
	if err != nil {
		v.note.SetText(err.Error())
		return
	}

	if err := v.ui.app.OpenURL(base); err != nil {
		v.note.SetText("The browser did not open: " + err.Error())
	}
}

func (v *serverView) chooseFolder() {
	dialog.ShowFolderOpen(func(uri fyne.ListableURI, err error) {
		if err != nil {
			v.note.SetText(err.Error())
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
		v.note.SetText("There is no .env here yet — create it from the example, then set the database password.")
	case err != nil:
		v.create.Hide()
		v.note.SetText(err.Error())
	default:
		v.create.Hide()
		v.note.SetText("")
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
}

func (v *serverView) createEnv() {
	if _, err := serverctl.LoadOrCreate(v.folderPath()); err != nil {
		v.note.SetText(err.Error())
		return
	}

	v.loadConfig()
	v.note.SetText(".env created for a local server. Set the database password, then start.")
}

func (v *serverView) saveConfig() {
	if v.env == nil {
		v.note.SetText("There is no .env to save — create one from the example first.")
		return
	}

	port := strings.TrimSpace(v.fields["API_PORT"].Text)
	if number, err := strconv.Atoi(port); err != nil || number <= 0 || number > 65535 {
		v.note.SetText("The API port must be a whole number between 1 and 65535.")
		return
	}

	for key, entry := range v.fields {
		v.env.Set(key, strings.TrimSpace(entry.Text))
	}

	if password := v.password.Text; password != "" {
		if err := serverctl.WriteSecret(v.folderPath(), password); err != nil {
			v.note.SetText("The password file could not be written: " + err.Error())
			return
		}

		// The host binary reads the file, not a variable: that is what keeps
		// the password out of the process table.
		v.env.Set("DB_PASSWORD_FILE", "./"+serverctl.SecretPath)
		v.env.Unset("DB_PWD")
	}

	if err := v.env.Save(); err != nil {
		v.note.SetText("The .env could not be saved: " + err.Error())
		return
	}

	state := v.server().State()
	v.loadConfig()

	note := "Saved."
	if state.Running {
		note += " Restart the server to use the new values."
	}
	v.note.SetText(note)
}
