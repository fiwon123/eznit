//go:build fyne

package fyneui

import (
	"fmt"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/fiwon123/eznit/internal/apiclient"
	"github.com/fiwon123/eznit/internal/config"
)

// loginCard is the size of the sign-in card. It is fixed rather than fitted so
// the form does not stretch across a wide window; the card keeps its own
// minimum, and a test asserts both modes fit inside it.
const (
	loginWidth  float32 = 440
	loginHeight float32 = 560
)

// loginView is the sign-in and account-creation form. It is one form with a
// mode: the fields stay put when the mode changes, so a typo in the address is
// not lost.
type loginView struct {
	ui      *ui
	api     *widget.Entry
	email   *widget.Entry
	pass    *widget.Entry
	confirm *widget.Entry
	status  *widget.Label
	signup  bool
	busy    bool

	// card is the padded body, so a status that wraps can lay the card out
	// again and the layout test can measure what it holds.
	card fyne.CanvasObject
}

func newLoginView(u *ui, message string) *loginView {
	view := &loginView{
		ui:      u,
		api:     widget.NewEntry(),
		email:   widget.NewEntry(),
		pass:    widget.NewPasswordEntry(),
		confirm: widget.NewPasswordEntry(),
		status:  widget.NewLabel(message),
	}

	view.api.SetText(u.client.BaseURL)
	view.email.SetPlaceHolder("you@example.com")
	view.pass.SetPlaceHolder("your password")
	view.confirm.SetPlaceHolder("the same password again")

	// Enter submits from any field: the form's own button is gone, and a
	// person typing a password expects the key to finish the job.
	for _, entry := range []*widget.Entry{view.api, view.email, view.pass, view.confirm} {
		entry.OnSubmitted = func(string) { view.submit() }
	}

	return view
}

func (l *loginView) content() fyne.CanvasObject {
	logo := canvas.NewImageFromResource(logoResource())
	logo.FillMode = canvas.ImageFillContain
	logo.SetMinSize(fyne.NewSize(84, 84))

	title := widget.NewLabelWithStyle("eznit", fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	subtitle := widget.NewLabelWithStyle("Your project's setup, in one place you host.",
		fyne.TextAlignCenter, fyne.TextStyle{})

	useLocal := widget.NewButtonWithIcon("This machine", theme.ComputerIcon(), func() {
		l.api.SetText(l.ui.localAddress())
	})
	useLocal.Importance = widget.LowImportance

	serverRow := container.NewBorder(nil, nil, nil, useLocal, l.api)

	rows := []formRow{
		{caption: "Server", input: serverRow},
		{caption: "Email", input: l.email},
		{caption: "Password", input: l.pass},
	}
	if l.signup {
		rows = append(rows, formRow{caption: "Confirm", input: l.confirm})
	}

	submit := widget.NewButton(l.submitLabel(), l.submit)
	submit.Importance = widget.HighImportance

	mode := widget.NewButton(l.modeLabel(), l.toggleMode)
	mode.Importance = widget.LowImportance

	l.status.Alignment = fyne.TextAlignCenter
	l.status.Wrapping = fyne.TextWrapWord

	l.card = container.NewPadded(container.NewVBox(
		container.NewCenter(logo),
		title,
		subtitle,
		widget.NewSeparator(),
		formRows(rows...),
		submit,
		container.NewCenter(mode),
		l.status,
	))

	// One primary action on the page, the width of the card, and the mode
	// switch under it: a form with one way forward is a form a person finishes.
	return container.NewCenter(container.NewGridWrap(fyne.NewSize(loginWidth, loginHeight), l.card))
}

// submitLabel is the primary action's verb for the current mode.
func (l *loginView) submitLabel() string {
	if l.signup {
		return "Create account"
	}

	return "Log in"
}

// modeLabel is what the secondary action would do.
func (l *loginView) modeLabel() string {
	if l.signup {
		return "Back to log in"
	}

	return "Create an account"
}

func (l *loginView) toggleMode() {
	if l.busy {
		return
	}

	l.signup = !l.signup
	l.setStatus("")
	l.ui.setFilesTab(l.content())
}

// setStatus writes the line under the form. A longer message makes the card
// taller, and the card's rows were measured before the text arrived.
func (l *loginView) setStatus(text string) {
	l.status.SetText(text)
	relayout(l.card)
}

func (l *loginView) submit() {
	if l.busy {
		return
	}

	email := strings.TrimSpace(l.email.Text)
	password := l.pass.Text

	switch {
	case email == "":
		l.setStatus("Enter the email address of your account.")
		return
	case password == "":
		l.setStatus("Enter the password.")
		return
	case l.signup && password != l.confirm.Text:
		l.setStatus("The two passwords do not match.")
		return
	}

	base, err := apiclient.ValidateBaseURL(l.api.Text)
	if err != nil {
		l.setStatus(err.Error())
		return
	}

	if err := config.Set("api_url", base); err != nil {
		l.setStatus("The server address could not be saved: " + err.Error())
		return
	}

	l.busy = true
	l.setStatus("Talking to " + base + "…")

	signup := l.signup
	confirm := l.confirm.Text

	go func() {
		client := apiclient.New(base)

		var token string
		var err error

		if signup {
			err = client.Signup(email, password, confirm)
			if err == nil {
				token, err = client.Login(email, password)
			}
		} else {
			token, err = client.Login(email, password)
		}

		fyne.Do(func() {
			l.busy = false

			if err != nil {
				l.setStatus(messageOf(err))
				return
			}

			if token == "" {
				l.setStatus("The server answered without a session token.")
				return
			}

			if err := config.Set("token", token); err != nil {
				l.setStatus("The session could not be saved: " + err.Error())
				return
			}

			l.ui.client = apiclient.New(base)
			l.ui.client.Token = token
			l.ui.showMain()
		})
	}()
}

// messageOf is the one line a person needs: the host's message when there is
// one, the error otherwise.
func messageOf(err error) string {
	if apiErr, ok := err.(*apiclient.APIError); ok {
		if apiErr.Message != "" {
			return apiErr.Message
		}

		return fmt.Sprintf("The server answered %d.", apiErr.Status)
	}

	return err.Error()
}
