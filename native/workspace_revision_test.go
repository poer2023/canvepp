package main

import (
	"math"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func TestWideSettingsCenterTheContentColumn(t *testing.T) {
	a := testDesktop()
	for _, width := range []int{640, 850, 1440, 1800} {
		tt := ui.NewTester(a.settingsView, width, 690)
		column, ok := tt.Find("设置正文")
		paneWidth := float32(width) - settingsSidebarWidth - canvasInset
		wantWidth := min(680, paneWidth-2*pageInset)
		wantX := settingsSidebarWidth + (paneWidth-wantWidth)/2
		if !ok || math.Abs(float64(column.X-wantX)) > 2 || column.W != wantWidth {
			t.Fatalf("settings content at %d is %v, want x=%v and w=%v", width, column, wantX, wantWidth)
		}
	}
}

func TestPictureTimelineSelectionUsesTheSamePreview(t *testing.T) {
	a := testDesktop()
	e := a.editor
	for i, example := range []struct{ name, at string }{
		{"今天的图片", timestamp()},
		{"近期的图片", time.Now().AddDate(0, 0, -3).UTC().Format(time.RFC3339Nano)},
		{"更早的图片", time.Now().AddDate(0, 0, -9).UTC().Format(time.RFC3339Nano)},
	} {
		n := &e.Project.Graph.Nodes[i]
		n.Asset = n.ID
		e.Assets[n.Asset] = Asset{ID: n.Asset, ProjectID: e.Project.ID, NodeID: n.ID, Name: example.name + ".png", CreatedAt: example.at, Width: 800, Height: 500}
	}
	tt := ui.NewTester(a.view, 1440, 900)
	for _, label := range []string{allPictures, "全部图片布局", "时间线"} {
		if err := tt.Click(label); err != nil {
			t.Fatal(err)
		}
	}
	for _, title := range []string{"今天", "近 7 天", "更早"} {
		if !tt.HasText(title) {
			t.Fatal("missing timeline group", title)
		}
	}
	if err := tt.Click("选择图片：今天的图片"); err != nil || a.previewOpen {
		t.Fatal("timeline click should select only", err)
	}
	tt.Key(0, ui.KeySpace)
	if !a.previewOpen || a.previewAsset.ID != e.Project.Graph.Nodes[0].Asset {
		t.Fatal("timeline did not open the selected original")
	}
	tt.Key(0, ui.KeyEscape)
	for _, label := range []string{"全部图片布局", "齐边拼排"} {
		if err := tt.Click(label); err != nil {
			t.Fatal(err)
		}
	}
	if a.overview.selected == "" || a.overview.layout == pictureTimeline {
		t.Fatal("switching layout lost the current picture")
	}
}

func TestCanvasTextToolWritesAndRestoresText(t *testing.T) {
	a := testDesktop()
	a.createCanvas()
	e := a.editor
	tt := ui.NewTester(a.view, 1440, 900)
	canvas, _ := tt.Find("图片画布")
	if err := tt.Click("文字工具"); err != nil {
		t.Fatal(err)
	}
	x, y := canvas.X+110, canvas.Y+114
	tt.ClickAt(x, y)
	if !tt.Focused("编辑画布文字") {
		t.Fatal("placing text failed to focus the editor")
	}
	field, _ := tt.Find("编辑画布文字")
	if field.X != x || field.Y != y || field.H >= 64 || field.W >= 160 {
		t.Fatal("text editor did not start inline at the clicked position", field)
	}
	tt.Type("初版说明")
	tt.Key(0, ui.KeyEnter)
	tt.Type("第二行")
	multiline, _ := tt.Find("编辑画布文字")
	if multiline.Y != field.Y || multiline.X != field.X || multiline.H <= field.H {
		t.Fatal("multiline text did not grow in place", multiline, field)
	}
	tt.ClickAt(x+280, y+140)
	if len(e.Project.Graph.Nodes) != 1 || len(e.selectedImages()) != 0 {
		t.Fatal("text was inserted as an image")
	}
	n := e.Project.Graph.Nodes[0]
	if n.Config.Text != "初版说明\n第二行" || n.TextSize.Y < 50 || a.prompt != "" {
		t.Fatal("text, dimensions or the creation prompt was lost", n, a.prompt)
	}
	tt.RightClickAt(x+5, y+5)
	if err := tt.ChooseMenuItem("编辑文字"); err != nil {
		t.Fatal(err)
	}
	tt.Key(ui.Cmd, ui.KeyA)
	tt.Type("修订后的文字")
	tt.Key(ui.Cmd, ui.KeyEnter)
	if e.Node(n.ID).Config.Text != "修订后的文字" {
		t.Fatal("edit did not commit")
	}
	tt.ClickAt(x+270, y+160)
	tt.Key(0, ui.KeyT)
	tt.ClickAt(x+200, y+16)
	tt.Type("取消草稿")
	tt.Key(0, ui.KeyEscape)
	if a.textEdit != nil || len(e.Project.Graph.Nodes) != 1 || e.Node(n.ID).Config.Text != "修订后的文字" {
		t.Fatal("Escape saved a text draft")
	}
	e.Undo()
	if e.Node(n.ID).Config.Text != n.Config.Text {
		t.Fatal("undo lost the original text")
	}
	e.Redo()
	e.Selection[n.ID] = true
	a.moveSelectionToTrash()
	if e.Node(n.ID) != nil || len(e.Trash) == 0 {
		t.Fatal("text did not reach the trash")
	}
	e.Restore(e.Trash[len(e.Trash)-1].ID)
	s := testStore(t)
	e.Project.Graph.View = e.View
	if err := s.SaveWithOperations(e.Project, e.Trash, e.Events); err != nil {
		t.Fatal(err)
	}
	reopened, err := newDesktop(s)
	if err != nil {
		t.Fatal(err)
	}
	text := reopened.editor.Node(n.ID)
	if text == nil || text.Kind != textNodeKind || text.Config.Text != "修订后的文字" || !strings.Contains(reopened.editor.Events[0].Text, "文字") {
		t.Fatal("saved text or its history could not be reloaded")
	}
}

func TestInlineCanvasTextNearAnEdgeKeepsWorldPosition(t *testing.T) {
	a := testDesktop()
	a.createCanvas()
	e := a.editor
	e.View.Zoom = .7
	tt := ui.NewTester(a.view, 1000, 680)
	canvas, _ := tt.Find("图片画布")
	tt.ClickAt(canvas.X+75, canvas.Y+55)
	tt.Key(0, ui.KeyT)
	x, y := canvas.W-20, canvas.H-105
	at := e.World(Point{float64(x), float64(y)})
	tt.ClickAt(canvas.X+x, canvas.Y+y)
	tt.Type("画布边缘的文字")
	field, ok := tt.Find("编辑画布文字")
	if !ok || field.X+field.W > canvas.X+canvas.W+1 || !tt.Focused("编辑画布文字") {
		t.Fatal("the inline editor was clipped at the canvas edge", field)
	}
	tt.Key(ui.Cmd, ui.KeyEnter)
	if len(e.Project.Graph.Nodes) != 1 || e.Project.Graph.Nodes[0].Position != at || e.View.Zoom != .7 {
		t.Fatal("revealing text changed its world position or the zoom")
	}
}

func TestVisibleHistoryIncludesPreviouslyFilteredRecords(t *testing.T) {
	s := testStore(t)
	now := time.Now()
	project := Project{ID: "legacy", Name: "旧版记录"}
	old := now.AddDate(0, 0, -1).UTC().Format(time.RFC3339Nano)
	events := []Operation{
		{ID: "move", ProjectID: project.ID, Action: "move", Text: "移动了 1 张图片", At: old, UpdatedAt: old},
		{ID: "create", ProjectID: project.ID, Action: "create", Text: "新建了 1 张图片", At: timestamp(), UpdatedAt: timestamp()},
	}
	if err := s.SaveWithOperations(project, nil, events); err != nil {
		t.Fatal(err)
	}
	a, err := newDesktop(s)
	if err != nil {
		t.Fatal(err)
	}
	tt := ui.NewTester(a.view, 1440, 900)
	if err := tt.Click("打开操作历史"); err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{"今天", "昨天", "移动了 1 张图片", "新建了 1 张图片"} {
		if !tt.HasText(text) {
			t.Fatal("history is missing", text)
		}
	}
	if got := summaryEvents(a.editor.Events); len(got) != 1 || got[0].Action != "create" {
		t.Fatal("legacy moves entered the summary again", got)
	}
}
