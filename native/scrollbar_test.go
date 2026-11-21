package main

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func TestThinScrollShowsOnInputAndHidesWhilePointerRests(t *testing.T) {
	for _, mode := range []string{"light", "dark"} {
		var pos ui.ScrollState
		tt := ui.NewTester(func(c *ui.Context) {
			c.SetTheme(workspaceTheme(c, mode))
			thinScroll(c, ui.Scroll(c.Key("samples")), &pos).Size(160, 120).Background(c.Theme().Background).Children(func() {
				ui.Box(c).Height(640).FillWidth().Shrink(0)
			})
		}, 160, 120)
		visible := func() bool {
			im := tt.Image()
			for y := 2; y < 116; y++ {
				if im.RGBAAt(156, y) != im.RGBAAt(145, y) {
					return true
				}
			}
			return false
		}
		tt.Move(70, 50)
		tt.Frame()
		if visible() {
			t.Fatal(mode, "visible before input")
		}
		tt.Scroll(70, 50, 0, 70)
		tt.Frame()
		if pos.Y == 0 || !visible() {
			t.Fatal(mode, "missing active thumb", pos)
		}
		tt.Advance(scrollIdle + time.Millisecond)
		tt.Frame()
		if visible() {
			t.Fatal(mode, "thumb did not hide")
		}
		tt.Move(156, 50)
		tt.Frame()
		if !visible() {
			t.Fatal(mode, "edge cannot reveal thumb")
		}
	}
}
