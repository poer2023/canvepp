package main

import (
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func TestCanvasTimelineVisitsAndEditorNavigation(t *testing.T) {
	store, err := openStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	now := time.Now().In(time.Local)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
	for _, p := range []Project{
		{ID: "first", Name: "海报构图", UpdatedAt: today.AddDate(0, 0, -3).Format(time.RFC3339)},
		{ID: "second", Name: "旅行画布", UpdatedAt: today.Format(time.RFC3339)},
		{ID: "legacy", Name: "旧画布"},
		{ID: "deleted", Name: "已删除", DeletedAt: today.Format(time.RFC3339)},
	} {
		if err := store.Save(p, nil); err != nil {
			t.Fatal(err)
		}
	}
	a, err := newDesktop(store)
	if err != nil {
		t.Fatal(err)
	}
	first := a.editors["first"]
	view := Viewport{X: 23, Y: -13, Zoom: 1.57}
	updated := first.Project.UpdatedAt
	tt := ui.NewTester(a.view, 1000, 680)
	click := func(label string) {
		t.Helper()
		if err := tt.Click(label); err != nil {
			t.Fatal(err)
		}
	}
	click(allCanvases)
	first.View = view
	for _, label := range []string{"今天", "近 7 天", "更早"} {
		if !tt.HasText(label) {
			t.Fatal("missing date group", label)
		}
	}
	if _, found := tt.Find("查看画布：已删除"); found {
		t.Fatal("deleted canvas is in the timeline")
	}
	click("全部画布排序")
	click("名称 A–Z")
	if !tt.HasText("按名称") {
		t.Fatal("name order remained grouped by date")
	}
	click("搜索全部画布")
	tt.Advance(200 * time.Millisecond)
	tt.Type("hbg")
	if _, found := tt.Find("查看画布：旅行画布"); found {
		t.Fatal("timeline search did not filter by pinyin")
	}
	click("查看画布：海报构图")
	checkEditor := func() {
		t.Helper()
		if _, found := tt.Find("图片画布"); !found || a.left != "画布" || a.editor != first || first.View != view || a.previewOpen {
			t.Fatal("wrong editor destination or viewport", a.left, a.editor.Project.ID, first.View)
		}
	}
	checkEditor()
	click(allCanvases)
	if a.canvasTimeline.query != "hbg" || a.canvasTimeline.order != timelineName {
		t.Fatal("return lost search or sort")
	}
	click("打开画布：海报构图")
	checkEditor()
	if first.Project.UpdatedAt != updated {
		t.Fatal("opening changed the modification date")
	}
	reopened, err := newDesktop(store)
	if err != nil {
		t.Fatal(err)
	}
	reopened.canvasTimeline.order = timelineOpened
	cards := reopened.canvasTimelineCards()
	if len(cards) != 3 || cards[0].editor.Project.ID != "first" || cards[0].date.IsZero() {
		t.Fatal("latest visit lost after reopening the library", cards)
	}
}

func TestOverviewSelectionPreviewAndNavigation(t *testing.T) {
	a := testDesktop()
	e := a.editor
	e.Project.Name = "练习"
	for i := range 128 {
		id := fmt.Sprintf("image-%03d", i)
		e.Assets[id] = Asset{ID: id, ProjectID: e.Project.ID, Name: id + ".png", Width: 400, Height: 200 + (i%3)*180}
	}
	e.Project.Graph.Nodes[0].Asset = "image-127"
	e.Trash = []Trash{{Assets: []string{"image-000", "image-127"}}}
	hidden := newEditor(Project{ID: "archived", Name: "已归档", DeletedAt: "2026-11-16"}, nil, nil)
	a.editors[hidden.Project.ID] = hidden
	a.order = append(a.order, hidden.Project.ID)
	tt := ui.NewTester(a.view, 1000, 680)
	click := func(name string) {
		t.Helper()
		if err := tt.Click(name); err != nil {
			t.Fatal(err)
		}
	}
	click("全部画布")
	if len(a.canvasTimelineCards()) != 1 {
		t.Fatal("archived canvas shown")
	}
	click("查看画布：练习")
	click("全部图片")
	o := &a.overview
	if o.count != 127 || len(o.rows) < 2 {
		t.Fatal("unexpected gallery layout", o.count, len(o.rows))
	}
	if o.rows[0].y != 0 || o.rows[1].y != o.rows[0].height+overviewGap {
		t.Fatal("rows lost their spacing")
	}
	const lastImage = "选择图片：image-127"
	if _, ok := tt.Find(lastImage); ok {
		t.Fatal("offscreen tile mounted")
	}
	area, ok := tt.Find("全部图片滚动区")
	if !ok {
		t.Fatal("scroll area missing")
	}
	tt.Scroll(area.X+30, area.Y+30, 0, o.height)
	if _, ok := tt.Find(lastImage); !ok {
		t.Fatal("last tile missing after scrolling", o.imageScroll.Y, o.height)
	}
	position := o.imageScroll.Y
	click(lastImage)
	if a.previewOpen || o.selected != e.Project.ID+"/image-127" {
		t.Fatal("click should select only")
	}
	tt.Key(0, ui.KeySpace)
	if _, found := tt.Find("原图预览"); !found || a.previewAsset.ID != "image-127" {
		t.Fatal("space did not open the existing preview")
	}
	tt.Key(0, ui.KeyEscape)
	if a.previewOpen || o.imageScroll.Y != position || o.selected != e.Project.ID+"/image-127" {
		t.Fatal("closing preview lost the gallery position or selection")
	}
	if err := tt.RightClick(lastImage); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("进入画布"); err != nil {
		t.Fatal(err)
	}
	if a.left != "画布" || !e.Selection["0"] {
		t.Fatal("wrong source or no selection")
	}
}

func TestOverviewPreservesExtremeImageRatios(t *testing.T) {
	var images []libraryEntry
	for _, size := range [][2]int{{4800, 800}, {640, 3840}, {429, 625}, {32, 32}, {640, 640}, {4800, 400}, {96, 96}} {
		images = append(images, libraryEntry{asset: &Asset{Width: size[0], Height: size[1]}})
	}
	for _, width := range []float32{217, 820, 1530} {
		rows, _ := justifiedPictureRows(images, width)
		shown := 0
		for _, row := range rows {
			edge := -overviewGap
			for _, tile := range row.tiles {
				asset := tile.entry.asset
				if math.Abs(float64(tile.x-edge-overviewGap)) > .002 || tile.x+tile.width > width+.002 {
					t.Fatalf("overlap or overflow at %v: %#v", width, tile)
				}
				edge = tile.x + tile.width
				box := pictureThumbnailRect(ui.Rect{W: tile.width, H: row.height}, asset)
				if math.Abs(float64(box.W/box.H-galleryPictureRatio(asset))) > .001 {
					t.Fatal("photo stretched", box, asset)
				}
				if galleryPictureRatio(asset) >= 5 && (tile.width != width || len(row.tiles) != 1) {
					t.Fatal("panorama should span the row")
				}
				if asset.Width == 32 && (box.W > 64 || tile.width > 76) {
					t.Fatal("small image enlarged past the cap", box, tile)
				}
				shown++
			}
		}
		if shown != len(images) {
			t.Fatal("layout dropped an image", width, shown)
		}
	}
	landscape := libraryEntry{asset: &Asset{Width: 480, Height: 320}}
	rows, _ := justifiedPictureRows([]libraryEntry{landscape, landscape, images[0], landscape, landscape}, 720)
	if len(rows) != 3 || len(rows[0].tiles) != 3 || rows[1].tiles[0].entry.asset != images[0].asset {
		t.Fatal("adjacent frames should fill the partial row before a panorama", rows)
	}
	last := rows[0].tiles[len(rows[0].tiles)-1]
	if last.x+last.width < 719.99 {
		t.Fatal("ordinary row did not reach the right edge", rows[0])
	}
}

func TestOverviewMoveAndRestorePersist(t *testing.T) {
	s, err := openStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	from := Project{ID: "from", Name: "来源"}
	to := Project{ID: "to", Name: "目标"}
	n := newNode(from.ID, Point{48, 48})
	asset, err := s.ImportImage(tinyPNG(), from.ID, n.ID, "海报.png", "import", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	n.Asset, n.Name = asset.ID, "海报"
	from.Graph.Nodes = []Node{n}
	for _, p := range []Project{from, to} {
		if err := s.Save(p, nil); err != nil {
			t.Fatal(err)
		}
	}
	a, err := newDesktop(s)
	if err != nil {
		t.Fatal(err)
	}
	a.saves = newSaveQueue(s, func(string, int, error) {})
	defer close(a.saves.wake)
	a.left = allPictures
	tt := ui.NewTester(a.view, 1000, 680)
	const tile = "选择图片：海报"
	if err := tt.RightClick(tile); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("移动到画布", "目标"); err != nil {
		t.Fatal(err)
	}
	source, target := a.editors[from.ID], a.editors[to.ID]
	if len(source.Project.Graph.Nodes) != 0 || len(target.Project.Graph.Nodes) != 1 ||
		target.Project.Graph.Nodes[0].Asset != asset.ID || len(source.Assets) != 0 || len(source.Trash) != 0 {
		t.Fatal("move did not change ownership", source.Project.Graph.Nodes, target.Project.Graph.Nodes)
	}
	cachedRef := asset
	cachedRef.File = ""
	if _, err := s.AssetPath(cachedRef); err != nil {
		t.Fatal("existing task reference no longer resolves to its file", err)
	}
	reopened, err := newDesktop(s)
	if err != nil {
		t.Fatal(err)
	}
	moved, ok := reopened.editors[to.ID].Assets[asset.ID]
	if !ok || moved.File != asset.File || moved.ProjectID != to.ID {
		t.Fatal("move was not saved", moved)
	}
	if err := tt.RightClick(tile); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("移入回收站"); err != nil {
		t.Fatal(err)
	}
	if len(target.Project.Graph.Nodes) != 0 || len(target.Trash) != 1 {
		t.Fatal("delete did not move the target card to trash")
	}
	if err := a.saves.Flush(); err != nil {
		t.Fatal(err)
	}
	if persisted, err := s.Trashed(to.ID); err != nil || len(persisted) != 1 || persisted[0].Display != moved.ID {
		t.Fatal("deletion was not saved in trash", persisted, err)
	}
	if err := tt.Click("回收站"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("恢复图片：海报"); err != nil {
		t.Fatal(err)
	}
	if restored := target.Node(moved.NodeID); restored == nil || restored.Asset != moved.ID {
		t.Fatal("restore did not recover the original version", restored)
	}
	if err := a.saves.Flush(); err != nil {
		t.Fatal(err)
	}
}

func TestOverviewMoveRetainsSharedPhotoAndRollsBackOnStaleTarget(t *testing.T) {
	s, err := openStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	from, to := Project{ID: "from"}, Project{ID: "to"}
	first, copy := newNode(from.ID, Point{}), newNode(from.ID, Point{300, 0})
	asset, err := s.ImportImage(tinyPNG(), from.ID, first.ID, "共享.png", "import", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	first.Asset, copy.Asset = asset.ID, asset.ID
	from.Graph.Nodes = []Node{first, copy}
	for _, p := range []Project{from, to} {
		if err := s.Save(p, nil); err != nil {
			t.Fatal(err)
		}
	}
	a, err := newDesktop(s)
	if err != nil {
		t.Fatal(err)
	}
	source, target := a.editors[from.ID], a.editors[to.ID]
	entry := libraryEntry{projectID: from.ID, asset: &asset, name: asset.Name}
	to.Revision++
	if err := s.Save(to, nil); err != nil {
		t.Fatal(err)
	}
	if err := a.moveOverviewPicture(entry, target); err == nil {
		t.Fatal("move should reject the stale target")
	}
	if source.Node(first.ID) == nil || len(target.Assets) != 0 {
		t.Fatal("failed transaction updated the in-memory gallery")
	}
	projects, err := s.Projects()
	if err != nil || len(projects) != 2 || len(projects[0].Graph.Nodes) != 2 {
		t.Fatal("failed transaction left a partially saved canvas", projects, err)
	}
	target.Project = to
	if err := a.moveOverviewPicture(entry, target); err != nil {
		t.Fatal(err)
	}
	placed := target.Project.Graph.Nodes[0]
	if source.Node(first.ID) != nil || source.Node(copy.ID).Asset != asset.ID ||
		placed.Asset == asset.ID || target.Assets[placed.Asset].File != asset.File {
		t.Fatal("transfer changed a shared placement or duplicated the file", source.Project.Graph.Nodes, placed)
	}
}

func TestOverviewArchivesOnlySelectedVersion(t *testing.T) {
	a := testDesktop()
	e := a.editor
	card := &e.Project.Graph.Nodes[0]
	previous := Asset{ID: "previous", NodeID: card.ID}
	active := Asset{ID: "active", NodeID: card.ID}
	card.Asset = active.ID
	e.Assets[previous.ID], e.Assets[active.ID] = previous, active
	a.archiveOverviewPicture(libraryEntry{projectID: e.Project.ID, asset: &active, name: "当前版本"})
	if card.Asset != previous.ID || len(e.Trash) != 1 || e.Trash[0].Display != active.ID ||
		archivedPictureVersions(e)[previous.ID] || !archivedPictureVersions(e)[active.ID] {
		t.Fatal("archiving one thumbnail touched an unrelated version")
	}
}

func TestWorkspaceThemeSelectorAndInspectorInsets(t *testing.T) {
	a := testDesktop()
	a.funding.apiStatus = "未配置"
	tt := ui.NewTester(a.settingsView, 850, 704)
	for _, label := range []string{"深色模式", "浅色模式", "跟随系统", "浅色模式"} {
		if err := tt.Click(label); err != nil {
			t.Fatal(err)
		}
	}
	if a.appearance != "light" {
		t.Fatal("theme selection failed")
	}
	if _, ok := tt.Find("OpenAI API Key 输入"); ok {
		t.Fatal("key editor should start collapsed")
	}
	if err := tt.Click("添加密钥"); err != nil {
		t.Fatal(err)
	}
	if _, ok := tt.Find("OpenAI API Key 输入"); !ok {
		t.Fatal("missing key field")
	}
	if _, ok := tt.Find("打开素材库"); ok {
		t.Fatal("unexpected settings shortcut")
	}
	const w, h, ph = 900, 650, 300
	b := panelBounds(w, h, ph)
	if b.X != 12 || b.Y != 12 || w-(b.X+b.W+panelWidth) != 12 || h-(b.Y+b.H+ph) != 12 {
		t.Fatal("uneven panel margins", b)
	}
}
