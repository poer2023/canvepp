package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func TestOperationPagingKeepsSameTimestampEventsAndCancellation(t *testing.T) {
	s := testStore(t)
	e := fixture()
	at := timestamp()
	for i := 0; i < 1001; i++ {
		e.Events = append(e.Events, Operation{ID: fmt.Sprintf("op-%04d", i), ProjectID: e.Project.ID, Action: "generation", Text: "生成完成", At: at, UpdatedAt: at})
	}
	if err := s.SaveWithOperations(e.Project, e.Trash, e.Events); err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	before := Operation{}
	for {
		page, err := s.OperationPage(e.Project.ID, before)
		if err != nil {
			t.Fatal(err)
		}
		for _, op := range page {
			if seen[op.ID] {
				t.Fatal("duplicate page item", op.ID)
			}
			seen[op.ID] = true
		}
		if len(page) < 500 {
			break
		}
		before = page[0]
	}
	if len(seen) != 1001 {
		t.Fatal("older logs lost", len(seen))
	}
	r := NativeRun{ID: "cancel-log", ProjectID: e.Project.ID, NodeID: "0", Status: "queued"}
	if err := s.QueueRuns([]NativeRun{r}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := s.CancelRun(r.ID); err != nil {
			t.Fatal(err)
		}
	}
	var count int
	if err := s.DB.QueryRow("SELECT count(*) FROM operation_events WHERE id=?", "run/"+r.ID+"/cancelled").Scan(&count); err != nil || count != 1 {
		t.Fatal("cancel receipt not exactly once", count, err)
	}
}

func TestOperationLogKeepsKeyChangesAndPersistsQueue(t *testing.T) {
	s := testStore(t)
	e := fixture()
	q := newSaveQueue(s, func(_ string, _ int, err error) {
		if err != nil {
			t.Error(err)
		}
	})
	e.OnChange = func() { q.Put(e) }
	for i := 0; i < 40; i++ {
		n := *e.Node("0")
		n.Config.Text += "字"
		e.Edit(n)
	}
	if len(e.Events) != 0 {
		t.Fatal("typing became keystroke log", len(e.Events))
	}
	e.Selection = map[string]bool{"0": true, "1": true}
	e.BeginDrag()
	for i := 0; i < 500; i++ {
		e.MoveDrag(float64(i), 20)
	}
	if len(e.Events) != 0 {
		t.Fatal("pointer events logged")
	}
	e.EndDrag()
	e.Delete()
	e.Undo()
	e.Redo()
	if err := q.Flush(); err != nil {
		t.Fatal(err)
	}
	events, err := s.Operations(e.Project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 3 {
		t.Fatal("queue lost or duplicated events", len(events))
	}
	for i, action := range []string{"trash", "undo", "redo"} {
		if events[i].Action != action {
			t.Fatal(i, events[i])
		}
	}
	projects, _ := s.Projects()
	if len(projects[0].Graph.Nodes) != 1 {
		t.Fatal("log and graph diverged")
	}
	reopened, err := newDesktop(s)
	if err != nil {
		t.Fatal(err)
	}
	if len(reopened.editor.Events) != 3 {
		t.Fatal("logs did not survive reopening")
	}
	backup := filepath.Join(t.TempDir(), "backup")
	if err = s.Backup(backup); err != nil {
		t.Fatal(err)
	}
	copyStore := testStore(t)
	if _, err = copyStore.ImportLegacy(backup); err != nil {
		t.Fatal(err)
	}
	copied, _ := copyStore.Operations(e.Project.ID)
	if len(copied) != 3 {
		t.Fatal("backup dropped logs")
	}
}
func TestPermanentDeletePreservesSharedAssetsAndCannotUndo(t *testing.T) {
	s := testStore(t)
	e := fixture()
	asset, err := s.ImportImage(tinyPNG(), e.Project.ID, "0", "shared.png", "import", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	e.Assets[asset.ID] = asset
	e.Node("0").Asset = asset.ID
	e.Node("1").Asset = asset.ID
	e.Selection = map[string]bool{"0": true, "2": true}
	e.Delete()
	if err = s.SaveWithOperations(e.Project, e.Trash, e.Events); err != nil {
		t.Fatal(err)
	}
	old := e.Project
	old.Graph = cloneGraph(e.Project.Graph)
	oldTrash := append([]Trash(nil), e.Trash...)
	result, err := s.PurgeCard(e.Project.ID, "0")
	if err != nil {
		t.Fatal(err)
	}
	e.forgetPurged("0", result)
	if len(result.Removed) != 0 {
		t.Fatal("deleted a shared version")
	}
	path, _ := s.AssetPath(asset)
	if _, err = os.Stat(path); err != nil {
		t.Fatal("shared file gone")
	}
	e.Undo()
	if e.Node("0") != nil || e.Node("2") == nil {
		t.Fatal("purge broke mixed undo")
	}
	// An old trash snapshot at a newer revision still cannot resurrect the entry.
	old.Revision = result.Project.Revision + 1
	if err = s.Save(old, oldTrash); err != nil {
		t.Fatal(err)
	}
	tr, _ := s.Trashed(e.Project.ID)
	for _, v := range tr {
		if v.Node.ID == "0" {
			t.Fatal("stale snapshot resurrected trash")
		}
	}
	n := newNode(e.Project.ID, Point{})
	n.ID = "0"
	old.Graph.Nodes = append(old.Graph.Nodes, n)
	old.Revision++
	if s.Save(old, nil) == nil {
		t.Fatal("stale graph revived a permanently deleted card")
	}
	purged, _ := s.PurgedNodes(e.Project.ID)
	if !purged["0"] {
		t.Fatal("tombstone missing")
	}
}
func TestPermanentDeleteRemovesUnsharedFileAndThumbnail(t *testing.T) {
	s := testStore(t)
	e := fixture()
	asset, err := s.ImportImage(tinyPNG(), e.Project.ID, "0", "only.png", "import", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	e.Assets[asset.ID] = asset
	e.Node("0").Asset = asset.ID
	e.Selection = map[string]bool{"0": true}
	e.Delete()
	s.Save(e.Project, e.Trash)
	thumb := filepath.Join(s.Dir, "thumbnails", asset.SHA+"-512.png")
	os.WriteFile(thumb, tinyPNG(), 0600)
	result, err := s.PurgeCard(e.Project.ID, "0")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Removed) != 1 {
		t.Fatal(result)
	}
	path, _ := s.AssetPath(asset)
	for _, p := range []string{path, thumb} {
		if _, err = os.Stat(p); !os.IsNotExist(err) {
			t.Fatal("unshared content survived", p, err)
		}
	}
	if result.CleanupError != "" {
		t.Fatal(result.CleanupError)
	}
}
func TestPurgeKeepsProvenanceAndRejectsUnfinishedGeneration(t *testing.T) {
	s := testStore(t)
	e := fixture()
	asset, _ := s.ImportImage(tinyPNG(), e.Project.ID, "0", "parent.png", "import", "", nil)
	child, _ := s.ImportImage(tinyPNG(), e.Project.ID, "1", "child.png", "native", "", []string{asset.ID})
	e.Assets[asset.ID] = asset
	e.Assets[child.ID] = child
	e.Node("0").Asset = asset.ID
	e.Node("1").Asset = child.ID
	e.Selection = map[string]bool{"0": true}
	e.Delete()
	s.Save(e.Project, e.Trash)
	run := NativeRun{ID: uuid(), ProjectID: e.Project.ID, NodeID: "0", Status: "unknown"}
	s.SaveRun(run)
	if _, err := s.PurgeCard(e.Project.ID, "0"); err == nil {
		t.Fatal("purged unresolved task")
	}
	run.Status = "failed"
	s.SaveRun(run)
	result, err := s.PurgeCard(e.Project.ID, "0")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Removed) != 0 {
		t.Fatal("cut connection lost parent provenance")
	}
}
func TestTripleSpaceIgnoresTypingHoldRepeatAndPan(t *testing.T) {
	tests := []struct {
		name  string
		input [][2]int
		want  bool
	}{
		{"three taps", [][2]int{{1, 0}, {2, 40}, {1, 150}, {2, 200}, {1, 350}, {2, 400}}, true},
		{"typing", [][2]int{{1, 0}, {2, 40}, {0, 90}, {1, 150}, {2, 200}, {1, 350}, {2, 400}}, false},
		{"held", [][2]int{{1, 0}, {2, 300}, {1, 350}, {2, 400}, {1, 500}, {2, 550}}, false},
		{"repeat", [][2]int{{1, 0}, {1, 40}, {1, 80}, {2, 100}}, false},
		{"pan", [][2]int{{1, 0}, {0, 20}, {2, 40}, {1, 150}, {2, 200}, {1, 350}, {2, 400}}, false},
		{"slow", [][2]int{{1, 0}, {2, 40}, {1, 650}, {2, 690}, {1, 800}, {2, 850}}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var seq tripleSpace
			got := false
			for _, input := range tc.input {
				got = seq.feed(input[0], time.Duration(input[1])*time.Millisecond) || got
			}
			if got != tc.want {
				t.Fatal(got)
			}
		})
	}
}
func TestWorkspaceSettingsAndLogsNavigationKeepsContent(t *testing.T) {
	a := testDesktop()
	tt := ui.NewTester(a.view, 1000, 680)
	tt.Click("打开设置")
	settings := ui.NewTester(a.settingsView, 850, 680)
	for _, tab := range []string{"通用", "数据", "快捷键"} {
		if err := settings.Click("设置：" + tab); err != nil {
			t.Fatal(err)
		}
	}
	settings.Key(ui.Cmd, ui.KeyComma)
	if a.settingsOpen {
		t.Fatal("settings shortcut did not close the dialog")
	}
	tt.Click("添加侧栏页面")
	tt.ChooseMenuItem("操作历史")
	if _, ok := tt.Find("暂无操作记录"); !ok {
		t.Fatal("missing truthful empty state")
	}
	if a.summaryBusy {
		t.Fatal("opening logs spent model quota")
	}
	a.editor.Selection = map[string]bool{"0": true}
	a.editor.Delete()
	tt.Frame()
	if _, ok := tt.Find("将 1 张图片移入回收站"); !ok {
		t.Fatal("log not updated")
	}
	previous := a.editor
	if err := tt.Click("新建画布"); err != nil {
		t.Fatal(err)
	}
	if a.dock.active != dockOperations || !tt.HasText("暂无操作记录") || tt.HasText("将 1 张图片移入回收站") {
		t.Fatal("history did not follow the current canvas")
	}
	a.chooseSession(previous)
	tt.Frame()
	if !tt.HasText("将 1 张图片移入回收站") {
		t.Fatal("switching back lost the operation")
	}
	tt.Click("关闭当前侧栏页面")
	tt.Advance(time.Second)
	if tt.FramePending() {
		t.Fatal("closed controls redraw continuously")
	}
}
func TestCodexTextUsesSelectedModelAndResumesWithoutDuplicateSubmission(t *testing.T) {
	log := fixtureCodex(t)
	modelLog := log + ".model"
	t.Setenv("CANVEPP_RPC_MODEL_LOG", modelLog)
	t.Setenv("CANVEPP_TEXT_FIXTURE", "1")
	s := testStore(t)
	reply, err := requestCodexText(context.Background(), s, modelChoice{}, "", logInstructions, "只总结这些动作", false, nil)
	if err != nil || reply.Text != "已导入两张图片，随后撤销了删除。" {
		t.Fatal(reply, err)
	}
	next := modelChoice{"gpt-6.2-astra", "high"}
	reply, err = requestCodexText(context.Background(), s, next, reply.ThreadID, assistantInstructions, "继续", false, nil)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(log)
	if countLines(string(b), "thread/start") != 1 || countLines(string(b), "thread/resume") != 1 || countLines(string(b), "turn/start") != 2 {
		t.Fatal(string(b))
	}
	models, err := os.ReadFile(modelLog)
	if err != nil || !strings.Contains(string(models), "turn/start gpt-6.1-terra medium\n") ||
		!strings.Contains(string(models), "turn/start gpt-6.2-astra high\n") {
		t.Fatal("summary/resume model and effort mismatch", string(models), err)
	}
	t.Setenv("CANVEPP_RPC_DISCONNECT", "1")
	_, err = requestCodexText(context.Background(), s, next, reply.ThreadID, assistantInstructions, "不要重发", false, nil)
	if err == nil {
		t.Fatal("missing unknown receipt")
	}
	b, _ = os.ReadFile(log)
	if countLines(string(b), "turn/start") != 3 {
		t.Fatal("repeated uncertain turn")
	}
}
func TestSelectedTextModelUnavailableDoesNotFallbackOrStartTurn(t *testing.T) {
	log := fixtureCodex(t)
	t.Setenv("CANVEPP_RPC_NO_MODELS", "1")
	s := testStore(t)
	_, err := requestCodexText(context.Background(), s, modelChoice{}, "", logInstructions, "测试", true, nil)
	if err == nil || !strings.Contains(err.Error(), textModel) {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(log)
	if strings.Contains(string(b), "turn/start") || strings.Contains(string(b), "thread/start") {
		t.Fatal("silently selected another model")
	}
}

func TestLiveTextSummary(t *testing.T) {
	if os.Getenv("CANVEPP_LIVE_TEXT_CHECK") != "1" {
		t.Skip("explicit local text-only acceptance")
	}
	s := testStore(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	result, err := requestCodexText(ctx, s, modelChoice{}, "", logInstructions, `[{"action":"import","text":"导入了 2 张图片"},{"action":"trash","text":"将 1 张图片移入回收站"},{"action":"undo","text":"撤销：将 1 张图片移入回收站"}]`, true, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.Text == "" {
		t.Fatal("empty model response")
	}
	if path := os.Getenv("CANVEPP_TEXT_RECEIPT"); path != "" {
		if err = os.WriteFile(path, encode(map[string]any{"model": textModel, "threadId": result.ThreadID, "turnId": result.TurnID, "text": result.Text, "ephemeral": true}), 0600); err != nil {
			t.Fatal(err)
		}
	}
}

func TestQuietOperationsPreserveOldDataAndPagePastNoise(t *testing.T) {
	s := testStore(t)
	at := timestamp()
	events := []Operation{{ID: "000-before", ProjectID: "p", Action: "import", Text: "导入了一张图片", At: at, UpdatedAt: at}}
	for i := 0; i < 650; i++ {
		events = append(events, Operation{ID: fmt.Sprintf("050-%03d", i), ProjectID: "p", Action: "undo", Text: "撤销：移动了 1 张图片", At: at, UpdatedAt: at})
	}
	events = append(events, Operation{ID: "100-after", ProjectID: "p", Action: "undo", Text: "撤销：断开了 1 条连线", At: at, UpdatedAt: at})
	tx, err := s.DB.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if err := saveOperations(tx, events); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	got, err := s.Operations("p")
	if err != nil || len(got) != 2 || got[0].ID != "000-before" || got[1].ID != "100-after" {
		t.Fatal("old noise hid significant operations", got, err)
	}
	var count int
	if err = s.DB.QueryRow("select count(*) from operation_events where project_id='p'").Scan(&count); err != nil || count != len(events) {
		t.Fatal("historical data was modified", count, err)
	}
}

func TestDisconnectAndLocateIndividualImages(t *testing.T) {
	a := testDesktop()
	e := a.editor
	e.Node("0").Name = "海岸"
	e.Node("1").Name = "日出"
	tt := ui.NewTester(a.view, 1000, 680)
	r, _ := tt.Find("图片画布")
	tt.RightClickAt(r.X+120, r.Y+125)
	if err := tt.ChooseMenuItem("断开连线", "海岸 → 日出"); err != nil {
		t.Fatal(err)
	}
	if len(e.Events) != 1 || e.Events[0].Action != "disconnect" || len(e.Events[0].Nodes) != 2 || len(e.Project.Graph.Edges) != 1 {
		t.Fatal(e.Events, e.Project.Graph.Edges)
	}
	// Open the history tab while the canvas still has a hovered image.
	if err := tt.Click("添加侧栏页面"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("操作历史"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(foldDuration)
	if err := tt.Click("定位：断开了 1 条连线"); err != nil {
		t.Fatal(err, tt.Texts())
	}
	if err := tt.Click("定位图片：日出"); err != nil {
		t.Fatal(err, tt.Texts())
	}
	if a.dock.active != dockOperations || !e.Selection["1"] || len(e.Selection) != 1 || e.Project.Revision != 1 {
		t.Fatal("locate changed graph or selected wrong endpoint", e.Selection, e.Project.Revision)
	}
	e.Undo()
	if len(e.Project.Graph.Edges) != 2 || e.Events[1].Base != "disconnect" {
		t.Fatal("undo lost edge")
	}
	e.Delete()
	tt.Frame()
	tt.Click("定位：将 1 张图片移入回收站")
	if a.left != "回收站" || a.locatedTrashKey == "" || e.Node("1") != nil || len(e.Trash) == 0 {
		t.Fatal("delete receipt did not locate original trash node", a.left, a.locatedTrashKey)
	}
	tt.Click("恢复图片：日出")
	if e.Node("1") == nil {
		t.Fatal("wrong card restored")
	}
}

func TestBottomControlsShareHitboxAndCenter(t *testing.T) {
	for _, size := range [][2]int{{1000, 680}, {1440, 900}} {
		a := testDesktop()
		tt := ui.NewTester(a.view, size[0], size[1])
		for _, compact := range []bool{false, true} {
			a.rail.Collapsed = compact
			tt.Frame()
			labels := []string{"打开设置", "文字工具", "缩小画布", "放大画布", "撤销", "重做", "适应画布"}
			if !compact {
				labels = append(labels, "回收站")
			}
			for _, label := range labels {
				r, ok := tt.Find(label)
				if !ok || r.W != 30 || r.H != 30 || r.Y+r.H/2 != float32(size[1])-windowFooterInset-17 {
					t.Fatal(label, r, size, compact)
				}
			}
			typeTool, _ := tt.Find("文字工具")
			zoomOut, _ := tt.Find("缩小画布")
			if gap := zoomOut.X - typeTool.X - typeTool.W; gap < 0 || gap > 4 {
				t.Fatal("text tool did not join the zoom controls", typeTool, zoomOut)
			}
		}
	}
}

func TestTextAndZoomControlsFitTheNarrowestCanvas(t *testing.T) {
	a := testDesktop()
	a.openDock(dockOperations)
	a.dock.fraction = .99
	tt := ui.NewTester(a.view, 1000, 680)
	canvas, ok := tt.Find("图片画布")
	if !ok || canvas.W > 270 {
		t.Fatal("fixture did not reach the narrow canvas layout", canvas)
	}
	for _, label := range []string{"文字工具", "缩小画布", "放大画布", "适应画布", "撤销", "重做"} {
		r, ok := tt.Find(label)
		if !ok || r.W != 30 || r.X < canvas.X || r.X+r.W > canvas.X+canvas.W {
			t.Fatal(label, r, canvas)
		}
	}
}

func TestHistoryLocatesOffscreenTrashInNewGrid(t *testing.T) {
	for _, width := range []int{1000, 1440} {
		a := testDesktop()
		e := a.editor
		for i := range 90 {
			id := fmt.Sprintf("deleted-%03d", i)
			n := newNode(e.Project.ID, Point{})
			n.ID = id
			n.Name = id
			e.Trash = append(e.Trash, Trash{ID: id, ProjectID: e.Project.ID, Node: n, DeletedAt: "2026-11-15T06:42:00Z"})
		}
		o := Operation{ID: "event", ProjectID: e.Project.ID, Action: "trash", Text: "删除图片", Nodes: []string{"deleted-042"}}
		e.Events = []Operation{o}
		tt := ui.NewTester(a.view, width, 680)
		tt.Click("添加侧栏页面")
		tt.ChooseMenuItem("操作历史")
		tt.Advance(foldDuration)
		if err := tt.Click("定位：删除图片"); err != nil {
			t.Fatal(err)
		}
		tt.Advance(300 * time.Millisecond)
		r, ok := tt.Find("恢复图片：deleted-042")
		if !ok || r.Y < 100 || r.Y+r.H > 680 {
			t.Fatal("requested trash item did not enter viewport", width, r, a.trashView.scroll.Y)
		}
	}
}
