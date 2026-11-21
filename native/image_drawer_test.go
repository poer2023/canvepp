package main

import (
	"fmt"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func drawerFixture() *desktop {
	a := testDesktop()
	e := a.editor
	assets := []Asset{
		{ID: "a", ProjectID: e.Project.ID, NodeID: "0", Source: "import", Name: "原图.png", Width: 1024, Height: 1024},
		{ID: "b", ProjectID: e.Project.ID, NodeID: "1", Source: "codex", Name: "夕阳.png", RunID: "r1", Parents: []string{"a"}, Width: 1024, Height: 1024},
		{ID: "c", ProjectID: e.Project.ID, NodeID: "2", Source: "codex", Name: "透明.png", RunID: "r2", Parents: []string{"a", "b"}, Width: 1024, Height: 1024},
	}
	for i, asset := range assets {
		e.Assets[asset.ID] = asset
		e.Project.Graph.Nodes[i].Asset = asset.ID
		e.Project.Graph.Nodes[i].Config = Config{Params: defaults()}
	}
	a.runs = []NativeRun{
		{ID: "r1", ProjectID: e.Project.ID, NodeID: "1", Prompt: "背景改成夕阳", Params: defaults(), Sources: []Asset{assets[0]}, Result: "b", Status: "succeeded", CreatedAt: "2026-11-06T00:00:00Z"},
		{ID: "r2", ProjectID: e.Project.ID, NodeID: "2", Prompt: "保留人物，透明背景", Params: defaults(), Sources: []Asset{assets[0], assets[1]}, Result: "c", Status: "succeeded", CreatedAt: "2026-11-06T01:00:00Z"},
	}
	e.Node("1").Config.Text = a.runs[0].Prompt
	e.Node("2").Config.Text = a.runs[1].Prompt
	e.Selection["1"] = true
	return a
}

func TestImageLineageUsesVersionsAndSurvivesMissingEdgesAndNodes(t *testing.T) {
	a := drawerFixture()
	e := a.editor
	e.Node("1").Config.Text = "下一步的可修改草稿"
	e.Project.Graph.Edges = nil
	r, ok := a.sourceRecord(e.Assets["b"])
	if !ok || r.Prompt != "背景改成夕阳" || !reflect.DeepEqual(r.Sources, []string{"a"}) {
		t.Fatal("source was inferred from editable config", r)
	}
	if children := a.subsequentRecords(e.Assets["a"]); !reflect.DeepEqual(children, []string{"r2", "r1"}) {
		t.Fatal("ordered multiple-parent history", children)
	}
	e.Selection = map[string]bool{"1": true, "2": true}
	e.Delete()
	if !slices.Contains(a.subsequentRecords(e.Assets["b"]), "r2") {
		t.Fatal("deletion erased provenance")
	}
	unknown := Asset{ID: "unknown", ProjectID: e.Project.ID, NodeID: "0", Source: "codex", RunID: "missing"}
	e.Assets[unknown.ID] = unknown
	e.Node("0").Config.Text = "不可作为历史的草稿"
	r, _ = a.sourceRecord(unknown)
	if r.Prompt != "" {
		t.Fatal("missing history was fabricated", r)
	}
	h := a.imageHistory()
	for i := 0; i < 100; i++ {
		e.Project.Revision++
		if a.imageHistory() != h {
			t.Fatal("moving a card rebuilt history")
		}
	}
}

func TestLegacyImageRecordsReadImmutableReferencesAndParameters(t *testing.T) {
	s, err := openStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	raw := `{"id":"legacy","projectId":"project","nodeId":"1","mode":"manual","status":"succeeded","inputSnapshot":{"prompt":"原始提示词","params":{"size":"2048x1024","n":2},"sourceAssetVersionIds":["b","a"]},"assetVersionIds":["c","d"]}`
	if _, err := s.DB.Exec("INSERT INTO runs VALUES(?,?,?)", "legacy", "project", raw); err != nil {
		t.Fatal(err)
	}
	records, err := s.ImageRecords()
	if err != nil || len(records) != 1 {
		t.Fatal(records, err)
	}
	r := records[0]
	if r.Prompt != "原始提示词" || r.Params.Size != "2048x1024" || r.Params.N != 2 || !reflect.DeepEqual(r.Sources, []string{"b", "a"}) || len(r.Results) != 2 {
		t.Fatal(r)
	}
}

func TestImageDrawerSwitchingProtectsDraftAndLocatesResults(t *testing.T) {
	a := drawerFixture()
	before := cloneGraph(a.editor.Project.Graph)
	tt := ui.NewTester(a.view, 1440, 900)
	if _, ok := tt.Find("编辑提示词"); !ok || a.imageDrawer != "creation" || a.prompt != "" {
		t.Fatal("creation is not the default, or reused the source prompt", a.imageDrawer, a.prompt)
	}
	if !reflect.DeepEqual(before, a.editor.Project.Graph) {
		t.Fatal("opening creation wrote an empty draft")
	}
	if err := tt.Click("展开来源抽屉"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(800 * time.Millisecond)
	if _, ok := tt.Find("原始提示词"); !ok {
		t.Fatal("source not reachable")
	}
	if _, ok := tt.Find("编辑提示词"); ok {
		t.Fatal("immutable source presented as editable")
	}
	tt.Click("展开创作抽屉")
	tt.Advance(800 * time.Millisecond)
	tt.Click("查看创作记录")
	tt.Advance(800 * time.Millisecond)
	if _, ok := tt.Find("预览创作结果：c"); !ok {
		t.Fatal("missing subsequent result")
	}
	if !reflect.DeepEqual(before, a.editor.Project.Graph) {
		t.Fatal("reading history modified canvas")
	}
	tt.Click("开始新创作")
	tt.Advance(800 * time.Millisecond)
	if a.prompt != "" {
		t.Fatal("source prompt silently became a next creation")
	}
	tt.Click("编辑提示词")
	tt.Type("新的创作草稿")
	tt.Click("展开来源抽屉")
	tt.Advance(800 * time.Millisecond)
	if a.prompt != "新的创作草稿" {
		t.Fatal("switch lost draft")
	}
	tt.Click("沿用原始提示词")
	if a.reusePending == nil || a.prompt != "新的创作草稿" {
		t.Fatal("reuse overwrote draft without confirmation")
	}
	tt.Click("保留当前草稿")
	tt.Click("展开创作抽屉")
	tt.Advance(800 * time.Millisecond)
	if a.prompt != "新的创作草稿" {
		t.Fatal("return lost draft")
	}
	tt.Click("查看创作记录")
	tt.Click("定位创作结果：c")
	tt.Advance(800 * time.Millisecond)
	if !a.editor.Selection["2"] || len(a.editor.Project.Graph.Nodes) != 3 {
		t.Fatal("result navigation changed content")
	}
	if _, ok := tt.Find("编辑提示词"); !ok || a.prompt != "" {
		t.Fatal("result did not default to a clean creation form")
	}
	if _, ok := tt.Find("查看创作记录"); ok {
		t.Fatal("invented empty history")
	}
}

func TestImageDrawerSmallWindowIdleAndReduceMotion(t *testing.T) {
	for _, reduced := range []bool{false, true} {
		a := drawerFixture()
		tt := ui.NewTester(a.view, 1000, 680)
		tt.SetPreferences(ui.Preferences{ReduceMotion: reduced})
		for _, label := range []string{"展开创作抽屉", "展开来源抽屉", "开始新创作", "展开来源抽屉"} {
			if err := tt.Click(label); err != nil {
				t.Fatal(err)
			}
			tt.Advance(800 * time.Millisecond)
			if tt.FramePending() {
				t.Fatal("drawer keeps rendering at rest", label, reduced)
			}
			panel, _ := tt.Find("悬浮图片设置")
			canvas, _ := tt.Find("图片画布")
			if panel.Y+panel.H > canvas.Y+canvas.H || panel.X+panel.W > canvas.X+canvas.W {
				t.Fatal("small window clipping", panel)
			}
		}
	}
}

func TestImageDrawerOnlyDecodesVisibleHistoryAndPreservesParamDraft(t *testing.T) {
	a := drawerFixture()
	n := a.editor.Node("1")
	n.Config.Params.Size = "2048x1024"
	if !a.hasCreationDraft(*n, a.editor.Assets["b"]) {
		t.Fatal("legacy dimension-only draft was hidden")
	}
	for i := 0; i < 1000; i++ {
		id := fmt.Sprintf("history-%04d", i)
		asset := Asset{ID: id, SHA: id, Width: 1024, Height: 1024, RunID: id, Parents: []string{"b"}}
		a.editor.Assets[id] = asset
		a.runs = append(a.runs, NativeRun{ID: id, Prompt: "历史结果", Sources: []Asset{a.editor.Assets["b"]}, Result: id, Status: "succeeded"})
	}
	a.images = &ImageCache{entries: map[string]*cachedImage{}, pending: map[string]bool{}, queue: make(chan imageRequest, 2000)}
	tt := ui.NewTester(a.view, 1440, 900)
	tt.Click("查看创作记录")
	tt.Advance(800 * time.Millisecond)
	if len(a.images.pending) > 25 {
		t.Fatal("history decoded offscreen images", len(a.images.pending))
	}
	if tt.FramePending() {
		t.Fatal("history did not settle")
	}
}
