package main

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"errors"
	"image"
	_ "image/gif"
	"io"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"
	_ "golang.org/x/image/bmp"
	"golang.org/x/text/encoding/simplifiedchinese"
	textunicode "golang.org/x/text/encoding/unicode"
)

const filePreviewTextLimit = 128 << 10
const filePreviewImageLimit = 40 << 20

type filePreview struct {
	path, name, detail, notice string
	bitmap                     *ui.Bitmap
	lines                      []string
	list                       ui.ListState
	useShell                   bool
	pageRatio                  float64
	pdf                        *filePDFPreview
}

func loadFilePreview(path string) filePreview {
	p := filePreview{path: path, name: filepath.Base(path)}
	info, err := os.Stat(path)
	if err != nil {
		p.notice = "文件暂不可读取"
		return p
	}
	if info.IsDir() {
		p.detail, p.notice = "文件夹", "这是一个文件夹"
		return p
	}
	if !info.Mode().IsRegular() {
		p.notice = "文件内容暂不可预览"
		return p
	}
	p.detail = imageByteSize(info.Size())
	extension := strings.ToLower(filepath.Ext(path))
	switch extension {
	case ".bmp", ".gif", ".jpeg", ".jpg", ".png", ".webp":
		err = p.readImage(info.Size())
	case "", ".txt", ".log", ".md", ".markdown", ".csv", ".tsv", ".json", ".xml", ".yaml", ".yml", ".toml", ".ini", ".rst", ".srt",
		".go", ".py", ".rs", ".rb", ".c", ".h", ".cpp", ".cs", ".java", ".js", ".ts", ".jsx", ".tsx", ".css", ".html", ".sql", ".sh", ".ps1", ".bat", ".cmd":
		p.pageRatio = paperPreviewRatio
		err = p.readText()
	case ".pdf":
		p.pdf, err = openFilePDFPreview(path)
		p.pageRatio = paperPreviewRatio
		if p.pdf != nil {
			p.pageRatio = p.pdf.ratio
		} else {
			p.useShell = true
		}
	case ".doc", ".docx", ".odt", ".rtf", ".xls", ".xlsx", ".ods", ".ppt", ".pptx", ".odp":
		p.useShell = true
		p.pageRatio = officePreviewRatio(path, extension)
	default:
		p.useShell = true
	}
	if err != nil {
		p.notice = err.Error()
	}
	return p
}

const paperPreviewRatio = 210.0 / 297
const filePreviewPadding = 12

func filePreviewSize(p filePreview, workWidth, workHeight int, scale float64) (int, int) {
	const top = filePreviewHeaderHeight + 1
	const padding = filePreviewPadding * 2
	width, height := max(1, workWidth-2*filePreviewPadding), max(1, workHeight-2*filePreviewPadding)
	contentW, contentH := float64(max(1, width-padding)), float64(max(1, height-top-padding))
	var w, h float64
	switch {
	case p.bitmap != nil:
		bw, bh := p.bitmap.Size()
		scale = max(1, scale)
		w, h = float64(bw)/scale, float64(bh)/scale
	case p.pageRatio > 0:
		h = .75 * float64(workHeight)
		w = h * p.pageRatio
	default:
		return min(width, 820), min(height, 620)
	}
	fit := min(1, contentW/w, contentH/h)
	return min(width, max(148, int(math.Round(w*fit))+padding)), min(height, int(math.Round(h*fit))+top+padding)
}

func officePreviewRatio(path, extension string) float64 {
	var member, element, width, height string
	ratio := paperPreviewRatio
	switch extension {
	case ".docx":
		member, element, width, height = "word/document.xml", "pgSz", "w", "h"
	case ".pptx":
		ratio = 16.0 / 9
		member, element, width, height = "ppt/presentation.xml", "sldSz", "cx", "cy"
	case ".ppt", ".odp":
		return 16.0 / 9
	default:
		return ratio
	}
	archive, err := zip.OpenReader(path)
	if err != nil {
		return ratio
	}
	defer archive.Close()
	for _, file := range archive.File {
		if file.Name != member {
			continue
		}
		reader, err := file.Open()
		if err != nil {
			return ratio
		}
		defer reader.Close()
		decoder := xml.NewDecoder(io.LimitReader(reader, 4<<20))
		for {
			token, err := decoder.Token()
			if err != nil {
				return ratio
			}
			start, ok := token.(xml.StartElement)
			if !ok || start.Name.Local != element {
				continue
			}
			var w, h float64
			for _, attr := range start.Attr {
				switch attr.Name.Local {
				case width:
					w, _ = strconv.ParseFloat(attr.Value, 64)
				case height:
					h, _ = strconv.ParseFloat(attr.Value, 64)
				}
			}
			if aspect := w / h; aspect >= .1 && aspect <= 10 {
				return aspect
			}
			return ratio
		}
	}
	return ratio
}

func (p *filePreview) readImage(size int64) error {
	if size > filePreviewImageLimit {
		return errors.New("图片超出快速预览大小")
	}
	data, err := readBounded(p.path, filePreviewImageLimit)
	if err != nil {
		return errors.New("无法读取图片")
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || cfg.Width < 1 || cfg.Height < 1 || int64(cfg.Width) > 28_000_000/int64(cfg.Height) {
		return errors.New("图片损坏或超出快速预览分辨率")
	}
	p.bitmap, err = ui.DecodeBitmap(data)
	if err != nil {
		return errors.New("无法解码图片")
	}
	return nil
}

func (p *filePreview) readText() error {
	f, err := os.Open(p.path)
	if err != nil {
		return errors.New("无法读取文件")
	}
	defer f.Close()
	raw, err := io.ReadAll(io.LimitReader(f, filePreviewTextLimit+1))
	if err != nil {
		return errors.New("无法读取文件")
	}
	truncated := len(raw) > filePreviewTextLimit
	raw = raw[:min(len(raw), filePreviewTextLimit)]
	var decoded []byte
	switch {
	case bytes.HasPrefix(raw, []byte{0xff, 0xfe}), bytes.HasPrefix(raw, []byte{0xfe, 0xff}):
		decoded, err = textunicode.UTF16(textunicode.LittleEndian, textunicode.UseBOM).NewDecoder().Bytes(raw)
	default:
		decoded = bytes.TrimPrefix(raw, []byte{0xef, 0xbb, 0xbf})
		if truncated && !utf8.Valid(decoded) && len(decoded) > utf8.UTFMax {
			// Keep the final complete UTF-8 rune at the byte limit.
			for i := 1; i < utf8.UTFMax; i++ {
				if utf8.Valid(decoded[:len(decoded)-i]) {
					decoded = decoded[:len(decoded)-i]
					break
				}
			}
		}
		if !utf8.Valid(decoded) {
			decoded, err = simplifiedchinese.GB18030.NewDecoder().Bytes(decoded)
			if bytes.Contains(decoded, []byte("\uFFFD")) {
				return errors.New("此文本编码暂不可预览")
			}
		}
	}
	if err != nil {
		return errors.New("此文本编码暂不可预览")
	}
	for _, r := range string(decoded) {
		if unicode.IsControl(r) && r != '\t' && r != '\n' && r != '\r' {
			return errors.New("此文件没有可显示的文本")
		}
	}
	if len(decoded) == 0 {
		p.notice = "空文件"
		return nil
	}
	lines := strings.NewReplacer("\r\n", "\n", "\r", "\n").Replace(string(decoded))
	p.lines = strings.Split(lines, "\n")
	if truncated {
		p.notice = "仅显示文件开头"
	}
	return nil
}

const filePreviewHeaderHeight = 46

func filePreviewView(c *ui.Context, p *filePreview, appearance string, close func()) {
	defer beginHints(c)()
	c.SetTheme(workspaceTheme(c, appearance))
	c.Root().Background(c.Theme().Background)
	c.OnShortcut(0, ui.KeyEscape, close)
	c.OnShortcut(0, ui.KeySpace, close)
	ui.Column(c).Fill().Children(func() {
		ui.Row(c).FillWidth().Height(filePreviewHeaderHeight).Padding(0, 14).Gap(9).AlignItems(ui.Center).DragWindow().Children(func() {
			fittingLabel(c, p.name, 13, 500).Grow(1).Role(ui.RoleHeading).Level(1)
			ui.Text(c, p.detail).SingleLine().FontSize(11).TextColor(c.Theme().TextMuted)
			iconButton(c, "close", "关闭预览", close, "关闭预览 · Esc")
		})
		ui.Box(c).FillWidth().Height(1).Shrink(0).Background(c.Theme().SurfacePressed)
		switch {
		case p.bitmap != nil:
			ui.Box(c).Grow(1).FillWidth().MinHeight(0).Label("图片预览").Draw(func(container *ui.Painter, r ui.Rect) {
				const pad = filePreviewPadding
				container.Image(p.bitmap, ui.Rect{X: r.X + pad, Y: r.Y + pad, W: max(0, r.W-2*pad), H: max(0, r.H-2*pad)}, ui.Contain)
			})
		case p.pdf != nil:
			p.pdf.view(c)
		case len(p.lines) != 0:
			thinScroll(c, ui.List(c.Key("file-lines"), &p.list, len(p.lines), func(i int) {
				ui.Text(c, p.lines[i]).FontSize(13).Font("monospace").MinHeight(18)
			}).Label("文本预览"), nil).Grow(1).MinHeight(0).FillWidth().Padding(12, 16)
			if p.notice != "" {
				ui.Text(c, p.notice).Padding(6, 16).FontSize(11).TextColor(c.Theme().TextMuted)
			}
		default:
			ui.Column(c).Grow(1).FillWidth().Center().Gap(8).Children(func() {
				if p.notice != "" {
					uiIcon(c, "file", 24).TextColor(c.Theme().TextMuted)
					ui.Text(c, p.notice).FontSize(12).TextColor(c.Theme().TextMuted)
				}
			})
		}
	})
}
