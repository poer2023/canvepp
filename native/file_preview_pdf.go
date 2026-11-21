package main

import (
	"context"
	"errors"
	"fmt"
	"image"
	"image/draw"
	"io"
	"math"

	"github.com/egoist/mygo/ui"
	"github.com/klippa-app/go-pdfium"
	"github.com/klippa-app/go-pdfium/references"
	"github.com/klippa-app/go-pdfium/requests"
	"github.com/klippa-app/go-pdfium/webassembly"
	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/experimental"
)

var documentPDFCompilation = wazero.NewCompilationCache()

func newLocalPDFPool(ctx context.Context) (pdfium.Pool, error) {
	return webassembly.Init(webassembly.Config{
		Context: ctx, MaxTotal: 1,
		FSConfig: wazero.NewFSConfig(),
		RuntimeConfig: wazero.NewRuntimeConfig().
			WithCoreFeatures(api.CoreFeaturesV2 | experimental.CoreFeaturesExceptionHandling).
			WithCompilationCache(documentPDFCompilation).
			WithMemoryLimitPages(4096).
			WithCloseOnContextDone(true),
		Stdout: io.Discard, Stderr: io.Discard,
	})
}

type filePDFPreview struct {
	ratio float64
	count int
	list  ui.ListState

	ctx      context.Context
	cancel   context.CancelFunc
	release  func()
	pdf      pdfium.Pdfium
	document references.FPDF_DOCUMENT
	started  bool
	requests chan int
	rendered map[int]*ui.Bitmap
	waiting  map[int]bool
	width    int
	height   int
}

func openFilePDFPreview(path string) (*filePDFPreview, error) {
	data, err := readBounded(path, 64<<20)
	if err != nil {
		return nil, errors.New("PDF 无法读取或超出预览大小")
	}
	ctx, cancel := context.WithCancel(context.Background())
	p := &filePDFPreview{ctx: ctx, cancel: cancel}
	pool, err := newLocalPDFPool(ctx)
	if err != nil {
		p.close()
		return nil, errors.New("PDF 预览初始化失败")
	}
	p.release = func() { _ = pool.Close() }
	p.pdf, err = pool.GetInstanceWithContext(ctx)
	if err != nil {
		p.close()
		return nil, errors.New("PDF 预览初始化失败")
	}
	p.release = func() { _ = p.pdf.Close(); _ = pool.Close() }
	doc, err := p.pdf.OpenDocument(&requests.OpenDocument{File: &data})
	if err != nil {
		p.close()
		return nil, errors.New("PDF 已加密或文件不完整")
	}
	p.document = doc.Document
	pages, err := p.pdf.FPDF_GetPageCount(&requests.FPDF_GetPageCount{Document: p.document})
	if err != nil || pages.PageCount < 1 {
		p.close()
		return nil, errors.New("PDF 未找到可预览的页面")
	}
	p.count = pages.PageCount
	bounds, err := p.pdf.GetPageSize(&requests.GetPageSize{Page: p.pageRef(0)})
	if err != nil || bounds.Width <= 0 || bounds.Height <= 0 {
		p.close()
		return nil, errors.New("无法读取 PDF 页面尺寸")
	}
	p.ratio = bounds.Width / bounds.Height
	return p, nil
}

func (p *filePDFPreview) pageRef(index int) requests.Page {
	return requests.Page{ByIndex: &requests.PageByIndex{Document: p.document, Index: index}}
}

func (p *filePDFPreview) render(index, width, height int) *ui.Bitmap {
	page, err := p.pdf.RenderPageInPixels(&requests.RenderPageInPixels{Page: p.pageRef(index), Width: width, Height: height})
	if err != nil {
		return nil
	}
	defer page.Cleanup()
	if page.Result.RenderedImage == nil {
		return nil
	}
	source := page.Result.RenderedImage
	bounds := source.Bounds()
	bitmap := image.NewRGBA(image.Rect(0, 0, bounds.Dx(), bounds.Dy()))
	draw.Draw(bitmap, bitmap.Bounds(), source, bounds.Min, draw.Src)
	return ui.NewBitmap(bitmap)
}

func (p *filePDFPreview) start(w, h int, scale float64, publish func(func())) {
	p.started, p.width, p.height = true, max(1, w), max(1, h)
	p.requests = make(chan int, 4)
	p.rendered, p.waiting = map[int]*ui.Bitmap{}, map[int]bool{}
	scale = min(max(1, scale), 1600/float64(max(p.width, p.height)))
	width, height := max(1, int(float64(p.width)*scale)), max(1, int(float64(p.height)*scale))
	go func() {
		defer p.release()
		for {
			select {
			case <-p.ctx.Done():
				return
			case index := <-p.requests:
				bitmap := p.render(index, width, height)
				publish(func() {
					delete(p.waiting, index)
					p.rendered[index] = bitmap
					for cached := range p.rendered {
						if cached < index-4 || cached > index+4 {
							delete(p.rendered, cached)
						}
					}
				})
			}
		}
	}()
}

func (p *filePDFPreview) close() {
	p.cancel()
	if !p.started && p.release != nil {
		p.release()
	}
}

func (p *filePDFPreview) view(c *ui.Context) {
	list := ui.List(c.Key("preview-pages"), &p.list, p.count, func(index int) {
		bitmap, loaded := p.rendered[index]
		if !loaded && !p.waiting[index] && p.requests != nil {
			select {
			case p.requests <- index:
				p.waiting[index] = true
			default:
			}
		}
		h := float32(max(1, p.height))
		if bitmap != nil {
			width, height := bitmap.Size()
			h = min(h, float32(math.Round(float64(p.width)*float64(height)/float64(width))))
		}
		ui.Column(c.Key(index)).FillWidth().Height(h+filePreviewPadding).Padding(0, filePreviewPadding, filePreviewPadding).
			Label(fmt.Sprintf("第 %d 页", index+1)).Children(func() {
			if bitmap != nil {
				ui.Box(c).Fill().Draw(func(painter *ui.Painter, rect ui.Rect) { painter.Image(bitmap, rect, ui.Contain) })
			} else {
				caption := "正在载入页面"
				if loaded {
					caption = "无法预览此页"
				}
				ui.Column(c).Fill().Center().Children(func() { ui.Text(c, caption).TextColor(c.Theme().TextMuted) })
			}
		})
	}).Label("PDF 页面").AutoFocus().Padding(filePreviewPadding, 0, 0)
	thinScroll(c, list, nil).Grow(1).MinHeight(0).FillWidth()
}
