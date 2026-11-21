package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func fixture() *Editor {
	p := Project{ID: "project", Name: "画布", SchemaVersion: 1, Graph: Graph{Nodes: []Node{}, Edges: []Edge{}}}
	for i := 0; i < 3; i++ {
		n := newNode(p.ID, Point{float64(i * 300), 20})
		n.ID = fmt.Sprint(i)
		n.Config.Text = "提示词 " + n.ID
		n.Config.Note = "保留备注"
		n.Config.Params = Params{"1920x1080", "high", 4}
		p.Graph.Nodes = append(p.Graph.Nodes, n)
	}
	p.Graph.Edges = []Edge{{"e1", "0", "image", "1", "image"}, {"e2", "1", "image", "2", "image"}}
	return newEditor(p, nil, nil)
}
func TestMultiCopyDeleteRestoreUndo(t *testing.T) {
	e := fixture()
	e.Selection["0"], e.Selection["1"] = true, true
	clip := e.Copy()
	if err := e.Paste(clip, Point{40, 500}); err != nil {
		t.Fatal(err)
	}
	if len(e.Selected()) != 2 || len(e.Project.Graph.Edges) != 3 {
		t.Fatal("copy lost cards or edge")
	}
	for _, n := range e.Selected() {
		if n.Config.Note != "保留备注" || n.Config.Params.N != 4 {
			t.Fatal("copy lost content")
		}
	}
	e.Undo()
	if len(e.Project.Graph.Nodes) != 3 {
		t.Fatal("paste undo")
	}
	e.Redo()
	if len(e.Project.Graph.Nodes) != 5 {
		t.Fatal("paste redo")
	}
	e.Selection = map[string]bool{"0": true, "1": true}
	e.Delete()
	if e.Node("0") != nil || e.Node("1") != nil {
		t.Fatal("delete")
	}
	var a, b string
	for _, tr := range e.Trash {
		if tr.Node.ID == "0" {
			a = tr.ID
		}
		if tr.Node.ID == "1" {
			b = tr.ID
		}
	}
	e.Restore(a)
	e.Restore(b)
	if e.Node("0").Config.Text != "提示词 0" || len(e.Project.Graph.Edges) != 3 {
		t.Fatal("sequential restore lost content or links")
	}
}
func TestDragIsOneUndoAndKeepsLateAsset(t *testing.T) {
	e := fixture()
	e.Selection = map[string]bool{"0": true, "1": true}
	e.BeginDrag()
	for i := 0; i < 500; i++ {
		e.MoveDrag(float64(i), float64(i/2))
	}
	if len(e.undo) != 0 {
		t.Fatal("drag allocates history per pointer event")
	}
	e.EndDrag()
	if len(e.undo) != 1 {
		t.Fatal("drag must be one command")
	}
	e.Node("0").Asset = "late-result"
	e.Undo()
	if e.Node("0").Position.X != 0 || e.Node("1").Position.X != 300 || e.Node("0").Asset != "late-result" {
		t.Fatal("undo lost late result")
	}
	e.Redo()
	if e.Node("0").Position.X != 499 {
		t.Fatal("redo moved wrong distance")
	}
}
func TestZoomAnchorAndPartialBoxSelect(t *testing.T) {
	e := fixture()
	e.View = Viewport{44, 33, .7}
	p := Point{456, 321}
	before := e.World(p)
	e.ZoomAt(p, 3)
	after := e.World(p)
	if math.Abs(before.X-after.X)+math.Abs(before.Y-after.Y) > 1e-8 {
		t.Fatal("zoom jumps away from cursor")
	}
	e.SelectRect(Rect{230, 25, 100, 100}, false)
	if !e.Selection["0"] || !e.Selection["1"] || e.Selection["2"] {
		t.Fatal("partial intersection selection")
	}
}
func TestPasteRejectsForeignAndInvalidWithoutMutation(t *testing.T) {
	e := fixture()
	e.Selection["0"] = true
	clip := e.Copy()
	clip.ProjectID = "foreign"
	before := cloneGraph(e.Project.Graph)
	if e.Paste(clip, Point{}) == nil || !reflect.DeepEqual(before, e.Project.Graph) {
		t.Fatal("foreign paste mutated canvas")
	}
	clip = e.Copy()
	clip.Nodes[0].Position.X = math.NaN()
	if e.Paste(clip, Point{}) == nil {
		t.Fatal("invalid coordinates accepted")
	}
}
func tinyPNG() []byte {
	im := image.NewNRGBA(image.Rect(0, 0, 8, 6))
	im.SetNRGBA(2, 2, color.NRGBA{R: 255, A: 127})
	var b bytes.Buffer
	_ = png.Encode(&b, im)
	return b.Bytes()
}
func TestImportIsExactAndAtomic(t *testing.T) {
	src, err := openStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer src.Close()
	e := fixture()
	asset, err := src.ImportImage(tinyPNG(), e.Project.ID, "0", "透明图.png", "import", "", []string{})
	if err != nil {
		t.Fatal(err)
	}
	e.Node("0").Asset = asset.ID
	e.Selection["1"] = true
	e.Delete()
	if err = src.Save(e.Project, e.Trash); err != nil {
		t.Fatal(err)
	}
	_, err = src.DB.Exec("INSERT INTO runs VALUES('old','project',?)", `{"status":"unknown","inputSnapshot":{"prompt":"旧提示词"}}`)
	if err != nil {
		t.Fatal(err)
	}
	dst, err := openStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer dst.Close()
	n, err := dst.ImportLegacy(src.Dir)
	if err != nil || n != 1 {
		t.Fatal(n, err)
	}
	projects, _ := dst.Projects()
	if !reflect.DeepEqual(e.Project, projects[0]) {
		t.Fatal("project changed during import")
	}
	assets, _ := dst.Assets("project")
	if len(assets) != 1 || assets[0].SHA != asset.SHA {
		t.Fatal("asset metadata changed")
	}
	b, err := os.ReadFile(filepath.Join(dst.Dir, "assets", asset.File))
	if err != nil || !bytes.Equal(b, tinyPNG()) {
		t.Fatal("alpha image bytes changed")
	}
	var old string
	if err = dst.DB.QueryRow("SELECT body FROM runs WHERE id='old'").Scan(&old); err != nil || !stringsContain(old, "旧提示词") {
		t.Fatal("history lost")
	}
	trash, _ := dst.Trashed("project")
	if len(trash) != 1 || len(trash[0].Edges) != 2 {
		t.Fatal("trash incomplete")
	}
	if err = os.WriteFile(filepath.Join(src.Dir, "assets", asset.File), []byte("corrupt"), 0600); err != nil {
		t.Fatal(err)
	}
	broken, err := openStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer broken.Close()
	if _, err = broken.ImportLegacy(src.Dir); err == nil {
		t.Fatal("accepted corrupt asset")
	}
	got, _ := broken.Projects()
	if len(got) != 0 {
		t.Fatal("partial imported project committed")
	}
}
func stringsContain(s, sub string) bool { return bytes.Contains([]byte(s), []byte(sub)) }
func TestSaveRejectsOlderRevision(t *testing.T) {
	s, err := openStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	p := fixture().Project
	p.Revision = 8
	if err = s.Save(p, nil); err != nil {
		t.Fatal(err)
	}
	p.Revision = 7
	p.Name = "old"
	if s.Save(p, nil) == nil {
		t.Fatal("stale revision overwrote data")
	}
	got, _ := s.Projects()
	if got[0].Name == "old" {
		t.Fatal("lost data")
	}
}
func TestClipboardJSONRetainsParameters(t *testing.T) {
	e := fixture()
	e.Selection["0"] = true
	raw := encode(e.Copy())
	var clip Clipboard
	if err := json.Unmarshal(raw, &clip); err != nil {
		t.Fatal(err)
	}
	if clip.Nodes[0].Config.Params.Size != "1920x1080" {
		t.Fatal("pixel intent lost")
	}
}
func BenchmarkDrag500(b *testing.B) {
	e := fixture()
	for len(e.Project.Graph.Nodes) < 500 {
		n := newNode(e.Project.ID, Point{float64(len(e.Project.Graph.Nodes) * 280), 0})
		e.Project.Graph.Nodes = append(e.Project.Graph.Nodes, n)
	}
	e.reindex()
	for _, n := range e.Project.Graph.Nodes {
		e.Selection[n.ID] = true
	}
	e.BeginDrag()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		e.MoveDrag(float64(i), float64(i))
	}
}
func BenchmarkHit500(b *testing.B) {
	e := fixture()
	for len(e.Project.Graph.Nodes) < 500 {
		n := newNode(e.Project.ID, Point{float64(len(e.Project.Graph.Nodes) % 25 * 280), float64(len(e.Project.Graph.Nodes) / 25 * 320)})
		e.Project.Graph.Nodes = append(e.Project.Graph.Nodes, n)
	}
	e.reindex()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		e.Hit(Point{float64(i % 5000), float64(i % 3000)})
	}
}
