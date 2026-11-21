package main

import (
	"runtime"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

func (a *desktop) chooseSession(e *Editor) {
	if e.Project.DeletedAt != "" {
		return
	}
	a.editor = e
	a.openWorkspace("画布")
	a.draftKey, a.focusNodeID = "", ""
	a.fitNext = false
	a.rememberCanvasOpen(e)
}

func (a *desktop) sessionRow(c *ui.Context, e *Editor) {
	name := e.Project.Name
	selected := e == a.editor && a.left == "画布" && !a.settingsOpen
	row := ui.Row(c.Key(e.Project.ID)).FillWidth().Height(30).Radius(5).AlignItems(ui.Center)
	active := row.Hovered() || row.FocusWithin()
	bg := ui.Transparent
	if active {
		bg = c.Theme().SurfaceHover
	} else if selected {
		bg = c.Theme().Background
	}
	row.Background(bg).Children(func() {
		button := ui.ButtonBase(c.Key("open")).Label("打开画布："+name).Selected(selected).Grow(1).MinWidth(0).FillHeight().Justify(ui.Start).Padding(0, 8).Border(0, ui.Transparent).Background(ui.Transparent).TextColor(menuText(c, selected)).Cursor(ui.CursorPointer)
		button.Children(func() { ui.Text(c, name).Grow(1).SingleLine().MinWidth(0).FontSize(11.5).FontWeight(400) }).OnClick(func() { a.chooseSession(e) }).ContextMenu(func(m *ui.Menu) { a.sessionMenu(m, e) })
		tail := ui.Box(c).Size(30, 30).Center()
		tail.Children(func() {
			more := ui.ButtonBase(c.Key("more")).Label("更多："+name).Absolute().Fill().Center().Padding(0).Radius(5).Border(0, ui.Transparent)
			menuOpen := false
			more.PopoverMenuButton(func(m *ui.Menu) { menuOpen = true; a.sessionMenu(m, e) })
			if !menuOpen {
				commandHint(c, more, "更多操作")
			}
			feedback(c, more, ui.Transparent, c.Theme().TextMuted).Children(func() {
				if active || menuOpen || more.Hovered() || more.FocusVisible() {
					uiIcon(c, "more", 16)
				} else {
					ui.Textf(c, "%d", e.Project.imageNodeCount()).FontFeatures("tnum").FontSize(10).TextColor(menuText(c, false))
				}
			})

		})
	})
}

func (a *desktop) sessionMenu(m *ui.Menu, e *Editor) {
	pin := "置顶"
	if e.Project.Pinned {
		pin = "取消置顶"
	}
	if m.Item(pin).Chosen() {
		e.Project.Pinned = !e.Project.Pinned
		a.canvasIndex.ids = nil
		e.changed()
		a.libraryList.ScrollTo(0, ui.Start)
	}
	if m.Item("重命名…").Chosen() {
		a.renameProjectID = e.Project.ID
		a.renameDraft = e.Project.Name
		a.renameOpen = true
	}
	reveal := "在文件管理器中显示"
	if runtime.GOOS == "darwin" {
		reveal = "在 Finder 中显示"
	} else if runtime.GOOS == "windows" {
		reveal = "在文件资源管理器中显示"
	}
	if m.Item(reveal).Chosen() {
		snapshot, err := snapshotSession(e)
		if err != nil {
			a.message(err.Error())
		} else {
			go func() {
				path, err := a.store.sessionFile(snapshot)
				a.publish(func() {
					if err != nil {
						a.message(err.Error())
					} else {
						mygo.Shell.ShowItemInFolder(path)
					}
				})
			}()
		}
	}
	m.Separator()
	if m.Item("删除画布…").Chosen() {
		a.deleteSessionID = e.Project.ID
		a.deleteSessionOpen = true
	}
}

func (a *desktop) archiveSession(e *Editor, archive bool) {
	if archive == (e.Project.DeletedAt != "") {
		return
	}
	if archive {
		e.Project.DeletedAt = timestamp()
	} else {
		e.Project.DeletedAt = ""
	}
	e.changed()
	a.canvasIndex.ids = nil
	a.libraryKey = ""
	if archive && e == a.editor {
		var next *Editor
		for _, id := range a.order {
			if candidate := a.editors[id]; candidate.Project.DeletedAt == "" {
				next = candidate
				break
			}
		}
		if next == nil {
			a.createCanvas()
		} else {
			a.chooseSession(next)
		}
	}
	if archive {
		a.message("画布已移入回收站")
	} else {
		a.message("已恢复「" + e.Project.Name + "」")
	}
}

func (a *desktop) deleteSessionDialog(c *ui.Context) {
	e := a.editors[a.deleteSessionID]
	if e == nil || !a.deleteSessionOpen {
		return
	}
	ui.DialogBase(c, &a.deleteSessionOpen, func(backdrop, panel ui.Element) {
		backdrop.Background(ui.RGBA(0, 0, 0, .30))
		toolSurface(c, panel, 8).Width(320).Padding(16).Gap(12)
		ui.Text(c, "删除画布「"+e.Project.Name+"」？").MaxLines(3).FontSize(16).FontWeight(500)
		count := len(e.Project.Graph.Nodes)
		ui.Text(c, "画布及其中的 "+nodeQuantity(count, count-e.Project.imageNodeCount())+"将移入回收站，可一起恢复。").FontSize(12).TextColor(c.Theme().TextMuted)
		ui.Row(c).Justify(ui.End).Gap(8).Children(func() {
			iconButton(c, "close", "取消删除画布", func() { a.deleteSessionOpen = false })
			iconButton(c, "trash", "确认删除画布", func() { a.archiveSession(e, true); a.deleteSessionOpen = false }).TextColor(c.Theme().Danger)
		})
	})
}
