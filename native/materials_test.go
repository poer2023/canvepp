package main

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func TestAssistantAttachmentSnapshotAndInput(t *testing.T) {
	root := t.TempDir()
	original := filepath.Join(root, "图.png")
	if err := os.WriteFile(original, tinyPNG(), 0600); err != nil {
		t.Fatal(err)
	}
	f, err := stageAssistantAttachment(filepath.Join(root, "app"), original)
	if err != nil || !f.Image {
		t.Fatal(f, err)
	}
	os.Remove(original)
	b, err := os.ReadFile(f.Path)
	if err != nil || !bytes.Equal(b, tinyPNG()) {
		t.Fatal("attachment lost after source deletion", err)
	}
	doc := filepath.Join(root, "note.txt")
	os.WriteFile(doc, []byte("文档内容 \"保留\""), 0600)
	d, err := stageAssistantAttachment(filepath.Join(root, "app"), doc)
	if err != nil || d.Image {
		t.Fatal(d, err)
	}
	input := assistantInputs("看看附件", []assistantAttachment{f, d})
	if len(input) != 4 || input[1]["type"] != "localImage" || input[1]["path"] != f.Path || input[2]["type"] != "mention" || !strings.Contains(input[3]["text"].(string), "文档内容") {
		t.Fatal(input)
	}
	if _, err := stageAssistantAttachment(root, root); err == nil {
		t.Fatal("accepted directory")
	}
	big := filepath.Join(root, "too-large.bin")
	file, _ := os.Create(big)
	file.Truncate(maxAttachmentBytes + 1)
	file.Close()
	if _, err := stageAssistantAttachment(root, big); err == nil {
		t.Fatal("accepted oversized attachment")
	}
}
func TestEagleReadOnlySearchAndImportPaths(t *testing.T) {
	for _, v2 := range []bool{false, true} {
		t.Run(map[bool]string{true: "v2", false: "legacy"}[v2], func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "素材.library")
			dir := filepath.Join(root, "images", "ABC.info")
			os.MkdirAll(dir, 0700)
			original := filepath.Join(dir, "青色 茶杯.png")
			os.WriteFile(original, tinyPNG(), 0600)
			item := eagleItem{ID: "ABC", Name: "青色 茶杯", Ext: "png"}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "GET" {
					t.Error("mutated Eagle", r.Method)
				}
				var data any
				switch r.URL.Path {
				case "/api/v2/library/info":
					if !v2 {
						http.NotFound(w, r)
						return
					}
					data = map[string]any{"path": root, "name": "素材"}
				case "/api/library/info":
					data = map[string]any{"applicationVersion": "3"}
				case "/api/item/thumbnail":
					if r.URL.Query().Get("id") != "ABC" {
						t.Error(r.URL)
					}
					data = original
				case "/api/item/list", "/api/v2/item/get":
					param := "keyword"
					if v2 {
						param = "keywords"
					}
					if r.URL.Query().Get(param) != "茶 & 杯" || r.URL.Query().Get("offset") != "48" {
						t.Error("lost escaped search or page", r.URL)
					}
					items := []eagleItem{item, {ID: "video", Name: "movie", Ext: "mp4"}, {ID: "deleted", Name: "gone", Ext: "png", Deleted: true}}
					data = items
					if v2 {
						data = map[string]any{"data": items, "total": 100}
					}
				default:
					t.Error("unexpected API", r.URL)
					http.NotFound(w, r)
					return
				}
				json.NewEncoder(w).Encode(map[string]any{"status": "success", "data": data})
			}))
			defer server.Close()
			e := newEagleClient()
			e.Base = server.URL
			e.HTTP = server.Client()
			if err := e.Connect(context.Background()); err != nil {
				t.Fatal(err)
			}
			page, err := e.List(context.Background(), "茶 & 杯", 48)
			if err != nil || len(page.Items) != 1 || page.Offset != 51 {
				t.Fatal(page, err)
			}
			path, err := e.Original(context.Background(), item)
			expected, _ := filepath.EvalSymlinks(original)
			if err != nil || path != expected {
				t.Fatal(path, err)
			}
			item.Name = "../../escape"
			if _, err := e.Original(context.Background(), item); err == nil {
				t.Fatal("path traversal")
			}
			os.Remove(original)
			outside := filepath.Join(t.TempDir(), "private.png")
			os.WriteFile(outside, tinyPNG(), 0600)
			os.Symlink(outside, original)
			item.Name = "青色 茶杯"
			if _, err := e.Original(context.Background(), item); err == nil {
				t.Fatal("symlink escape")
			}
		})
	}
}
func TestPinterestURLsAndMaterialProvenance(t *testing.T) {
	for _, bad := range []string{"http://i.pinimg.com/a.png", "https://i.pinimg.com.evil.test/a.png", "https://localhost/a.png", "file:///tmp/p.png", "https://user@i.pinimg.com/a.png", "https://i.pinimg.com:444/a.png"} {
		if pinImageURL(bad) {
			t.Fatal(bad)
		}
	}
	if !pinImageURL("https://i.pinimg.com/originals/ab/cd.png") || !pinterestURL("https://www.pinterest.com/pin/123/") {
		t.Fatal("expected Pinterest URL")
	}
	s := testStore(t)
	e := fixture()
	s.Save(e.Project, nil)
	a, err := s.ImportImage(tinyPNG(), e.Project.ID, "0", "Pin.png", "pinterest", "", nil, assetOrigin{URL: "https://www.pinterest.com/pin/123/", ID: "pin/123"})
	if err != nil {
		t.Fatal(err)
	}
	assets, err := s.Assets(e.Project.ID)
	if err != nil || len(assets) != 1 || assets[0].SourceURL != a.SourceURL || assets[0].ExternalID != "pin/123" {
		t.Fatal(assets, err)
	}
	// Undo only removes the canvas placement; imported pixels remain recoverable.
	e.Assets[a.ID] = a
	n := newNode(e.Project.ID, Point{})
	n.Asset = a.ID
	e.AddNamed([]Node{n}, nil, "从 Pinterest 导入")
	e.Undo()
	path, _ := s.AssetPath(a)
	if b, err := os.ReadFile(path); err != nil || !bytes.Equal(b, tinyPNG()) {
		t.Fatal("lost original", err)
	}
}

func TestOperationFractionalTimestampOrdering(t *testing.T) {
	s := testStore(t)
	events := []Operation{
		{ID: "a", ProjectID: "fraction", Action: "import", Text: "early", At: "2026-11-06T12:00:00.12Z", UpdatedAt: "2026-11-06T12:00:00.12Z"},
		{ID: "b", ProjectID: "fraction", Action: "trash", Text: "late", At: "2026-11-06T12:00:00.12001Z", UpdatedAt: "2026-11-06T12:00:00.12001Z"},
		{ID: "0", ProjectID: "fraction", Action: "restore", Text: "same tick", At: "2026-11-06T12:00:00.12001Z", UpdatedAt: "2026-11-06T12:00:00.12001Z"},
	}
	tx, _ := s.DB.Begin()
	if err := saveOperations(tx, events); err != nil {
		t.Fatal(err)
	}
	tx.Commit()
	got, err := s.Operations("fraction")
	if err != nil || len(got) != 3 || got[0].ID != "a" || got[1].ID != "b" || got[2].ID != "0" {
		t.Fatal(got, err)
	}
	older, err := s.OperationPage("fraction", events[1])
	if err != nil || len(older) != 1 || older[0].ID != "a" {
		t.Fatal(older, err)
	}
	older, err = s.OperationPage("fraction", events[2])
	if err != nil || len(older) != 2 || older[1].ID != "b" {
		t.Fatal(older, err)
	}
	events[0].Text = "updated"
	events[0].UpdatedAt = "2026-11-06T12:00:00.12002Z"
	tx, _ = s.DB.Begin()
	saveOperations(tx, events[:1])
	tx.Commit()
	got, _ = s.Operations("fraction")
	if got[0].Text != "updated" {
		t.Fatal("coalesced edit did not save", got)
	}
}

func TestAssistantLayoutContainsControls(t *testing.T) {
	for _, draft := range []string{"", "第一行\n第二行\n第三行", strings.Repeat("这是自动换行的长文本。", 100)} {
		a := testDesktop()
		a.assistant = &assistantBar{draft: draft, attachments: []assistantAttachment{{Name: "note.txt", Path: "/fixture/note.txt"}}}
		ui.NewTester(a.assistantView, 640, 400).Frame()
		height := a.assistant.contentHeight
		tt := ui.NewTester(a.assistantView, 640, height)
		tt.Frame()
		for _, label := range []string{"助手输入", "关闭助手 · Esc", "添加图片或文件", "移除 note.txt"} {
			r, ok := tt.Find(label)
			if !ok || r.Y < 0 || r.Y+r.H > float32(height)-1 {
				t.Fatalf("%q clipped: %v in %d", label, r, height)
			}
		}
		if height > 350 {
			t.Fatal("unbounded assistant height", height)
		}
	}
}
func TestLiveAssistantAttachments(t *testing.T) {
	if os.Getenv("CANVEPP_LIVE_ATTACH_CHECK") != "1" {
		t.Skip("explicit synthetic attachment acceptance")
	}
	s := testStore(t)
	root := t.TempDir()
	p := filepath.Join(root, "note.txt")
	os.WriteFile(p, []byte("唯一验收标记是 CANVEPP-ATTACH-602。"), 0600)
	f, err := stageAssistantAttachment(s.Dir, p)
	if err != nil {
		t.Fatal(err)
	}
	img := filepath.Join(root, "pixel.png")
	pixels := image.NewRGBA(image.Rect(0, 0, 128, 128))
	draw.Draw(pixels, pixels.Bounds(), image.NewUniform(color.RGBA{20, 90, 230, 255}), image.Point{}, draw.Src)
	var imageBytes bytes.Buffer
	png.Encode(&imageBytes, pixels)
	os.WriteFile(img, imageBytes.Bytes(), 0600)
	g, err := stageAssistantAttachment(s.Dir, img)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	reply, err := requestCodexInput(ctx, s, modelChoice{}, "", assistantInstructions, assistantInputs("请读取附件 note.txt 的唯一验收标记，并观察所附图片的主色。只回复标记与中文颜色名，不需要生成或修改任何图片。", []assistantAttachment{f, g}), true, nil)
	if err != nil || !strings.Contains(reply.Text, "CANVEPP-ATTACH-602") || !strings.Contains(reply.Text, "蓝") {
		t.Fatal(reply.Text, err)
	}
	t.Log("Codex read the actual document token and correctly recognized the attached image color; no image generation")
}

func TestLiveAssistantPDF(t *testing.T) {
	if os.Getenv("CANVEPP_LIVE_PDF_CHECK") != "1" {
		t.Skip("explicit synthetic PDF attachment acceptance")
	}
	s := testStore(t)
	path := filepath.Join(t.TempDir(), "sample.pdf")
	os.WriteFile(path, attachmentPDF("CANVEPP-PDF-603"), 0600)
	f, err := stageAssistantAttachment(s.Dir, path)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	reply, err := requestCodexInput(ctx, s, modelChoice{}, "", assistantInstructions, assistantInputs("请阅读所附 sample.pdf 的已提取正文，只回复其中的 Verification token。", []assistantAttachment{f}), true, nil)
	if err != nil || !strings.Contains(reply.Text, "CANVEPP-PDF-603") {
		t.Fatal(reply.Text, err)
	}
	t.Log("Codex received and read the actual PDF text extracted locally; no image generation")
}

func attachmentPDF(token string) []byte {
	var pdf bytes.Buffer
	pdf.WriteString("%PDF-1.4\n")
	stream := "BT /F1 14 Tf 40 100 Td (Verification token: " + token + ") Tj ET"
	objects := []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 400 200] /Resources << /Font << /F1 4 0 R >> >> /Contents 5 0 R >>",
		"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
		fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(stream), stream),
	}
	offsets := []int{}
	for i, o := range objects {
		offsets = append(offsets, pdf.Len())
		fmt.Fprintf(&pdf, "%d 0 obj\n%s\nendobj\n", i+1, o)
	}
	xref := pdf.Len()
	fmt.Fprintf(&pdf, "xref\n0 %d\n0000000000 65535 f \n", len(objects)+1)
	for _, offset := range offsets {
		fmt.Fprintf(&pdf, "%010d 00000 n \n", offset)
	}
	fmt.Fprintf(&pdf, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objects)+1, xref)
	return pdf.Bytes()
}

func TestAssistantDocumentContentsAndFailure(t *testing.T) {
	root := t.TempDir()
	var word bytes.Buffer
	z := zip.NewWriter(&word)
	entries := map[string]string{
		"[Content_Types].xml": `<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="xml" ContentType="application/xml"/><Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/></Types>`,
		"_rels/.rels":         `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/></Relationships>`,
		"word/document.xml":   `<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body><w:p><w:r><w:t>WORD-TOKEN 文档正文</w:t></w:r></w:p></w:body></w:document>`,
	}
	for name, body := range entries {
		w, _ := z.Create(name)
		w.Write([]byte(body))
	}
	z.Close()
	for _, tc := range []struct {
		name string
		data []byte
		want string
	}{
		{"sample.pdf", attachmentPDF("PDF-LOCAL-604"), "PDF-LOCAL-604"},
		{"sample.docx", word.Bytes(), "WORD-TOKEN 文档正文"},
		{"sample.rtf", []byte(`{\rtf1\ansi RTF-TOKEN}`), "RTF-TOKEN"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(root, tc.name)
			os.WriteFile(path, tc.data, 0600)
			f, err := stageAssistantAttachment(root, path)
			if err != nil || !strings.Contains(f.Text, tc.want) {
				t.Fatal("missing actual document contents", f.Text, err)
			}
			os.Remove(path)
			os.Remove(f.Path)
			inputs := assistantInputs("read", []assistantAttachment{f})
			if !strings.Contains(inputs[2]["text"].(string), tc.want) {
				t.Fatal("input should use the staged extraction, not synchronous disk reads")
			}
		})
	}
	for _, tc := range []struct {
		name string
		data []byte
	}{
		{"broken.pdf", []byte("not a PDF")},
		{"binary.bin", []byte{0, 1, 2, 3}},
		{"too-long.txt", bytes.Repeat([]byte("x"), maxDocumentTextBytes+1)},
		{"empty.txt", []byte(" \n\t")},
	} {
		path := filepath.Join(root, tc.name)
		os.WriteFile(path, tc.data, 0600)
		if _, err := stageAssistantAttachment(root, path); err == nil {
			t.Fatal("silently accepted unreadable attachment", tc.name)
		}
	}
}

func TestPinterestDropCacheHonorsResponseFreshness(t *testing.T) {
	for _, tc := range []struct {
		control string
		age     string
		want    time.Duration
	}{
		{"public, max-age=90", "20", 70 * time.Second},
		{"public, max-age=86400", "", 5 * time.Minute},
		{"public, max-age=90, no-store", "", 0},
		{"no-cache, max-age=2", "", 0},
		{"private, max-age=60", "", 0},
		{"", "", 0},
	} {
		h := http.Header{}
		h.Set("Cache-Control", tc.control)
		h.Set("Age", tc.age)
		if got := pinFreshness(h); got != tc.want {
			t.Errorf("%s age %s: got %v, want %v", tc.control, tc.age, got, tc.want)
		}
	}
}
