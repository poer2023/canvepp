package main

import (
	"slices"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func TestCompactDirectoryStaysInPlaceAcrossPages(t *testing.T) {
	a := galleryFixture()
	a.left = "画布"
	tt := ui.NewTester(a.view, 1000, 680)
	before, ok := tt.Find("打开画布：旅行画布")
	if !ok || before.H > 30 {
		t.Fatal("non-compact canvas row", before)
	}
	if tt.HasText("当前画布") {
		t.Fatal("removed workspace category is still shown")
	}
	for _, page := range []string{allCanvases, allPictures, "回收站", "打开设置"} {
		if err := tt.Click(page); err != nil {
			t.Fatal(err)
		}
		row, ok := tt.Find("打开画布：旅行画布")
		if !ok || row != before {
			t.Fatal("directory moved", page, row, before)
		}
		if err := tt.Click("打开画布：旅行画布"); err != nil || a.left != "画布" || a.settingsOpen {
			t.Fatal("return from page", page, err)
		}
	}
	initial := slices.Clone(a.order)
	a.canvasSort = 1
	reverse := slices.Clone(initial)
	slices.Reverse(reverse)
	if !slices.Equal(a.filteredCanvases(), reverse) || !slices.Equal(a.order, initial) {
		t.Fatal("directory sort mutated creation order")
	}
}

func TestWorkspacePointerAndFastHints(t *testing.T) {
	a := testDesktop()
	tt := ui.NewTester(a.view, 1100, 740)
	menu, _ := tt.Find(allPictures)
	canvases, _ := tt.Find(allCanvases)
	point := func(r ui.Rect) { tt.Move(r.X+r.W/2, r.Y+r.H/2) }
	sample := func(r ui.Rect) [3]uint8 {
		color := tt.Image().RGBAAt(int(r.X+r.W-10), int(r.Y+8))
		return [3]uint8{color.R, color.G, color.B}
	}
	rest := sample(menu)
	point(menu)
	if tt.Cursor() != ui.CursorPointer || sample(menu) == rest {
		t.Fatal("menu hand or immediate highlight missing")
	}
	tt.Advance(hintDelay + time.Millisecond)
	point(canvases)
	if tt.Cursor() != ui.CursorPointer || sample(menu) != rest {
		t.Fatal("hover color lingered")
	}
	tt.Move(60, 500)
	if tt.Cursor() != ui.CursorDefault {
		t.Fatal("blank space shows a hand")
	}
	tt.Move(390, 140)
	if tt.Cursor() != ui.CursorGrab {
		t.Fatal("canvas image needs the grab cursor", tt.Cursor())
	}
	tt.Press(390, 140)
	if tt.Cursor() != ui.CursorGrabbing {
		t.Fatal("drag did not close the hand", tt.Cursor())
	}
	tt.Release(390, 140)
	tt.Move(630, 490)
	if tt.Cursor() != ui.CursorDefault {
		t.Fatal("canvas did not release hand", tt.Cursor())
	}
}

func TestSidebarResizesAndRestoresWithoutEditingCanvas(t *testing.T) {
	a := testDesktop()
	a.editor.Project.Name = "明暗研究"
	tt := ui.NewTester(a.view, 1000, 680)
	rect := func(label string) ui.Rect {
		t.Helper()
		r, ok := tt.Find(label)
		if !ok {
			t.Fatalf("missing %s", label)
		}
		return r
	}
	width := func() float32 { return rect("图片画布").X }
	if width() != defaultSidebarWidth {
		t.Fatal("initial width", width())
	}
	view := a.editor.View
	edge := rect("调整侧栏宽度")
	x := edge.X + edge.W/2
	tt.Press(x, 490)
	tt.Move(x+47, 490)
	tt.Move(x+83, 490)
	tt.Release(x+83, 490)
	if want := defaultSidebarWidth + 83; width() != want || a.editor.View != view || a.gesture != "" {
		t.Fatal("sidebar drag", width(), a.editor.View)
	}
	tt.Advance(hintDelay + time.Millisecond)
	click := func(label string) { r := rect(label); tt.ClickAt(r.X+r.W/2, r.Y+r.H/2) }
	click("折叠侧栏")
	tt.Advance(foldDuration / 3)
	if current := width(); current <= collapsedSidebarWidth || current >= defaultSidebarWidth+83 {
		t.Fatal("rail collapse skipped its intermediate width", current)
	}
	tt.Advance(foldDuration)
	if width() != collapsedSidebarWidth || tt.HasText("Canvepp") {
		t.Fatal("icon rail", width())
	}
	name := a.editor.Project.Name
	for _, hidden := range []string{"打开画布：" + name, "画布排序"} {
		if _, ok := tt.Find(hidden); ok {
			t.Fatal("unexpected compact control", hidden)
		}
	}
	click("搜索画布")
	click("搜索结果：" + name)
	if a.editor.Project.Name != name || a.canvasSearch.open {
		t.Fatal("compact search did not open result")
	}
	click("展开侧栏")
	tt.Advance(foldDuration + 32*time.Millisecond)
	if width() != defaultSidebarWidth+83 {
		t.Fatal("previous width not restored", width())
	}
}

func TestTooltipTextLetsClicksReachUnderlyingControl(t *testing.T) {
	clicked := false
	tt := ui.NewTester(func(c *ui.Context) {
		defer beginHints(c)()
		ui.Column(c).Padding(24).Gap(2).Children(func() {
			iconButton(c, "plus", "新建", func() { clicked = true })
			iconButton(c, "search", "搜索", func() {}, "搜索画布文件")
		})
	}, 180, 160)
	search, _ := tt.Find("搜索")
	tt.Move(search.X+search.W/2, search.Y+search.H/2)
	tt.Advance(hintDelay + time.Millisecond)
	tip, ok := tt.Find("搜索画布文件")
	button, _ := tt.Find("新建")
	x := button.X + button.W/2
	y := tip.Y + tip.H/2
	if !ok || x <= tip.X || x >= tip.X+tip.W || y <= button.Y || y >= button.Y+button.H {
		t.Fatal("fixture needs the tooltip over the button", tip, button)
	}
	tt.ClickAt(x, y)
	if !clicked {
		t.Fatal("tooltip intercepted the first click")
	}
}
