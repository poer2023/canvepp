package main

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"golang.org/x/image/draw"
)

type materialsPanel struct {
	pageStart          int
	pin                *pinterestState
	status, query      string
	eagle              *eagleClient
	connected, loading bool
	items              []eagleItem
	offset             int
	more               bool
	grid               ui.GridState
	thumbs             map[string]*ui.Bitmap
	thumbErrors        map[string]string
	importing          map[string]pendingMaterial
	pinDownloads       pinImageCache
	generation         uint64
	cancel             context.CancelFunc
	searchAt           time.Time
	search             searchControl
	web                *pinterestWeb
	showNativePins     bool
	pinViewChosen      bool
}

func (m *materialsPanel) stop() {
	if m.web != nil && m.web.browser != nil {
		m.web.browser.close()
	}
	if m.pin != nil {
		m.pin.stop()
	}
	if m.cancel != nil {
		m.cancel()
	}
}
func (a *desktop) prepareMaterialBrowser() {
	if a.materials == nil {
		m := &materialsPanel{thumbs: map[string]*ui.Bitmap{}, thumbErrors: map[string]string{}, importing: map[string]pendingMaterial{}}
		a.materials = m
		if a.store != nil {
			var view string
			if a.store.DB.QueryRow("SELECT value FROM native_meta WHERE key='pinterest_view'").Scan(&view) == nil {
				m.pinViewChosen = view == "web" || view == "collections"
				m.showNativePins = view == "collections"
			}
		}
	}
	if a.dock.active != dockPinterest {
		return
	}
	if a.win != nil && a.store != nil {
		a.initPinterest()
	}
	if !a.materials.showNativePins {
		a.initPinterestWeb()
	}
}

func (a *desktop) setPinterestView(collections bool) {
	m := a.materials
	m.showNativePins, m.pinViewChosen = collections, true
	if a.store != nil {
		view := "web"
		if collections {
			view = "collections"
		}
		if _, err := a.store.DB.Exec("INSERT OR REPLACE INTO native_meta VALUES('pinterest_view',?)", view); err != nil {
			m.status = "未能保存 Pinterest 视图偏好"
		}
	}
}

func (a *desktop) invalidateMaterials() {
	if a.win != nil && a.dockVisible() {
		a.win.Invalidate()
	}
}

func (a *desktop) backInMaterials() {
	m := a.materials
	if m != nil && a.dock.active == dockPinterest && m.showNativePins && m.pin != nil {
		if m.pin.previewOpen {
			m.pin.previewOpen = false
			return
		}
		if m.pin.configure {
			m.pin.configure = false
			m.pin.draft.ClientSecret = ""
			return
		}
		a.setPinterestView(false)
		a.initPinterestWeb()
		return
	}
	a.closeDockPage()
}
func (a *desktop) dockVisible() bool {
	return (a.dock.active == dockPinterest || a.dock.active == dockEagle) && a.left == "画布"
}

func (a *desktop) materialBrowser(c *ui.Context) {
	m := a.materials
	if a.dock.active == dockPinterest {
		if m.showNativePins {
			ui.Row(c).Padding(0, 18).Children(func() {
				iconButton(c, "back", "返回 Pinterest 网页", func() { a.setPinterestView(false); a.initPinterestWeb() })
			})
			a.pinterestView(c)
		} else {
			a.pinterestWebView(c)
		}
	} else {
		a.eagleView(c)
	}
	if m.status != "" {
		flatTip(c, ui.Text(c, m.status).Label("素材状态").Height(30).Padding(6, 20).FontSize(12).TextColor(c.Theme().TextMuted).SingleLine(), m.status)
	}
}
func (a *desktop) eagleView(c *ui.Context) {
	m := a.materials
	ui.Row(c).Height(36).Padding(2, 8).Gap(8).AlignItems(ui.Center).Children(func() {
		searchWidth := float32(320)
		if a.dockVisible() {
			searchWidth = min(154, max(72, a.dock.width-122))
		}
		if quietSearch(c, &m.query, &m.search, "搜索 Eagle 图片", "搜索图片、标签", searchWidth) {
			m.searchAt = time.Now().Add(300 * time.Millisecond)
			if m.cancel != nil {
				m.cancel()
			}
			m.generation++
			m.loading = false
		}
		activityIcon(c, "refresh", "连接或刷新 Eagle", "正在读取 Eagle 素材", m.loading, false, func() { a.loadEagle(false) })
		iconButton(c, "external", "打开 Eagle", func() {
			if err := mygo.Shell.OpenExternal("eagle://"); err != nil {
				m.status = "请先安装并打开 Eagle"
			}
		})
	})
	if !m.searchAt.IsZero() {
		if time.Now().Before(m.searchAt) {
			c.After(time.Until(m.searchAt))
		} else {
			m.searchAt = time.Time{}
			a.loadEagle(false)
		}
	}
	if !m.connected {
		ui.Column(c).Grow(1).FillWidth().Center().Gap(14).Children(func() {
			if m.loading {
				emptyIcon(c, "hourglass")
			} else {
				emptyIcon(c, "unlink")
			}
			text := "Eagle 未连接"
			if m.loading {
				text = "正在连接 Eagle…"
			}
			ui.Text(c, text).FontSize(15).TextColor(c.Theme().TextMuted)
			primaryIcon(c, "link", "连接 Eagle", "连接本机 Eagle 素材库", m.loading, func() { a.loadEagle(false) })
		})
		return
	}
	ui.Text(c, m.eagle.Name).FontSize(12).TextColor(c.Theme().TextMuted).Padding(0, 22, 10, 22)
	key := resultsMotionKey{query: m.query, count: len(m.items), order: m.pageStart}
	if len(m.items) > 0 {
		key.group = m.items[0].ID
	} else if m.loading {
		key.mode = 1
	}
	result := ui.Column(c.Key("eagle-results")).FillWidth().Grow(1).MinHeight(0).Clip()
	resultsMotion(c, result, key).Children(func() {
		if len(m.items) == 0 && m.loading {
			pendingMaterialGrid(c, 210, 226, 16, 20, 8)
		} else if len(m.items) == 0 {
			ui.Column(c).Grow(1).FillWidth().Center().Gap(12).Children(func() {
				scene := "library"
				if m.query != "" {
					scene = "search-empty"
				}
				emptyIcon(c, scene)
				ui.Text(c, "没有找到可导入的图片").FontSize(13).TextColor(c.Theme().TextMuted)
			})
		} else {
			m.grid.Key = func(i int) any { return m.items[i].ID }
			m.grid.Label = func(i int) string { return m.items[i].Name }
			thinScroll(c, ui.GridView(c.Key("eagle-grid"), &m.grid, len(m.items), 210, 226, func(i int) {
				item := m.items[i]
				ui.Column(c).Fill().Gap(4).Children(func() {
					addAt := func(e *Editor, at Point) {
						client := *m.eagle
						a.importMaterialInto(e, at, "eagle", item.ID, item.Name+"."+item.Ext, item.URL, func(ctx context.Context) ([]byte, error) {
							path, err := client.Original(ctx, item)
							if err != nil {
								return nil, err
							}
							return readBounded(path, maxImageBytes)
						})
					}
					add := func() { addAt(a.editor, a.center()) }
					busy := a.materialImporting("eagle", item.ID)
					hint := "导入当前画布"
					if m.thumbErrors[item.ID] != "" {
						hint = "缩略图不可用 · 导入原图"
					}
					b := imageCommand(c, ui.Button(c, "").Label("导入 Eagle 图片："+item.Name), hint).Grow(1).FillWidth().Padding(0).Border(0, ui.Transparent).Background(ui.Transparent)
					disable(b, busy || a.editor == nil).OnClick(add)
					if a.dockVisible() && !busy && a.editor != nil {
						b.Drag(materialDrag{addAt}).Cursor(ui.CursorGrab)
					}
					b.Draw(func(p *ui.Painter, r ui.Rect) {
						if bitmap := m.thumbs[item.ID]; bitmap != nil {
							p.Image(bitmap, r, ui.Contain)
						} else {
							pendingThumbnail(p, r, m.thumbErrors[item.ID] != "")
						}
					})
					ui.Row(c).Height(30).Gap(6).AlignItems(ui.Center).Children(func() {
						fittingLabel(c, item.Name, 12, 400).Grow(1).TextColor(c.Theme().TextMuted)
						activityIcon(c, "plus", "导入 "+item.Name, "正在导入 "+item.Name, busy, a.editor == nil, add, "导入当前画布")
					})
				})
			}), nil).Grow(1).Padding(8, 20).Gap(16)
		}
	})
	if m.pageStart > 0 || m.more {
		ui.Row(c).Height(42).Center().Children(func() {
			if m.pageStart > 0 {
				disable(iconButton(c, "back", "上一页", func() { m.offset = max(0, m.pageStart-48); a.loadEagle(true) }), m.loading)
			}
			disable(iconButton(c, "forward", "下一页", func() { a.loadEagle(true) }), m.loading || !m.more)
		})
	}
}
func (a *desktop) loadEagle(more bool) {
	m := a.materials
	if m == nil || m.loading {
		return
	}
	if m.cancel != nil {
		m.cancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	m.generation++
	generation := m.generation
	m.loading = true
	m.status = ""
	query := m.query
	offset := 0
	client := newEagleClient()
	if more && m.eagle != nil {
		*client = *m.eagle
		offset = m.offset
	} else {
		m.items = nil
		m.pageStart, m.more = 0, false
		m.thumbs = map[string]*ui.Bitmap{}
		m.thumbErrors = map[string]string{}
		m.grid.ScrollTo(0, ui.Start)
	}
	a.invalidateMaterials()
	go func() {
		err := client.Connect(ctx)
		var page eaglePage
		if err == nil {
			page, err = client.List(ctx, query, offset)
		}
		a.publish(func() {
			if m.generation != generation {
				return
			}
			m.loading = false
			if err != nil {
				m.status = err.Error()
				if !more {
					m.connected = false
				}
			} else {
				m.connected = true
				m.eagle = client
				m.items = page.Items
				m.pageStart = offset
				m.thumbs = map[string]*ui.Bitmap{}
				m.thumbErrors = map[string]string{}
				m.grid.ScrollTo(0, ui.Start)
				m.more = page.More
				m.offset = page.Offset
			}
			a.invalidateMaterials()
		})
		if err != nil {
			return
		}
		// At most two decoders; only this page is prefetched. Cancel on a new query.
		work := make(chan eagleItem)
		for worker := 0; worker < 2; worker++ {
			go func() {
				for item := range work {
					if ctx.Err() != nil {
						return
					}
					path, err := client.Original(ctx, item)
					var thumb image.Image
					if err == nil {
						var data []byte
						data, err = readBounded(path, maxImageBytes)
						if err == nil {
							thumb, err = materialThumbnail(data)
						}
					}
					a.publish(func() {
						if m.generation != generation {
							return
						}
						if err == nil {
							m.thumbs[item.ID] = ui.NewBitmap(thumb)
						} else {
							m.thumbErrors[item.ID] = err.Error()
						}
						a.invalidateMaterials()
					})
				}
			}()
		}
		for _, item := range page.Items {
			select {
			case work <- item:
			case <-ctx.Done():
				close(work)
				return
			}
		}
		close(work)
	}()
}
func materialThumbnail(b []byte) (image.Image, error) {
	return materialThumbnailSize(b, 256)
}
func materialThumbnailSize(b []byte, size float64) (image.Image, error) {
	cfg, _, err := image.DecodeConfig(bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || int64(cfg.Width)*int64(cfg.Height) > 40_000_000 {
		return nil, fmt.Errorf("图片尺寸过大")
	}
	src, _, err := image.Decode(bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	scale := min(size/float64(cfg.Width), size/float64(cfg.Height), 1)
	dst := image.NewRGBA(image.Rect(0, 0, max(1, int(float64(cfg.Width)*scale)), max(1, int(float64(cfg.Height)*scale))))
	draw.ApproxBiLinear.Scale(dst, dst.Bounds(), src, src.Bounds(), draw.Src, nil)
	return dst, nil
}
