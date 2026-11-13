//go:build fyne

package fyneui

import (
	"fmt"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/fiwon123/eznit/internal/apiclient"
)

// filesView is the caller's library: list, upload, download, delete and share,
// on one server — the one the header names.
type filesView struct {
	ui       *ui
	table    *widget.Table
	files    []apiclient.File
	selected int
	status   *prose

	// built is the tab's outer container, so a status or a selection that
	// changes the height of a wrapped line can lay the tab out again.
	built fyne.CanvasObject

	// ready is set by the first successful listing. Actions that need the
	// server stay off until then, so a stale or wrong address cannot look like
	// an empty library.
	ready bool

	address *widget.Label
	source  *widget.Label
	dot     *canvas.Circle
	test    *widget.Button
	change  *widget.Button

	upload   *widget.Button
	download *widget.Button
	remove   *widget.Button
	share    *widget.Button

	// The table's cells cannot be selectable — a selectable cell would take the
	// tap that selects the row — so the selected file is described under the
	// table, in a line that can be selected and copied.
	detail      *prose
	copyName    *widget.Button
	copyDetails *widget.Button
}

func newFilesView(u *ui) *filesView {
	view := &filesView{
		ui:       u,
		selected: -1,
		status:   newProse(""),
	}

	view.table = widget.NewTable(
		func() (int, int) { return len(view.files) + 1, 3 },
		func() fyne.CanvasObject { return widget.NewLabel("") },
		func(id widget.TableCellID, cell fyne.CanvasObject) {
			label, ok := cell.(*widget.Label)
			if !ok {
				return
			}

			if id.Row == 0 {
				label.TextStyle = fyne.TextStyle{Bold: true}
				label.SetText([]string{"Name", "Version", "Visibility"}[id.Col])
				return
			}

			label.TextStyle = fyne.TextStyle{}

			index := id.Row - 1
			if index < 0 || index >= len(view.files) {
				label.SetText("")
				return
			}

			file := view.files[index]
			switch id.Col {
			case 0:
				label.SetText(file.Name)
			case 1:
				label.SetText(fmt.Sprintf("%d", file.Version))
			case 2:
				if file.IsPublic {
					label.SetText("shared")
				} else {
					label.SetText("private")
				}
			}
		},
	)

	view.table.SetColumnWidth(0, 380)
	view.table.SetColumnWidth(1, 90)
	view.table.SetColumnWidth(2, 110)

	view.table.OnSelected = func(id widget.TableCellID) {
		if id.Row == 0 {
			view.table.UnselectAll()
			view.selected = -1
			view.updateActions()
			return
		}

		view.selected = id.Row - 1
		view.updateActions()
	}

	view.upload = widget.NewButtonWithIcon("Upload", theme.ContentAddIcon(), view.uploadFile)
	view.download = widget.NewButtonWithIcon("Download", theme.DownloadIcon(), view.downloadSelected)
	view.remove = widget.NewButtonWithIcon("Delete", theme.DeleteIcon(), view.deleteSelected)
	view.share = widget.NewButton("Share", view.toggleShare)

	view.updateActions()

	return view
}

// header is the line that says which server this tab is talking to, and what
// put that address there. It is rebuilt with the content, so a change shows up
// without a restart.
//
// The address is one line and ellipsizes: a URL is one unbreakable word, and a
// wrapped label would draw it straight over the buttons beside it. The source
// is its own row underneath, where it cannot compete for the same width.
func (v *filesView) header() fyne.CanvasObject {
	v.address = oneLine(v.ui.address.url)
	v.source = muted("(" + v.ui.address.source + ")")

	v.test = widget.NewButton("Test", v.testServer)
	v.change = widget.NewButtonWithIcon("Change…", theme.SettingsIcon(), v.ui.changeServer)

	line := container.NewBorder(nil, nil,
		widget.NewLabel("Server"),
		container.NewHBox(v.test, v.change),
		v.address,
	)

	return container.NewVBox(container.NewPadded(container.NewVBox(line, v.source)), widget.NewSeparator())
}

func (v *filesView) content() fyne.CanvasObject {
	refresh := widget.NewButtonWithIcon("Refresh", theme.ViewRefreshIcon(), v.refresh)

	toolbar := container.NewHBox(refresh, v.upload, widget.NewSeparator(), v.download, v.remove, v.share)

	v.detail = newProse("")
	v.copyName = widget.NewButton("Copy name", v.copySelectedName)
	v.copyDetails = widget.NewButton("Copy details", v.copySelectedDetails)

	detailRow := container.NewBorder(nil, nil, nil,
		container.NewHBox(v.copyName, v.copyDetails), v.detail)

	// The dot lives in the status bar, beside the message it colours, so it is
	// created before the bar that holds it.
	v.dot = canvas.NewCircle(statusColour(v.ui.app.Settings().ThemeVariant(), statusIdle))

	footer := container.NewVBox(container.NewPadded(detailRow), statusBar(v.dot, v.status))
	body := container.NewBorder(toolbar, footer, nil, nil, v.table)

	// A rebuilt view has not listed anything yet.
	v.ready = false
	v.updateActions()

	v.built = container.NewBorder(v.header(), nil, nil, nil, body)

	return v.built
}

// setStatus writes the one line under the table and colours the dot with it.
func (v *filesView) setStatus(text string, kind statusKind) {
	if v.status == nil {
		return
	}

	v.status.SetText(text)

	if v.dot != nil {
		v.dot.FillColor = statusColour(v.ui.app.Settings().ThemeVariant(), kind)
		v.dot.Refresh()
	}

	// A longer message is a taller bar, and the row it sits in was measured
	// before the text arrived.
	relayout(v.built)
}

func (v *filesView) selectedFile() (apiclient.File, bool) {
	if v.selected < 0 || v.selected >= len(v.files) {
		return apiclient.File{}, false
	}

	return v.files[v.selected], true
}

func (v *filesView) updateActions() {
	if v.upload != nil {
		if v.ready {
			v.upload.Enable()
		} else {
			v.upload.Disable()
		}
	}

	file, ok := v.selectedFile()

	if v.detail != nil {
		if !ok {
			v.detail.SetText("Select a file to see its details here.")
		} else {
			v.detail.SetText(fileDescription(file))
		}
	}

	if v.copyName != nil {
		for _, button := range []*widget.Button{v.copyName, v.copyDetails} {
			if ok {
				button.Enable()
			} else {
				button.Disable()
			}
		}
	}

	if !ok || !v.ready {
		v.download.Disable()
		v.remove.Disable()
		v.share.Disable()
		v.share.SetText("Share")
		relayout(v.built)

		return
	}

	v.download.Enable()
	v.remove.Enable()
	v.share.Enable()

	if file.IsPublic {
		v.share.SetText("Unshare")
	} else {
		v.share.SetText("Share")
	}

	// The description can wrap to a second line, which is taller than the row
	// the footer was measured for.
	relayout(v.built)
}

// fileDescription is the line under the table: what the row is, in words a
// person can read and copy.
func fileDescription(file apiclient.File) string {
	visibility := "private"
	if file.IsPublic {
		visibility = "shared"
	}

	return fmt.Sprintf("%s · version %d · %s · id %s", file.Name, file.Version, visibility, file.ID)
}

// fileRecord is the same thing as one tab-separated line, for a spreadsheet or
// a script.
func fileRecord(file apiclient.File) string {
	visibility := "private"
	if file.IsPublic {
		visibility = "shared"
	}

	return fmt.Sprintf("%s\t%d\t%s\t%s", file.Name, file.Version, visibility, file.ID)
}

func (v *filesView) copySelectedName() {
	file, ok := v.selectedFile()
	if !ok {
		return
	}

	if !v.ui.copyToClipboard(file.Name) {
		v.setStatus("This window has no clipboard available — the name could not be copied.", statusError)
		return
	}

	v.setStatus("Copied the name of "+file.Name+".", statusOK)
}

func (v *filesView) copySelectedDetails() {
	file, ok := v.selectedFile()
	if !ok {
		return
	}

	if !v.ui.copyToClipboard(fileRecord(file)) {
		v.setStatus("This window has no clipboard available — the details could not be copied.", statusError)
		return
	}

	v.setStatus("Copied the details of "+file.Name+".", statusOK)
}

// refresh reloads the library. The request runs off the UI goroutine and the
// widgets are touched only inside fyne.Do.
func (v *filesView) refresh() {
	address := v.ui.client.BaseURL
	v.setStatus("Loading your files from "+address+"…", statusPending)

	go func() {
		files, err := v.ui.client.ListFiles()

		fyne.Do(func() {
			if err != nil {
				v.takeOffline(address, err)
				return
			}

			v.ready = true
			v.files = files
			v.selected = -1
			v.table.UnselectAll()
			v.table.Refresh()
			v.updateActions()

			if len(files) == 0 {
				v.setStatus("No files yet. Upload one with the button above.", statusOK)
				return
			}

			v.setStatus(fmt.Sprintf("%d file(s) on %s.", len(files), address), statusOK)
		})
	}()
}

// takeOffline is the answer to a server that did not answer: say which address
// failed and what to do about it, and switch the actions off.
func (v *filesView) takeOffline(address string, err error) {
	v.ready = false
	v.files = nil
	v.selected = -1
	v.table.Refresh()
	v.updateActions()

	v.setStatus(fmt.Sprintf(
		"The server at %s did not answer (%s). Use Change… above to point the tab somewhere else, or Refresh to try again.",
		address, messageOf(err)), statusError)
}

// testServer asks the instance whether it answers, without listing anything:
// it is the "is this address right" button.
func (v *filesView) testServer() {
	address := v.ui.client.BaseURL
	client := apiclient.New(address)

	v.setStatus("Asking "+address+"…", statusPending)

	go func() {
		started := time.Now()
		err := client.Health()
		took := time.Since(started)

		fyne.Do(func() {
			if err != nil {
				v.setStatus(fmt.Sprintf("No answer from %s: %s", address, messageOf(err)), statusError)
				return
			}

			v.setStatus(fmt.Sprintf("%s answered in %s.", address, took.Round(time.Millisecond)), statusOK)
		})
	}()
}

func (v *filesView) uploadFile() {
	if !v.ready {
		v.setStatus("There is no server to upload to yet — check the address above.", statusError)
		return
	}

	dialog.ShowFileOpen(func(reader fyne.URIReadCloser, err error) {
		if err != nil {
			v.setStatus("The file could not be chosen: "+err.Error(), statusError)
			return
		}
		if reader == nil {
			return
		}

		path := reader.URI().Path()
		reader.Close()

		v.setStatus("Uploading "+path+"…", statusPending)

		go func() {
			err := v.ui.client.UploadFile(path)

			fyne.Do(func() {
				if err != nil {
					v.setStatus("The upload failed: "+messageOf(err), statusError)
					return
				}

				v.refresh()
			})
		}()
	}, v.ui.window)
}

func (v *filesView) downloadSelected() {
	file, ok := v.selectedFile()
	if !ok {
		return
	}

	dialog.ShowFileSave(func(writer fyne.URIWriteCloser, err error) {
		if err != nil {
			v.setStatus("The destination could not be chosen: "+err.Error(), statusError)
			return
		}
		if writer == nil {
			return
		}

		path := writer.URI().Path()
		writer.Close()

		v.setStatus("Downloading "+file.Name+"…", statusPending)

		go func() {
			saved, err := v.ui.client.DownloadFile(file.ID, path)

			fyne.Do(func() {
				if err != nil {
					v.setStatus("The download failed: "+messageOf(err), statusError)
					return
				}

				v.setStatus("Saved to "+saved+".", statusOK)
			})
		}()
	}, v.ui.window)
}

func (v *filesView) deleteSelected() {
	file, ok := v.selectedFile()
	if !ok {
		return
	}

	dialog.ShowConfirm("Delete "+file.Name,
		"This removes the file from the server. It cannot be undone.",
		func(confirmed bool) {
			if !confirmed {
				return
			}

			v.setStatus("Deleting "+file.Name+"…", statusPending)

			go func() {
				err := v.ui.client.DeleteFile(file.ID)

				fyne.Do(func() {
					if err != nil {
						v.setStatus("The delete failed: "+messageOf(err), statusError)
						return
					}

					v.refresh()
				})
			}()
		}, v.ui.window)
}

func (v *filesView) toggleShare() {
	file, ok := v.selectedFile()
	if !ok {
		return
	}

	apply := func(public bool) {
		v.setStatus("Updating "+file.Name+"…", statusPending)

		go func() {
			err := v.ui.client.SetFilePublic(file.ID, public)

			fyne.Do(func() {
				if err != nil {
					v.setStatus("The change failed: "+messageOf(err), statusError)
					return
				}

				v.refresh()
			})
		}()
	}

	if file.IsPublic {
		apply(false)
		return
	}

	dialog.ShowConfirm("Share "+file.Name,
		"Anyone on this instance will be able to download it.",
		func(confirmed bool) {
			if confirmed {
				apply(true)
			}
		}, v.ui.window)
}
