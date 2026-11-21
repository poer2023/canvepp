package main

import (
	"image"
	"image/color"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func TestEmptyCanvasActionsInNarrowSplit(t *testing.T) {
	a := testDesktop()
	a.createCanvas()
	a.openDock(dockHistory)
	a.dock.fraction = .9
	tt := ui.NewTester(a.view, 1000, 680)
	canvas, _ := tt.Find("图片画布")
	importButton, hasImport := tt.Find("导入图片")
	newButton, hasNew := tt.Find("新建图片")
	if !hasImport || !hasNew || importButton.W < 56 || importButton.W != newButton.W ||
		importButton.H != newButton.H || importButton.Y != newButton.Y ||
		importButton.X <= canvas.X || newButton.X < importButton.X+importButton.W ||
		newButton.X+newButton.W >= canvas.X+canvas.W {
		t.Fatalf("expected equally sized actions within the canvas: canvas=%v import=%v new=%v", canvas, importButton, newButton)
	}

	x := importButton.X + importButton.W/2
	y := importButton.Y + importButton.H/2
	tt.Move(x, y)
	tt.Advance(hintDelay + time.Millisecond)
	if tt.Cursor() != ui.CursorPointer || !tt.HasText("也可拖入画布") {
		t.Fatal("import action is missing pointer feedback or help")
	}
	// Only routing is under test here. A missing file keeps the fixture read-only.
	if !tt.DropFiles(x, y, filepath.Join(t.TempDir(), "absent.png")) {
		t.Fatal("import action intercepted the canvas file drop")
	}

	tt.ClickAt(newButton.X+newButton.W/2, y)
	_, hasImport = tt.Find("导入图片")
	if len(a.editor.Project.Graph.Nodes) != 1 || hasImport || a.gesture != "" {
		t.Fatal("clicking the new image action must create a node and dismiss the empty state")
	}
	tt.Key(ui.Cmd, ui.KeyZ)
	if len(a.editor.Project.Graph.Nodes) != 0 {
		t.Fatal("undo did not remove the new image")
	}
	// The inspector stays open after undo and covers the empty state until dismissed.
	if err := tt.Click("关闭图片设置"); err != nil {
		t.Fatal(err)
	}
	// Begin the Tab sequence on the canvas with the pointer off the tiles.
	tt.ClickAt(canvas.X+20, canvas.Y+70)
	tt.Key(0, ui.KeyTab)
	if !tt.Focused("导入图片") || !tt.HasText("也可拖入画布") {
		t.Fatalf("keyboard focus did not reach the import action: focused=%t", tt.Focused("导入图片"))
	}
	tt.Key(0, ui.KeyTab)
	if !tt.Focused("新建图片") || !tt.HasText("新建图片 · ⌘ N") {
		t.Fatalf("keyboard focus did not reach the new image action: focused=%t", tt.Focused("新建图片"))
	}
	tt.Key(0, ui.KeyEnter)
	if len(a.editor.Project.Graph.Nodes) != 1 {
		t.Fatal("keyboard activation did not create a node")
	}
}

func TestPhotoAtCanvasCornerKeepsTheSurroundAndTransparency(t *testing.T) {
	picture := image.NewNRGBA(image.Rect(0, 0, 16, 8))
	for y := 0; y < 8; y++ {
		for x := 8; x < 16; x++ {
			picture.SetNRGBA(x, y, color.NRGBA{R: 200, G: 50, B: 30, A: 255})
		}
	}
	bitmap := ui.NewBitmap(picture)
	surround, canvas := ui.Hex("#e8eaeb"), ui.Hex("#fafafa")
	frame := ui.Rect{X: 10, Y: 10, W: 80, H: 80}
	var checkSize float32
	tester := ui.NewTester(func(c *ui.Context) {
		ui.Box(c).Fill().Background(surround).Draw(func(p *ui.Painter, _ ui.Rect) {
			p.Fill(frame, canvas, canvasCorner)
			p.Clip(frame, canvasCorner, func() {
				drawCanvasPhoto(p, frame, ui.Rect{Y: -25, W: 100, H: 100}, 4, bitmap, checkSize)
			})
		})
	}, 100, 100)
	for _, step := range []float32{0, 12} {
		checkSize = step
		tester.Frame()
		im := tester.Image()
		if im.RGBAAt(89, 10) != im.RGBAAt(92, 10) {
			t.Fatal("the image covered the outside of the rounded viewport")
		}
		// The left half is transparent and the lower letterbox has no photo.
		left, right := im.RGBAAt(32, 35), im.RGBAAt(73, 35)
		if checkSize == 0 && left != im.RGBAAt(73, 65) {
			t.Fatal("the transparent portion no longer shows the canvas")
		}
		if !(right.R > 150 && right.G < 80 && right.B < 80) {
			t.Fatal("the opaque half of the image is missing", right)
		}
		if checkSize > 0 && left == im.RGBAAt(44, 35) {
			t.Fatal("hover checkerboard missing under the transparent half")
		}
	}
}

func TestDefaultCreationPreservesExistingDraftsWithoutWrites(t *testing.T) {
	for _, kind := range []string{"original", "prompt", "size", "explicit"} {
		t.Run(kind, func(t *testing.T) {
			a := drawerFixture()
			n := a.editor.Node("1")
			want := ""
			switch kind {
			case "prompt":
				n.Config.Text, want = "保留原始人物，改成夜景", "保留原始人物，改成夜景"
			case "size":
				n.Config.Params.Size, want = "2048x1024", n.Config.Text
			case "explicit":
				n.Config.CreationStarted, want = true, n.Config.Text
			}
			before := cloneGraph(a.editor.Project.Graph)
			tt := ui.NewTester(a.view, 1440, 900)
			if a.imageDrawer != "creation" || !a.creationEditing || a.prompt != want {
				t.Fatal("incorrect default or lost draft", a.imageDrawer, a.prompt, want)
			}
			tt.Click("展开来源抽屉")
			tt.Advance(300 * time.Millisecond)
			tt.Click("展开创作抽屉")
			tt.Advance(300 * time.Millisecond)
			if a.prompt != want || !reflect.DeepEqual(before, a.editor.Project.Graph) || len(a.editor.undo) != 0 {
				t.Fatal("viewing drawers changed content/history")
			}
			if kind == "size" && (a.width != 2048 || a.height != 1024) {
				t.Fatal("lost dimension-only draft")
			}
		})
	}
}

func TestImageConnectionsFaceSidesAndAvoidPictures(t *testing.T) {
	for _, boxes := range [][2]ui.Rect{
		{{X: 0, Y: 60, W: 180, H: 240}, {X: 420, Y: 10, W: 240, H: 160}},
		{{X: 420, Y: 10, W: 240, H: 160}, {X: 0, Y: 60, W: 180, H: 240}},
		{{X: 0, Y: 0, W: 180, H: 200}, {X: 0, Y: 280, W: 180, H: 200}},
		{{X: 120, Y: 0, W: 180, H: 200}, {X: 0, Y: 260, W: 180, H: 200}},
		{{X: 0, Y: 0, W: 180, H: 200}, {X: 183, Y: 260, W: 180, H: 200}},
	} {
		s, d := boxes[0], boxes[1]
		route := imageConnection(s, d)
		if route[0][1] != s.Y+s.H/2 || route[3][1] != d.Y+d.H/2 || route[0][1] != route[1][1] || route[2][1] != route[3][1] {
			t.Fatal("connection lost horizontal side tangents", route)
		}
		var buf [257][2]float32
		points := route.points(buf[:0])
		for _, pt := range points {
			for _, box := range boxes {
				if pt[0] > box.X && pt[0] < box.X+box.W && pt[1] > box.Y && pt[1] < box.Y+box.H {
					t.Fatal("curve crosses a picture", pt, box, route)
				}
			}
		}
		if len(points) > len(buf) || !route.visible(ui.Rect{X: -100, Y: -100, W: 1000, H: 1000}) || route.visible(ui.Rect{X: 4000, Y: 4000, W: 100, H: 100}) {
			t.Fatal("unbounded or incorrectly culled path")
		}
		if n := testing.AllocsPerRun(100, func() { imageConnection(s, d).points(buf[:0]) }); n != 0 {
			t.Fatal("routing allocated during pointer movement", n)
		}
	}
}

func TestDetachedToolbarReachableWithoutCoveringImage(t *testing.T) {
	a := testDesktop()
	a.editor.View.Y = 150
	tt := ui.NewTester(a.view, 1440, 900)
	canvas, _ := tt.Find("图片画布")
	n := *a.editor.Node("0")
	q := nodeRect(n, a.editor.Assets)
	x := canvas.X + float32((q.X+q.W/2)*a.editor.View.Zoom+a.editor.View.X)
	y := canvas.Y + float32(q.Y*a.editor.View.Zoom+a.editor.View.Y)
	tt.Move(x, y+15)
	if a.hover != n.ID {
		t.Fatal("image cannot be hovered")
	}
	tool := a.nodeToolbar(n)
	if tool.Y+tool.H >= y-canvas.Y {
		t.Fatal("toolbar covers image", tool)
	}
	tt.Move(x, y-5)
	if a.hover != n.ID {
		t.Fatal("toolbar vanishes in gap")
	}
	tt.Move(canvas.X+tool.X+15, canvas.Y+tool.Y+20)
	for _, label := range []string{"放大图片", "下载图片", "锁定节点位置", "删除图片"} {
		r, ok := tt.Find(label)
		if !ok {
			t.Fatalf("toolbar action %s cannot be reached", label)
		}
		if r.X < canvas.X+tool.X || r.X+r.W > canvas.X+tool.X+tool.W {
			t.Fatalf("toolbar action %s exceeds the toolbar", label)
		}
	}
	for _, label := range []string{"创作图片", "复制图片"} {
		if _, ok := tt.Find(label); ok {
			t.Fatalf("toolbar still contains %s", label)
		}
	}
	tt.Move(canvas.X+600, canvas.Y+650)
	if _, ok := tt.Find("放大图片"); ok {
		t.Fatal("toolbar persisted away from image")
	}
	tt.Move(x, y+15)
	tt.Move(150, 500)
	if a.hover != "" {
		t.Fatal("toolbar persisted after leaving canvas")
	}
	a.editor.View.Zoom, a.editor.View.Y = 4, -250
	n.Asset = "tall"
	a.editor.Assets[n.Asset] = Asset{ID: n.Asset, Width: 800, Height: 1400}
	if a.nodeToolbar(n).W != 0 {
		t.Fatal("zoomed image covered by pinned toolbar")
	}
}

func TestDraggingSelectedImageKeepsActiveDrawer(t *testing.T) {
	a := drawerFixture()
	tt := ui.NewTester(a.view, 1440, 900)
	tt.Click("展开来源抽屉")
	tt.Advance(300 * time.Millisecond)
	canvas, _ := tt.Find("图片画布")
	q := nodeRect(*a.editor.Node("1"), a.editor.Assets)
	x := canvas.X + float32((q.X+q.W/2)*a.editor.View.Zoom+a.editor.View.X)
	y := canvas.Y + float32((q.Y+q.H/2)*a.editor.View.Zoom+a.editor.View.Y)
	tt.Press(x, y)
	tt.Move(x+60, y+20)
	tt.Release(x+60, y+20)
	if a.imageDrawer != "source" || len(a.editor.undo) != 1 || a.editor.Node("1").Config.Text != "背景改成夕阳" {
		t.Fatal("drag changed drawer or contents", a.imageDrawer, len(a.editor.undo))
	}
}
