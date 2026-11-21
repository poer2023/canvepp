package main

import (
	"strings"

	"github.com/egoist/mygo/ui"
)

type galleryViewport struct {
	grid       ui.GridState
	scroll     ui.ScrollState
	locateItem int // one-based pending item; wait for grid columns to be laid out
}

func (a *desktop) galleryName() string {
	if a.left == "回收站" {
		return a.left
	}
	return "历史"
}

func (a *desktop) galleryViewport() *galleryViewport {
	if a.left == "回收站" {
		return &a.trashView
	}
	return &a.historyView
}
func (a *desktop) resetGalleryScroll() {
	v := a.galleryViewport()
	v.scroll.Y = 0
	v.grid.ScrollTo(0, ui.Start)
}

func (a *desktop) galleryFilters(c *ui.Context) {
	all := "全部图片"
	if a.left == "回收站" {
		all = "全部删除"
	}
	ui.Row(c).FillWidth().Height(36).Padding(0, 20).Gap(3).AlignItems(ui.Center).Children(func() {
		if a.left != "回收站" {
			searchWidth := float32(164)
			if a.left == "画布" {
				searchWidth = min(searchWidth, max(62, a.dock.width-174))
			}
			if quietSearch(c, &a.galleryQuery, &a.gallerySearch, "搜索历史图片", "搜索图片", searchWidth) {
				a.selectLibraryGroup("")
			}
			choiceIcon(c, "calendar", "按时间分组", a.galleryGrouping == 0, func() { a.galleryGrouping = 0; a.selectLibraryGroup("") })
			choiceIcon(c, "canvas", "按画布分组", a.galleryGrouping == 1, func() { a.galleryGrouping = 1; a.selectLibraryGroup("") })
		}
		anchor := iconButton(c, "group", "选择分组", func() { a.groupOpen = !a.groupOpen })
		ui.PopoverBase(c, anchor, &a.groupOpen, func(panel ui.Element) {
			a.chromeSurface(c, panel, 6).Padding(4).Width(210)
			paneMotion(c, panel, 0, 5)
			items := a.libraryItems()
			a.groupButton(c, "", all, len(items))
			if len(a.libraryGroups) > 0 {
				a.groupList.Key = func(i int) any { return a.libraryGroups[i].key }
				thinScroll(c, ui.List(c.Key("groups-"+a.left), &a.groupList, len(a.libraryGroups), func(i int) {
					g := a.libraryGroups[i]
					a.groupButton(c, g.key, g.name, g.count)
				}), nil).Height(float32(min(7, len(a.libraryGroups)) * 30)).FillWidth()
			}
		})
		a.gallerySortControl(c)
	})
}

func (a *desktop) groupButton(c *ui.Context, key, name string, count int) {
	bg := ui.Transparent
	if a.selectedLibraryGroup() == key {
		bg = c.Theme().Surface
	}
	b := ui.Button(c.Key("group-"+key), "").Label("分组："+name).Selected(a.selectedLibraryGroup() == key).FillWidth().Height(30).Padding(0, 8).Gap(8).Border(0, ui.Transparent).Radius(8)
	commandHint(c, feedback(c, b, selectionColor(c, b, bg), menuText(c, a.selectedLibraryGroup() == key)), name).Children(func() {
		ui.Text(c, name).Grow(1).MinWidth(0).SingleLine().FontSize(12)
		ui.Textf(c, "%d", count).FontFeatures("tnum").FontSize(10).TextColor(c.Theme().TextMuted)
	}).OnClick(func() { a.selectLibraryGroup(key); a.groupOpen = false })
}

func (a *desktop) gallerySortControl(c *ui.Context) {
	order := &a.gallerySort
	options := []string{"最近优先", "最早优先", "名称 A–Z"}
	if a.left == "回收站" {
		order = &a.trashSort
		options = []string{"最近删除", "最早删除", "名称 A–Z"}
	}
	if sortMenu(c, a.galleryName()+"排序", order, options) {
		a.resetGalleryScroll()
	}
}

func sortMenu(c *ui.Context, label string, order *int, options []string) bool {
	return pickerMenu(c, label, "sort", "排序", order, options)
}

func pickerMenu(c *ui.Context, label, icon, kind string, order *int, options []string) bool {
	before := *order
	sel := ui.SelectBase(c.Key(label), order)
	feedback(c, commandHint(c, sel.Trigger.Label(label), kind+"："+options[*order]).Size(30, 30).Padding(7).Radius(6).Border(0, ui.Transparent), ui.Transparent, c.Theme().TextMuted).Children(func() {
		uiIcon(c, icon, 15)
	})
	sel.Popup(func(panel ui.Element) {
		toolSurface(c, panel, 6).MinWidth(132).Padding(4).Background(c.Theme().Surface.Mix(c.Theme().SurfacePressed, .36))
		paneMotion(c, panel, 0, 0)
		for i, label := range options {
			item := commandHint(c, sel.Item(i).Label(label).Selected(i == *order), label).Row().AlignItems(ui.Center).Gap(8).Height(28).Padding(4, 10).Radius(4)
			if item.Highlighted() {
				item.Background(c.Theme().SurfaceHover)
			}
			item.Children(func() {
				ui.Text(c, label).Grow(1).SingleLine().FontSize(12).FontWeight(400).TextColor(menuText(c, i == *order))
				if i == *order {
					uiIcon(c, "check", 13)
				}
			})
		}
	})
	return *order != before
}

func (a *desktop) galleryView(c *ui.Context) {
	items := a.visibleLibraryItems()
	title := "全部图片"
	if a.left == "回收站" {
		title = "回收站"
	}
	for _, g := range a.libraryGroups {
		if g.key == a.selectedLibraryGroup() {
			title = g.name
			break
		}
	}
	ui.Column(c.Key("gallery-" + a.left)).Fill().Radius(10).Clip().Background(c.Theme().Background).Label("图片画廊").Children(func() {
		if a.left != "画布" {
			pageHeader(c, title, "返回当前画布", func() { a.openWorkspace("画布") }, func() {
				ui.Textf(c, "%d 项", len(items)).FontFeatures("tnum").FontSize(11).TextColor(c.Theme().TextMuted)
			})
		}

		a.galleryFilters(c)
		order := a.gallerySort
		if a.left == "回收站" {
			order = a.trashSort
		}
		results := ui.Column(c.Key("gallery-results")).Grow(1).FillWidth().MinHeight(0).Clip()
		resultsMotion(c, results, resultsMotionKey{query: a.galleryQuery, group: a.selectedLibraryGroup(), order: order, mode: a.galleryGrouping, count: len(items)}).Children(func() {
			if len(items) == 0 {
				ui.Column(c).Grow(1).FillWidth().Center().Gap(12).Children(func() {
					scene, name := "history", "还没有历史图片"
					if a.galleryQuery != "" {
						scene, name = "search-empty", "没有匹配的图片"
					}
					if a.left == "回收站" {
						scene, name = "trash", "回收站为空"
					}
					emptyIcon(c, scene)
					ui.Text(c, name).FontSize(13).TextColor(c.Theme().TextMuted)
					if a.galleryQuery != "" && a.left != "回收站" {
						iconButton(c, "close", "清除搜索", func() { a.galleryQuery = "" })
					}
				})
				return
			}
			v := a.galleryViewport()
			v.grid.Key = func(i int) any { return items[i].key }
			v.grid.Label = func(i int) string { return items[i].name }
			grid := thinScroll(c, ui.GridView(c.Key("image-grid"), &v.grid, len(items), 240, 276, func(i int) { a.galleryCard(c, items[i]) }), &v.scroll).Grow(1).Padding(8, pageInset, 24, pageInset).Gap(16)
			if v.locateItem > 0 && grid.Bounds().W > 0 {
				v.grid.ScrollTo(v.locateItem-1, ui.Center)
				v.locateItem = 0
				c.After(0)
			}
		})
	})
}

func (a *desktop) galleryCard(c *ui.Context, item libraryEntry) {
	card := ui.Column(c.Key(item.key)).Fill().Gap(4)
	if item.key == a.locatedTrashKey {
		card.Selected(true).Radius(6).Background(c.Theme().SurfacePressed).TextColor(c.Theme().Text).DrawOver(func(p *ui.Painter, r ui.Rect) { p.Stroke(r, p.Theme().TextMuted, 6, 1) })
	}
	card.Children(func() {
		if item.deletedCanvas {
			ui.Box(c).Grow(1).FillWidth().Center().Radius(6).Background(c.Theme().Surface).Children(func() { uiIcon(c, "canvas", 26).TextColor(c.Theme().TextMuted) })
			ui.Row(c).FillWidth().Height(28).AlignItems(ui.Center).Children(func() {
				ui.Text(c, item.name).Grow(1).MinWidth(0).SingleLine().FontSize(12)
				iconButton(c, "undo", "恢复画布："+item.name, func() { a.archiveSession(a.editors[item.projectID], false) }, "恢复整张画布")
			})
			return
		}
		if item.run != nil {
			a.galleryRunCard(c, item)
			return
		}
		details := item.name + " · " + a.editors[item.projectID].Project.Name
		if item.asset != nil && item.asset.Name != "" {
			details = item.asset.Name + " · " + a.editors[item.projectID].Project.Name
		}
		textItem := item.trash != nil && item.trash.Node.Kind == textNodeKind
		kind := "图片"
		if textItem {
			kind = "文字"
		}
		preview := imageCommand(c, ui.Button(c.Key("preview"), "").Label("预览图片："+item.name), "查看大图").Grow(1).FillWidth().Padding(0).Border(0, ui.Transparent).Radius(0).Clip().Background(ui.Transparent).Cursor(ui.CursorPointer)
		if item.asset != nil {
			asset := *item.asset
			_, viewportHeight := c.Size()
			preview.OnClick(func() { a.preview(asset) }).Draw(func(p *ui.Painter, r ui.Rect) {
				fitted := r
				if asset.Width > 0 && asset.Height > 0 {
					scale := min(r.W/float32(asset.Width), r.H/float32(asset.Height))
					fitted.W, fitted.H = float32(asset.Width)*scale, float32(asset.Height)*scale
					fitted.X, fitted.Y = r.X+(r.W-fitted.W)/2, r.Y+(r.H-fitted.H)/2
				}
				p.Clip(fitted, 4, func() { drawCheckers(p, fitted, 10) })
				if a.images != nil {
					// GridView may measure many estimated rows on its first
					// layout. Decode only cells actually reaching the viewport.
					if r.Y+r.H > 94 && r.Y < viewportHeight-6 {
						a.images.Request(asset, 512)
					}
					if b := a.images.Get(asset, 512); b != nil {
						p.Clip(fitted, 4, func() { p.Image(b, fitted, ui.Contain) })
					}
				}
			})
		} else if textItem {
			disable(preview, true).Label("已删除的文字：" + item.name).Children(func() {
				ui.Column(c).Fill().Padding(16).Gap(9).AlignItems(ui.Start).Radius(6).Background(c.Theme().Surface).Children(func() {
					uiIcon(c, "type", 18).TextColor(c.Theme().TextMuted)
					ui.Text(c, item.trash.Node.Config.Text).MaxLines(6).FontSize(13)
				})
			})
		} else {
			disable(preview, true).Children(func() { uiIcon(c, "canvas", 24).TextColor(c.Theme().TextMuted) })
		}
		ui.Row(c).FillWidth().Height(28).Gap(6).AlignItems(ui.Center).Children(func() {
			flatTip(c, ui.Text(c, item.name).Grow(1).MinWidth(0).SingleLine().FontSize(11).TextColor(c.Theme().TextMuted), details)
			if item.trash != nil {
				iconButton(c, "undo", "恢复"+kind+"："+item.name, func() { a.restoreGalleryItem(item) }, "恢复到原画布").Size(28, 28)
				anchor := iconButton(c, "trash", "永久删除"+kind+"："+item.name, func() { a.purgeKey = item.key; a.purgeOpen = true }, "永久删除").Size(28, 28)
				if a.purgeOpen && a.purgeKey == item.key {
					a.purgeAnchorDrawn = true
					a.confirmPurge(c, anchor, item)
				}
			} else {
				iconButton(c, "return", "回到画布："+item.name, func() { a.returnToAsset(item) }, "回到来源画布").Size(28, 28)
			}
		})
	})
}

func (a *desktop) galleryRunCard(c *ui.Context, item libraryEntry) {
	r := *item.run
	ui.Column(c).Grow(1).FillWidth().Center().Gap(12).Padding(12).Background(c.Theme().SurfaceHover).Radius(7).Children(func() {
		icon, detail := "hourglass", r.Prompt
		switch r.Status {
		case "failed":
			icon = "warning"
			if r.Error != "" {
				detail = r.Error
			}
		case "cancelled":
			icon = "close"
		case "unknown":
			icon = "history"
			if r.Error != "" {
				detail = r.Error
			}
		case "succeeded":
			icon = "check"
		}
		uiIcon(c, icon, 24).TextColor(c.Theme().TextMuted)
		ui.Text(c, statusLabel(r.Status)).FontSize(14)
		flatTip(c, ui.Text(c, detail).MaxLines(3).FontSize(11).TextColor(c.Theme().TextMuted), detail)
	})
	ui.Text(c, a.editors[item.projectID].Project.Name).SingleLine().FontSize(11).TextColor(c.Theme().TextMuted)
	ui.Row(c).FillWidth().Gap(4).Children(func() {
		if r.ThreadID != "" && !a.running[r.ID] {
			iconButton(c, "refresh", "查询原任务", func() { a.recoverRun(r) })
		}
		if !r.Cancelled && (r.Status == "queued" || r.Status == "running" || r.Status == "preparing" || r.Status == "submitted") {
			iconButton(c, "close", "取消生成", func() { a.cancelRun(r.ID) })
		}
		ui.Spacer(c)
		iconButton(c, "return", "回到任务画布", func() { a.returnToAsset(item) })
	})
}

func (a *desktop) renameDialog(c *ui.Context) {
	ui.DialogBase(c, &a.renameOpen, func(backdrop, panel ui.Element) {
		backdrop.Background(ui.RGBA(0, 0, 0, .3))
		toolSurface(c, panel, 8).Width(320).Padding(16).Gap(12)
		ui.Text(c, "重命名画布").FontSize(16).FontWeight(500)
		confirm := func() {
			if name := strings.TrimSpace(a.renameDraft); name != "" {
				if e := a.editors[a.renameProjectID]; e != nil && e.Project.Name != name {
					e.Project.Name = name
					a.canvasIndex.ids = nil
					e.changed()
				}
				a.renameOpen = false
			}
		}
		editHint(c, ui.TextInput(c, &a.renameDraft).Label("画布名称"), "编辑画布名称").AutoFocus().FillWidth().OnSubmit(confirm)
		ui.Row(c).Justify(ui.End).Gap(8).Children(func() {
			iconButton(c, "close", "取消", func() { a.renameOpen = false })
			primaryIcon(c, "check", "保存名称", "保存名称 · 回车", strings.TrimSpace(a.renameDraft) == "", confirm)
		})
	})
}
