//go:build fyne

package fyneui

import (
	"strings"
	"sync"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
)

// progressShown is how many lines of the container runtime's output the dialog
// shows at once: the last few are where the reason is, and a modal that grows
// to a thousand lines is not a progress report.
const progressShown = 8

// progressKept is how much of that output is kept for the dialog a failure
// opens. It is the runtime's own words, which are the fix.
const progressKept = 200

// databaseProgress is the modal the database step writes to while it runs.
//
// A first start pulls two images, and a window that says nothing for a minute
// looks hung. The step runs off the UI goroutine, so the text is replaced
// through fyne.Do, and the mutex covers the two pipes the runtime writes on.
type databaseProgress struct {
	dialog *dialog.CustomDialog
	text   *widget.Label

	mu    sync.Mutex
	lines []string
}

func (v *serverView) showDatabaseProgress() *databaseProgress {
	message := widget.NewLabel("Starting or checking the database. " +
		"The first start downloads the images, so it can take a few minutes.")
	message.Wrapping = fyne.TextWrapWord

	text := widget.NewLabel("…")
	text.TextStyle = fyne.TextStyle{Monospace: true}
	text.Wrapping = fyne.TextWrapWord

	scroll := container.NewVScroll(text)
	scroll.SetMinSize(fyne.NewSize(560, 160))

	content := container.NewBorder(container.NewVBox(message, widget.NewSeparator()), nil, nil, nil, scroll)

	modal := dialog.NewCustomWithoutButtons("Preparing the database", content, v.ui.window)
	modal.Show()

	return &databaseProgress{dialog: modal, text: text}
}

// Write keeps the runtime's output: the dialog shows the tail of it, and the
// whole thing is available to the dialog a failure opens.
func (p *databaseProgress) Write(data []byte) (int, error) {
	p.mu.Lock()

	p.lines = append(p.lines, strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")...)
	if len(p.lines) > progressKept {
		p.lines = append([]string(nil), p.lines[len(p.lines)-progressKept:]...)
	}

	shown := p.lines
	if len(shown) > progressShown {
		shown = shown[len(shown)-progressShown:]
	}
	text := strings.Join(shown, "\n")
	p.mu.Unlock()

	fyne.Do(func() { p.text.SetText(text) })

	return len(data), nil
}

// Output is everything the runtime said, for the failure dialog.
func (p *databaseProgress) Output() string {
	p.mu.Lock()
	defer p.mu.Unlock()

	return strings.Join(p.lines, "\n")
}

// Close takes the modal away, whether the step worked or not.
func (p *databaseProgress) Close() {
	if p.dialog != nil {
		p.dialog.Hide()
	}
}
