//go:build fyne

package fyneui

import (
	"fmt"
	"io"
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

	address *widget.Label
	source  *widget.Label
	dot     *canvas.Circle

	summary   *widget.Label
	files     *widget.Label
	variables *widget.Label
	commands  *widget.Label
	status    *widget.Label
}

func newTemplatesView(u *ui) *templatesView {
	view := &templatesView{
		ui:       u,
		selected: -1,
		status:   widget.NewLabel(""),
	}
	view.status.Selectable = true
	view.status.Wrapping = fyne.TextWrapWord

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
	v.address = widget.NewLabel(v.ui.address.url)
	v.address.Wrapping = fyne.TextWrapWord
	v.address.Selectable = true

	v.source = widget.NewLabel("(" + v.ui.address.source + ")")
	v.source.Wrapping = fyne.TextWrapWord
	v.source.Selectable = true

	v.dot = canvas.NewCircle(statusColour(v.ui.app.Settings().ThemeVariant(), statusIdle))
	dotBox := container.NewGridWrap(fyne.NewSize(10, 10), v.dot)

	header := container.NewBorder(nil, nil,
		container.NewHBox(dotBox, widget.NewLabel("Server")),
		container.NewHBox(v.refresh, v.export),
		container.NewHBox(v.address, v.source),
	)

	filters := container.NewHBox(v.kind, v.mine)

	v.buildDetail()
	split := container.NewVSplit(v.table, container.NewVScroll(v.detailPane()))
	split.Offset = 0.5

	top := container.NewVBox(container.NewPadded(header), widget.NewSeparator(), container.NewPadded(filters))

	return container.NewBorder(top, v.status, nil, nil, split)
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
func section(caption string, body *widget.Label) fyne.CanvasObject {
	title := widget.NewLabel(caption)
	title.TextStyle = fyne.TextStyle{Bold: true}
	title.Selectable = true

	return container.NewVBox(title, body)
}

// selectableWrap is the one shape every value on this tab takes: wrapped, so a
// long path cannot widen the window, and selectable, so it can be copied.
func selectableWrap() *widget.Label {
	label := widget.NewLabel("")
	label.Wrapping = fyne.TextWrapWord
	label.Selectable = true

	return label
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

	if v.detail != nil {
		v.export.Enable()
	} else {
		v.export.Disable()
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
}
