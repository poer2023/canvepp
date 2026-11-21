package main

import (
	"strconv"
	"sync"
	"time"
	"unsafe"

	"github.com/egoist/mygo/ui"
)

// Coalesce pointer moves while the helper applies the previous scroll. Visibility
// changes wake the worker as well; a seek always implies a visible panel.
type scrollMailbox struct {
	sync.Mutex
	command string
}

func (m *scrollMailbox) take() string { m.Lock(); defer m.Unlock(); return m.command }
func (b *pinterestBrowser) scrollCommand(command string) {
	if b.mailbox == nil {
		return
	}
	b.mailbox.Lock()
	b.mailbox.command = command
	b.mailbox.Unlock()
	select {
	case b.scrollWake <- struct{}{}:
	default:
	}
}

var pinCursorPosition = pinUser32.NewProc("GetCursorPos")
var pinButtonState = pinUser32.NewProc("GetAsyncKeyState")

func scrollPointer() (screenY int32, held bool) {
	var point struct{ X, Y int32 }
	ok, _, _ := pinCursorPosition.Call(uintptr(unsafe.Pointer(&point)))
	state, _, _ := pinButtonState.Call(1) // VK_LBUTTON
	return point.Y, ok != 0 && state&0x8000 != 0
}

type webScroll struct {
	trackPointer  bool
	pressScreenY  int32
	pressY        float32
	rail          ui.Handle
	percent, view float64
	dragging      bool
	grab          float32
	until         time.Time
}

func (b *pinterestBrowser) updateScroll(percent, view float64) {
	if percent < 0 || view <= 0 {
		percent, view = 0, 100
	}
	if (view >= 100) != (b.view >= 100) {
		b.last = [4]int{}
	}
	if !b.dragging {
		if b.view > 0 && b.percent != percent {
			b.until = time.Now().Add(scrollIdle)
		}
		b.percent = percent
	}
	b.view = view
	b.invalidate()
}

func (b *pinterestBrowser) scrollbar(c *ui.Context) {
	if b.view <= 0 || b.view >= 100 {
		b.dragging = false
		return
	}
	rail := ui.Box(c.Key("web-scrollbar")).Width(thumbEdge).FillHeight().Shrink(0).Role(ui.RoleSlider).Focusable().Bind(&b.rail).Label("Pinterest 网页滚动条").Cursor(ui.CursorPointer)
	visible := b.dragging || rail.Hovered() || rail.FocusVisible() || b.until.After(c.Now())
	if b.until.After(c.Now()) {
		c.After(b.until.Sub(c.Now()))
	}
	size := func(h float32) float32 { return min(h, max(18, h*float32(b.view/100))) }
	seek := func(percent float64) {
		percent = max(0, min(100, percent))
		if b.percent == percent {
			return
		}
		b.percent = percent
		b.until = time.Now().Add(scrollIdle)
		b.scrollCommand(strconv.FormatFloat(b.percent, 'f', 4, 64))
	}
	// UIA scrolling gives focus back to Chrome. MyGo can deliver a synthetic
	// release then, even though the user still holds the captured mouse button.
	// Follow the physical gesture only from a press on this scrollbar to release.
	if b.dragging && b.trackPointer {
		y, held := scrollPointer()
		h := b.rail.Bounds(c).H
		if travel := h - size(h); travel > 0 {
			localY := b.pressY + float32(y-b.pressScreenY)/b.deviceScale
			seek(float64((localY-b.grab)/travel) * 100)
		}
		b.dragging = held
		if held {
			c.After(16 * time.Millisecond)
		}
	}
	rail.HandleInput(func(ev ui.InputEvent) bool {
		h := b.rail.Bounds(c).H
		thumb := size(h)
		travel := h - thumb
		if travel <= 0 {
			return false
		}
		top := travel * float32(b.percent/100)
		move := func() { seek(float64((ev.Y-b.grab)/travel) * 100) }
		fullRange := float64(h) * (100/b.view - 1)
		switch ev.Kind {
		case ui.InputPointerDown:
			if ev.Button != 0 {
				return false
			}
			b.dragging = true
			b.pressY = ev.Y
			b.pressScreenY, b.trackPointer = scrollPointer()
			b.trackPointer = b.trackPointer && b.deviceScale > 0
			b.grab = ev.Y - top
			if b.grab < 0 || b.grab > thumb {
				b.grab = thumb / 2
				move()
			}
		case ui.InputPointerMove:
			if !b.dragging {
				return false
			}
			move()
		case ui.InputPointerUp:
			if !b.dragging {
				return false
			}
			if b.trackPointer {
				if _, held := scrollPointer(); held {
					return true
				}
			}
			move()
			b.dragging = false
		case ui.InputScroll:
			seek(b.percent + 100*float64(ev.DY)/fullRange)
		case ui.InputKeyDown:
			step := 100 * float64(h) / fullRange
			switch ev.Key {
			case ui.KeyHome:
				seek(0)
			case ui.KeyEnd:
				seek(100)
			case ui.KeyPageUp:
				seek(b.percent - step)
			case ui.KeyPageDown:
				seek(b.percent + step)
			case ui.KeyUp:
				seek(b.percent - step/12)
			case ui.KeyDown:
				seek(b.percent + step/12)
			default:
				return false
			}
		default:
			return false
		}
		return true
	})
	ink := c.Theme().Scrollbar
	rail.Draw(func(p *ui.Painter, r ui.Rect) {
		if visible {
			height := size(r.H)
			p.Fill(ui.Rect{X: r.X + (r.W-thumbWidth)/2, Y: r.Y + (r.H-height)*float32(b.percent/100), W: thumbWidth, H: height}, ink, thumbWidth/2)
		}
	})
}
