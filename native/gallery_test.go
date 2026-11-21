package main

import (
	"fmt"
	"github.com/egoist/mygo/ui"
	"reflect"
	"testing"
	"time"
)

func galleryFixture() *desktop {
	a := testDesktop()
	for _, e := range a.editors {
		e.Assets = map[string]Asset{}
	}
	p := Project{ID: "second", Name: "旅行画布", Graph: Graph{Nodes: []Node{{ID: "photo", ProjectID: "second", Name: "同名", Asset: "second-a", Config: Config{Text: "海边夕阳"}}}}}
	a.editors[p.ID] = newEditor(p, nil, nil)
	a.order = append(a.order, p.ID)
	for i, id := range a.order {
		e := a.editors[id]
		node := e.Project.Graph.Nodes[0].ID
		x := Asset{ID: id + "-a", ProjectID: id, Name: "同名.png", NodeID: node, CreatedAt: "2026-11-06T01:00:00Z", Width: 1024, Height: 1024, SHA: id}
		if i == 1 {
			x.CreatedAt = "2026-11-06T01:00:00.100Z"
		}
		e.Assets[x.ID] = x
		e.Node(node).Asset = x.ID
	}
	a.left = "历史"
	return a
}

func TestGalleryCrossCanvasSearchGroupsAndChronologicalSorting(t *testing.T) {
	a := galleryFixture()
	items := a.libraryItems()
	if len(items) != 2 || items[0].projectID != "second" {
		t.Fatal("newest first with fractional timestamp", items)
	}
	a.gallerySort = 1
	if a.libraryItems()[0].projectID == "second" {
		t.Fatal("oldest first failed")
	}
	a.galleryQuery = "海边夕阳"
	if got := a.libraryItems(); len(got) != 1 || got[0].projectID != "second" {
		t.Fatal("cross-project prompt search", got)
	}
	a.galleryQuery = "旅行"
	if len(a.libraryItems()) != 1 {
		t.Fatal("canvas name search")
	}
	a.galleryQuery = "同名"
	a.galleryGrouping = 1
	a.libraryItems()
	if len(a.libraryGroups) != 2 {
		t.Fatal("same name merged owners")
	}
	a.selectLibraryGroup("second")
	if got := a.visibleLibraryItems(); len(got) != 1 || got[0].projectID != "second" {
		t.Fatal("canvas group failed")
	}
	a.galleryQuery = "no matches"
	if len(a.visibleLibraryItems()) != 0 {
		t.Fatal("empty search retained stale results")
	}
}

func TestGalleryReturnNeverAdoptsOrDuplicatesHistoricalVersion(t *testing.T) {
	a := galleryFixture()
	e := a.editors["second"]
	current := e.Assets["second-a"]
	old := current
	old.ID = "old"
	e.Assets[old.ID] = old
	before := cloneGraph(e.Project.Graph)
	rev := e.Project.Revision
	a.canvasW, a.canvasH = 1100, 800
	a.returnToAsset(libraryEntry{projectID: "second", asset: &old})
	a.focusCanvasNode()
	if a.editor != e || a.left != "画布" || !e.Selection["photo"] || e.View.Zoom <= 0 {
		t.Fatal("source not focused")
	}
	if !reflect.DeepEqual(before, e.Project.Graph) || rev != e.Project.Revision {
		t.Fatal("history navigation changed content")
	}
	e.Delete()
	a.left = "历史"
	before = cloneGraph(e.Project.Graph)
	rev = e.Project.Revision
	a.returnToAsset(libraryEntry{projectID: "second", asset: &current})
	if len(e.Selection) != 0 || !reflect.DeepEqual(before, e.Project.Graph) || rev != e.Project.Revision {
		t.Fatal("deleted source automatically restored")
	}
	a.left = "历史"
	if len(a.libraryItems()) != 3 {
		t.Fatal("deleted image disappeared from gallery")
	}
}

func TestGalleryTrashRestoresOtherCanvasWithContent(t *testing.T) {
	a := galleryFixture()
	e := a.editors["second"]
	e.Add([]Node{{ID: "target", ProjectID: "second", Config: Config{Text: "关联图"}}}, []Edge{{ID: "edge", Source: "photo", Target: "target"}})
	clear(e.Selection)
	e.Selection["photo"] = true
	e.Delete()
	a.left = "回收站"
	items := a.libraryItems()
	if len(items) != 1 || items[0].projectID != "second" || items[0].asset == nil {
		t.Fatal("cross-canvas trash missing", items)
	}
	a.restoreGalleryItem(items[0])
	if n := e.Node("photo"); n == nil || n.Asset != "second-a" || n.Config.Text != "海边夕阳" || len(e.Project.Graph.Edges) != 1 {
		t.Fatal("restoration lost content or edges")
	}
	if len(a.libraryItems()) != 0 {
		t.Fatal("restored entry still in trash")
	}
	e.Undo()
	if e.Node("photo") != nil || len(a.libraryItems()) != 1 {
		t.Fatal("restore undo broken")
	}
}

func TestGalleryLayoutPreviewSettingsAndRename(t *testing.T) {
	for _, size := range [][2]int{{1440, 900}, {1000, 680}} {
		a := galleryFixture()
		a.left = "画布"
		tt := ui.NewTester(a.view, size[0], size[1])
		tt.SetPreferences(ui.Preferences{ReduceMotion: true})
		if _, ok := tt.Find("新建图片"); ok {
			t.Fatal("image create still in sidebar")
		}
		if _, ok := tt.Find("导入图片"); ok {
			t.Fatal("image import still in sidebar")
		}
		r, _ := tt.Find("新建画布")
		menu, _ := tt.Find(allPictures)
		search, _ := tt.Find("搜索画布")
		sortButton, _ := tt.Find("画布排序")
		trash, _ := tt.Find("回收站")
		if r.Y <= menu.Y+menu.H || search.X >= sortButton.X || sortButton.X >= r.X || trash.Y+trash.H/2 != float32(size[1])-windowFooterInset-17 {
			t.Fatal("canvas actions or footer misplaced", menu, search, sortButton, r, trash)
		}
		if err := tt.Click("添加侧栏页面"); err != nil {
			t.Fatal(err)
		}
		if err := tt.ChooseMenuItem("图片历史"); err != nil {
			t.Fatal(err)
		}
		r, ok := tt.Find("预览图片：同名")
		if !ok || r.X <= a.rail.shownWidth() || r.W < 220 {
			t.Fatal("gallery image not on right", r, ok)
		}
		tt.ClickAt(r.X+r.W/2, r.Y+r.H/2)
		if !a.previewOpen {
			t.Fatal("image click did not open preview")
		}
		tt.Key(0, ui.KeyEscape)
		if a.previewOpen {
			t.Fatal("preview did not close")
		}
		r, _ = tt.Find("回到画布：同名")
		tt.Move(r.X+r.W/2, r.Y+r.H/2)
		tt.Advance(time.Second)
		if _, ok := tt.Find("回到来源画布"); !ok {
			t.Fatal("icon action has no hover explanation")
		}
		tt.ClickAt(r.X+r.W/2, r.Y+r.H/2)
		if a.left != "画布" {
			t.Fatal("icon did not navigate to canvas")
		}
		tt.Click("侧栏：图片历史")
		tt.Click("按画布分组")
		if a.galleryGrouping != 1 {
			t.Fatal("canvas grouping icon did not work")
		}
		tt.Click("按时间分组")
		if a.galleryGrouping != 0 {
			t.Fatal("time grouping icon did not work")
		}
		r, _ = tt.Find("打开设置")
		tt.ClickAt(r.X+15, r.Y+r.H/2)
		if !a.settingsOpen {
			t.Fatal("settings icon not part of button")
		}
		tt.ClickAt(r.X+r.W-12, r.Y+r.H/2)
		if a.settingsOpen {
			t.Fatal("settings icon did not toggle the page closed")
		}
		tt.RightClick("打开画布：旅行画布")
		if err := tt.ChooseMenuItem("重命名…"); err != nil {
			t.Fatal(err)
		}
		tt.Click("画布名称")
		tt.Key(ui.Cmd, ui.KeyA)
		tt.Type("旅行 2026")
		tt.Key(0, ui.KeyEnter)
		if a.renameOpen || a.editors["second"].Project.Name != "旅行 2026" {
			t.Fatal("rename with Enter failed")
		}
	}
}

func TestGalleryOnlyLoadsVisibleThumbnailsAndSettles(t *testing.T) {
	a := galleryFixture()
	e := a.editor
	for i := 0; i < 1000; i++ {
		id := fmt.Sprintf("image-%04d", i)
		e.Assets[id] = Asset{ID: id, ProjectID: e.Project.ID, Name: id, SHA: id, CreatedAt: "2026-11-06T00:00:00Z"}
	}
	a.images = &ImageCache{entries: map[string]*cachedImage{}, pending: map[string]bool{}, queue: make(chan imageRequest, 2000)}
	tt := ui.NewTester(a.view, 1440, 900)
	if n := len(a.images.pending); n == 0 || n > 40 {
		t.Fatal("gallery decoded outside visible grid", n)
	}
	first := a.libraryItems()
	second := a.libraryItems()
	if &first[0] != &second[0] {
		t.Fatal("catalog rebuilt while idle")
	}
	tt.Advance(time.Second)
	if tt.FramePending() {
		t.Fatal("gallery keeps drawing while idle")
	}
}

func TestGalleryKeepsUnresolvedNativeRunsAccessible(t *testing.T) {
	a := galleryFixture()
	a.runs = []NativeRun{{ID: "pending", ProjectID: "second", NodeID: "photo", Prompt: "待查询", Status: "unknown", ThreadID: "original-thread", CreatedAt: "2026-11-06T02:00:00Z"}}
	tt := ui.NewTester(a.view, 1000, 680)
	if _, ok := tt.Find("查询原任务"); !ok {
		t.Fatal("recovery action lost in gallery")
	}
	if len(a.running) != 0 {
		t.Fatal("gallery started a generation or query")
	}
}
