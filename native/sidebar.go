package main

import (
	"encoding/json"

	"github.com/egoist/mygo/ui"
)

const (
	workspaceHeaderHeight         = 40
	defaultSidebarWidth   float32 = 244
	minSidebarWidth       float32 = 200
	maxSidebarWidth       float32 = 384
	collapsedSidebarWidth float32 = 52
)

type sidebarLayout struct {
	Width     float32 `json:"width"`
	Collapsed bool    `json:"collapsed"`
}

func (r sidebarLayout) expandedWidth() float32 {
	if r.Width <= 0 {
		return defaultSidebarWidth
	}
	return min(maxSidebarWidth, max(minSidebarWidth, r.Width))
}
func (r sidebarLayout) shownWidth() float32 {
	if r.Collapsed {
		return collapsedSidebarWidth
	}
	return r.expandedWidth()
}

func (s *Store) loadSidebar() sidebarLayout {
	var raw string
	var r sidebarLayout
	if s.DB.QueryRow("SELECT value FROM native_meta WHERE key='sidebar_layout'").Scan(&raw) == nil {
		if json.Unmarshal([]byte(raw), &r) != nil {
			return sidebarLayout{}
		}
	}
	return r
}

func (a *desktop) saveSidebar() {
	if a.store != nil {
		if _, err := a.store.DB.Exec("INSERT OR REPLACE INTO native_meta VALUES('sidebar_layout',?)", string(encode(a.rail))); err != nil {
			a.message(err.Error())
		}
	}
}

func (a *desktop) toggleSidebar() {
	a.rail.Collapsed = !a.rail.Collapsed
	a.canvasSearch.open, a.canvasSearch.hadFocus, a.canvasSearch.focusNext = false, false, false
	a.saveSidebar()
}

func (a *desktop) sidebarWidth() float32 {
	if a.railVisualWidth > 0 {
		return a.railVisualWidth
	}
	return a.rail.shownWidth()
}

func (a *desktop) controlPanel(c *ui.Context) {
	compact := a.rail.Collapsed
	expanded := float32(1)
	if compact {
		expanded = 0
	}
	h := hints(c)
	h.inIconRail = compact
	defer func() { h.inIconRail = false }()
	rail := ui.Column(c.Key("left-controls")).Shrink(0).FillHeight().Clip()
	var fold float32
	if a.railDragging {
		fold = rail.Jump("fold", expanded)
	} else {
		fold = rail.AnimateWith("fold", expanded, foldDuration, ui.EaseOut)
	}
	a.railVisualWidth = collapsedSidebarWidth + fold*(a.rail.expandedWidth()-collapsedSidebarWidth)
	rail.Width(a.railVisualWidth).Children(func() {
		header := ui.Row(c.Key("rail-title")).FillWidth().Height(workspaceHeaderHeight).AlignItems(ui.Center)
		if compact {
			header.Justify(ui.Center).Children(func() { iconButton(c, "sidebar-open", "展开侧栏", a.toggleSidebar) })
		} else {
			header.Padding(0, 8, 0, 16).Children(func() {
				ui.Text(c, "Canvepp").Grow(1).MinWidth(0).SingleLine().FontSize(12).FontWeight(400).TextColor(c.Theme().TextMuted)
				iconButton(c, "sidebar-close", "折叠侧栏", a.toggleSidebar)
			})
		}
		ui.Column(c.Key("fixed-menu")).FillWidth().Padding(2, 9, 6, 9).Gap(1).Children(func() {
			active := !a.settingsOpen
			sidebarNavRow(c, "canvases", allCanvases, allCanvases, "查看全部画布", active && a.left == allCanvases, compact, func() { a.openWorkspace(allCanvases) })
			sidebarNavRow(c, "library", allPictures, allPictures, "查看全部图片", active && a.left == allPictures, compact, func() { a.openWorkspace(allPictures) })
		})
		ui.Divider(c).Margin(0, 12).Background(c.Theme().Border.Mix(c.Theme().Surface, .48))
		a.canvasDirectory(c)
		settings := func() {
			choiceIcon(c, "settings", "打开设置", a.settingsOpen, a.toggleSettings)
		}
		trash := func() {
			choiceIcon(c, "trash", "回收站", a.left == "回收站" && !a.settingsOpen, func() { a.openWorkspace("回收站") })
		}
		if compact {
			ui.Column(c.Key("sidebar-footer")).FillWidth().Padding(0, 0, windowFooterInset+2, 0).Gap(4).AlignItems(ui.Center).Children(func() { trash(); settings() })
		} else {
			ui.Row(c.Key("sidebar-footer")).Height(34).Margin(0, 8, windowFooterInset, 8).AlignItems(ui.Center).Children(func() { settings(); ui.Spacer(c); trash() })
		}
		a.sidebarEdge(c)
	})
}

func sidebarNavRow(c *ui.Context, icon, title, label, tip string, selected, compact bool, action func()) ui.Element {
	t := c.Theme()
	row := ui.ButtonBase(c.Key(label)).Label(label).Selected(selected).FillWidth().Height(30).Gap(8).Radius(5)
	if compact {
		row.Justify(ui.Center)
		tip = title
	} else {
		row.Padding(0, 8).Justify(ui.Start)
	}
	bg := ui.Transparent
	if selected {
		bg = t.Surface.Mix(t.SurfacePressed, .52)
	}
	return commandHint(c, feedback(c, row, selectionColor(c, row, bg), menuText(c, selected)), tip).Children(func() {
		uiIcon(c, icon, 16)
		if !compact {
			ui.Text(c, title).SingleLine().FontSize(12).FontWeight(400)
		}
	}).OnClick(action)
}

func (a *desktop) canvasDirectory(c *ui.Context) {
	compact := a.rail.Collapsed
	tools := func() {
		if compact || a.rail.expandedWidth() < 232 {
			a.canvasSearchPopover(c)
		} else {
			width := min(float32(148), a.rail.expandedWidth()-122)
			if quietSearch(c, &a.canvasQuery, &a.canvasSearch, "搜索画布", "搜索画布", width) {
				a.libraryList.ScrollTo(0, ui.Start)
			}
		}
		if !compact && sortMenu(c, "画布排序", &a.canvasSort, []string{"创建顺序", "最近创建", "名称 A–Z"}) {
			a.libraryList.ScrollTo(0, ui.Start)
		}
		iconButton(c, "plus", "新建画布", a.createCanvas)
	}
	if compact {
		ui.Column(c.Key("canvas-header")).FillWidth().Padding(4, 0).AlignItems(ui.Center).Children(tools)
		ui.Spacer(c)
		return
	} else {
		ui.Row(c.Key("canvas-header")).FillWidth().Height(36).Padding(0, 8, 0, 16).Gap(1).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "画布").SingleLine().FontSize(11).FontWeight(400).TextColor(menuText(c, false))
			ui.Spacer(c)
			tools()
		})
	}
	ids := a.filteredCanvases()
	result := ui.Column(c.Key("sidebar-results")).Grow(1).FillWidth().MinHeight(0)
	resultsMotion(c, result, resultsMotionKey{query: a.canvasQuery, order: a.canvasSort, count: len(ids)}).Children(func() {
		if len(ids) == 0 {
			ui.Text(c, "没有匹配的画布").Padding(14).FontSize(12).TextColor(c.Theme().TextMuted)
			return
		}
		a.libraryList.Key = func(i int) any { return ids[i] }
		a.libraryList.Label = func(i int) string { return a.editors[ids[i]].Project.Name }
		thinScroll(c, ui.List(c.Key("canvas-directory"), &a.libraryList, len(ids), func(i int) {
			a.sessionRow(c, a.editors[ids[i]])
		}), nil).Grow(1).Margin(0, 7, 0, 0).Padding(0, 2, 0, 9).Gap(1)
	})
}

func (a *desktop) canvasSearchPopover(c *ui.Context) {
	s := &a.canvasSearch
	openResult := func(e *Editor) { a.chooseSession(e); s.open = false; a.canvasQuery = "" }
	resetResults := func() { a.searchResults.ScrollTo(0, ui.Start); a.libraryList.ScrollTo(0, ui.Start) }
	anchor := choiceIcon(c, "search", "搜索画布", a.canvasQuery != "" || s.open, func() { s.open = !s.open })
	ui.PopoverBase(c, anchor, &s.open, func(panel ui.Element) {
		a.chromeSurface(c, panel, 6).Width(196).Padding(4)
		paneMotion(c, panel, 0, 5)
		if a.rail.Collapsed {
			panel.AttachTo(anchor, ui.AnchorRight, ui.AnchorLeft).MarginStart(7)
		}
		ui.Row(c).FillWidth().AlignItems(ui.Center).Children(func() {
			f := editHint(c, ui.TextInput(c.Key("rail-query"), &a.canvasQuery).Label("画布搜索词"), "按名称搜索画布").Bind(&s.input).AutoFocus().Placeholder("搜索画布").Grow(1).MinWidth(0).Height(30).Border(0, ui.Transparent).Background(c.Theme().Background)
			if a.rail.Collapsed {
				f.OnSubmit(func() {
					if ids := a.filteredCanvases(); len(ids) > 0 {
						openResult(a.editors[ids[0]])
					}
				})
			}
			if f.Changed() {
				resetResults()
			}
			if a.canvasQuery != "" {
				iconButton(c, "close", "清除搜索画布", func() { a.canvasQuery = ""; resetResults(); s.input.Focus() })
			}
		})
		if a.rail.Collapsed {
			ids := a.filteredCanvases()
			results := ui.Column(c.Key("rail-search-content")).FillWidth()
			resultsMotion(c, results, resultsMotionKey{query: a.canvasQuery, count: len(ids)}).Children(func() {
				if len(ids) == 0 {
					ui.Text(c, "没有匹配的画布").Padding(6).TextColor(c.Theme().TextMuted)
					return
				}
				a.searchResults.Key = func(i int) any { return ids[i] }
				thinScroll(c, ui.List(c.Key("rail-search-results"), &a.searchResults, len(ids), func(i int) {
					e := a.editors[ids[i]]
					b := ui.ButtonBase(c.Key(e.Project.ID)).Label("搜索结果："+e.Project.Name).Height(30).FillWidth().Justify(ui.Start).Padding(0, 8).Radius(5).Cursor(ui.CursorPointer)
					feedback(c, b, ui.Transparent, menuText(c, false)).Children(func() { ui.Text(c, e.Project.Name).SingleLine().FontSize(12) }).OnClick(func() { openResult(e) })
				}), nil).Height(float32(min(7, len(ids)) * 30)).FillWidth()
			})
		}

	})
}

func (a *desktop) sidebarEdge(c *ui.Context) {
	edge := ui.Box(c.Key("sidebar-edge")).Absolute().Top(workspaceHeaderHeight).Bottom(0).Right(0).Width(7).Focusable().Role(ui.RoleSplitter).Label("调整侧栏宽度").Bind(&a.railHandle)
	dragHint(c, edge, a.railDragging, "调整侧栏宽度").HandleInput(func(ev ui.InputEvent) bool {
		x := a.railHandle.Bounds(c).X + ev.X
		switch ev.Kind {
		case ui.InputPointerDown:
			if ev.Button != 0 {
				return false
			}
			a.railDragging = true
			a.railGrabOffset = x - a.sidebarWidth()
			return true
		case ui.InputPointerMove:
			if !a.railDragging {
				return false
			}
			width := x - a.railGrabOffset
			if a.rail.Collapsed {
				if width < minSidebarWidth/2 {
					return true
				}
				a.rail.Collapsed = false
				a.canvasSearch.open = false
			}
			a.rail.Width = min(maxSidebarWidth, max(minSidebarWidth, width))
			return true
		case ui.InputPointerUp:
			if !a.railDragging {
				return false
			}
			a.railDragging = false
			a.saveSidebar()
			return true
		case ui.InputKeyDown:
			if ev.Key != ui.KeyLeft && ev.Key != ui.KeyRight {
				return false
			}
			if a.rail.Collapsed {
				if ev.Key == ui.KeyRight {
					a.toggleSidebar()
				}
				return true
			}
			step := float32(8)
			if ev.Key == ui.KeyLeft {
				step = -step
			}
			a.rail.Width = min(maxSidebarWidth, max(minSidebarWidth, a.rail.expandedWidth()+step))
			a.saveSidebar()
			return true
		}
		return false
	})
	if edge.Hovered() && !a.railDragging {
		hintAtPointer(c, edge, "拖动调整侧栏宽度")
	}
	if edge.Hovered() || edge.FocusVisible() || a.railDragging {
		edge.Draw(func(p *ui.Painter, r ui.Rect) {
			p.Fill(ui.Rect{X: r.X + r.W - 2, Y: r.Y, W: 2, H: r.H}, p.Theme().TextMuted.Alpha(.42), 1)
		})
	}
}
