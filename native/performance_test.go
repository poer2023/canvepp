package main

import (
	"fmt"
	"github.com/egoist/mygo/ui"
	"image"
	"image/color"
	"testing"
)

func perfDesktop(count int) *desktop {
	a := testDesktop()
	e := a.editor
	e.Project.Graph.Nodes = nil
	e.Project.Graph.Edges = nil
	e.View = Viewport{X: 20, Y: 60, Zoom: .31}
	a.images = &ImageCache{entries: map[string]*cachedImage{}, pending: map[string]bool{}, queue: make(chan imageRequest, 1)}
	im := image.NewRGBA(image.Rect(0, 0, 512, 512))
	for y := 0; y < 512; y++ {
		for x := 0; x < 512; x++ {
			im.SetRGBA(x, y, color.RGBA{uint8(x / 2), uint8(y / 2), 144, 255})
		}
	}
	bitmap := ui.NewBitmap(im)
	for i := 0; i < count; i++ {
		id := fmt.Sprint(i)
		asset := Asset{ID: id, SHA: id, Width: 1024, Height: 1024}
		e.Assets[id] = asset
		n := newNode(e.Project.ID, Point{float64(i%20) * 270, float64(i/20) * 270})
		n.ID = id
		n.Asset = id
		e.Project.Graph.Nodes = append(e.Project.Graph.Nodes, n)
		a.images.entries[imageKey(asset, 512)] = &cachedImage{Bitmap: bitmap}
	}
	e.reindex()
	a.panel.Open = true
	return a
}
func BenchmarkNativeView(b *testing.B) {
	for _, count := range []int{100, 500} {
		b.Run(fmt.Sprint(count), func(b *testing.B) {
			a := perfDesktop(count)
			tt := ui.NewTester(a.view, 1440, 900)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				a.editor.View.X = 20 + float64(i%80)
				tt.Frame()
			}
			b.StopTimer()
			b.ReportMetric(a.performanceReceipt()["paintCPU_p95_ms"].(float64), "paint-p95-ms")
		})
	}
}

func BenchmarkSelectedDraft500(b *testing.B) {
	a := perfDesktop(500)
	for _, n := range a.editor.Project.Graph.Nodes {
		a.editor.Selection[n.ID] = true
	}
	a.syncDraft()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		a.syncDraft()
	}
}
