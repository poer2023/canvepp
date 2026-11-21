package main

import (
	"time"

	"github.com/egoist/mygo/ui"
)

type searchControl struct {
	open, hadFocus, focusNext, dirty bool
	input, toggle                    ui.Handle
}

// Search occupies one icon until requested. Only its short opening/closing
// transition schedules frames; a retained query is never cleared on blur/Esc.
func quietSearch(c *ui.Context, value *string, s *searchControl, label, placeholder string, width float32) bool {
	changed := s.dirty
	s.dirty = false
	row := ui.Row(c.Key(label + "-field")).Height(30).Radius(7).Clip().AlignItems(ui.Center)
	if s.open && !s.focusNext && s.hadFocus && !row.FocusWithin() && *value == "" {
		s.open, s.hadFocus = false, false
	}
	target := float32(0)
	if s.open {
		target = 1
	}
	progress := target
	if !c.Preferences().ReduceMotion {
		ease := ui.EaseIn
		if s.open {
			ease = ui.EaseOut
		}
		progress = row.AnimateTo("reveal", target, ui.Animation{Duration: 160 * time.Millisecond, Ease: ease})
	}
	row.Width(30 + (width-30)*progress).Background(c.Theme().Surface.Alpha(progress)).Children(func() {
		toggleLabel, tip := label, label
		if s.open {
			toggleLabel = "收起" + label
			tip = toggleLabel
		} else if *value != "" {
			tip += " · " + *value
		}
		iconButton(c, "search", toggleLabel, func() {
			s.open = !s.open
			s.hadFocus = false
			s.focusNext = s.open
		}, tip).Bind(&s.toggle)
		if s.open {
			c.OnShortcut(0, ui.KeyEscape, func() { s.open, s.hadFocus = false, false; s.toggle.Focus() })
			input := editHint(c, ui.TextInput(c.Key("input"), value).Label(label), label).Placeholder(placeholder).Grow(1).MinWidth(0).Height(30).Padding(0).FontSize(12).Background(ui.Transparent).Border(0, ui.Transparent).Bind(&s.input)
			changed = input.Changed() || changed
			if s.focusNext {
				s.input.Focus()
				s.focusNext = false
			} else if input.Focused() {
				s.hadFocus = true
			}
			if *value != "" {
				iconButton(c, "close", "清除"+label, func() { *value = ""; s.dirty = true; s.input.Focus() }).Size(24, 26).Padding(5)
			} else {
				ui.Box(c).Width(8)
			}
		} else if *value != "" {
			ui.Box(c).Absolute().Top(3).Left(24).Size(4, 4).Radius(2).Background(c.Theme().Text).Label(label + "已筛选")
		}
	})
	return changed
}
