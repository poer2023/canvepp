package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestSessionArchiveReopenAndRestore(t *testing.T) {
	s, err := openStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.DB.Close()
	for _, id := range []string{"first", "second"} {
		p := Project{ID: id, Name: "同名画布", Pinned: id == "second", Graph: Graph{Nodes: []Node{newNode(id, Point{48, 57})}}}
		if err = s.Save(p, nil); err != nil {
			t.Fatal(err)
		}
	}
	open := func() *desktop {
		t.Helper()
		a, err := newDesktop(s)
		if err != nil {
			t.Fatal(err)
		}
		return a
	}
	a := open()
	for order := 0; order < 3; order++ {
		a.canvasSort = order
		if got := a.filteredCanvases(); !slices.Equal(got, []string{"second", "first"}) {
			t.Fatal("pin order", got)
		}
	}
	a.chooseSession(a.editors["second"])
	a.saves = newSaveQueue(s, func(string, int, error) {})
	defer close(a.saves.wake)
	a.editor.changed()
	a.archiveSession(a.editor, true)
	if err = a.saves.Flush(); err != nil {
		t.Fatal(err)
	}
	if a.editor.Project.ID != "first" {
		t.Fatal("archived session still active")
	}
	_, err = s.DB.Exec("INSERT OR REPLACE INTO native_meta VALUES('active_project','second')")
	if err != nil {
		t.Fatal(err)
	}
	b := open()
	if b.editor.Project.ID != "first" || !slices.Equal(b.filteredCanvases(), []string{"first"}) {
		t.Fatal("archive lost after restart")
	}
	b.openWorkspace("回收站")
	items := b.libraryItems()
	if len(items) != 1 || !items[0].deletedCanvas || items[0].projectID != "second" {
		t.Fatal("restorable session missing", items)
	}
	b.saves = a.saves
	e := b.editors["second"]
	b.archiveSession(e, false)
	if err = a.saves.Flush(); err != nil {
		t.Fatal(err)
	}
	restored := open()
	if restored.filteredCanvases()[0] != "second" || restored.editors["second"].Project.Graph.Nodes[0].Position != (Point{48, 57}) {
		t.Fatal("restore lost pin or picture")
	}
	firstSnapshot, err := snapshotSession(e)
	if err != nil {
		t.Fatal(err)
	}
	file, err := s.sessionFile(firstSnapshot)
	if err != nil {
		t.Fatal(err)
	}
	otherSnapshot, err := snapshotSession(b.editors["first"])
	if err != nil {
		t.Fatal(err)
	}
	other, err := s.sessionFile(otherSnapshot)
	if err != nil || file == other {
		t.Fatal("duplicate name reveal", err, file, other)
	}
	raw, err := os.ReadFile(file)
	var snapshot Project
	if err != nil || json.Unmarshal(raw, &snapshot) != nil || snapshot.ID != "second" || snapshot.Graph.View != e.View {
		t.Fatal("wrong session snapshot")
	}
}

func TestRevealMaterializesOriginalsAndRefreshesActiveNodes(t *testing.T) {
	s := testStore(t)
	p := Project{ID: "reveal", Graph: Graph{Nodes: []Node{{ID: "live"}}}}
	raw := tinyPNG()
	asset := func(node string, parents ...string) Asset {
		t.Helper()
		a, err := s.ImportImage(raw, p.ID, node, "透明.png", "import", "", parents)
		if err != nil {
			t.Fatal(err)
		}
		return a
	}
	parent := asset("old-node")
	display := asset("live", parent.ID)
	variant := asset("live")
	trashOnly := asset("removed")
	p.Graph.Nodes[0].Asset = display.ID
	e := newEditor(p, []Asset{parent, display, variant, trashOnly}, nil)
	reveal := func() (string, sessionSnapshot) {
		t.Helper()
		captured, err := snapshotSession(e)
		if err != nil {
			t.Fatal(err)
		}
		path, err := s.sessionFile(captured)
		if err != nil {
			t.Fatal(err)
		}
		b, err := os.ReadFile(path)
		var doc sessionSnapshot
		if err != nil || json.Unmarshal(b, &doc) != nil {
			t.Fatal("unreadable manifest", err)
		}
		return filepath.Dir(path), doc
	}
	dir, doc := reveal()
	if len(doc.Images) != 3 || doc.Graph.Nodes[0].Asset != display.ID {
		t.Fatal(doc)
	}
	var exported string
	for _, image := range doc.Images {
		if image.ID == trashOnly.ID || !filepath.IsLocal(image.Path) {
			t.Fatal(image)
		}
		exported = filepath.Join(dir, filepath.FromSlash(image.Path))
		b, err := os.ReadFile(exported)
		if err != nil || !bytes.Equal(b, raw) {
			t.Fatal("original bytes changed", err)
		}
	}
	folder := filepath.Join(dir, "assets")
	if files, err := os.ReadDir(folder); err != nil || len(files) != 1 {
		t.Fatal("identical versions duplicated", files, err)
	}
	note := filepath.Join(folder, "my-note.txt")
	if err := os.WriteFile(note, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	e.Project.Graph.Nodes = nil
	_, doc = reveal()
	if len(doc.Images) != 0 {
		t.Fatal("deleted node still in export")
	}
	if _, err := os.Stat(exported); !os.IsNotExist(err) {
		t.Fatal("stale exported image", err)
	}
	if _, err := os.Stat(note); err != nil {
		t.Fatal(err)
	}
	src, _ := s.AssetPath(display)
	if b, err := os.ReadFile(src); err != nil || !bytes.Equal(b, raw) {
		t.Fatal("library original changed", err)
	}
}
