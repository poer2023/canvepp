package main

import (
	"time"

	"github.com/egoist/mygo/ui"
)

const pageInset float32 = 20

// Content pages share the space below the draggable window title region.
func pageHeader(c *ui.Context, title, backLabel string, back func(), meta func()) {
	ui.Row(c).FillWidth().Height(workspaceHeaderHeight).Padding(0, pageInset).Gap(8).AlignItems(ui.Center).Children(func() {
		iconButton(c, "back", backLabel, back)
		fittingLabel(c, title, 18, 500).Grow(1)
		if meta != nil {
			meta()
		}
	})
}

// Complete single-line labels need no duplicate hover text. A clipped label
// also enters the focus order so its full value is available from a keyboard.
func fittingLabel(c *ui.Context, text string, size float32, weight int) ui.Element {
	label := ui.Text(c, text).SingleLine().MinWidth(0).FontSize(size).FontWeight(weight)
	available := label.Bounds().W
	if available > 0 {
		width, _ := c.MeasureText(0, ui.Span{Text: text, Size: size, Weight: weight})
		if width > available+.5 {
			flatTip(c, label.Focusable(), text)
		}
	}
	return label
}

func sectionTitle(c *ui.Context, title string) ui.Element {
	return ui.Text(c, title).FontSize(13).FontWeight(500)
}

// Reserve the command's footprint immediately. Fast work ends without flashing
// a progress glyph; slow work has a stable, readable state without a frame loop.
func activityIcon(c *ui.Context, icon, label, progress string, busy, disabled bool, action func(), help ...string) {
	slot := ui.Box(c.Key(label+"-slot")).Size(30, 30).Shrink(0)
	show := delayedActivity(c, slot, busy)
	slot.Children(func() {
		if show {
			flatTip(c, ui.Box(c).Fill().Center().Role(ui.RoleStatus).Label(progress), progress).Children(func() { uiIcon(c, "hourglass", 15).TextColor(c.Theme().TextMuted) })
		} else {
			disable(iconButton(c, icon, label, action, help...), busy || disabled)
		}
	})
}

func delayedActivity(c *ui.Context, owner ui.Element, busy bool) bool {
	since := ui.Local(owner, "since", func() time.Time { return time.Time{} })
	if !busy {
		*since = time.Time{}
		return false
	}
	if since.IsZero() {
		*since = c.Now()
	}
	if left := 300*time.Millisecond - c.Now().Sub(*since); left > 0 {
		c.After(left)
		return false
	}
	return true
}

// Draw a non-interactive stand-in using the same column count and row geometry
// as the material grid. Decorative cells never appear as assets to VoiceOver.
func pendingMaterialGrid(c *ui.Context, minWidth, height, gap, inset, top float32) {
	area := ui.Box(c.Key("pending-material-grid")).Grow(1).FillWidth().Clip().Role(ui.RoleStatus).Label("正在读取图片")
	if !delayedActivity(c, area, true) {
		return
	}
	area.Draw(func(p *ui.Painter, r ui.Rect) {
		usable := r.W - 2*inset
		if usable <= 0 {
			return
		}
		cols := max(1, int((usable+gap)/(minWidth+gap)))
		w := (usable - float32(cols-1)*gap) / float32(cols)
		p.Clip(r, 0, func() {
			for y := r.Y + top; y < r.Y+r.H; y += height + gap {
				for column := 0; column < cols; column++ {
					x := r.X + inset + float32(column)*(w+gap)
					p.Fill(ui.Rect{X: x, Y: y, W: w, H: height - 34}, p.Theme().SurfaceHover, 5)
					p.Fill(ui.Rect{X: x, Y: y + height - 19, W: w * .44, H: 5}, p.Theme().SurfaceHover, 2)
				}
			}
		})
	})
}

func pendingThumbnail(p *ui.Painter, r ui.Rect, failed bool) {
	p.Fill(r, p.Theme().SurfaceHover, 5)
	if failed {
		p.Icon(icons["warning"], ui.Rect{X: r.X + (r.W-18)/2, Y: r.Y + (r.H-18)/2, W: 18, H: 18}, p.Theme().TextMuted)
	}
}
