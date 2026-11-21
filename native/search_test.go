package main

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func TestIconSearchFocusFilterClearAndEscape(t *testing.T) {
	for _, reduced := range []bool{false, true} {
		a := testDesktop()
		tt := ui.NewTester(a.view, 1000, 680)
		tt.SetPreferences(ui.Preferences{ReduceMotion: reduced})
		if r, ok := tt.Find("搜索画布"); !ok || r.W > 31 {
			t.Fatal("closed search must only occupy one icon", r)
		}
		if err := tt.Click("搜索画布"); err != nil {
			t.Fatal(err)
		}
		tt.Advance(180 * time.Millisecond)
		if !tt.Focused("搜索画布") {
			t.Fatal("opening search did not focus input")
		}
		tt.Type("找不到的画布")
		if a.canvasQuery != "找不到的画布" || len(a.filteredCanvases()) != 0 {
			t.Fatal("search did not filter", a.canvasQuery)
		}
		tt.Key(0, ui.KeyEscape)
		tt.Advance(180 * time.Millisecond)
		if a.canvasSearch.open || a.canvasQuery == "" {
			t.Fatal("Escape must collapse without losing the filter")
		}
		if _, ok := tt.Find("搜索画布已筛选"); !ok {
			t.Fatal("collapsed filter is invisible")
		}
		if err := tt.Click("搜索画布"); err != nil {
			t.Fatal(err)
		}
		tt.Advance(180 * time.Millisecond)
		if err := tt.Click("清除搜索画布"); err != nil {
			t.Fatal(err)
		}
		if a.canvasQuery != "" || len(a.filteredCanvases()) != 1 {
			t.Fatal("clear did not restore results")
		}
		tt.Click("打开画布：" + a.editor.Project.Name)
		tt.Advance(200 * time.Millisecond)
		if a.canvasSearch.open {
			t.Fatal("empty unfocused search must collapse")
		}
		tt.Move(800, 500)
		tt.Advance(2 * time.Second)
		if tt.FramePending() {
			t.Fatal("search animation keeps repainting at rest")
		}
	}
}

func TestSearchEscapeStaysInMaterialsAndReportsClear(t *testing.T) {
	a := testDesktop()
	a.materials = &materialsPanel{connected: true, eagle: &eagleClient{Name: "本地库"}}
	a.openDock(dockEagle)
	tt := ui.NewTester(a.view, 1000, 680)
	tt.SetPreferences(ui.Preferences{ReduceMotion: true})
	tt.Click("搜索 Eagle 图片")
	tt.Type("茶杯")
	if a.materials.searchAt.IsZero() {
		t.Fatal("typing did not schedule search")
	}
	tt.Key(0, ui.KeyEscape)
	if a.dock.active != dockEagle || a.materials.search.open || a.materials.query != "茶杯" {
		t.Fatal("Escape left materials or cleared the filter")
	}
	tt.Click("搜索 Eagle 图片")
	a.materials.searchAt = time.Time{}
	tt.Click("清除搜索 Eagle 图片")
	if a.materials.query != "" || a.materials.searchAt.IsZero() {
		t.Fatal("clear did not trigger unfiltered reload")
	}
}

func TestLibrarySearchNamesAndReadings(t *testing.T) {
	for _, tc := range []struct {
		query, name string
		want        bool
	}{
		{"飞书文档", "文档 - 飞书", true}, {"文档 飞书", "飞书设计文档", true}, {"AI设计", "设计 - AI", true}, {"飞书 飞轮", "文档 - 飞书", false},
		{"wxds", "微信读书", true}, {"weixindushu", "微信读书", true}, {"weiXduS", "微信读书", true},
		{"ｗｘｄｓ Cover", "微信读书 Cover", true}, {"w", "微信读书", false}, {"w", "Web 稿件", true},
		{"ws", "微信读书", false}, {"xxx", "微信读书", false}, {"sb", "松 柏", false},
	} {
		if got := parseLibraryQuery(tc.query).matches(tc.name); got != tc.want {
			t.Errorf("%q in %q: got %t", tc.query, tc.name, got)
		}
	}
}
