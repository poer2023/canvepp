package main

import (
	"bytes"
	"math"
	"slices"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func testDesktop() *desktop {
	e := fixture()
	return &desktop{editor: e, editors: map[string]*Editor{e.Project.ID: e}, order: []string{e.Project.ID}, left: "画布", count: 1, width: 1024, height: 1024, status: "已保存", running: map[string]bool{}, frameTimes: make([]int64, 2048)}
}
func TestNativeInputKeepsCanvasAndTextSeparate(t *testing.T) {
	a := testDesktop()
	tt := ui.NewTester(a.view, 1440, 900)
	tt.ClickAt(390, 140)
	if len(a.editor.Selection) != 1 {
		t.Fatal("image click selection", a.editor.Selection)
	}
	if err := tt.Click("编辑提示词"); err != nil {
		t.Fatal(err)
	}
	tt.Key(ui.Cmd, ui.KeyA)
	tt.Type("透明背景，保留人物")
	if a.editor.Node("0").Config.Text != "透明背景，保留人物" {
		t.Fatal("native Chinese text not persisted", a.editor.Node("0").Config.Text)
	}
	tt.Key(0, ui.KeyBackspace)
	if len(a.editor.Project.Graph.Nodes) != 3 || a.canUndoDelete() {
		t.Fatal("text editing deleted canvas")
	}
}
func TestNativeMarqueeDragDeleteUndo(t *testing.T) {
	a := testDesktop()
	tt := ui.NewTester(a.view, 1440, 900)
	// Start on the empty canvas and include the first two cards.
	canvas, _ := tt.Find("图片画布")
	tt.Press(canvas.X+26, canvas.Y+49)
	tt.Move(canvas.X+606, canvas.Y+294)
	tt.Release(canvas.X+606, canvas.Y+294)
	if len(a.editor.Selection) != 2 {
		t.Fatal("marquee expected two", a.editor.Selection)
	}
	tt.Press(390, 150)
	tt.Move(430, 180)
	tt.Release(430, 180)
	if len(a.editor.undo) != 1 {
		t.Fatal("drag history length", len(a.editor.undo))
	}
	tt.Key(0, ui.KeyDelete)
	if !a.canUndoDelete() {
		t.Fatal("deletion must offer undo")
	}
	if len(a.editor.Project.Graph.Nodes) != 1 {
		t.Fatal("Delete did not move selected cards to trash")
	}
	tt.ClickAt(900, 400)
	tt.Key(ui.Cmd, ui.KeyZ)
	if len(a.editor.Project.Graph.Nodes) != 3 {
		t.Fatal("undo lost cards")
	}
	tt.SetSize(1100, 740)
	tt.Advance(250 * time.Millisecond)
	if tt.FramePending() {
		t.Fatal("idle view keeps drawing")
	}
}
func TestNativeContextMenuAndZoom(t *testing.T) {
	a := testDesktop()
	tt := ui.NewTester(a.view, 1440, 900)
	tt.RightClick("图片画布")
	if err := tt.ChooseMenuItem("新建图片"); err != nil {
		t.Fatal(err)
	}
	if len(a.editor.Project.Graph.Nodes) != 4 {
		t.Fatal("context create")
	}
	old := a.editor.View.Zoom
	tt.Pinch(600, 300, 2)
	if a.editor.View.Zoom <= old {
		t.Fatal("pinch did not zoom")
	}
}

func TestCornerDragMovesPreviouslyResizedImage(t *testing.T) {
	a := testDesktop()
	e := a.editor
	n := e.Node("0")
	n.Asset, n.CanvasScale = "photo", 2
	e.Assets[n.Asset] = Asset{ID: n.Asset, Width: 1024, Height: 1024}
	original := *n
	tt := ui.NewTester(a.view, 1440, 900)
	tt.ClickAt(390, 140)
	if !e.Selection[n.ID] || nodeRect(*n, e.Assets) != (Rect{X: 0, Y: 20, W: 240, H: 240}) {
		t.Fatal("expected the original fixed-size image to be selected")
	}
	// Grab just inside the bottom-right corner of the selected image.
	canvas, _ := tt.Find("图片画布")
	x, y := canvas.X+287, canvas.Y+307
	tt.Press(x, y)
	tt.Move(x+40, y+30)
	tt.Release(x+40, y+30)
	want := original
	want.Position = Point{40, 50}
	if *e.Node("0") != want || len(e.undo) != 1 {
		t.Fatal("corner drag must only move the image")
	}
	e.Undo()
	if *e.Node("0") != original {
		t.Fatal("undo lost the image or its saved metadata")
	}
}

func TestCanvasZoomButtonsDoNotStartSelection(t *testing.T) {
	a := testDesktop()
	tt := ui.NewTester(a.view, 1440, 900)
	a.editor.Selection["0"] = true
	tt.Frame()
	// Click by the rendered bounds, not accessibility actions which bypass
	// pointer routing and hid this regression in the first release.
	r, ok := tt.Find("放大画布")
	if !ok {
		t.Fatal("missing zoom button")
	}
	before := a.editor.View.Zoom
	tt.ClickAt(r.X+r.W/2, r.Y+r.H/2)
	if a.editor.View.Zoom <= before || !a.editor.Selection["0"] || a.gesture != "" {
		t.Fatalf("zoom click was intercepted: zoom=%v selection=%v gesture=%q", a.editor.View.Zoom, a.editor.Selection, a.gesture)
	}
	r, _ = tt.Find("缩小画布")
	tt.ClickAt(r.X+r.W/2, r.Y+r.H/2)
	if math.Abs(a.editor.View.Zoom-before) > 1e-9 {
		t.Fatal("zoom out did not reverse the zoom step")
	}
	r, _ = tt.Find("适应画布")
	tt.ClickAt(r.X+r.W/2, r.Y+r.H/2)
	if !a.editor.Selection["0"] || a.gesture != "" {
		t.Fatal("fit cleared selection")
	}
	r, _ = tt.Find("放大画布")
	tt.ClickAt(r.X+r.W/2, r.Y+r.H/2)
	before = a.editor.View.Zoom
	tt.Key(0, ui.KeyEnter)
	if a.editor.View.Zoom <= before {
		t.Fatal("keyboard zoom activation failed")
	}
}

func TestSidebarFeedbackSettlesAndRespectsReducedMotion(t *testing.T) {
	for _, reduced := range []bool{false, true} {
		a := testDesktop()
		tt := ui.NewTester(a.view, 1100, 740)
		tt.SetPreferences(ui.Preferences{ReduceMotion: reduced})
		r, _ := tt.Find("回收站")
		tt.ClickAt(r.X+r.W/2, r.Y+r.H/2)
		if a.left != "回收站" {
			t.Fatal("tab action must take effect immediately")
		}
		tt.Advance(300 * time.Millisecond)
		if tt.FramePending() {
			t.Fatal("idle sidebar keeps drawing")
		}
	}
}

func TestSelectionDraftSyncAvoidsPerFrameCopies(t *testing.T) {
	a := perfDesktop(500)
	for _, n := range a.editor.Project.Graph.Nodes {
		a.editor.Selection[n.ID] = true
	}
	a.syncDraft()
	if allocs := testing.AllocsPerRun(100, a.syncDraft); allocs != 0 {
		t.Fatalf("unchanged selection allocated %v times", allocs)
	}
	delete(a.editor.Selection, "0")
	a.syncDraft()
	if len(a.draftSelection) != 499 {
		t.Fatal("selection change not reflected")
	}
	clear(a.editor.Selection)
	a.editor.Selection["1"] = true
	a.syncDraft()
	if a.prompt != a.editor.Node("1").Config.Text {
		t.Fatal("wrong selected prompt")
	}
}

func TestCanvasScrollZoomsAtPointerAndSidebarStaysSeparate(t *testing.T) {
	for _, precise := range []bool{false, true} {
		a := testDesktop()
		tt := ui.NewTester(a.view, 1100, 740)
		r, _ := tt.Find("图片画布")
		anchor := Point{340, 280}
		world := a.editor.World(anchor)
		old := a.editor.View.Zoom
		if precise {
			tt.Scroll(r.X+340, r.Y+280, 0, -80)
		} else {
			tt.Wheel(r.X+340, r.Y+280, 0, -80)
		}
		if a.editor.View.Zoom <= old {
			t.Fatalf("scroll must zoom, precise=%v", precise)
		}
		next := a.editor.World(anchor)
		if math.Abs(world.X-next.X)+math.Abs(world.Y-next.Y) > 1e-7 {
			t.Fatal("zoom anchor moved")
		}
		view := a.editor.View
		tt.Scroll(150, 500, 0, 80)
		if view != a.editor.View {
			t.Fatal("sidebar scroll changed canvas")
		}
	}
}

func TestFloatingPanelAndFullHeightCanvas(t *testing.T) {
	a := testDesktop()
	tt := ui.NewTester(a.view, 1100, 740)
	canvas, ok := tt.Find("图片画布")
	if !ok || canvas != (ui.Rect{X: defaultSidebarWidth, Y: 28, W: 1100 - defaultSidebarWidth - 6, H: 706}) {
		t.Fatal("canvas has a header or right inspector", canvas)
	}
	drag, ok := tt.Find("拖动窗口")
	if !ok || drag.Y != 0 || drag.H != 28 {
		t.Fatal("missing transparent window drag area", drag)
	}
	tt.ClickAt(390, 140)
	prompt, ok := tt.Find("编辑提示词")
	if !ok || prompt.X < canvas.X || prompt.X+prompt.W > canvas.X+canvas.W {
		t.Fatal("settings must float inside the canvas", prompt)
	}
	a.preview(Asset{ID: "image", Width: 1024, Height: 1024})
	tt.Frame()
	tt.Pinch(600, 300, 20)
	if a.previewView.Zoom != 4 {
		t.Fatal("preview must reach 400 percent", a.previewView.Zoom)
	}
}

func TestTrashToastCannotUndoAnotherEditOrCanvas(t *testing.T) {
	a := testDesktop()
	original := a.editor
	original.Selection["0"] = true
	a.moveSelectionToTrash()
	if !a.canUndoDelete() {
		t.Fatal("missing undo after delete")
	}
	a.editor = fixture()
	a.undoDelete()
	if original.Node("0") != nil {
		t.Fatal("undid a different canvas")
	}
	a.editor = original
	original.Undo()
	original.Redo()
	a.undoDelete()
	if original.Node("0") != nil {
		t.Fatal("stale toast undid a later command")
	}
}

func TestWorkspaceAppearance(t *testing.T) {
	a := testDesktop()
	a.left = "历史"
	tt := ui.NewTester(a.view, 1000, 680)
	tt.SetDark(false)
	day := tt.Image().RGBAAt(850, 600)
	tt.SetDark(true)
	night := tt.Image().RGBAAt(850, 600)
	if day.R <= night.R || day == night {
		t.Fatal("gallery did not follow appearance", day, night)
	}
}

func TestAssistantRelief(t *testing.T) {
	a := testDesktop()
	for _, check := range []struct {
		appearance string
		reduced    bool
		background uint8
	}{
		{"light", false, 255},
		{"dark", false, 32},
		{"light", true, 255},
	} {
		a.appearance = check.appearance
		b := &assistantBar{}
		a.assistant = b
		builds, paints := 0, 0
		panel := ui.NewTester(func(c *ui.Context) {
			builds++
			c.Root().Draw(func(*ui.Painter, ui.Rect) { paints++ })
			a.assistantView(c)
		}, assistantWidth, assistantChromeH+assistantTextMinH)
		panel.SetPreferences(ui.Preferences{Accent: ui.Hex("#1f61eb"), ReduceMotion: check.reduced})
		panel.SetFocused(false)
		band := func(left, right, first, last int) []byte {
			img := panel.Image()
			var pixels []byte
			for y := first; y < last; y++ {
				pixels = append(pixels, img.Pix[img.PixOffset(left, y):img.PixOffset(right, y)]...)
			}
			return pixels
		}
		body := func() []byte { return band(180, 570, 40, 120) }
		rest := body()
		for _, area := range [][]byte{band(40, 180, 43, 85), band(390, 570, 55, 124)} {
			var inkPixels int
			for i := 0; i < len(area); i += 4 {
				if area[i] != area[i+1] || area[i] != area[i+2] {
					t.Fatal("ASCII surface introduced color outside the monochrome theme")
				}
				if diff := int(area[i]) - int(check.background); diff > 6 || diff < -6 {
					inkPixels++
				}
			}
			if inkPixels < 100 {
				t.Fatalf("missing ASCII shading on the %s surface: %d pixels", check.appearance, inkPixels)
			}
		}
		b.visible = true
		b.opening++
		panel.Frame()
		atOpen := body()
		built, painted := builds, paints
		panel.Advance(1400 * time.Millisecond)
		// MyGo can redraw only the relief when its bitmap masks change.
		if pixelsEqual := bytes.Equal(atOpen, body()); pixelsEqual != check.reduced || (check.reduced && paints != painted) {
			t.Fatalf("reduced motion %t, identical pixels %t, whole-view repaints %d", check.reduced, pixelsEqual, paints-painted)
		}
		if builds != built || paints-painted > 17 {
			t.Fatal("ASCII motion should only repaint the existing view")
		}
		b.visible = false
		panel.Frame()
		panel.Advance(scrollIdle + assistantASCIIInterval)
		if !bytes.Equal(rest, body()) {
			t.Fatal("assistant did not return to its static surface")
		}
		idle := paints
		panel.Advance(time.Second)
		if paints != idle {
			t.Fatalf("assistant painted %d frames while hidden", paints-idle)
		}
		if _, ok := panel.Find("助手输入"); !ok || a.assistant.contentHeight < 70 {
			t.Fatal("assistant input missing or collapsed")
		}
	}
}

func TestAssistantTextSurfaceAcceptsInputWithoutHoverTint(t *testing.T) {
	a := testDesktop()
	a.assistant = &assistantBar{draft: "第一行", projectID: a.editor.Project.ID}
	tt := ui.NewTester(a.assistantView, assistantWidth, assistantChromeH+assistantTextMinH)
	tt.SetFocused(false)
	r, ok := tt.Find("助手输入")
	if !ok || r.X > 1 || r.W < assistantWidth-2 {
		t.Fatal("input does not reach the sides of the panel", r)
	}
	x, y := r.X+4, r.Y+r.H/2
	atRest := tt.Image().RGBAAt(int(x)+1, int(y))
	tt.Move(x, y)
	tt.Advance(2 * hintDelay)
	if tt.Cursor() != ui.CursorText || tt.Image().RGBAAt(int(x)+1, int(y)) != atRest || tt.HasText("输入消息 · Shift + 回车换行") {
		t.Fatal("hover changes the text surface")
	}
	tt.SetFocused(true)
	tt.ClickAt(x, y)
	if !tt.Focused("助手输入") {
		t.Fatal("clicking the side of the text surface should focus it")
	}
	tt.Key(ui.Cmd, ui.KeyA)
	tt.Type("测试")
	tt.Key(ui.Shift, ui.KeyEnter)
	tt.Compose("zai", 3)
	tt.Type("再来一行")
	if a.assistant.draft != "测试\n再来一行" {
		t.Fatalf("assistant did not preserve multiline composition: %q", a.assistant.draft)
	}
}

func TestAssistantCanvasMenuKeepsDraftAndLoadsTargetChat(t *testing.T) {
	a := testDesktop()
	store, err := openStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	a.store = store
	current := a.editor
	current.Project.Name = "未命名画布"
	target := newEditor(Project{ID: "second", Name: current.Project.Name}, nil, nil)
	deleted := newEditor(Project{ID: "deleted", Name: "回收站画布", DeletedAt: timestamp()}, nil, nil)
	for _, e := range []*Editor{target, deleted} {
		a.editors[e.Project.ID] = e
		a.order = append(a.order, e.Project.ID)
		if err := store.saveAssistantChat(assistantChat{ID: e.Project.ID, ProjectID: e.Project.ID, ThreadID: e.Project.ID + "-thread"}); err != nil {
			t.Fatal(err)
		}
	}
	a.assistant = &assistantBar{draft: "保留的草稿", attachments: []assistantAttachment{{Name: "sample.txt"}}}
	a.setAssistantCanvas(current, false)
	b := a.assistant
	tt := ui.NewTester(a.assistantView, assistantWidth, assistantChromeH+assistantTextMinH+assistantChipH+assistantGap)
	for _, label := range []string{"拖动助手顶部", "拖动助手底部"} {
		r, ok := tt.Find(label)
		if !ok || r.W < 200 || r.H < 30 {
			t.Fatal("missing drag area", label, r)
		}
		tt.Move(r.X+r.W/2, r.Y+r.H/2)
		if tt.Cursor() != ui.CursorGrab {
			t.Fatal("missing drag cursor", label)
		}
	}
	const picker = "选择画布 · 未命名画布"
	if err := tt.Click(picker); err != nil {
		t.Fatal(err)
	}
	menu := []string{"跟随当前画布", "-", "未命名画布 · 1 · 当前画布", "未命名画布 · 2"}
	if !slices.Equal(tt.Menu(), menu) {
		t.Fatal(tt.Menu())
	}
	if err := tt.ChooseMenuItem("未命名画布 · 2"); err != nil {
		t.Fatal(err)
	}
	if a.editor != current || b.projectID != target.Project.ID || !b.pinnedCanvas || b.chat.ThreadID != "second-thread" {
		t.Fatal("assistant did not load the selected canvas's chat", b.chat)
	}
	if b.draft != "保留的草稿" || len(b.attachments) != 1 || b.attachments[0].Name != "sample.txt" || !tt.Focused("助手输入") {
		t.Fatal("selection did not preserve the composer")
	}
	b.busy = true
	a.setAssistantCanvas(current, false)
	if b.projectID != target.Project.ID {
		t.Fatal("destination can change during a send")
	}
	b.busy = false
	tt.Frame()
	if err := tt.Click(picker); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("跟随当前画布"); err != nil {
		t.Fatal(err)
	}
	if b.pinnedCanvas || b.projectID != current.Project.ID || b.chat.ThreadID != "" {
		t.Fatal("follow should reload the active canvas without reusing the other thread", b.chat)
	}
}

func TestIconCommandShowsHelpOnHoverAndKeyboardFocus(t *testing.T) {
	const tip = "导入图片 · ⌘ O，也可拖入画布"
	const nextTip = "从素材库选择图片"
	clicked := false
	tt := ui.NewTester(func(c *ui.Context) {
		defer beginHints(c)()
		c.SetTheme(workspaceTheme(c, ""))
		ui.Row(c).Padding(40).Children(func() {
			primaryIcon(c, "upload", "导入图片", tip, false, func() { clicked = true })
			iconButton(c, "library", "素材库", func() {}, nextTip)
		})
	}, 450, 180)
	b, ok := tt.Find("导入图片")
	if !ok || tt.HasText(tip) {
		t.Fatal("the icon must be named and its explanation hidden at rest")
	}
	tt.Move(b.X+b.W/2, b.Y+b.H/2)
	tt.Advance(hintDelay / 2)
	if tt.HasText(tip) {
		t.Fatal("tooltip shown before first dwell")
	}
	tt.Advance(hintDelay/2 + time.Millisecond)
	if tt.Cursor() != ui.CursorPointer || !tt.HasText(tip) {
		t.Fatal("missing hover help")
	}
	next, _ := tt.Find("素材库")
	tt.Move(400, 150)
	tt.Advance(16 * time.Millisecond)
	tt.Move(next.X+next.W/2, next.Y+next.H/2)
	if tt.HasText(tip) || !tt.HasText(nextTip) {
		t.Fatal("adjacent help repeated the initial delay")
	}
	tt.Move(400, 150)
	if tt.HasText(tip) || tt.HasText(nextTip) {
		t.Fatal("help remained after leaving")
	}
	tt.Key(0, ui.KeyTab)
	if !tt.Focused("导入图片") || !tt.HasText(tip) {
		t.Fatal("missing keyboard help")
	}
	tt.Key(0, ui.KeyEnter)
	if !clicked {
		t.Fatal("keyboard did not activate the icon")
	}
}
