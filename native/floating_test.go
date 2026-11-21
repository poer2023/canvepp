package main

import (
	"reflect"
	"runtime"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func TestPanelDragCoalescedNativeEventsDoNotOvershoot(t *testing.T) {
	a := testDesktop()
	a.canvasW, a.canvasH = 1130, 888
	a.panel = panelPlacement{X: 600, Y: 60, Open: true, Placed: true}
	a.editor.Selection["0"] = true
	// macOS may deliver multiple moves against the same rendered header.
	header := ui.Rect{X: 915, Y: 67, W: 230, H: 44}
	a.panelInput(ui.InputEvent{Kind: ui.InputPointerDown, Button: 0, X: 30, Y: 20}, header)
	a.panelInput(ui.InputEvent{Kind: ui.InputPointerMove, X: 10, Y: 25}, header)
	a.panelInput(ui.InputEvent{Kind: ui.InputPointerMove, X: -10, Y: 30}, header)
	if a.panel.X != 560 || a.panel.Y != 70 {
		t.Fatal("coalesced events accumulated stale deltas", a.panel)
	}
	// Once layout catches up, local coordinates change but the global point does not.
	header.X -= 40
	header.Y += 10
	a.panelInput(ui.InputEvent{Kind: ui.InputPointerMove, X: 30, Y: 20}, header)
	if a.panel.X != 560 || a.panel.Y != 70 {
		t.Fatal("layout catch-up moved panel again", a.panel)
	}
}

func TestFloatingDragDockResizeAndSelectionIsolation(t *testing.T) {
	a := testDesktop()
	tt := ui.NewTester(a.view, 1440, 900)
	tt.ClickAt(390, 140)
	before := cloneGraph(a.editor.Project.Graph)
	drag := func(x, y float32) {
		t.Helper()
		canvas, _ := tt.Find("图片画布")
		header, ok := tt.Find("拖动图片设置")
		if !ok {
			t.Fatal("missing drag header")
		}
		panel, _ := tt.Find("悬浮图片设置")
		sX, sY := header.X+30, header.Y+20
		dX, dY := canvas.X+x+(sX-panel.X), canvas.Y+y+(sY-panel.Y)
		tt.Press(sX, sY)
		for i := 1; i <= 5; i++ {
			f := float32(i) / 5
			tt.Move(sX+(dX-sX)*f, sY+(dY-sY)*f)
		}
		tt.Release(dX, dY)
	}
	// Free placement does not snap to a distant edge.
	drag(300, 100)
	if a.panel.DockX != "" || a.panel.DockY != "" || a.panel.X != 300 || a.panel.Y != 100 {
		t.Fatal("free drag", a.panel)
	}
	bottom := panelBounds(float32(a.canvasW), float32(a.canvasH), a.panelHeight())
	nearRight := bottom.X + bottom.W - 8
	for _, target := range []struct {
		x, y   float32
		dx, dy string
	}{
		{18, 100, "left", ""}, {nearRight, 100, "right", ""}, {300, 8, "", "top"}, {300, bottom.Y + bottom.H - 5, "", "bottom"},
	} {
		drag(target.x, target.y)
		if a.panel.DockX != target.dx || a.panel.DockY != target.dy {
			t.Fatalf("dock got %+v want %+v", a.panel, target)
		}
	}
	if !reflect.DeepEqual(before, a.editor.Project.Graph) || len(a.editor.Selection) != 1 || !a.editor.Selection["0"] || a.gesture != "" {
		t.Fatal("panel drag changed content/selection")
	}
	// Keep the chosen edge when the window changes size.
	drag(nearRight, bottom.Y+bottom.H-5)
	tt.SetSize(1000, 680)
	tt.Frame()
	canvas, _ := tt.Find("图片画布")
	panel, _ := tt.Find("悬浮图片设置")
	if panel.X < canvas.X || panel.Y < canvas.Y || panel.X+panel.W > canvas.X+canvas.W || panel.Y+panel.H > canvas.Y+canvas.H {
		t.Fatal("panel escaped small window", panel, canvas)
	}
	view := a.editor.View
	prompt, _ := tt.Find("编辑提示词")
	tt.Scroll(prompt.X+30, prompt.Y+30, 0, 100)
	if a.editor.View != view {
		t.Fatal("panel scroll zoomed canvas")
	}
	for _, label := range []string{"收起图片设置", "展开图片设置", "关闭图片设置"} {
		r, ok := tt.Find(label)
		if !ok {
			t.Fatal(label)
		}
		tt.ClickAt(r.X+r.W/2, r.Y+r.H/2)
		if a.panelDragging || a.gesture != "" {
			t.Fatal("panel button intercepted", label)
		}
	}
	if _, ok := tt.Find("显示图片设置"); ok {
		t.Fatal("removed sidebar button is still present")
	}
	tt.ClickAt(390, 140)
	if !a.panel.Open {
		t.Fatal("clicking the selected card did not reopen image settings")
	}
	tt.Advance(800 * time.Millisecond)
	if tt.FramePending() {
		t.Fatal("floating UI renders while idle")
	}
}

func TestSettingsReturnsToSameCanvasWithoutSideEffects(t *testing.T) {
	a := testDesktop()
	tt := ui.NewTester(a.view, 1440, 900)
	tt.ClickAt(390, 140)
	a.left = "历史"
	tt.Frame()
	graph, view := cloneGraph(a.editor.Project.Graph), a.editor.View
	if err := tt.Click("打开设置"); err != nil {
		t.Fatal(err)
	}
	if !a.settingsOpen || tt.HasText("模型额度") {
		t.Fatal("the workspace should open settings in its own window")
	}
	settings := ui.NewTester(a.settingsView, 850, 680)
	for _, name := range []string{"ChatGPT 账户", "登录", "设置：数据", "设置：快捷键"} {
		if r, ok := settings.Find(name); !ok || r.H <= 0 {
			t.Fatal("missing setting", name)
		}
	}
	if a.accountBusy || len(a.runs) != 0 {
		t.Fatal("opening settings performed account/generation work")
	}
	if err := settings.Click("深色模式"); err != nil || a.appearance != "dark" {
		t.Fatal("dark thumbnail should select the dark theme", err, a.appearance)
	}
	if err := settings.Click("跟随系统"); err != nil || a.appearance != "" {
		t.Fatal("system thumbnail should restore system theme", err, a.appearance)
	}
	a.chatGPTAccount = &chatGPTAccount{Email: "studio@example.com"}
	a.funding.apiStatus = "已保存"
	settings.Frame()
	if !settings.HasText("studio@example.com") || settings.HasText("登录") {
		t.Fatal("signed-in account should replace the login action")
	}
	if err := settings.Click("管理密钥"); err != nil {
		t.Fatal(err)
	}
	if err := settings.Click("移除密钥…"); err != nil || !settings.HasText("移除此设备保存的 OpenAI API Key？") {
		t.Fatal("removing a key should require confirmation", err)
	}
	settings.Key(0, ui.KeyEscape)
	if !a.settingsOpen || a.funding.keyMenu {
		t.Fatal("Escape should dismiss the key popover first")
	}
	a.funding.keyDraft = "fixture-draft"
	settings.Key(0, ui.KeyEscape)
	if a.settingsOpen || a.funding.keyDraft != "" || a.left != "历史" ||
		a.editor.View != view || !reflect.DeepEqual(graph, a.editor.Project.Graph) || !a.editor.Selection["0"] {
		t.Fatal("closing settings did not preserve the workspace or clear the key draft")
	}
	tt.Key(ui.Cmd, ui.KeyComma)
	if !a.settingsOpen {
		t.Fatal("settings shortcut")
	}
}

func TestSettingsFormFitsAndScrollsAtMinimumSize(t *testing.T) {
	a := testDesktop()
	a.loginError = "检查连接失败"
	a.hotkeyEnabled, a.hotkeyStatus = true, "需要输入监控权限"
	a.funding.Image = apiFunding
	a.funding.apiStatus = "未配置"
	settings := ui.NewTester(a.settingsView, settingsMinWidth, settingsMinHeight)
	bar := ui.TitleBar{Height: 32, Right: 138}
	settings.SetTitleBar(bar)
	visible := func(label string) ui.Rect {
		t.Helper()
		r, ok := settings.Find(label)
		if !ok || r.H <= 0 || r.W <= 0 {
			t.Fatalf("missing %s: %v", label, r)
		}
		return r
	}
	rail := visible("设置侧栏")
	theme := visible("外观")
	drag := visible("拖动设置窗口")
	if drag.Y+drag.H != rail.Y || drag.X+drag.W > settingsMinWidth-bar.Right {
		t.Fatal("drag region should meet the sidebar and clear the window controls", drag, rail)
	}
	if theme.X != rail.X+rail.W+pageInset || theme.Y-rail.Y > 24 {
		t.Fatal("settings content needs to start at the workspace page inset", theme, rail)
	}
	visible("登录")
	visible("检查连接失败")
	for _, name := range []string{"跟随系统", "浅色模式", "深色模式"} {
		tile := visible(name)
		if tile.X < theme.X || tile.X+tile.W > settingsMinWidth-canvasInset {
			t.Fatal("preview tile exceeds settings pane", name, tile)
		}
	}
	for _, copy := range []string{"需要权限", "Codex 内置生图", textModel, apiImageModel} {
		if settings.HasText(copy) {
			t.Fatalf("unexpected inline settings copy: %s", copy)
		}
	}
	settings.Scroll(320, 300, 0, 2000)
	label, source := visible("生图额度"), visible("生图额度来源")
	if label.X+label.W > source.X || source.X+source.W > settingsMinWidth-canvasInset {
		t.Fatal("row label and selector need to fit at minimum width", label, source)
	}
	if err := settings.Click("设置：快捷键"); err != nil {
		t.Fatal(err)
	}
	firstShortcutGroup := "Adobe 快照"
	if runtime.GOOS == "windows" {
		firstShortcutGroup = "文件资源管理器"
		visible("在文件资源管理器按空格预览")
	}
	if heading := visible(firstShortcutGroup); heading.X != theme.X || heading.Y != theme.Y {
		t.Fatal("shortcut groups should align with the general settings", heading, theme)
	}
	settings.Scroll(320, 300, 0, 2000)
	visible("吸附图片面板")
	if err := settings.Click("设置：数据"); err != nil {
		t.Fatal(err)
	}
	if heading := visible("备份与恢复"); heading.X != theme.X || heading.Y != theme.Y {
		t.Fatal("data groups should align with the general settings", heading, theme)
	}
	export, merge := visible("导出备份"), visible("导入画布或备份")
	restore := visible("恢复到新资料库")
	if export.Y >= merge.Y || merge.Y >= restore.Y {
		t.Fatal("backup, merge and restore need their own form rows", export, merge, restore)
	}
}

func TestPanelPersistenceDoesNotWriteProjects(t *testing.T) {
	s, err := openStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	q := newSaveQueue(s, func(_ string, _ int, err error) {
		if err != nil {
			t.Error(err)
		}
	})
	defer close(q.wake)
	p := panelPlacement{X: 180, Y: 90, DockY: "bottom", Open: true, Placed: true}
	for i := 0; i < 20; i++ {
		p.X = float32(i + 180)
		q.PutUI(p)
	}
	if err = q.Flush(); err != nil {
		t.Fatal(err)
	}
	if got := s.loadPanel(); got != p {
		t.Fatal("persisted wrong position", got, p)
	}
	var count int
	if err = s.DB.QueryRow("SELECT COUNT(*) FROM projects").Scan(&count); err != nil || count != 0 {
		t.Fatal("layout mutated project data", count, err)
	}
}

func TestRememberedFreePositionSurvivesFirstLayoutAndSettingsReturn(t *testing.T) {
	a := testDesktop()
	a.panel = panelPlacement{X: 350, Y: 100, Open: true, Placed: true}
	a.editor.Selection["0"] = true
	tt := ui.NewTester(a.view, 1440, 900)
	tt.Frame()
	if a.panel.X != 350 || a.panel.Y != 100 {
		t.Fatal("initial zero bounds discarded remembered position", a.panel)
	}
	tt.Click("打开设置")
	tt.Click("返回画布")
	tt.Frame()
	if a.panel.X != 350 || a.panel.Y != 100 {
		t.Fatal("settings return discarded position", a.panel)
	}
}
