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
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"

	"github.com/fiwon123/eznit/internal/apiclient"
	"github.com/fiwon123/eznit/internal/config"
)

// projectURL is where About sends a reader.
const projectURL = "https://github.com/fiwon123/eznit"

type ui struct {
	app     fyne.App
	window  fyne.Window
	version string

	client  *apiclient.Client
	files   *filesView
	updater *appUpdater
}

// Run opens the window and blocks until it closes.
func Run(version string, apiURL string) error {
	a := app.NewWithID("com.fiwon123.eznit")
	a.Settings().SetTheme(newBrandTheme())
	a.SetIcon(logoResource())

	window := a.NewWindow("eznit")
	window.Resize(fyne.NewSize(980, 640))

	u := &ui{
		app:     a,
		window:  window,
		version: version,
		client:  apiclient.New(apiFrom(apiURL)),
		updater: newAppUpdater(version),
	}
	u.files = newFilesView(u)

	if token, err := config.Value("token"); err == nil {
		u.client.Token = token
	}

	if u.client.Token == "" {
		u.showLogin("")
	} else {
		u.showMain()
	}

	window.ShowAndRun()

	return nil
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

// showMain swaps the window to the signed-in view.
func (u *ui) showMain() {
	tabs := container.NewAppTabs(
		container.NewTabItem("Files", u.files.content()),
		container.NewTabItem("About", u.aboutContent()),
	)

	u.window.SetContent(tabs)
	u.files.refresh()
}

// showLogin swaps the window to the sign-in view with a line of context.
func (u *ui) showLogin(message string) {
	u.window.SetContent(newLoginView(u, message).content())
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
