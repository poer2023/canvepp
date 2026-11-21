package main

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func TestWorkspaceFoldingInterpolatesAndRespectsReducedMotion(t *testing.T) {
	for _, reduced := range []bool{false, true} {
		a := testDesktop()
		a.editor.Selection["0"] = true
		tt := ui.NewTester(a.view, 1100, 740)
		tt.SetPreferences(ui.Preferences{ReduceMotion: reduced})
		dimension := func(label string) ui.Rect {
			t.Helper()
			r, ok := tt.Find(label)
			if !ok {
				t.Fatal("missing", label)
			}
			return r
		}
		click := func(label string) {
			t.Helper()
			if err := tt.Click(label); err != nil {
				t.Fatal(err)
			}
		}
		panel := "悬浮图片设置"
		full := dimension(panel).H
		click("收起图片设置")
		tt.Advance(foldDuration / 3)
		h := dimension(panel).H
		if reduced && h != imagePanelHeaderHeight || !reduced && !(h < full && h > imagePanelHeaderHeight) {
			t.Fatal("unexpected inspector fold height", reduced, h, full)
		}
		click("展开图片设置")
		tt.Advance(foldDuration + 40*time.Millisecond)
		if !a.panel.Open || a.panel.Collapsed || dimension(panel).H != full {
			t.Fatal("reversing a fold did not restore the inspector", reduced, dimension(panel))
		}

		canvas := "图片画布"
		fullWidth := dimension(canvas).W
		click("打开操作历史")
		tt.Advance(foldDuration / 3)
		middle := dimension(canvas).W
		tt.Advance(foldDuration + 40*time.Millisecond)
		docked := dimension(canvas).W
		if reduced && middle != docked || !reduced && !(middle < fullWidth && middle > docked) {
			t.Fatal("unexpected dock opening width", reduced, fullWidth, middle, docked)
		}
		click("关闭当前侧栏页面")
		tt.Advance(800 * time.Millisecond)
		if dimension(canvas).W != fullWidth || tt.FramePending() {
			t.Fatal("closed dock did not settle", reduced, dimension(canvas))
		}
	}
}
