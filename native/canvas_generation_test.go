package main

import (
	"bytes"
	"image"
	"image/color"
	"image/draw"
	"os"
	"slices"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func TestPlaceholderGenerationKeepsNodeAndVersions(t *testing.T) {
	log := fixtureCodex(t)
	a := testDesktop()
	a.store = testStore(t)
	a.saves = newSaveQueue(a.store, func(string, int, error) {})
	defer close(a.saves.wake)
	e := a.editor
	e.OnChange = func() { a.changed(e) }
	e.Selection["0"] = true
	e.Node("0").PositionLocked = true
	before := *e.Node("0")
	edges := slices.Clone(e.Project.Graph.Edges)
	a.prompt, a.count = "测试原地生成", 2
	a.generate()
	a.generate()
	if len(a.runs) != 2 || !slices.Equal(e.Project.Graph.Edges, edges) || len(e.Project.Graph.Nodes) != 3 {
		t.Fatal("a repeated click or placeholder generation altered the graph")
	}
	for _, r := range a.runs {
		if r.NodeID != before.ID || len(r.Sources) != 0 || r.Params.N != 1 {
			t.Fatal("placeholder generated with an incorrect target or references", r)
		}
	}
	var results []NativeRun
	for deadline := time.Now().Add(8 * time.Second); time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
		var err error
		results, err = a.store.NativeRuns()
		if err != nil {
			t.Fatal(err)
		}
		if len(results) == 2 && results[0].Status == "succeeded" && results[1].Status == "succeeded" {
			break
		}
	}
	if len(results) != 2 || results[0].Result == "" || results[1].Result == "" || results[0].Result == results[1].Result {
		t.Fatal("both generation results must be durable variants", results)
	}
	rpc, err := os.ReadFile(log)
	if err != nil || countLines(string(rpc), "turn/start") != 2 {
		t.Fatal("the run group did not submit each expected variant once", err)
	}
	reopened, err := newDesktop(a.store)
	if err != nil {
		t.Fatal(err)
	}
	node := reopened.editor.Node(before.ID)
	if node == nil || node.Asset != results[1].Result || node.Position != before.Position || node.Name != before.Name ||
		!node.PositionLocked || node.Config.Note != before.Config.Note || !slices.Equal(reopened.editor.Project.Graph.Edges, edges) {
		t.Fatal("reopening failed to adopt the first result at the original slot", node)
	}
	reopened.editor.Selection[node.ID] = true
	tt := ui.NewTester(reopened.view, 1440, 900)
	if err := tt.Click("展开来源抽屉"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("切换到图片版本 2"); err != nil || node.Asset != results[0].Result {
		t.Fatal("the alternate image could not be selected from the source drawer", err, node.Asset)
	}
	reopened.editor.Undo()
	if node.Asset != results[1].Result {
		t.Fatal("switching versions was not reversible")
	}
}

func TestGenerationWithMixedSelectionLinksOnlyImageReferences(t *testing.T) {
	a := testDesktop()
	e := a.editor
	e.Assets["reference"] = Asset{ID: "reference", ProjectID: e.Project.ID, NodeID: "1", Width: 100, Height: 100}
	e.Node("1").Asset = "reference"
	e.Selection["0"], e.Selection["1"] = true, true
	a.prompt, a.count = "参考已有图", 2
	runs := a.prepareCanvasGeneration()
	if len(runs) != 2 || len(e.Project.Graph.Nodes) != 5 || len(e.Project.Graph.Edges) != 4 || e.Node("0").Asset != "" {
		t.Fatal("mixed selection created the wrong set of targets", runs)
	}
	for i, run := range runs {
		edge := e.Project.Graph.Edges[2+i]
		if len(run.Sources) != 1 || run.Sources[0].ID != "reference" || edge.Source != "1" || edge.Target != run.NodeID {
			t.Fatal("a blank slot was treated as an image reference", run, edge)
		}
	}
}

func TestCanvasGenerationIndicatorFollowsActivityAndMotionPreference(t *testing.T) {
	a := testDesktop()
	e := a.editor
	e.Selection["0"] = true
	tt := ui.NewTester(a.view, 1440, 900)
	a.prompt, a.count = "等候状态", 1
	if runs := a.prepareCanvasGeneration(); len(runs) != 1 {
		t.Fatal(runs)
	}
	tt.Frame()
	if !tt.HasText("等待生成") {
		t.Fatal("queued status missing from the canvas")
	}
	r := &a.runs[0]
	r.Status = "running"
	a.runRevision++
	tt.Frame()
	box, ok := tt.Find("正在生成")
	if !ok {
		t.Fatal("running status missing from the canvas")
	}
	bounds := image.Rect(int(box.X+20), int(box.Y+20), int(box.X+box.W-20), int(box.Y+box.H-20))
	pixels := func() []byte {
		crop := image.NewRGBA(bounds)
		draw.Draw(crop, bounds, tt.Image(), bounds.Min, draw.Src)
		return crop.Pix
	}
	before := pixels()
	tt.Advance(110 * time.Millisecond)
	if bytes.Equal(before, pixels()) {
		t.Fatal("generation indicator stayed still")
	}
	tt.SetPreferences(ui.Preferences{ReduceMotion: true})
	tt.Advance(500 * time.Millisecond)
	before = pixels()
	tt.Advance(440 * time.Millisecond)
	if !tt.HasText("正在生成") || !bytes.Equal(before, pixels()) {
		t.Fatal("reduced motion did not retain a static status")
	}
	e.View.X -= 4000
	tt.Frame()
	if !tt.HasText("正在生成 · 当前选区") {
		t.Fatal("generation became invisible when the selected card moved offscreen")
	}
	e.View.X += 4000
	r.Status = "failed"
	delete(a.running, r.ID)
	a.runRevision++
	tt.Frame()
	if tt.HasText("正在生成") || !tt.HasText("生成未完成") || a.selectionGeneration().pending {
		t.Fatal("failed generation did not stop the indicator and allow another request")
	}
}

func TestCanvasGenerationTilesSampleTheResultAndSettle(t *testing.T) {
	a := testDesktop()
	e := a.editor
	node := e.Node("0")
	e.Selection[node.ID] = true
	a.images = &ImageCache{entries: map[string]*cachedImage{}, pending: map[string]bool{}, queue: make(chan imageRequest, 2)}
	tt := ui.NewTester(a.view, 1080, 720)
	a.prompt, a.count = "图片揭晓", 1
	a.prepareCanvasGeneration()
	tt.Frame()
	card, ok := tt.Find("等待生成")
	if !ok {
		t.Fatal("the placeholder did not enter the wait state")
	}

	asset := Asset{ID: "result", SHA: "result", ProjectID: e.Project.ID, NodeID: node.ID, Width: 240, Height: 200}
	run := &a.runs[0]
	delete(a.running, run.ID)
	run.Status, run.Result = "succeeded", asset.ID
	a.runRevision++
	e.Assets[asset.ID] = asset
	adoptGeneratedResult(e, node.ID, asset.ID)
	a.beginImageReveal(e, node.ID, asset.ID)
	tt.Frame()
	if !tt.HasText("正在载入结果") || !a.imageReveals[node.ID].start.IsZero() {
		t.Fatal("the reveal began before the thumbnail was available")
	}

	src := image.NewRGBA(image.Rect(0, 0, 60, 50))
	draw.Draw(src, src.Bounds(), image.NewUniform(color.RGBA{R: 230, G: 82, B: 38, A: 255}), image.Point{}, draw.Src)
	a.images.entries[imageKey(asset, 512)] = &cachedImage{Bitmap: ui.NewBitmap(src)}
	tt.Frame()
	redPixels := func() int {
		n := 0
		img := tt.Image()
		for y := int(card.Y + 10); y < int(card.Y+72); y++ {
			for x := int(card.X + 12); x < int(card.X+112); x++ {
				pixel := img.RGBAAt(x, y)
				if int(pixel.R)-int(pixel.G) > 35 && int(pixel.R)-int(pixel.B) > 45 {
					n++
				}
			}
		}
		return n
	}
	start := redPixels()
	tt.Advance(imageRevealDuration * 2 / 5)
	middle := redPixels()
	tt.Advance(imageRevealDuration + 200*time.Millisecond)
	end := redPixels()
	if start != 0 || middle <= start || middle >= end || end < 6000 {
		t.Fatal("expected neutral tiles, sampled colors, then the complete image", start, middle, end)
	}
	if len(a.imageReveals) != 0 || tt.FramePending() || tt.HasText("正在载入结果") || len(e.Project.Graph.Nodes) != 3 {
		t.Fatal("the reveal did not settle in place")
	}
}
