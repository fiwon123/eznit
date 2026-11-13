//go:build fyne

package fyneui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
)

// statusDotSize is the diameter of the dot that colours a message. It is a
// shape, not text, so it does not scale with the font.
const statusDotSize float32 = 10

// dotCell puts the status dot in the middle of the height it is given.
//
// A bare canvas.Circle has no minimum of its own, so it is placed in a grid-wrap
// cell to get its ten pixels — but an HBox stretches every child to the row's
// height and a grid wrap pins its child to the top, which is why the dot used
// to float above the text's baseline. A centre container between the two fixes
// the vertical position without adding a spacer of its own.
func dotCell(dot *canvas.Circle) fyne.CanvasObject {
	return container.NewCenter(container.NewGridWrap(fyne.NewSize(statusDotSize, statusDotSize), dot))
}

// dotWithLabel keeps the dot in the same block as the label it belongs to. An
// HBox stretches both children to the block's height, which is the text's, so
// the dot centres on the text instead of on a taller button in the same row.
func dotWithLabel(dot *canvas.Circle, label fyne.CanvasObject) fyne.CanvasObject {
	return container.NewCenter(container.NewHBox(dotCell(dot), label))
}

// statusBar is the message line at the bottom of a tab: the dot that colours it
// and the message itself, so the colour is never on a different line from what
// it describes. The message is the middle of a Border, so it gets the width that
// is left rather than its own minimum.
func statusBar(dot *canvas.Circle, message fyne.CanvasObject) fyne.CanvasObject {
	return container.NewPadded(container.NewBorder(nil, nil, dotCell(dot), nil, message))
}
