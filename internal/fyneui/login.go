//go:build fyne

package fyneui

import (
	"fmt"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"github.com/fiwon123/eznit/internal/apiclient"
	"github.com/fiwon123/eznit/internal/config"
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

	return view
}

func (l *loginView) content() fyne.CanvasObject {
	logo := canvas.NewImageFromResource(logoResource())
	logo.FillMode = canvas.ImageFillContain
	logo.SetMinSize(fyne.NewSize(84, 84))

	title := widget.NewLabelWithStyle("eznit", fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	subtitle := widget.NewLabelWithStyle("Your project's setup, in one place you host.",
		fyne.TextAlignCenter, fyne.TextStyle{})
	subtitle.Wrapping = fyne.TextWrapWord

	items := []*widget.FormItem{
		widget.NewFormItem("Server", l.api),
		widget.NewFormItem("Email", l.email),
		widget.NewFormItem("Password", l.pass),
	}
	if l.signup {
		items = append(items, widget.NewFormItem("Confirm", l.confirm))
	}

	form := widget.NewForm(items...)
	form.SubmitText = "Log in"
	if l.signup {
		form.SubmitText = "Create account"
	}
	form.OnSubmit = l.submit

	modeLabel := "Create an account"
	if l.signup {
		modeLabel = "Back to log in"
	}
	mode := widget.NewButton(modeLabel, l.toggleMode)

	body := container.NewVBox(
		container.NewCenter(logo),
		title,
		subtitle,
		widget.NewSeparator(),
		form,
		mode,
		l.status,
	)

	// The form is the whole page, so it is capped rather than stretched across
	// a wide window.
	return container.NewCenter(container.NewGridWrap(fyne.NewSize(440, 560),
		container.NewPadded(body)))
}

func (l *loginView) toggleMode() {
	if l.busy {
		return
	}

	l.signup = !l.signup
	l.status.SetText("")
	l.ui.setFilesTab(l.content())
}

func (l *loginView) submit() {
	if l.busy {
		return
	}

	email := strings.TrimSpace(l.email.Text)
	password := l.pass.Text

	switch {
	case email == "":
		l.status.SetText("Enter the email address of your account.")
		return
	case password == "":
		l.status.SetText("Enter the password.")
		return
	case l.signup && password != l.confirm.Text:
		l.status.SetText("The two passwords do not match.")
		return
	}

	base := apiclient.NormalizeBaseURL(l.api.Text)
	if err := config.Set("api_url", base); err != nil {
		l.status.SetText("The server address could not be saved: " + err.Error())
		return
	}

	l.busy = true
	l.status.SetText("Talking to " + base + "…")

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
				l.status.SetText(messageOf(err))
				return
			}

			if token == "" {
				l.status.SetText("The server answered without a session token.")
				return
			}

			if err := config.Set("token", token); err != nil {
				l.status.SetText("The session could not be saved: " + err.Error())
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
