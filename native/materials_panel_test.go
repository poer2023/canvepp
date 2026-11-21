package main

import (
	"reflect"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestMaterialTabsInNativeTitleBar(t *testing.T) {
	for _, bar := range []ui.TitleBar{{Height: 28, Left: 78}, {Height: 32, Right: 138}} {
		a := testDesktop()
		a.materials = &materialsPanel{connected: true, eagle: &eagleClient{Name: "测试库"}}
		a.editor.Selection["0"] = true
		graph, view := cloneGraph(a.editor.Project.Graph), a.editor.View
		tt := ui.NewTester(a.view, 1000, 680)
		tt.SetTitleBar(bar)
		click := func(label string) {
			t.Helper()
			if err := tt.Click(label); err != nil {
				t.Fatal(err)
			}
		}
		bounds := func(label string) ui.Rect {
			t.Helper()
			r, ok := tt.Find(label)
			if !ok {
				t.Fatal(label)
			}
			return r
		}
		plus := bounds("添加侧栏页面")
		if (plus.X+plus.W) > 1000-bar.Right || (plus.X+plus.W) < 1000-bar.Right-8 || plus.Y+plus.H > bar.Height {
			t.Fatal("plus obscured by window controls", bar, plus)
		}
		for _, page := range []string{dockPinterest, dockEagle, dockHistory, dockOperations} {
			click("添加侧栏页面")
			if err := tt.ChooseMenuItem(page); err != nil {
				t.Fatal(err)
			}
			tab := bounds("侧栏：" + page)
			if tab.Y+tab.H > bar.Height || tab.X < bar.Left {
				t.Fatal("tab outside titlebar", tab)
			}
		}
		click("侧栏：Pinterest")
		dock := bounds("画布右侧栏")
		toolbar := bounds("Pinterest 后退")
		if toolbar.Y < dock.Y || toolbar.Y-dock.Y > 8 {
			t.Fatal("extra material header", dock, toolbar)
		}
		for _, label := range []string{"侧栏：素材库", "素材：Pinterest", "登录 Pinterest", "Pinterest 我的收藏", "切换到内置网页"} {
			if _, ok := tt.Find(label); ok {
				t.Fatal("obsolete material chrome", label)
			}
		}
		click("添加侧栏页面")
		if err := tt.ChooseMenuItem("Pinterest"); err != nil {
			t.Fatal(err)
		}
		if len(a.dock.pages) != 4 {
			t.Fatal("reopening duplicated a tab")
		}
		click("关闭当前侧栏页面")
		if a.dock.active != dockEagle {
			t.Fatal("neighbor tab not selected")
		}
		bounds("搜索 Eagle 图片")
		click("全部图片")
		if _, ok := tt.Find("图片画布"); ok {
			t.Fatal("canvas overlaps gallery")
		}
		click("侧栏：Eagle")
		bounds("图片画布")
		if a.editor.View != view || !reflect.DeepEqual(graph, a.editor.Project.Graph) || !a.editor.Selection["0"] {
			t.Fatal("switching pages changed the canvas")
		}
	}
}
