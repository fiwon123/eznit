//go:build fyne

package fyneui

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/fiwon123/eznit/internal/apiclient"
	"github.com/fiwon123/eznit/internal/domain/templates"
	"github.com/fiwon123/eznit/internal/templateapply"
)

// allKinds is the filter's "no filter" entry. A Select cannot hold an empty
// label, so the empty kind lives behind this name.
const allKinds = "All kinds"

// bundleExtension is what a template is written to: the file
// `eznit template apply --from-file` reads.
const bundleExtension = ".eznit"

// templatesView is the catalogue: every public template plus the caller's own,
// one template's contents, and the offline bundle to take away.
//
// It reads only. Applying a template into a folder writes files, which is the
// other half of the feature and a change of its own.
type templatesView struct {
	ui *ui

	items    []templates.TemplateData
	selected int
	detail   *templates.TemplateDetail
	loaded   bool

	table   *widget.Table
	kind    *widget.Select
	mine    *widget.Check
	refresh *widget.Button
	export  *widget.Button
	apply   *widget.Button

	address *widget.Label
	source  *widget.Label
	dot     *canvas.Circle

	// built is the tab's outer container, so a status message that changes
	// height can lay the tab out again.
	built fyne.CanvasObject

	summary   *prose
	files     *prose
	variables *prose
	commands  *prose
	status    *prose
}

func newTemplatesView(u *ui) *templatesView {
	view := &templatesView{
		ui:       u,
		selected: -1,
		status:   newProse(""),
	}

	view.table = widget.NewTable(
		func() (int, int) { return len(view.items) + 1, 6 },
		func() fyne.CanvasObject { return widget.NewLabel("") },
		func(id widget.TableCellID, cell fyne.CanvasObject) {
			label, ok := cell.(*widget.Label)
			if !ok {
				return
			}

			if id.Row == 0 {
				label.TextStyle = fyne.TextStyle{Bold: true}
				label.SetText([]string{"Name", "Kind", "Version", "Owner", "Files", "Visibility"}[id.Col])
				return
			}

			label.TextStyle = fyne.TextStyle{}

			index := id.Row - 1
			if index < 0 || index >= len(view.items) {
				label.SetText("")
				return
			}

			item := view.items[index]
			switch id.Col {
			case 0:
				label.SetText(item.Name)
			case 1:
				label.SetText(item.Kind)
			case 2:
				label.SetText(fmt.Sprintf("%d", item.Version))
			case 3:
				label.SetText(item.OwnerEmail)
			case 4:
				label.SetText(fmt.Sprintf("%d", item.FileCount))
			case 5:
				if item.IsPublic {
					label.SetText("shared")
				} else {
					label.SetText("private")
				}
			}
		},
	)

	view.table.SetColumnWidth(0, 260)
	view.table.SetColumnWidth(1, 110)
	view.table.SetColumnWidth(2, 70)
	view.table.SetColumnWidth(3, 180)
	view.table.SetColumnWidth(4, 60)
	view.table.SetColumnWidth(5, 90)

	view.table.OnSelected = func(id widget.TableCellID) {
		if id.Row == 0 {
			view.table.UnselectAll()
			view.selected = -1
			view.showDetail(nil)
			view.updateActions()
			return
		}

		view.selected = id.Row - 1
		view.loadSelected()
	}

	view.refresh = widget.NewButtonWithIcon("Refresh", theme.ViewRefreshIcon(), view.reload)
	view.export = widget.NewButtonWithIcon("Save as bundle…", theme.DownloadIcon(), view.exportSelected)
	view.apply = widget.NewButtonWithIcon("Apply…", theme.ConfirmIcon(), view.applySelected)

	// The handler goes on after the first selection: SetSelected calls it, and
	// a fetch during construction would race the tab that owns the status line.
	view.kind = widget.NewSelect(append([]string{allKinds}, templates.Kinds...), nil)
	view.kind.SetSelected(allKinds)
	view.kind.OnChanged = func(string) { view.reload() }

	view.mine = widget.NewCheck("Only mine", func(bool) { view.reload() })

	view.updateActions()

	return view
}

// content is the tab: which server, the filters, the catalogue, the selected
// template and the message line.
func (v *templatesView) content() fyne.CanvasObject {
	v.address = oneLine(v.ui.address.url)
	v.source = muted("(" + v.ui.address.source + ")")

	header := container.NewBorder(nil, nil,
		widget.NewLabel("Server"),
		container.NewHBox(v.refresh, v.export, v.apply),
		v.address,
	)

	// The dot colours the message at the bottom of the tab, so it is made
	// before the bar that holds it.
	v.dot = canvas.NewCircle(statusColour(v.ui.app.Settings().ThemeVariant(), statusIdle))

	filters := container.NewHBox(v.kind, v.mine)

	v.buildDetail()
	split := container.NewVSplit(v.table, container.NewVScroll(v.detailPane()))
	split.Offset = 0.5

	top := container.NewVBox(
		container.NewPadded(container.NewVBox(header, v.source)),
		widget.NewSeparator(),
		container.NewPadded(filters),
	)

	v.built = container.NewBorder(top, statusBar(v.dot, v.status), nil, nil, split)

	return v.built
}

// buildDetail creates the four selectable sections under the table.
func (v *templatesView) buildDetail() {
	v.summary = selectableWrap()
	v.files = selectableWrap()
	v.variables = selectableWrap()
	v.commands = selectableWrap()

	v.showDetail(nil)
}

// detailPane is the selected template, in four parts.
func (v *templatesView) detailPane() fyne.CanvasObject {
	return container.NewPadded(container.NewVBox(
		v.summary,
		section("Files", v.files),
		section("Variables", v.variables),
		section("Commands", v.commands),
	))
}

// section is a bold, selectable caption above its body.
func section(caption string, body *prose) fyne.CanvasObject {
	title := widget.NewLabel(caption)
	title.TextStyle = fyne.TextStyle{Bold: true}
	title.Selectable = true

	return container.NewVBox(title, body)
}

// selectableWrap is the one shape every value on this tab takes: wrapped, so a
// long path can neither widen the window nor paint over its neighbour, and
// selectable, so it can be copied.
func selectableWrap() *prose {
	return newProse("")
}

// reload fetches the catalogue again with the current filters.
func (v *templatesView) reload() {
	address := v.ui.client.BaseURL
	kind := v.selectedKind()
	mine := v.mine.Checked

	v.setStatus("Loading the catalogue from "+address+"…", statusPending)

	go func() {
		items, err := v.ui.client.ListTemplates(mine, kind)

		fyne.Do(func() {
			if err != nil {
				v.items = nil
				v.detail = nil
				v.selected = -1
				v.table.UnselectAll()
				v.table.Refresh()
				v.showDetail(nil)
				v.updateActions()

				v.setStatus(fmt.Sprintf(
					"The catalogue could not be loaded from %s (%s). Use Refresh to try again, or Change… on the Files tab to point the window somewhere else.",
					address, messageOf(err)), statusError)

				return
			}

			// The "mine" route filters nothing on the server, so the kind is
			// applied here for both shapes.
			items = apiclient.FilterTemplates(items, kind)

			v.items = items
			v.detail = nil
			v.selected = -1
			v.loaded = true
			v.table.UnselectAll()
			v.table.Refresh()
			v.showDetail(nil)
			v.updateActions()

			if len(items) == 0 {
				v.setStatus("No template matches. Publish one with `eznit template publish`, then Refresh.", statusOK)
				return
			}

			v.setStatus(fmt.Sprintf("%d template(s) on %s.", len(items), address), statusOK)
		})
	}()
}

// ensureLoaded fetches the catalogue the first time the tab is opened.
func (v *templatesView) ensureLoaded() {
	if v.loaded {
		return
	}

	v.reload()
}

// forget drops what was loaded, so the next visit reloads from the new server.
func (v *templatesView) forget() {
	v.loaded = false
	v.items = nil
	v.detail = nil
	v.selected = -1

	if v.table != nil {
		v.table.UnselectAll()
		v.table.Refresh()
	}

	v.showDetail(nil)
	v.updateActions()
}

// selectedKind is the filter's kind, or "" for all of them.
func (v *templatesView) selectedKind() string {
	if v.kind == nil || v.kind.Selected == allKinds || v.kind.Selected == "" {
		return ""
	}

	return v.kind.Selected
}

func (v *templatesView) selectedTemplate() (templates.TemplateData, bool) {
	if v.selected < 0 || v.selected >= len(v.items) {
		return templates.TemplateData{}, false
	}

	return v.items[v.selected], true
}

// loadSelected fetches the whole template: its files, questions and commands.
func (v *templatesView) loadSelected() {
	item, ok := v.selectedTemplate()
	if !ok {
		return
	}

	id := item.ID.String()

	v.detail = nil
	v.showDetail(nil)
	v.updateActions()
	v.setStatus("Loading "+item.Name+"…", statusPending)

	go func() {
		detail, err := v.ui.client.GetTemplate(id)

		fyne.Do(func() {
			// The selection may have moved while the request was in flight.
			current, ok := v.selectedTemplate()
			if !ok || current.ID.String() != id {
				return
			}

			if err != nil {
				v.setStatus("The template could not be loaded: "+messageOf(err), statusError)
				return
			}

			v.detail = detail
			v.showDetail(detail)
			v.updateActions()

			v.setStatus(fmt.Sprintf("%s — %d file(s), %d command(s), %d variable(s).",
				detail.Name, len(detail.Files), len(detail.Commands), len(detail.Variables)), statusOK)
		})
	}()
}

// showDetail fills the four sections, or says how to fill them.
func (v *templatesView) showDetail(detail *templates.TemplateDetail) {
	v.detail = detail

	if v.summary == nil {
		return
	}

	if detail == nil {
		v.summary.SetText("Select a template to see what is inside it.")
		v.files.SetText("")
		v.variables.SetText("")
		v.commands.SetText("")
		relayout(v.built)

		return
	}

	visibility := "private"
	if detail.IsPublic {
		visibility = "shared"
	}

	summary := fmt.Sprintf("%s — %s, version %d — %s — by %s", detail.Name, detail.Kind, detail.Version, visibility, detail.OwnerEmail)
	if strings.TrimSpace(detail.Description) != "" {
		summary += "\n\n" + detail.Description
	}

	v.summary.SetText(summary)
	v.files.SetText(lines(detail.Files, fileLine))
	v.variables.SetText(lines(detail.Variables, variableLine))
	v.commands.SetText(lines(detail.Commands, commandLine))

	// A description is prose: it can wrap to more lines than the pane was
	// measured for.
	relayout(v.built)
}

// lines renders a slice one per line, or a dash when the slice is empty, so a
// section that has nothing still says so.
func lines[T any](items []T, line func(T) string) string {
	if len(items) == 0 {
		return "—"
	}

	out := make([]string, 0, len(items))
	for _, item := range items {
		out = append(out, "• "+line(item))
	}

	return strings.Join(out, "\n")
}

func fileLine(file templates.TemplateFileData) string {
	line := fmt.Sprintf("%s — %s, %s", file.Path, file.Kind, byteSize(file.Size))
	if file.IsExecutable {
		line += ", executable"
	}

	return line
}

func variableLine(variable templates.TemplateVariableData) string {
	line := fmt.Sprintf("{{%s}} — %s", variable.Key, promptOf(variable))

	if variable.DefaultValue != "" {
		line += ", default " + variable.DefaultValue
	}
	if variable.Required {
		line += ", required"
	}

	return line
}

func promptOf(variable templates.TemplateVariableData) string {
	if strings.TrimSpace(variable.Prompt) == "" {
		return "no prompt"
	}

	return variable.Prompt
}

func commandLine(command templates.TemplateCommandData) string {
	return fmt.Sprintf("%s — %s, %s", command.TargetPath, command.Name, byteSize(command.Size))
}

// byteSize is a size a person reads: bytes under a kilobyte, then KiB and MiB.
func byteSize(size int64) string {
	switch {
	case size < 1024:
		return fmt.Sprintf("%d B", size)
	case size < 1024*1024:
		return fmt.Sprintf("%.1f KiB", float64(size)/1024)
	default:
		return fmt.Sprintf("%.1f MiB", float64(size)/(1024*1024))
	}
}

// updateActions keeps Save as bundle off until there is a template to save.
func (v *templatesView) updateActions() {
	if v.export == nil {
		return
	}

	for _, button := range []*widget.Button{v.export, v.apply} {
		if button == nil {
			continue
		}

		if v.detail != nil {
			button.Enable()
		} else {
			button.Disable()
		}
	}
}

// exportSelected writes the selected template as the offline bundle the CLI
// reads, so the window and `template export` produce the same file.
func (v *templatesView) exportSelected() {
	detail := v.detail
	if detail == nil {
		v.setStatus("Select a template first — Save as bundle writes the selected one.", statusWarn)
		return
	}

	save := dialog.NewFileSave(func(writer fyne.URIWriteCloser, err error) {
		if err != nil {
			v.setStatus("The destination could not be chosen: "+err.Error(), statusError)
			return
		}
		if writer == nil {
			return
		}

		path := writer.URI().Path()

		if err := exportBundle(writer, detail); err != nil {
			writer.Close()
			v.setStatus("The bundle could not be written: "+err.Error(), statusError)
			return
		}

		if err := writer.Close(); err != nil {
			v.setStatus("The bundle was written but could not be closed: "+err.Error(), statusError)
			return
		}

		v.setStatus(fmt.Sprintf("Saved %s. Apply it with `eznit template apply --from-file %s`.", path, path), statusOK)
	}, v.ui.window)

	save.SetFileName(bundleFilename(detail))
	save.SetFilter(storage.NewExtensionFileFilter([]string{bundleExtension}))
	save.Show()
}

// exportBundle turns a template into the bundle format and writes it. It is
// the same pair `template export` uses, so the two cannot drift.
func exportBundle(w io.Writer, detail *templates.TemplateDetail) error {
	bundle, err := templates.NewBundle(detail)
	if err != nil {
		return err
	}

	return templates.WriteBundle(w, bundle)
}

// bundleFilename suggests a name: the slug, or the name with its spaces turned
// into dashes, and the extension the CLI reads.
func bundleFilename(detail *templates.TemplateDetail) string {
	base := strings.ToLower(strings.TrimSpace(detail.Slug))
	if base == "" {
		base = strings.ToLower(strings.TrimSpace(detail.Name))
	}

	var kept strings.Builder

	for _, r := range base {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-', r == '_':
			kept.WriteRune(r)
		case r == ' ' || r == '.':
			kept.WriteRune('-')
		}
	}

	name := kept.String()
	for strings.Contains(name, "--") {
		name = strings.ReplaceAll(name, "--", "-")
	}

	name = strings.Trim(name, "-")
	if name == "" {
		name = "template"
	}

	return name + bundleExtension
}

// setStatus writes the message under the split and colours the dot with it.
func (v *templatesView) setStatus(text string, kind statusKind) {
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

// applySelected writes the selected template into a folder the person picks:
// check what is there, ask the questions the template needs, then write.
//
// The order is the CLI's: the files are checked before anything is asked,
// because a person should not answer what a template wants to know for a write
// that cannot happen.
func (v *templatesView) applySelected() {
	detail := v.detail
	if detail == nil {
		v.setStatus("Select a template first — Apply writes the selected one.", statusWarn)
		return
	}

	files, err := templateapply.Files(detail)
	if err != nil {
		v.setStatus("This template cannot be written: "+err.Error(), statusError)
		return
	}

	dialog.ShowFolderOpen(func(uri fyne.ListableURI, err error) {
		if err != nil {
			v.setStatus("The folder could not be chosen: "+err.Error(), statusError)
			return
		}
		if uri == nil {
			return
		}

		target := uri.Path()

		existing, err := templateapply.Existing(target, files)
		if err != nil {
			v.setStatus("This template cannot be written into that folder: "+err.Error(), statusError)
			return
		}

		if len(existing) == 0 {
			v.askForValues(detail, files, target)
			return
		}

		list := selectableWrap()
		list.SetText("These files are already in " + target + " and would be overwritten:\n\n" + strings.Join(existing, "\n"))

		dialog.ShowCustomConfirm(
			fmt.Sprintf("Overwrite %d file(s)?", len(existing)),
			"Overwrite", "Cancel",
			list,
			func(confirmed bool) {
				if confirmed {
					v.askForValues(detail, files, target)
				}
			}, v.ui.window)
	}, v.ui.window)
}

// askForValues collects the answers a template needs: its declared variables
// and the placeholders its files use without declaring them. Nothing is written
// while a required answer is empty.
func (v *templatesView) askForValues(detail *templates.TemplateDetail, files []templateapply.File, target string) {
	undeclared := templateapply.Undeclared(files, detail.Variables)

	rows := make([]formRow, 0, len(detail.Variables)+len(undeclared))
	entries := make(map[string]*widget.Entry, len(detail.Variables)+len(undeclared))

	for _, variable := range detail.Variables {
		entry := widget.NewEntry()
		entry.SetText(variable.DefaultValue)

		caption := promptOf(variable) + " (" + variable.Key + ")"
		if variable.Required {
			caption += " *"
		}

		rows = append(rows, formRow{caption: caption, input: entry})
		entries[variable.Key] = entry
	}

	for _, key := range undeclared {
		entry := widget.NewEntry()

		rows = append(rows, formRow{caption: key + " (not declared by the template)", input: entry})
		entries[key] = entry
	}

	status := selectableWrap()
	status.SetText("The files are written into " + target + ".")

	form := formRows(rows...)
	body := container.NewVBox(form, status)

	// A template may ask many questions: the dialog keeps its height and the
	// form scrolls inside it.
	if len(rows) > 4 {
		scroll := container.NewVScroll(form)
		scroll.SetMinSize(fyne.NewSize(420, 220))
		body = container.NewVBox(scroll, status)
	}

	var popup *dialog.CustomDialog

	write := func() {
		values := make(map[string]string, len(entries))
		for key, entry := range entries {
			values[key] = strings.TrimSpace(entry.Text)
		}

		if key := missingAnswer(detail.Variables, values); key != "" {
			status.SetText("The template asks for {{" + key + "}} and it has no answer yet.")
			return
		}

		if err := applyTemplate(target, files, values, true); err != nil {
			status.SetText("The template was not written: " + err.Error())
			return
		}

		if popup != nil {
			popup.Hide()
		}

		v.showWritten(target, files)
	}

	content := container.NewVBox(
		widget.NewLabel("Answers for "+detail.Name+":"),
		body,
		container.NewHBox(widget.NewButton("Write", write)),
	)

	popup = dialog.NewCustom("Apply "+detail.Name, "Cancel", content, v.ui.window)
	popup.Show()
}

// missingAnswer names the first required variable that has no answer, or "".
func missingAnswer(variables []templates.TemplateVariableData, values map[string]string) string {
	for _, variable := range variables {
		if variable.Required && strings.TrimSpace(values[variable.Key]) == "" {
			return variable.Key
		}
	}

	return ""
}

// applyTemplate fills in the answers and writes the files: the whole write
// path, apart from the dialogs that collect the answers.
func applyTemplate(target string, files []templateapply.File, values map[string]string, overwrite bool) error {
	for i := range files {
		if files[i].Kind == templates.FileKindBinary {
			continue
		}

		files[i].Content = templateapply.Substitute(files[i].Content, values)
	}

	return templateapply.Write(target, files, overwrite)
}

// showWritten names every path that was written, selectable, with a copy.
func (v *templatesView) showWritten(target string, files []templateapply.File) {
	paths := make([]string, 0, len(files))

	for _, file := range files {
		paths = append(paths, filepath.Join(target, filepath.FromSlash(file.Path)))
	}

	list := selectableWrap()
	list.SetText(strings.Join(paths, "\n"))

	copyPaths := widget.NewButton("Copy paths", func() {
		if !v.ui.copyToClipboard(strings.Join(paths, "\n")) {
			list.SetText(list.Text + "\n\n(This window has no clipboard available.)")
		}
	})

	body := container.NewBorder(nil,
		container.NewHBox(copyPaths), nil, nil,
		container.NewVScroll(list))

	dialog.ShowCustom(fmt.Sprintf("Wrote %d file(s)", len(paths)), "Close", body, v.ui.window)

	v.setStatus(fmt.Sprintf("Wrote %d file(s) into %s.", len(paths), target), statusOK)
}
