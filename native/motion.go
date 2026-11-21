package main

import (
	"time"

	"github.com/egoist/mygo/ui"
)

const (
	foldDuration      = 210 * time.Millisecond
	paneDuration      = 170 * time.Millisecond
	selectionDuration = 145 * time.Millisecond
)

// Feedback changes color without moving the control or simulating depth.
func feedback(c *ui.Context, b ui.Element, base, text ui.Color) ui.Element {
	t := c.Theme()
	hover, pressed := t.SurfaceHover, t.SurfacePressed
	if base == t.Accent {
		hover, pressed = t.AccentHover, t.AccentPressed
	}
	if !b.IsDisabled() {
		b.Cursor(ui.CursorPointer)
		if b.Pressed() {
			base = pressed
		} else if b.Hovered() {
			base = hover
		}
	}
	return b.Background(base).TextColor(text)
}

func selectionColor(c *ui.Context, e ui.Element, color ui.Color) ui.Color {
	if c.Preferences().ReduceMotion {
		return color
	}
	return e.AnimateColor("selection", color, ui.Animation{Duration: selectionDuration, Ease: ui.EaseOut})
}

func paneMotion(c *ui.Context, e ui.Element, x, y float32) ui.Element {
	if !c.Preferences().ReduceMotion {
		e.Transition(ui.ElementTransition{
			Duration: paneDuration, Ease: ui.EaseOut, Colors: true,
			Enter: &ui.Motion{X: x, Y: y, Opacity: .25},
			Exit:  &ui.Motion{X: -x, Y: -y},
		})
	}
	return e
}

type resultsMotionKey struct {
	query, group       string
	order, mode, count int
}

type resultsTransition struct {
	key   resultsMotionKey
	since time.Time
}

// Keep the scroll owner and focused search input mounted while the results
// update. Rapid edits share a transition so typing never restarts the fade.
func resultsMotion(c *ui.Context, e ui.Element, key resultsMotionKey) ui.Element {
	state := ui.Local(e, "results-motion", func() resultsTransition { return resultsTransition{key: key} })
	now := c.Now()
	if c.Preferences().ReduceMotion {
		state.key, state.since = key, time.Time{}
		return e
	}
	if state.key != key {
		state.key = key
		if state.since.IsZero() || now.Sub(state.since) >= paneDuration {
			state.since = now
		}
	}
	if !state.since.IsZero() {
		elapsed := now.Sub(state.since)
		if elapsed < paneDuration {
			progress := ui.EaseOut(float32(elapsed) / float32(paneDuration))
			e.Opacity(.67 + .33*progress)
			c.AnimationFrame()
		}
	}
	return e
}
