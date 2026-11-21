package main

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func adobeTestPNG(t *testing.T, dir, file string, w, h int) {
	t.Helper()
	im := image.NewNRGBA(image.Rect(0, 0, w, h))
	im.SetNRGBA(w/2, h/2, color.NRGBA{R: 30, G: 126, B: 220, A: 128})
	f, err := os.Create(filepath.Join(dir, file))
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, im); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestAdobeSnapshotImportsStackAndOriginalPixelsAtomically(t *testing.T) {
	dir, library := t.TempDir(), t.TempDir()
	m := adobeManifest{Host: "photoshop", Name: "拼贴", Version: 1, Width: 200, Height: 180, Layers: []adobeItem{
		{File: "layer-0.png", Name: "底片", Group: "摄影 / 建筑", X: 10, Y: 12, W: 160, H: 100},
		{File: "layer-1.png", Name: "遮罩叠图", Group: "版面", X: 62, Y: 56, W: 90, H: 65},
	}}
	saveManifest := func() {
		if err := os.WriteFile(filepath.Join(dir, "manifest.json"), encode(m), 0600); err != nil {
			t.Fatal(err)
		}
	}
	saveManifest()
	adobeTestPNG(t, dir, "composite.png", 200, 180)
	adobeTestPNG(t, dir, "layer-0.png", 640, 400)
	store, err := openStore(library)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.importAdobeSnapshot(dir); err == nil {
		t.Fatal("accepted partial export")
	}
	if ps, err := store.Projects(); err != nil || len(ps) != 0 {
		t.Fatalf("partial project leaked: %v %v", ps, err)
	}
	var count int
	if err := store.DB.QueryRow("SELECT count(*) FROM assets").Scan(&count); err != nil || count != 0 {
		t.Fatalf("partial assets leaked: %d %v", count, err)
	}
	adobeTestPNG(t, dir, "layer-1.png", 360, 260)
	id, err := store.importAdobeSnapshot(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err = store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = openStore(library)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ps, err := store.Projects()
	if err != nil || len(ps) != 1 || ps[0].ID != id || len(ps[0].Graph.Nodes) != 2 {
		t.Fatal(ps, err)
	}
	if b := ps[0].AdobeBoard; b == nil || b.Width != 200 || b.Height != 180 {
		t.Fatal("lost source artboard bounds", b)
	}
	assets, err := store.Assets(id)
	if err != nil || len(assets) != 3 {
		t.Fatal(assets, err)
	}
	editor := newEditor(ps[0], assets, nil)
	for i, n := range ps[0].Graph.Nodes {
		l := m.Layers[i]
		a := editor.Assets[n.Asset]
		if n.Name != l.Name || n.Adobe.Group != l.Group || nodeRect(n, editor.Assets) != (Rect{l.X, l.Y, l.W, l.H}) {
			t.Fatal("lost layer layout", i, n)
		}
		if a.NodeID != n.ID || a.Source != "photoshop" || a.Width != int(l.W)*4 || a.Height != int(l.H)*4 || a.Mime != "image/png" {
			t.Fatal("lost source resolution", i, a)
		}
		original, err := os.ReadFile(filepath.Join(dir, l.File))
		if err != nil {
			t.Fatal(err)
		}
		stored, err := os.ReadFile(filepath.Join(library, "assets", a.File))
		if err != nil || digest(original) != digest(stored) {
			t.Fatal("changed PNG", err)
		}
	}
	if events, err := store.Operations(id); err != nil || len(events) != 1 || len(events[0].Nodes) != 2 {
		t.Fatal(events, err)
	}
	m.Host = "illustrator"
	m.Layers = nil
	saveManifest()
	id, err = store.importAdobeSnapshot(dir)
	if err != nil {
		t.Fatal(err)
	}
	ps, err = store.Projects()
	if err != nil || len(ps) != 2 {
		t.Fatal(ps, err)
	}
	n := ps[1].Graph.Nodes[0]
	if ps[1].ID != id || n.Asset == "" || n.Adobe.W != 200 || n.Adobe.H != 180 {
		t.Fatal("empty separated export lost its composite", n)
	}
}
