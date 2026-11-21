package main

import (
	"time"

	"github.com/egoist/mygo/ui"
)

const hintDelay = 80 * time.Millisecond

// Cross the narrow gutter between controls without paying the initial delay twice.
const hintTravelTime = 120 * time.Millisecond

type hintTarget struct {
	anchor      ui.Element
	text        string
	atPointer   bool
	interactive bool
	beside      bool
}

// Keep timing and dismissal per window. Anchors only live for the current frame.
type windowHints struct {
	hover, focus                    hintTarget
	lastHover, lastFocus, dismissed uint64
	hoverAt, focusAt, leftAt        time.Time
	visible, inIconRail             bool
}

func hints(c *ui.Context) *windowHints {
	return ui.Local(c.Root(), "workspace-hints", func() windowHints { return windowHints{} })
}

func beginHints(c *ui.Context) func() {
	h := hints(c)
	h.hover, h.focus = hintTarget{}, hintTarget{}
	return func() { h.show(c) }
}

func registerHint(c *ui.Context, anchor ui.Element, text string, atPointer, interactive bool) ui.Element {
	anchor.Description(text)
	if text == "" || anchor.IsDisabled() {
		return anchor
	}
	hover, focus := anchor.Hovered(), anchor.FocusVisible()
	if hover || focus {
		h := hints(c)
		target := hintTarget{anchor: anchor, text: text, atPointer: atPointer, interactive: interactive, beside: h.inIconRail}
		if hover {
			h.hover = target
		}
		if focus && !atPointer {
			h.focus = target
		}
	}
	return anchor
}

func flatTip(c *ui.Context, anchor ui.Element, text string) ui.Element {
	return registerHint(c, anchor, text, false, false)
}

func commandHint(c *ui.Context, anchor ui.Element, text string) ui.Element {
	return registerHint(c, anchor.Border(0, ui.Transparent).Cursor(ui.CursorPointer), text, false, true)
}

func editHint(c *ui.Context, field ui.Element, text string) ui.Element {
	field.Border(0, ui.Transparent)
	if field.Hovered() && !field.Focused() {
		field.DrawOver(func(p *ui.Painter, r ui.Rect) { p.Fill(r, p.Theme().Text.Alpha(.035), 4) })
	}
	return registerHint(c, field.Cursor(ui.CursorPointer), text, false, false)
}

func imageCommand(c *ui.Context, button ui.Element, text string) ui.Element {
	if button.Hovered() {
		button.DrawOver(func(p *ui.Painter, r ui.Rect) { p.Stroke(r, p.Theme().TextMuted, 4, 1) })
	}
	return commandHint(c, button, text)
}

func dragHint(c *ui.Context, handle ui.Element, active bool, text string) ui.Element {
	if active {
		return handle.Cursor(ui.CursorGrabbing)
	}
	return flatTip(c, handle.Cursor(ui.CursorGrab), text)
}

// Call after styling so disabled controls override the action's cursor.
func disable(e ui.Element, disabled bool) ui.Element {
	if disabled {
		e.Cursor(ui.CursorDefault)
	}
	return e.Disabled(disabled)
}

func hintAtPointer(c *ui.Context, anchor ui.Element, text string) {
	registerHint(c, anchor, text, true, false)
}

func (h *windowHints) show(c *ui.Context) {
	now := c.Now()
	hover, focus := h.hover.anchor.ID(), h.focus.anchor.ID()
	if h.lastHover != hover {
		h.lastHover, h.hoverAt = hover, now
	}
	if h.lastFocus != focus {
		h.lastFocus, h.focusAt = focus, now
	}
	if h.dismissed != hover && h.dismissed != focus {
		h.dismissed = 0
	}
	target := h.hover
	byFocus := focus != 0 && (hover == 0 || !h.focusAt.Before(h.hoverAt))
	if byFocus {
		target = h.focus
	}
	id := target.anchor.ID()
	if id == 0 || target.anchor.IsDisabled() {
		if h.visible {
			h.leftAt = now
		}
		h.visible = false
		return
	}
	if target.interactive && target.anchor.Pressed() {
		h.dismissed = id
	}
	if h.dismissed == id {
		h.visible = false
		return
	}
	if !byFocus && !h.visible && (h.leftAt.IsZero() || now.Sub(h.leftAt) >= hintTravelTime) {
		if wait := hintDelay - now.Sub(h.hoverAt); wait > 0 {
			c.After(wait)
			return
		}
	}
	// Let the owning popover or dialog handle Escape before dismissing a hint.
	if c.Shortcut(0, ui.KeyEscape) {
		h.dismissed, h.visible = id, false
		return
	}
	h.visible = true
	ui.Overlay(c, func() {
		tip := ui.Box(c.Key("workspace-hint")).Absolute().PassThrough().Role(ui.RoleTooltip).Label(target.text)
		if target.atPointer {
			x, y, _ := target.anchor.PointerPosition()
			x, y = target.anchor.LocalToWindow(x, y)
			w, h := c.Size()
			width, height := c.MeasureText(284, ui.Span{Text: target.text, Size: 12})
			tip.Left(max(3, min(x+14, w-width-20))).Top(max(3, min(y+16, h-height-14)))
		} else if target.beside {
			tip.AttachTo(target.anchor, ui.AnchorRight, ui.AnchorLeft).MarginStart(6)
		} else {
			tip.AttachTo(target.anchor, ui.AnchorTop, ui.AnchorBottom).Margin(0, 0, 4, 0)
		}
		t := c.Theme()
		tip.MaxWidth(300).Padding(4, 8).Radius(5).Background(t.Inverse).TextColor(t.InverseText).
			Children(func() { ui.Text(c, target.text).FontSize(12).IgnoreInput(true) })
	})
}
