package main

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func TestActivityStaysInPlaceAndSettles(t *testing.T) {
	busy, activated := false, false
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Row(c).Gap(8).Children(func() {
			activityIcon(c, "refresh", "读取", "正在读取", busy, false, func() { activated = true })
			ui.Button(c, "相邻命令")
		})
	}, 320, 120)
	tt.SetPreferences(ui.Preferences{ReduceMotion: true})
	initial, _ := tt.Find("相邻命令")
	busy = true
	tt.Frame()
	r, ok := tt.Find("读取")
	if !ok {
		t.Fatal("command disappeared before progress threshold")
	}
	tt.ClickAt(r.X+r.W/2, r.Y+r.H/2)
	tt.Advance(200 * time.Millisecond)
	if _, ok := tt.Find("正在读取"); ok || activated {
		t.Fatal("premature progress or duplicate action")
	}
	tt.Advance(110 * time.Millisecond)
	if _, ok := tt.Find("正在读取"); !ok {
		t.Fatal("missing in-place status")
	}
	next, _ := tt.Find("相邻命令")
	if next != initial {
		t.Fatal("busy state shifted neighbor", initial, next)
	}
	tt.Advance(time.Second)
	if tt.FramePending() {
		t.Fatal("static progress continued redrawing")
	}
	busy = false
	tt.Frame()
	if err := tt.Click("读取"); err != nil || !activated {
		t.Fatal("command failed to return", err)
	}
}

func TestFailedSaveBelongsToCanvasAndRevision(t *testing.T) {
	a := galleryFixture()
	a.left = "画布"
	e := a.editor
	e.Project.Revision = 4
	e.dirty = true
	a.onCanvasSaved(e.Project.ID, 4, errors.New("磁盘空间不足"))
	tt := ui.NewTester(a.view, 1000, 680)
	tt.Advance(10 * time.Second)
	if !tt.HasText("磁盘空间不足") {
		t.Fatal("save failure timed out")
	}
	a.editor = a.editors["second"]
	tt.Frame()
	if tt.HasText("磁盘空间不足") {
		t.Fatal("save failure leaked into another canvas")
	}
	a.editor = e
	a.onCanvasSaved(e.Project.ID, 3, nil)
	tt.Frame()
	if !tt.HasText("磁盘空间不足") || !e.dirty {
		t.Fatal("stale receipt removed current failure")
	}
	a.onCanvasSaved(e.Project.ID, 4, nil)
	tt.Frame()
	if tt.HasText("磁盘空间不足") || e.dirty {
		t.Fatal("successful retry did not clear error")
	}
}

func TestGalleryResumesPositionAfterSettingsAndTrash(t *testing.T) {
	a := galleryFixture()
	a.left = "画布"
	a.openDock(dockHistory)
	for i := 0; i < 45; i++ {
		id := fmt.Sprintf("source-%02d", i)
		a.editor.Assets[id] = Asset{ID: id, ProjectID: a.editor.Project.ID, Name: id + ".png", SHA: id, CreatedAt: "2026-11-06T01:00:00Z"}
	}
	tt := ui.NewTester(a.view, 1000, 680)
	tt.SetPreferences(ui.Preferences{ReduceMotion: true})
	r, _ := tt.Find("图片画廊")
	tt.Scroll(r.X+r.W/2, 450, 0, 480)
	tt.Advance(time.Second)
	y := a.historyView.scroll.Y
	if y <= 0 {
		t.Fatal("gallery did not move", a.historyView.scroll)
	}
	if err := tt.Click("打开设置"); err != nil {
		t.Fatal(err)
	}
	settings := ui.NewTester(a.settingsView, 850, 704)
	settings.Key(0, ui.KeyEscape)
	for _, label := range []string{"回收站", "返回当前画布"} {
		if err := tt.Click(label); err != nil {
			t.Fatal(err)
		}
	}
	if a.historyView.scroll.Y != y || a.trashView.scroll.Y != 0 {
		t.Fatal("gallery position was lost", y, a.historyView.scroll.Y, a.trashView.scroll.Y)
	}
	a.selectLibraryGroup("")
	tt.Frame()
	if a.historyView.scroll.Y != 0 {
		t.Fatal("new filters should start at top", a.historyView.scroll.Y)
	}
}

func TestTooltipLeavesEscapeToImagePreview(t *testing.T) {
	a := galleryFixture()
	a.preview(a.editor.Assets[a.editor.Project.ID+"-a"])
	tt := ui.NewTester(a.view, 1000, 680)
	r, ok := tt.Find("关闭")
	if !ok {
		t.Fatal("preview absent")
	}
	tt.Move(r.X+r.W/2, r.Y+r.H/2)
	tt.Advance(time.Second)
	tt.Key(0, ui.KeyEscape)
	if a.previewOpen {
		t.Fatal("tooltip intercepted preview dismissal")
	}
}

func TestMaterialGridLoadingIsDistinctFromEmpty(t *testing.T) {
	a := testDesktop()
	m := &materialsPanel{connected: true, loading: true, eagle: &eagleClient{Name: "本地素材"}}
	a.materials = m
	a.openDock(dockEagle)
	tt := ui.NewTester(a.view, 1000, 680)
	for _, appearance := range []string{"light", "dark"} {
		m.loading = true
		a.appearance = appearance
		tt.Frame()
		tt.Advance(350 * time.Millisecond)
		r, ok := tt.Find("正在读取图片")
		if !ok || r.H < 210 || tt.HasText("没有找到可导入的图片") {
			t.Fatal("invalid pending grid", appearance, r)
		}
		tt.Advance(time.Second)
		if tt.FramePending() {
			t.Fatal("static placeholders continued rendering")
		}
		m.loading = false
		tt.Frame()
		if !tt.HasText("没有找到可导入的图片") {
			t.Fatal("missing empty state after completion")
		}
	}
}
