//go:build fyne

// Package fyneui is the eznit desktop app: a native window over the same /v1
// API the CLI and the web UI use.
//
// It is a client, not a server. The server's browser UI is unchanged; this is
// the tool a developer installs beside the CLI, reading the same
// ~/.eznit/config.json so one login serves both.
package fyneui

import (
	"os"
	"path/filepath"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"

	"github.com/fiwon123/eznit/internal/apiclient"
	"github.com/fiwon123/eznit/internal/config"
)

// projectURL is where About sends a reader.
const projectURL = "https://github.com/fiwon123/eznit"

type ui struct {
	app     fyne.App
	window  fyne.Window
	version string

	// clip is the system clipboard. It is a field rather than a call so a test
	// can supply one: the software driver's clipboard is nil.
	clip fyne.Clipboard

	client  *apiclient.Client
	files   *filesView
	server  *serverView
	updater *appUpdater

	tabs *container.AppTabs
}

// Run opens the window and blocks until it closes.
func Run(version string, apiURL string, serverDir string) error {
	a := app.NewWithID("com.fiwon123.eznit")
	a.Settings().SetTheme(newBrandTheme())
	a.SetIcon(logoResource())

	window := a.NewWindow("eznit")
	window.Resize(fyne.NewSize(980, 640))

	u := &ui{
		app:     a,
		window:  window,
		version: version,
		clip:    a.Clipboard(),
		client:  apiclient.New(apiFrom(apiURL)),
		updater: newAppUpdater(version),
	}
	u.files = newFilesView(u)
	u.server = newServerView(u, serverFrom(serverDir))

	// The tabs exist before a session does. The Server tab has to be usable on
	// a machine whose server has never run — running it and pointing it at a
	// database is how a first install gets something to log in to.
	u.tabs = container.NewAppTabs(
		container.NewTabItemWithIcon("Files", theme.FolderIcon(), u.files.content()),
		container.NewTabItemWithIcon("Server", theme.ComputerIcon(), u.server.content()),
		container.NewTabItemWithIcon("About", theme.InfoIcon(), u.aboutContent()),
	)
	window.SetContent(u.tabs)

	// A server this window started should not outlive it; ask before stopping
	// it, so closing the window is never a surprise shutdown.
	window.SetCloseIntercept(func() {
		if !u.server.running() {
			window.Close()
			return
		}

		dialog.ShowConfirm("Stop the local server?",
			"The server this window started is still running. Stop it and close the window?",
			func(confirmed bool) {
				if !confirmed {
					return
				}

				go func() {
					_ = u.server.server().Stop(stopTimeout)
					fyne.Do(window.Close)
				}()
			}, window)
	})

	if token, err := config.Value("token"); err == nil {
		u.client.Token = token
	}

	if u.client.Token == "" {
		u.showLogin("")
	} else {
		u.showMain()
	}

	go u.server.pollHealth()

	window.ShowAndRun()

	return nil
}

// serverFrom decides the server folder: the flag, then the stored setting,
// then the folder this binary was installed into.
func serverFrom(flagValue string) string {
	if value := strings.TrimSpace(flagValue); value != "" {
		return value
	}

	if stored, err := config.Value("server_dir"); err == nil && stored != "" {
		return stored
	}

	return defaultServerDir()
}

// defaultServerDir is the folder this window was installed into — the bundle a
// released window sits in. It is what "reset to default" returns to, and it is
// never written to the config file: a default that was stored on the first
// launch is indistinguishable from a choice.
func defaultServerDir() string {
	if executable, err := os.Executable(); err == nil {
		if resolved, err := filepath.EvalSymlinks(executable); err == nil {
			executable = resolved
		}

		return filepath.Dir(executable)
	}

	return "."
}

// apiFrom decides the API base: the flag, then the stored setting, then the
// environment, then the local stack.
func apiFrom(flagValue string) string {
	if value := strings.TrimSpace(flagValue); value != "" {
		return value
	}

	if stored, err := config.Value("api_url"); err == nil {
		return stored
	}

	return apiclient.NormalizeBaseURL(os.Getenv("GUI_API_URL"))
}

// showMain swaps the Files tab to the signed-in library.
func (u *ui) showMain() {
	u.setFilesTab(u.files.content())
	u.files.refresh()
}

// showLogin puts the sign-in form in the Files tab. The window keeps its other
// tabs: a signed-out user can still run a local server from here.
func (u *ui) showLogin(message string) {
	u.setFilesTab(newLoginView(u, message).content())
}

// setFilesTab replaces the first tab's content. The tab itself stays, so the
// Server tab keeps its log and its status across a sign-in or a sign-out.
func (u *ui) setFilesTab(content fyne.CanvasObject) {
	u.tabs.Items[0].Content = content
	u.tabs.Refresh()
}

// signOut forgets the session token and only the token: the API address and
// the git-host tokens stay.
func (u *ui) signOut() {
	if err := config.Remove("token"); err != nil {
		u.showLogin("The session could not be forgotten: " + err.Error())
		return
	}

	u.client.Token = ""
	u.showLogin("You are signed out.")
}

// copyToClipboard puts text where a person can paste it. It reports whether a
// clipboard existed: the software driver used by the tests has none, and a
// silent no-op would read as a successful copy.
func (u *ui) copyToClipboard(text string) bool {
	clipboard := u.clip

	if clipboard == nil && u.app != nil {
		clipboard = u.app.Clipboard()
	}

	if clipboard == nil {
		return false
	}

	clipboard.SetContent(text)

	return true
}
