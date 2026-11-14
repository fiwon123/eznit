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
	"fyne.io/fyne/v2/layout"
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
	mine    *checkRow
	refresh *widget.Button
	export  *widget.Button
	apply   *widget.Button

	// bar is the API address: the same component the Files tab has, without a
	// session to end.
	bar *addressBar
	dot *canvas.Circle

	// built is the tab's outer container, so a status message that changes
	// height can lay the tab out again.
	built fyne.CanvasObject

	summary   *prose
	files     *prose
	variables *prose
	commands  *prose
	status    *prose

	// parts is the selected template in three tabs; each tab's label carries
	// its count, so the detail is one pane instead of a long scroll.
	parts        *container.AppTabs
	filesTab     *container.TabItem
	variablesTab *container.TabItem
	commandsTab  *container.TabItem
}

func newTemplatesView(u *ui) *templatesView {
	view := &templatesView{
		ui:       u,
		selected: -1,
		status:   newProse(""),
	}

	view.table = widget.NewTable(
		func() (int, int) { return len(view.items) + 1, 4 },
		func() fyne.CanvasObject { return widget.NewLabel("") },
		func(id widget.TableCellID, cell fyne.CanvasObject) {
			label, ok := cell.(*widget.Label)
			if !ok {
				return
			}

			if id.Row == 0 {
				label.TextStyle = fyne.TextStyle{Bold: true}
				label.SetText([]string{"Name", "Kind", "Version", "Files"}[id.Col])
				alignTemplateColumn(label, id.Col)

				return
			}

			label.TextStyle = fyne.TextStyle{}
			alignTemplateColumn(label, id.Col)

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
				label.SetText(fmt.Sprintf("%d", item.FileCount))
			}
		},
	)

	// Four columns fit the smallest window without a squeeze; the owner and the
	// visibility are in the summary of the selected template, where a person
	// reads them once rather than on every row.
	view.table.SetColumnWidth(0, 320)
	view.table.SetColumnWidth(1, 120)
	view.table.SetColumnWidth(2, 80)
	view.table.SetColumnWidth(3, 70)

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
	view.apply.Importance = widget.HighImportance

	// The handler goes on after the first selection: SetSelected calls it, and
	// a fetch during construction would race the tab that owns the status line.
	view.kind = widget.NewSelect(append([]string{allKinds}, templates.Kinds...), nil)
	view.kind.SetSelected(allKinds)
	view.kind.OnChanged = func(string) { view.reload() }

	view.mine = newCheckRow("Only my templates", func(bool) { view.reload() })

	view.updateActions()

	return view
}

// content is the tab: the server bar, the filters, the catalogue, the selected
// template and the message line.
func (v *templatesView) content() fyne.CanvasObject {
	// The bar is the same one the Files tab has: the address is one choice for
	// both tabs, so it is editable here too. A connect from here reloads the
	// catalogue from the new server.
	v.bar = newAddressBar(v.ui, v.setStatus, false)
	v.bar.afterConnect = v.reload

	// The dot colours the message at the bottom of the tab, so it is made
	// before the bar that holds it.
	v.dot = canvas.NewCircle(statusColour(v.ui.app.Settings().ThemeVariant(), statusIdle))

	filters := container.NewHBox(muted("Kind"), v.kind, v.mine.row)

	v.buildDetail()
	split := container.NewVSplit(v.table, v.detailPane())
	// The catalogue gets the larger share: the detail is one pane with tabs now,
	// so it needs less room than four stacked sections did.
	split.Offset = 0.55

	top := container.NewVBox(
		v.bar.content(),
		widget.NewSeparator(),
		container.NewPadded(filters),
	)

	v.built = container.NewBorder(top, statusBar(v.dot, v.status), nil, nil, split)

	return v.built
}

// buildDetail creates the summary, the three bodies and the sub-tabs that hold
// them. With nothing selected the tabs are hidden and the summary says what to
// do.
func (v *templatesView) buildDetail() {
	v.summary = selectableWrap()
	v.files = selectableWrap()
	v.variables = selectableWrap()
	v.commands = selectableWrap()

	v.filesTab = container.NewTabItem("Files (0)", part(v.files))
	v.variablesTab = container.NewTabItem("Variables (0)", part(v.variables))
	v.commandsTab = container.NewTabItem("Commands (0)", part(v.commands))

	v.parts = container.NewAppTabs(v.filesTab, v.variablesTab, v.commandsTab)
	v.parts.Hide()

	v.showDetail(nil)
}

// part wraps one detail body in its own scroll: a template with forty files
// scrolls inside Files without moving Variables.
func part(body *prose) fyne.CanvasObject {
	return container.NewVScroll(container.NewPadded(body))
}

// detailPane is the selected template: its summary and the two actions that
// work on it, above the three tabs.
func (v *templatesView) detailPane() fyne.CanvasObject {
	summary := container.NewPadded(v.summary)
	actions := container.NewPadded(container.NewBorder(nil, nil, nil,
		container.NewHBox(v.export, v.apply), nil))

	return container.NewBorder(summary, actions, nil, nil, v.parts)
}

// alignTemplateColumn right-aligns the two numeric columns — version and file
// count — so their digits line up. The rest are words and stay leading.
func alignTemplateColumn(label *widget.Label, column int) {
	label.Alignment = fyne.TextAlignLeading

	if column == 2 || column == 3 {
		label.Alignment = fyne.TextAlignTrailing
	}
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
					"The catalogue could not be loaded from %s (%s). Use Refresh to try again, or change the address on the Files tab.",
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

// showDetail fills the summary and the three tabs, or says how to fill them.
func (v *templatesView) showDetail(detail *templates.TemplateDetail) {
	v.detail = detail

	if v.summary == nil {
		return
	}

	if detail == nil {
		v.summary.SetText("Select a template to see what is inside it: a tree of files with their paths, " +
			"the variables it asks for and the commands it pulls in. Applying it writes those files into a folder you pick.")
		v.files.SetText("")
		v.variables.SetText("")
		v.commands.SetText("")

		if v.parts != nil {
			v.parts.Hide()
		}

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
	v.files.SetText(linesOr(detail.Files, fileLine, "No files."))
	v.variables.SetText(linesOr(detail.Variables, variableLine, "None — this template asks nothing."))
	v.commands.SetText(linesOr(detail.Commands, commandLine, "No commands."))

	// The counts are in the tab labels, so a person can see that a template has
	// variables without opening the tab.
	if v.parts != nil {
		v.filesTab.Text = fmt.Sprintf("Files (%d)", len(detail.Files))
		v.variablesTab.Text = fmt.Sprintf("Variables (%d)", len(detail.Variables))
		v.commandsTab.Text = fmt.Sprintf("Commands (%d)", len(detail.Commands))
		v.parts.Show()
		v.parts.Refresh()
	}

	// A description is prose: it can wrap to more lines than the pane was
	// measured for.
	relayout(v.built)
}

// linesOr renders a slice one per line, or the empty state when it has
// nothing, so a section that is empty says what that means instead of holding a
// dash nobody can read.
func linesOr[T any](items []T, line func(T) string, empty string) string {
	if len(items) == 0 {
		return empty
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

// variableLine is one variable in the detail pane: the key, the question the
// template asks, the default and whether an answer is required. The braces
// belong to the files a template writes; a person reads a key here.
func variableLine(variable templates.TemplateVariableData) string {
	parts := []string{variable.Key}

	if prompt := strings.TrimSpace(variable.Prompt); prompt != "" {
		parts = append(parts, prompt)
	}
	if strings.TrimSpace(variable.DefaultValue) != "" {
		parts = append(parts, "default "+variable.DefaultValue)
	}
	if variable.Required {
		parts = append(parts, "required")
	}

	return strings.Join(parts, " — ")
}

// promptOf is the question a variable asks, or its key when it has no prompt.
func promptOf(variable templates.TemplateVariableData) string {
	if strings.TrimSpace(variable.Prompt) == "" {
		return variable.Key
	}

	return variable.Prompt
}

// variableCaption is the line above one answer box: the prompt, the key it
// fills in when the prompt does not already say it, and the marker for a
// required answer. No braces: the key is a name here.
func variableCaption(variable templates.TemplateVariableData) string {
	caption := variable.Key
	if prompt := strings.TrimSpace(variable.Prompt); prompt != "" && prompt != variable.Key {
		caption = prompt + " (" + variable.Key + ")"
	}

	if variable.Required {
		caption += " *"
	}

	return caption
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
	required := false

	for _, variable := range detail.Variables {
		entry := widget.NewEntry()
		entry.SetText(variable.DefaultValue)

		rows = append(rows, formRow{caption: variableCaption(variable), input: entry})
		entries[variable.Key] = entry
		required = required || variable.Required
	}

	for _, key := range undeclared {
		entry := widget.NewEntry()

		rows = append(rows, formRow{caption: key + " — not declared by the template", input: entry})
		entries[key] = entry
	}

	// The folder the files land in belongs above the questions: it is the thing
	// a person is agreeing to. The asterisk is explained once.
	hint := "The files are written into " + target + "."
	if required {
		hint += " Required fields are marked *."
	}

	input := formRows(rows...)

	// A template may ask many questions: the dialog keeps its height and the
	// form scrolls inside it.
	if len(rows) > 4 {
		scroll := container.NewVScroll(input)
		scroll.SetMinSize(fyne.NewSize(420, 220))
		input = scroll
	}

	title := widget.NewLabelWithStyle("Values for "+detail.Name,
		fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	status := selectableWrap()

	var popup *dialog.CustomDialog

	var content fyne.CanvasObject

	// showStatus resizes the dialog as well: a dialog takes its size from its
	// content once, so an error that arrives afterwards would be cut off.
	showStatus := func(message string) {
		status.SetText(message)
		relayout(content)
		popup.Resize(popup.MinSize())
	}

	write := widget.NewButton("Write", func() {
		values := make(map[string]string, len(entries))
		for key, entry := range entries {
			values[key] = strings.TrimSpace(entry.Text)
		}

		if key := missingAnswer(detail.Variables, values); key != "" {
			showStatus("The template asks for " + key + " and it has no answer yet.")
			return
		}

		if err := applyTemplate(target, files, values, true); err != nil {
			showStatus("The template was not written: " + err.Error())
			return
		}

		if popup != nil {
			popup.Hide()
		}

		v.showWritten(target, files)
	})
	write.Importance = widget.HighImportance

	actions := container.NewHBox(layout.NewSpacer(), write)

	content = container.NewVBox(title, newProse(hint), input, status, actions)

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
