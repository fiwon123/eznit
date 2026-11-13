//go:build fyne

package fyneui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"
)

// cell is one character position in the log grid: a row, and a column within
// it. A column past the end of a row means "the end of that line", which is
// where a drag that leaves the text lands.
type cell struct {
	row int
	col int
}

// selection is a drag's range. start is never after end, so a drag that went
// bottom-up reads the same as one that went top-down, and both cells are
// included in what it covers.
type selection struct {
	start cell
	end   cell
}

// rowRange is the part of one row a selection covers. to is exclusive, and -1
// means "to the end of the line" — what a row in the middle of a multi-line
// selection covers.
type rowRange struct {
	from int
	to   int
	ok   bool
}

// span is what the selection covers on row, and whether it covers anything.
func (s *selection) span(row int) rowRange {
	if s == nil || row < s.start.row || row > s.end.row {
		return rowRange{}
	}

	switch {
	case s.start.row == s.end.row:
		return rowRange{from: s.start.col, to: s.end.col + 1, ok: true}
	case row == s.start.row:
		return rowRange{from: s.start.col, to: -1, ok: true}
	case row == s.end.row:
		return rowRange{from: 0, to: s.end.col + 1, ok: true}
	default:
		return rowRange{from: 0, to: -1, ok: true}
	}
}

// shifted moves a selection by count rows: the log buffer drops its oldest
// lines, and a selection has to keep pointing at the same text when they go.
func (s *selection) shifted(count int) *selection {
	if s == nil {
		return nil
	}

	return &selection{
		start: cell{row: s.start.row + count, col: s.start.col},
		end:   cell{row: s.end.row + count, col: s.end.col},
	}
}

// clamped keeps a selection inside a grid that begins at row 0 and holds rows
// lines. A selection whose lines have all left the buffer is gone: there is no
// text left to copy, and pretending otherwise would copy the wrong lines.
func (s *selection) clamped(rows int) *selection {
	if s == nil || rows <= 0 || s.start.row >= rows || s.end.row < 0 {
		return nil
	}

	fixed := &selection{start: s.start, end: s.end}

	if fixed.start.row < 0 {
		fixed.start = cell{row: 0, col: 0}
	}
	if fixed.end.row >= rows {
		fixed.end = cell{row: rows - 1, col: 0}
	}

	return fixed
}

// rows is the first and last row a selection touches, for a caller that wants
// to paint the whole range.
func (s *selection) rows() (first, last int) {
	if s == nil {
		return 0, -1
	}

	return s.start.row, s.end.row
}

// unionRows is the span of rows the given selections touch together, so a
// repaint can walk that span and skip the rows whose coverage did not change.
// last < first means nothing was touched at all.
func unionRows(selections ...*selection) (first, last int) {
	first, last = 1, 0

	for _, sel := range selections {
		if sel == nil {
			continue
		}

		start, end := sel.rows()
		if start < first {
			first = start
		}
		if end > last {
			last = end
		}
	}

	return first, last
}

// logGrid is the log's text grid with the mouse added.
//
// Fyne 2.8 has no selection in a widget.TextGrid, so this wrapper turns a
// press and a drag into the cells the drag covers and hands them to the view.
// It draws nothing of its own: the grid inside it renders as before, and the
// selection is painted by restyling those cells, which keeps the per-level
// colours the log already had. The grid implements none of the mouse
// interfaces, so the events arrive here, and the wheel still finds the scroll
// around it.
type logGrid struct {
	widget.BaseWidget

	grid *widget.TextGrid

	// anchor is where the press landed, and pressed says whether a drag that
	// began on the grid is still in progress: a drag that started somewhere
	// else must not select.
	anchor  cell
	pressed bool

	// onSelect is told the range the drag covers, or nil when a press clears
	// the selection.
	onSelect func(*selection)
}

func newLogGrid(grid *widget.TextGrid) *logGrid {
	log := &logGrid{grid: grid}
	log.ExtendBaseWidget(log)

	return log
}

// MouseDown begins a selection, and a press on its own clears one: a drag is
// what makes a range, which is how a terminal behaves.
func (g *logGrid) MouseDown(ev *desktop.MouseEvent) {
	if ev.Button != desktop.MouseButtonPrimary {
		return
	}

	g.anchor = g.cellAt(ev.Position)
	g.pressed = true
	g.selectRange(nil)
}

// Dragged extends the selection to the cell under the pointer.
func (g *logGrid) Dragged(ev *fyne.DragEvent) {
	if !g.pressed {
		return
	}

	start, end := g.anchor, g.cellAt(ev.Position)
	if end.row < start.row || (end.row == start.row && end.col < start.col) {
		start, end = end, start
	}

	g.selectRange(&selection{start: start, end: end})
}

// MouseUp ends the drag. The selection stays, so it can be copied.
func (g *logGrid) MouseUp(*desktop.MouseEvent) {
	g.pressed = false
}

// DragEnd ends the drag when the pointer was released away from the grid.
func (g *logGrid) DragEnd() {
	g.pressed = false
}

// cellAt maps a pointer position inside this widget to a cell. The grid is at
// the widget's origin and has its size, so the position needs no translation.
func (g *logGrid) cellAt(pos fyne.Position) cell {
	row, col := g.grid.CursorLocationForPosition(pos)

	return cell{row: row, col: col}
}

func (g *logGrid) selectRange(sel *selection) {
	if g.onSelect != nil {
		g.onSelect(sel)
	}
}

func (g *logGrid) CreateRenderer() fyne.WidgetRenderer {
	g.ExtendBaseWidget(g)

	return &logGridRenderer{grid: g}
}

// logGridRenderer renders the grid and nothing else: the selection is a style
// on the grid's own cells, so there is no overlay to keep in step with them.
type logGridRenderer struct {
	grid *logGrid
}

func (r *logGridRenderer) Layout(size fyne.Size) {
	r.grid.grid.Resize(size)
}

func (r *logGridRenderer) MinSize() fyne.Size {
	return r.grid.grid.MinSize()
}

func (r *logGridRenderer) Refresh() {
	r.grid.grid.Refresh()
}

func (r *logGridRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.grid.grid}
}

func (r *logGridRenderer) Destroy() {}
