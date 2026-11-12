//go:build fyne

// Package fyneui is the eznit desktop app: a native window over the same /v1
// API the CLI and the web UI use.
//
// It is a client, not a server. The server's browser UI is unchanged; this is
// the tool a developer installs beside the CLI, reading the same
// ~/.eznit/config.json so one login serves both.
package fyneui

import (
	"fmt"
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
	"github.com/fiwon123/eznit/internal/serverctl"
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

	// address is the instance the Files tab talks to, and where that address
	// came from — shown in the tab's header rather than picked silently.
	address apiChoice

	client    *apiclient.Client
	files     *filesView
	templates *templatesView
	server    *serverView
	updater   *appUpdater

	tabs *container.AppTabs
}

// apiChoice is one server address and the reason it is the current one.
type apiChoice struct {
	url    string
	source string
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
		address: apiFrom(apiURL),
		updater: newAppUpdater(version),
	}

	u.client = apiclient.New(u.address.url)
	u.files = newFilesView(u)
	u.templates = newTemplatesView(u)
	u.server = newServerView(u, serverFrom(serverDir))

	// The tabs exist before a session does. The Server tab has to be usable on
	// a machine whose server has never run — running it and pointing it at a
	// database is how a first install gets something to log in to.
	templatesTab := container.NewTabItemWithIcon("Templates", theme.ListIcon(), u.templates.content())

	u.tabs = container.NewAppTabs(
		container.NewTabItemWithIcon("Files", theme.FolderIcon(), u.files.content()),
		templatesTab,
		container.NewTabItemWithIcon("Server", theme.ComputerIcon(), u.server.content()),
		container.NewTabItemWithIcon("About", theme.InfoIcon(), u.aboutContent()),
	)

	// The catalogue is fetched when the tab is first opened, not at startup: a
	// window whose server is still being set up on another tab should not open
	// with a red message about it.
	u.tabs.OnSelected = func(item *container.TabItem) {
		if item == templatesTab {
			u.templates.ensureLoaded()
		}
	}

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

// apiFrom decides the API base: the flag, then GUI_API_URL, then the saved
// address, then the local stack. The environment beats the saved address so
// the documented GUI_API_URL keeps working after a login has stored one; the
// Files tab's header names whichever source won, so it is never silent.
func apiFrom(flagValue string) apiChoice {
	if value := strings.TrimSpace(flagValue); value != "" {
		return apiChoice{url: apiclient.NormalizeBaseURL(value), source: "the -api flag"}
	}

	if value := strings.TrimSpace(os.Getenv("GUI_API_URL")); value != "" {
		return apiChoice{url: apiclient.NormalizeBaseURL(value), source: "GUI_API_URL"}
	}

	if stored, err := config.Value("api_url"); err == nil && strings.TrimSpace(stored) != "" {
		return apiChoice{url: apiclient.NormalizeBaseURL(stored), source: "the saved address"}
	}

	return apiChoice{url: apiclient.NormalizeBaseURL(""), source: "the default"}
}

// localAddress is the API that a server started from the Server tab's folder
// answers on. The port comes from that folder's configuration, so the two tabs
// cannot disagree about where "this machine" is.
func (u *ui) localAddress() string {
	if u.server == nil {
		return apiclient.NormalizeBaseURL("")
	}

	return serverctl.LocalAddress(u.server.folderPath())
}

// setAPI points the client at an instance and keeps the stored session: the
// caller decides whether that token still belongs there.
func (u *ui) setAPI(address apiChoice) {
	u.address = apiChoice{
		url:    apiclient.NormalizeBaseURL(address.url),
		source: address.source,
	}
	u.client = apiclient.New(u.address.url)

	if token, err := config.Value("token"); err == nil {
		u.client.Token = token
	}

	// The catalogue is per instance: a new address is a new catalogue.
	if u.templates != nil {
		u.templates.forget()
	}
}

// useServer points the Files tab at another instance and remembers the choice.
// A different host is a different instance, and the token was issued by the old
// one, so it is forgotten; it reports whether the session had to go.
func (u *ui) useServer(address apiChoice) (bool, error) {
	sameHost := apiclient.SameHost(u.client.BaseURL, address.url)

	if err := config.Set("api_url", address.url); err != nil {
		return false, fmt.Errorf("the address could not be saved: %w", err)
	}

	u.setAPI(address)

	if sameHost {
		return false, nil
	}

	if err := config.Remove("token"); err != nil {
		return false, fmt.Errorf("the old session could not be forgotten: %w", err)
	}

	u.client.Token = ""

	return true, nil
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
