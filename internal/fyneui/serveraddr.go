//go:build fyne

package fyneui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	"github.com/fiwon123/eznit/internal/apiclient"
)

// The two choices the address dialog offers.
const (
	optionLocal  = "This machine"
	optionRemote = "Another server"
)

// changeServer asks where the Files tab should point: the server this machine
// runs, or any address the person types. It validates and tests before it
// saves, because a wrong address otherwise only shows up as "did not answer".
func (u *ui) changeServer() {
	current := u.address
	local := u.localAddress()

	mode := widget.NewRadioGroup([]string{optionLocal, optionRemote}, nil)
	mode.Horizontal = true

	localLine := widget.NewLabel(local)
	localLine.Wrapping = fyne.TextWrapWord
	localLine.Selectable = true

	remote := widget.NewEntry()
	remote.SetPlaceHolder("https://eznit.example.com")

	status := widget.NewLabel("")
	status.Wrapping = fyne.TextWrapWord
	status.Selectable = true

	result := widget.NewLabel("")
	result.Wrapping = fyne.TextWrapWord
	result.Selectable = true

	// Selecting a radio option may run its handler, so the labels it writes to
	// exist before the first selection.
	isLocal := current.url == apiclient.NormalizeBaseURL(local)
	if isLocal {
		mode.SetSelected(optionLocal)
	} else {
		mode.SetSelected(optionRemote)
		remote.SetText(current.url)
	}

	var popup *dialog.CustomDialog

	// target is the address the current mode means, or an error the dialog
	// shows instead of saving.
	target := func() (apiChoice, error) {
		if mode.Selected == optionLocal {
			return apiChoice{url: local, source: "the local server"}, nil
		}

		checked, err := apiclient.ValidateBaseURL(remote.Text)
		if err != nil {
			return apiChoice{}, err
		}

		return apiChoice{url: checked, source: "the address you entered"}, nil
	}

	showMode := func() {
		if mode.Selected == optionLocal {
			localLine.Show()
			remote.Hide()
			return
		}

		localLine.Hide()
		remote.Show()
	}

	mode.OnChanged = func(string) {
		status.SetText("")
		result.SetText("")
		showMode()
	}

	test := widget.NewButton("Test", func() {
		choice, err := target()
		if err != nil {
			result.SetText(err.Error())
			return
		}

		result.SetText("Asking " + choice.url + "…")

		go func() {
			err := apiclient.New(choice.url).Health()

			fyne.Do(func() {
				if err != nil {
					result.SetText("No answer from " + choice.url + ": " + messageOf(err))
					return
				}

				result.SetText(choice.url + " answered.")
			})
		}()
	})

	finish := func(choice apiChoice) {
		signedOut, err := u.useServer(choice)
		if err != nil {
			status.SetText(err.Error())

			return
		}

		if popup != nil {
			popup.Hide()
		}

		if signedOut {
			u.showLogin("Pointing at " + choice.url + ". Sign in — a session belongs to the server that issued it.")
			return
		}

		// showMain reloads the library, which reports the new address itself.
		u.showMain()
	}

	use := func() {
		choice, err := target()
		if err != nil {
			status.SetText(err.Error())

			return
		}

		if u.client.Token != "" && !apiclient.SameHost(u.client.BaseURL, choice.url) {
			dialog.ShowConfirm("Sign out of the current server?",
				"A session belongs to the server that issued it.\n\nChanging to "+choice.url+
					" signs you out of "+u.client.BaseURL+". Continue?",
				func(confirmed bool) {
					if confirmed {
						finish(choice)
					}
				}, u.window)

			return
		}

		finish(choice)
	}

	remote.OnSubmitted = func(string) { use() }

	address := container.NewVBox(localLine, remote)
	buttons := container.NewHBox(test, widget.NewButton("Use this server", use))

	body := container.NewVBox(
		widget.NewLabel("The Files tab signs in to one server."),
		mode,
		address,
		buttons,
		result,
		status,
	)

	popup = dialog.NewCustom("Server address", "Cancel", body, u.window)
	showMode()
	popup.Show()
}
