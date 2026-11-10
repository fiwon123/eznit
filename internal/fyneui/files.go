//go:build fyne

package fyneui

import (
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/fiwon123/eznit/internal/apiclient"
)

// filesView is the caller's library: list, upload, download, delete and share.
type filesView struct {
	ui       *ui
	table    *widget.Table
	files    []apiclient.File
	selected int
	status   *widget.Label

	download *widget.Button
	remove   *widget.Button
	share    *widget.Button
}

func newFilesView(u *ui) *filesView {
	view := &filesView{
		ui:       u,
		selected: -1,
		status:   widget.NewLabel(""),
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

	view.download = widget.NewButtonWithIcon("Download", theme.DownloadIcon(), view.downloadSelected)
	view.remove = widget.NewButtonWithIcon("Delete", theme.DeleteIcon(), view.deleteSelected)
	view.share = widget.NewButton("Share", view.toggleShare)

	view.updateActions()

	return view
}

func (v *filesView) content() fyne.CanvasObject {
	refresh := widget.NewButtonWithIcon("Refresh", theme.ViewRefreshIcon(), v.refresh)
	upload := widget.NewButtonWithIcon("Upload", theme.ContentAddIcon(), v.upload)

	toolbar := container.NewHBox(refresh, upload, widget.NewSeparator(), v.download, v.remove, v.share)

	return container.NewBorder(toolbar, v.status, nil, nil, v.table)
}

func (v *filesView) selectedFile() (apiclient.File, bool) {
	if v.selected < 0 || v.selected >= len(v.files) {
		return apiclient.File{}, false
	}

	return v.files[v.selected], true
}

func (v *filesView) updateActions() {
	file, ok := v.selectedFile()

	if !ok {
		v.download.Disable()
		v.remove.Disable()
		v.share.Disable()
		v.share.SetText("Share")
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
}

// refresh reloads the library. The request runs off the UI goroutine and the
// widgets are touched only inside fyne.Do.
func (v *filesView) refresh() {
	v.status.SetText("Loading your files…")

	go func() {
		files, err := v.ui.client.ListFiles()

		fyne.Do(func() {
			if err != nil {
				v.status.SetText("The files could not be loaded: " + messageOf(err))
				return
			}

			v.files = files
			v.selected = -1
			v.table.UnselectAll()
			v.table.Refresh()
			v.updateActions()

			if len(files) == 0 {
				v.status.SetText("No files yet. Upload one with the button above.")
				return
			}

			v.status.SetText(fmt.Sprintf("%d file(s).", len(files)))
		})
	}()
}

func (v *filesView) upload() {
	dialog.ShowFileOpen(func(reader fyne.URIReadCloser, err error) {
		if err != nil {
			v.status.SetText("The file could not be chosen: " + err.Error())
			return
		}
		if reader == nil {
			return
		}

		path := reader.URI().Path()
		reader.Close()

		v.status.SetText("Uploading " + path + "…")

		go func() {
			err := v.ui.client.UploadFile(path)

			fyne.Do(func() {
				if err != nil {
					v.status.SetText("The upload failed: " + messageOf(err))
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
			v.status.SetText("The destination could not be chosen: " + err.Error())
			return
		}
		if writer == nil {
			return
		}

		path := writer.URI().Path()
		writer.Close()

		v.status.SetText("Downloading " + file.Name + "…")

		go func() {
			saved, err := v.ui.client.DownloadFile(file.ID, path)

			fyne.Do(func() {
				if err != nil {
					v.status.SetText("The download failed: " + messageOf(err))
					return
				}

				v.status.SetText("Saved to " + saved + ".")
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

			v.status.SetText("Deleting " + file.Name + "…")

			go func() {
				err := v.ui.client.DeleteFile(file.ID)

				fyne.Do(func() {
					if err != nil {
						v.status.SetText("The delete failed: " + messageOf(err))
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
		v.status.SetText("Updating " + file.Name + "…")

		go func() {
			err := v.ui.client.SetFilePublic(file.ID, public)

			fyne.Do(func() {
				if err != nil {
					v.status.SetText("The change failed: " + messageOf(err))
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
