package main

import (
	"slices"

	"github.com/egoist/mygo/ui"
)

const dockPinterest = "Pinterest"
const dockEagle = "Eagle"
const dockHistory = "图片历史"
const dockOperations = "操作历史"
const splitGrip float32 = 8

type dockLayout struct {
	divider  ui.Handle
	pages    []string
	active   string
	fraction float32
	width    float32
	dragging bool
	grab     float32
}

func (a *desktop) dockMenu(m *ui.Menu) {
	for _, page := range []string{dockPinterest, dockEagle, dockHistory, dockOperations} {
		if m.Item(page).Chosen() {
			a.openDock(page)
		}
	}
}
func (a *desktop) openDock(page string) {
	if a.closing {
		return
	}
	a.openWorkspace("画布")
	if a.materials != nil && page != a.dock.active {
		a.materials.status = ""
	}
	if !slices.Contains(a.dock.pages, page) {
		a.dock.pages = append(a.dock.pages, page)
	}
	a.dock.active = page
	if page == dockPinterest || page == dockEagle {
		a.prepareMaterialBrowser()
		m := a.materials
		if page == dockEagle && !m.connected && !m.loading {
			a.loadEagle(false)
		}
	}
}
func (a *desktop) closeDockPage() {
	d := &a.dock
	i := slices.Index(d.pages, d.active)
	if i < 0 {
		return
	}
	d.pages = slices.Delete(d.pages, i, i+1)
	d.active = ""
	if len(d.pages) > 0 {
		a.openDock(d.pages[min(i, len(d.pages)-1)])
	}
}

func titleBarRow(c *ui.Context, key string) ui.Element {
	bar := c.TitleBar()
	return ui.Row(c.Key(key)).FillWidth().Height(max(28, bar.Height)).Shrink(0).Padding(0, bar.Right+4, 0, bar.Left)
}

// Only blank title-bar space drags the window. The native insets track
// traffic lights and Windows caption buttons at the current display scale.
func (a *desktop) workspaceTitleBar(c *ui.Context) {
	row := titleBarRow(c, "workspace-titlebar").Gap(2).AlignItems(ui.Center)
	_, _, over := row.PointerPosition()
	row.Children(func() {
		ui.Box(c.Key("window-drag-area")).Grow(1).FillHeight().DragWindow().Label("拖动窗口").Cursor(ui.CursorGrab)
		for _, page := range a.dock.pages {
			selected := page == a.dock.active && a.left == "画布" && !a.settingsOpen
			bg := ui.Transparent
			if selected {
				bg = c.Theme().Background
			}
			ui.Row(c.Key(page)).AlignItems(ui.Center).Height(24).Radius(5).Background(bg).Children(func() {
				b := ui.ButtonBase(c.Key("tab")).Role(ui.RoleTab).Label("侧栏："+page).Selected(selected).Height(24).Padding(0, 8).Center().Radius(5)
				commandHint(c, feedback(c, b, selectionColor(c, b, bg), menuText(c, selected)), "查看"+page).Children(func() { ui.Text(c, page).FontSize(11).SingleLine() }).OnClick(func() { a.openDock(page) })
				if selected {
					iconButton(c, "close", "关闭当前侧栏页面", a.closeDockPage).Size(22, 22).Padding(4)
				}
			})
		}
		if !slices.Contains(a.dock.pages, dockOperations) {
			iconButton(c, "history", "打开操作历史", func() { a.openDock(dockOperations) }, "操作历史").Size(28, 26).Padding(4)
		}
		b := ui.ButtonBase(c.Key("add-dock")).Label("添加侧栏页面").Size(28, 26).Center().Radius(5)
		commandHint(c, feedback(c, b, ui.Transparent, c.Theme().TextMuted), "添加侧栏页面").Children(func() {
			if over || b.Hovered() || b.FocusVisible() {
				uiIcon(c, "plus", 16)
			}
		}).Menu(a.dockMenu)
	})
}

func (a *desktop) rightDock(c *ui.Context) {
	w, _ := c.Size()
	space := w - a.sidebarWidth() - canvasInset - splitGrip
	d := &a.dock
	ratio := d.fraction
	if !(ratio > 0 && ratio < 1) {
		ratio = .4
	}
	clamp := func(width float32) float32 { return max(248, min(space-250, width)) }
	width := clamp(space * ratio)
	d.width = width
	open := len(d.pages) > 0
	target := float32(0)
	if open {
		target = 1
	}
	host := ui.Row(c.Key("right-dock-host")).FillHeight().Shrink(0).Clip()
	reveal := host.AnimateWith("width", target, foldDuration, ui.EaseOut)
	host.Width((width + splitGrip) * reveal).Children(func() {
		if open {
			a.dockContent(c, w, space, width, clamp)
		}
	})
}

func (a *desktop) dockContent(c *ui.Context, w, space, width float32, clamp func(float32) float32) {
	d := &a.dock
	grip := ui.Box(c.Key("dock-divider")).Width(splitGrip).FillHeight().Shrink(0).Focusable().Role(ui.RoleSplitter).Bind(&d.divider).Label("调整右侧栏宽度")
	save := func() {
		if a.store != nil {
			if _, err := a.store.DB.Exec("INSERT OR REPLACE INTO native_meta VALUES('dock_fraction',?)", d.fraction); err != nil {
				a.message(err.Error())
			}
		}
	}
	dragHint(c, grip, d.dragging, "拖动调整两栏宽度").HandleInput(func(ev ui.InputEvent) bool {
		switch ev.Kind {
		case ui.InputPointerDown:
			if ev.Button != 0 {
				return false
			}
			d.dragging = true
			d.grab = ev.X
			return true
		case ui.InputPointerMove:
			if !d.dragging {
				return false
			}
			x := d.divider.Bounds(c).X + ev.X - d.grab
			d.fraction = clamp(w-canvasInset-splitGrip-x) / space
			return true
		case ui.InputPointerUp:
			if !d.dragging {
				return false
			}
			d.dragging = false
			save()
			return true
		case ui.InputKeyDown:
			if ev.Key != ui.KeyLeft && ev.Key != ui.KeyRight {
				return false
			}
			delta := float32(16)
			if ev.Key == ui.KeyRight {
				delta = -delta
			}
			d.fraction = clamp(width+delta) / space
			save()
			return true
		}
		return false
	})
	if grip.Hovered() || grip.FocusVisible() || d.dragging {
		grip.Draw(func(p *ui.Painter, r ui.Rect) {
			p.Fill(ui.Rect{X: r.X + r.W/2 - 1, Y: r.Y + r.H/2 - 20, W: 2, H: 40}, p.Theme().TextMuted, 1)
		})
	}
	panel := ui.Column(c.Key("right-dock")).Width(width).Shrink(0).FillHeight().Clip().Radius(10).Background(c.Theme().Background).Label("画布右侧栏")
	paneMotion(c, panel, 9, 0).Children(func() {
		body := ui.Box(c.Key(d.active)).Fill().Clip()
		if d.active != dockPinterest {
			paneMotion(c, body, 0, 5)
		}
		body.Children(func() {
			switch d.active {
			case dockPinterest, dockEagle:
				a.materialBrowser(c)
			case dockHistory:
				a.galleryView(c)
			case dockOperations:
				a.operationHistoryView(c)
			}
		})
	})
}
