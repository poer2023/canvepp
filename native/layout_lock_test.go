package main

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func TestCanvasAndNodeLayoutLocks(t *testing.T) {
	a := testDesktop()
	e := a.editor
	tt := ui.NewTester(a.view, 1440, 900)
	canvas, _ := tt.Find("图片画布")
	tt.Move(canvas.X+95, canvas.Y+100)
	if err := tt.Click("锁定节点位置"); err != nil {
		t.Fatal(err)
	}

	first, second := e.Node("0").Position, e.Node("1").Position
	e.Selection["0"], e.Selection["1"] = true, true
	drag := func() {
		e.BeginDrag()
		e.MoveDrag(20, 10)
		e.EndDrag()
	}
	drag()
	moved := Point{second.X + 20, second.Y + 10}
	if e.Node("0").Position != first || e.Node("1").Position != moved {
		t.Fatal("mixed selection moved the wrong nodes")
	}
	if err := tt.Click("锁定画布布局"); err != nil {
		t.Fatal(err)
	}
	drag()
	if e.Node("1").Position != moved || len(e.Events) != 0 {
		t.Fatal("locked drag changed layout or operation log")
	}
	store := testStore(t)
	if err := store.Save(e.Project, nil); err != nil {
		t.Fatal(err)
	}
	reopened, err := newDesktop(store)
	if err != nil {
		t.Fatal(err)
	}
	r := reopened.editor
	if !r.Project.LayoutLocked || !r.Node("0").PositionLocked || r.Node("1").PositionLocked {
		t.Fatal("locks did not round trip independently")
	}
	if err := tt.Click("解锁画布布局"); err != nil {
		t.Fatal(err)
	}
	e.Undo()
	if !e.Project.LayoutLocked {
		t.Fatal("undo did not restore lock")
	}
	e.Redo()
	drag()
	if !e.Node("0").PositionLocked || e.Node("0").Position != first || e.Node("1").Position == moved {
		t.Fatal("global unlock lost the individual lock")
	}
	tt.Move(canvas.X+95, canvas.Y+100)
	if err := tt.Click("解锁节点位置"); err != nil {
		t.Fatal(err)
	}
	drag()
	if e.Node("0").Position == first {
		t.Fatal("unlocked node did not move")
	}
}

func TestGenerateFromLockedLayout(t *testing.T) {
	fixtureCodex(t)
	a := testDesktop()
	a.store = testStore(t)
	a.saves = newSaveQueue(a.store, func(string, int, error) {})
	defer close(a.saves.wake)
	e := a.editor
	e.OnChange = func() { a.changed(e) }
	reference, err := a.store.ImportImage(tinyPNG(), e.Project.ID, "0", "reference.png", "import", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	e.Assets[reference.ID] = reference
	e.Node("0").Asset = reference.ID
	e.Selection["0"] = true
	e.toggleNodeLock("0")
	e.toggleCanvasLock()
	a.prompt = "布局锁生成测试"
	source := e.Node("0").Position
	a.generate()
	if len(a.runs) != 1 || len(e.Project.Graph.Nodes) != 4 || len(e.Project.Graph.Edges) != 3 {
		t.Fatal("generation did not create its output node and connection")
	}
	output := e.Node(a.runs[0].NodeID)
	pos := output.Position
	e.BeginDrag()
	e.MoveDrag(50, 50)
	e.EndDrag()
	if pos == source || pos != output.Position || !e.positionLocked(output) || e.Node("0").Position != source {
		t.Fatal("generated node shifted locked layout")
	}
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		run, err := a.store.NativeRun(a.runs[0].ID)
		if err == nil && run.Status == "succeeded" && run.Result != "" {
			return
		}
		if err == nil && (run.Status == "failed" || run.Status == "unknown") {
			t.Fatal(run.Error)
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("fixture generation did not complete")
}
