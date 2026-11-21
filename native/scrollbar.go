package main

import (
	"time"

	"github.com/egoist/mygo/ui"
)

const thumbWidth float32 = 2
const thumbEdge float32 = 10
const scrollIdle = 650 * time.Millisecond

type thumbActivity struct {
	offset ui.ScrollState
	x, y   float32
	until  time.Time
}

// Use MyGo's native thumb dragging and wheel routing. All scroll surfaces
// share our dimensions, visibility window and edge hit area.
func thinScroll(c *ui.Context, e ui.Element, external *ui.ScrollState) ui.Element {
	state := ui.Local(e, "canvepp-thumb", func() thumbActivity { return thumbActivity{} })
	position := external
	if position == nil {
		position = &state.offset
	}
	e.TrackScroll(position)
	now := c.Now()
	if state.x != position.X || state.y != position.Y {
		state.x, state.y = position.X, position.Y
		state.until = now.Add(scrollIdle)
	}
	x, y, over := e.PointerPosition()
	bounds := e.Bounds()
	edge := over && ((position.MaxY > 0 && x >= bounds.W-thumbEdge) || (position.MaxX > 0 && y >= bounds.H-thumbEdge))
	recent := state.until.After(now)
	if recent {
		c.After(state.until.Sub(now))
	}
	return e.Scrollbars(edge || recent)
}
