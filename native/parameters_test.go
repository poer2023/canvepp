package main

import (
	"math"
	"reflect"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func TestInspectorParameterRowsKeepDraftAndActionsInScroll(t *testing.T) {
	a := drawerFixture()
	n := a.editor.Node("1")
	n.Config.Text, n.Config.CreationStarted = "保留创作草稿", true
	n.Config.Params = Params{Size: "1920x1080", N: 4, Quality: "high"}
	tt := ui.NewTester(a.view, 1440, 900)
	before := cloneGraph(a.editor.Project.Graph)
	width, hasWidth := tt.Find("宽度像素")
	height, hasHeight := tt.Find("高度像素")
	picker, hasPicker := tt.Find("选择比例 · 16:9")
	if !hasWidth || !hasHeight || !hasPicker || a.ratioMenuOpen || width.Y <= picker.Y || height.Y != width.Y {
		t.Fatal("target pixels must be editable below the ratio picker", width, height, picker)
	}
	panel, _ := tt.Find("悬浮图片设置")
	if panel.H > 425+creationParameterHeight {
		t.Fatal("inspector and source tabs exceed their content budget", panel)
	}
	if err := tt.Click("选择比例 · 16:9"); err != nil {
		t.Fatal(err)
	}
	menu, ok := tt.Find("画幅比例")
	rect, _ := tt.Find("悬浮图片设置")
	if !ok || menu.W < 200 || rect != panel {
		t.Fatal("the anchored ratio picker moved the panel or did not open", menu, rect, panel)
	}
	tt.Key(0, ui.KeyEscape)
	if a.ratioMenuOpen {
		t.Fatal("Escape did not dismiss the ratio picker")
	}
	for _, label := range []string{"展开来源抽屉", "开始新创作"} {
		if err := tt.Click(label); err != nil {
			t.Fatal(err)
		}
		tt.Advance(800 * time.Millisecond)
	}
	if !reflect.DeepEqual(before, a.editor.Project.Graph) || a.prompt != "保留创作草稿" || a.count != 4 {
		t.Fatal("disclosure or drawer navigation changed the draft")
	}
	generate, ok := tt.Find("生成 4 张图片")
	if !ok || generate.Y+generate.H > panel.Y+panel.H {
		t.Fatal("generate action missing")
	}
	if toggle, found := tt.Find("多张图片并行生成"); !found || toggle.Y+toggle.H > generate.Y {
		t.Fatal("the batch switch must be accessible before the generate action", toggle, generate)
	}
	for _, label := range []string{"下载当前图片", "放大当前图片", "移入回收站"} {
		button, ok := tt.Find(label)
		if !ok || math.Abs(float64(button.Y+button.H/2-generate.Y-generate.H/2)) > 1 {
			t.Fatal("image action not in generate row", label, button, generate)
		}
		if button.X+button.W > generate.X {
			t.Fatal("generate needs its own trailing action", label, button, generate)
		}
	}
	// A short viewport must scroll the complete action row with the content.
	tt.SetSize(1000, 360)
	tt.Frame()
	generate, _ = tt.Find("生成 4 张图片")
	view := a.editor.View
	prompt, _ := tt.Find("编辑提示词")
	tt.Scroll(prompt.X+20, prompt.Y+20, 0, 220)
	tt.Advance(800 * time.Millisecond)
	after, _ := tt.Find("生成 4 张图片")
	if after.Y == generate.Y || a.editor.View != view {
		t.Fatal("actions are pinned or scrolling changed canvas zoom", generate, after)
	}
	if !reflect.DeepEqual(before, a.editor.Project.Graph) {
		t.Fatal("scroll changed content")
	}
	a.editor.Selection = map[string]bool{"2": true}
	tt.Frame()
	if a.ratioMenuOpen {
		t.Fatal("new image inherited an open ratio picker")
	}
}

func TestGenerationSwitchRemembersChoice(t *testing.T) {
	a := drawerFixture()
	a.store = testStore(t)
	n := a.editor.Node("1")
	n.Config.Text, n.Config.CreationStarted = "并行生成测试", true
	n.Config.Params = Params{Size: "1024x1024", N: 3, Quality: "high"}
	tt := ui.NewTester(a.view, 1440, 900)
	if a.parallelGeneration {
		t.Fatal("the old workflow should start in serial mode")
	}
	if err := tt.Click("多张图片并行生成"); err != nil {
		t.Fatal(err)
	}
	reopened, err := newDesktop(a.store)
	if err != nil || !a.parallelGeneration || !reopened.parallelGeneration {
		t.Fatal("parallel selection must survive reopening", err)
	}
	if err := tt.Click("多张图片并行生成"); err != nil {
		t.Fatal(err)
	}
	reopened, err = newDesktop(a.store)
	if err != nil || a.parallelGeneration || reopened.parallelGeneration {
		t.Fatal("serial selection must survive reopening", err)
	}
}

func TestPixelFieldsKeepPartialTextOutOfSavedDraft(t *testing.T) {
	a := testDesktop()
	a.editor.Selection["0"] = true
	tt := ui.NewTester(a.view, 1440, 900)
	panel, _ := tt.Find("悬浮图片设置")
	limit := float32(350)
	if a.count > 1 {
		limit += creationParameterHeight
	}
	if !tt.HasText("创作图片") || panel.H > limit {
		t.Fatal("new-image creation should fit in the compact panel", panel)
	}
	if err := tt.Click("选择比例 · 16:9"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("自由像素"); err != nil {
		t.Fatal(err)
	}
	edit := func(label, value string) {
		t.Helper()
		if err := tt.Click(label); err != nil {
			t.Fatal(err)
		}
		tt.Key(ui.Cmd, ui.KeyA)
		tt.Type(value)
	}
	edit("宽度像素", "1536")
	edit("高度像素", "768")
	if a.width != 1536 || a.height != 768 || a.editor.Node("0").Config.Params.Size != "1536x768" {
		t.Fatal("independent pixels not saved", a.width, a.height)
	}
	edit("宽度像素", "9")
	if a.width != 1536 {
		t.Fatal("partial value corrupted width")
	}
	tt.Type("60")
	if a.width != 960 {
		t.Fatal("partial numeric input could not be completed", a.width)
	}
	edit("高度像素", "bad")
	if a.height != 768 {
		t.Fatal("invalid value corrupted height")
	}
	if err := tt.Click("生成 3 张"); err != nil {
		t.Fatal(err)
	}
	if a.count != 3 || a.editor.Node("0").Config.Params.N != 3 || a.editor.Node("0").Config.Params.Size != "960x768" {
		t.Fatal("count changed dimensions or failed to persist")
	}
	if _, ok := tt.Find("Decrease"); ok {
		t.Fatal("pixel stepper still present")
	}
	if _, ok := tt.Find("Increase"); ok {
		t.Fatal("pixel stepper still present")
	}
	if err := tt.Click("选择比例 · 自由像素"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("16:9"); err != nil {
		t.Fatal(err)
	}
	edit("宽度像素", "1920")
	if a.height != 1080 {
		t.Fatal("fixed ratio did not link height", a.height)
	}
	edit("高度像素", "720")
	if a.width != 1280 {
		t.Fatal("fixed ratio did not link width", a.width)
	}
	before := cloneGraph(a.editor.Project.Graph)
	tt.Click("关闭图片设置")
	tt.ClickAt(390, 140)
	if !a.panel.Open || !reflect.DeepEqual(before, a.editor.Project.Graph) {
		t.Fatal("reopening panel lost content")
	}
	tt.Advance(800 * time.Millisecond)
	if tt.FramePending() {
		t.Fatal("parameters animate indefinitely")
	}
}

func TestLinkedPixelBoundsAndInvalidValues(t *testing.T) {
	for i, r := range ratios {
		for _, width := range []bool{true, false} {
			for _, input := range []float64{1, 64, 1024, 8192, 9000} {
				a := testDesktop()
				a.ratio = i
				a.setPixelDimension(width, input)
				if a.width < 64 || a.width > 8192 || a.height < 64 || a.height > 8192 {
					t.Fatal("linked axis out of bounds", r, width, input, a.width, a.height)
				}
				if math.Abs(a.height-a.width*float64(r.h)/float64(r.w)) > 1.2 {
					t.Fatal("ratio drift", r, a.width, a.height)
				}
			}
		}
	}
	a := testDesktop()
	a.setPixelDimension(true, math.NaN())
	a.setPixelDimension(false, math.Inf(1))
	if a.width != 1024 || a.height != 1024 {
		t.Fatal("nonfinite input damaged dimensions")
	}
}

func TestDragDefersInspectorAndCommitsOneUndo(t *testing.T) {
	a := testDesktop()
	a.editor.Selection["0"] = true
	a.editor.Node("0").Config.Text = "保留第一张图的草稿"
	a.editor.Node("1").Config.Text = "第二张图的草稿"
	tt := ui.NewTester(a.view, 1440, 900)
	old := a.editor.Node("1").Position
	tt.Press(700, 140)
	for i := 1; i <= 30; i++ {
		tt.Move(700+float32(i), 140+float32(i))
	}
	if a.prompt != "保留第一张图的草稿" || a.draftSelection[0] != "0" {
		t.Fatal("inspector rebuilt on drag path")
	}
	if a.editor.Node("1").Position != (Point{old.X + 30, old.Y + 30}) {
		t.Fatal("card trails pointer")
	}
	tt.Release(730, 170)
	if a.prompt != "第二张图的草稿" || a.draftSelection[0] != "1" || len(a.editor.undo) != 1 {
		t.Fatal("release did not synchronize exactly once")
	}
	a.editor.Undo()
	if a.editor.Node("1").Position != old || a.editor.Node("0").Config.Text != "保留第一张图的草稿" {
		t.Fatal("undo lost content")
	}
}
