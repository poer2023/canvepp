package main

import (
	"github.com/egoist/mygo/transfer"
	"github.com/egoist/mygo/ui"
)

// A typed, same-window transfer keeps the original material and the read
// operation together. The receiver chooses the destination when released.
type materialDrag struct{ place func(*Editor, Point) }

func (a *desktop) canvasMaterialDrop(c *ui.Context, box ui.Element, e *Editor) {
	if value, ok := ui.Drop[materialDrag](box); ok {
		x, y, _ := box.PointerPosition()
		value.place(e, e.World(Point{float64(x), float64(y)}))
	}
	if _, ok := ui.DragOver[materialDrag](box); ok {
		box.Border(1, c.Theme().TextMuted)
	}
	m := a.materials
	if m == nil || !a.dockVisible() || a.dock.active != dockPinterest || m.showNativePins || m.web == nil {
		return
	}
	opts := transfer.DropOptions{Formats: []transfer.Format{transfer.Text, transfer.HTML, transfer.URIList}, Operations: transfer.Copy}
	if drop, ok := ui.DropData(box, opts); ok {
		x, y, _ := box.PointerPosition()
		at := e.World(Point{float64(x), float64(y)})
		if m.web.view != nil {
			if raw, err := drop.Data.Read(transfer.Text); err == nil {
				if pin, valid := pinFromNavigation(m.web.view.Page().URL(), string(raw), true); valid {
					a.importPinInto(e, at, pin.Image, pin.Pin, pin.Name)
					return
				}
			}
		}
		a.importBrowserPin(e, at, browserPin(drop.Data))
	}
}
