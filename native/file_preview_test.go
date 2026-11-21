package main

import (
	"image"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestFilePreviewWindowTracksImageAndDocumentGeometry(t *testing.T) {
	const chrome = filePreviewHeaderHeight + 1 + 2*filePreviewPadding
	small := filePreview{bitmap: ui.NewBitmap(image.NewRGBA(image.Rect(0, 0, 320, 180)))}
	w, h := filePreviewSize(small, 1920, 1040, 1.25)
	if w-2*filePreviewPadding != 256 || h-chrome != 144 {
		t.Fatalf("small image at native pixel size: %dx%d", w, h)
	}
	tall := filePreview{bitmap: ui.NewBitmap(image.NewRGBA(image.Rect(0, 0, 1100, 3100)))}
	w, h = filePreviewSize(tall, 1920, 1040, 1)
	if h != 1040-2*filePreviewPadding || math.Abs(float64(w-2*filePreviewPadding)/float64(h-chrome)-1100.0/3100) > .005 {
		t.Fatalf("tall image fitted without stretching: %dx%d", w, h)
	}
	w, h = filePreviewSize(filePreview{pageRatio: paperPreviewRatio}, 1920, 1040, 1)
	if h-chrome != 780 || int(math.Round(paperPreviewRatio*780)) != w-2*filePreviewPadding {
		t.Fatalf("portrait page fits three quarters of the work area: %dx%d", w, h)
	}
	w, h = filePreviewSize(filePreview{pageRatio: 16.0 / 9}, 760, 1080, 1)
	if w != 760-2*filePreviewPadding || math.Abs(float64(w-2*filePreviewPadding)/float64(h-chrome)-16.0/9) > .01 {
		t.Fatalf("landscape page scales down on a narrow monitor: %dx%d", w, h)
	}
}

func TestLocalFilePreviewReadsOnlyRequestedContent(t *testing.T) {
	dir := t.TempDir()
	load := func(name string, data []byte) filePreview {
		t.Helper()
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
		return loadFilePreview(path)
	}
	text := load("notes.txt", []byte{0xff, 0xfe, 'A', 0, '\r', 0, '\n', 0, 0x2d, 0x4e})
	if len(text.lines) != 2 || text.lines[1] != "中" {
		t.Fatal("UTF-16 and CRLF not decoded", text)
	}
	large := load("large.log", []byte(strings.Repeat("a", filePreviewTextLimit-1)+"中文"))
	if len(large.lines) != 1 || len(large.lines[0]) != filePreviewTextLimit-1 || !strings.Contains(large.notice, "文件开头") {
		t.Fatal("partial UTF-8 at the preview limit")
	}
	binary := load("binary.txt", []byte("before\x00after"))
	if binary.notice == "" || len(binary.lines) != 0 {
		t.Fatal("binary bytes displayed as text")
	}
	png := load("photo.png", tinyPNG())
	if png.bitmap == nil {
		t.Fatal("image preview could not decode", png.notice)
	}
	if doc := load("document.pdf", []byte("%PDF-1.4")); !doc.useShell || doc.bitmap != nil || len(doc.lines) != 0 {
		t.Fatal("document should be offered to the Windows preview handler")
	}

	closed := false
	view := ui.NewTester(func(c *ui.Context) { filePreviewView(c, &text, "dark", func() { closed = true }) }, 680, 460)
	if !view.HasText("中") {
		t.Fatal("decoded text missing from rendered preview")
	}
	view.Key(0, ui.KeyEscape)
	if !closed {
		t.Fatal("Escape did not close the preview")
	}
}
