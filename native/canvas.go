package main

import (
	"image"
	"image/color"
	"math"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/shader"
)

const (
	canvasCorner      = 10
	canvasInset       = 6
	canvasToolsInset  = 14
	windowFooterInset = canvasInset + canvasToolsInset
)

func (a *desktop) canvasView(c *ui.Context) {
	ui.Box(c.Key("canvas-stage")).Grow(1).FillHeight().MinWidth(160).Radius(canvasCorner).Clip().Background(c.Theme().Background).Children(func() {
		box := ui.Box(c.Key("image-canvas")).Absolute().Fill().Radius(canvasCorner).Clip().Focusable().Bind(&a.canvas).Label("图片画布").HandleInput(a.canvasInput)
		r := box.Bounds()
		a.canvasW, a.canvasH = float64(r.W), float64(r.H)
		if r.W <= 0 || r.H <= 0 {
			// A newly mounted canvas has no committed bounds yet. Do not clamp
			// a remembered free panel position against a zero-sized surface.
			w, h := c.Size()
			a.canvasW, a.canvasH = float64(max(160, w-a.sidebarWidth()-6)), float64(max(160, h-12))
		}
		if a.editor == nil {
			return
		}
		e := a.editor
		if a.fitNext && r.W > 0 {
			e.Fit(float64(r.W), float64(r.H))
			a.fitNext = false
		}
		a.focusCanvasNode()
		view := Rect{-e.View.X / e.View.Zoom, -e.View.Y / e.View.Zoom, a.canvasW / e.View.Zoom, a.canvasH / e.View.Zoom}
		if a.images != nil && !a.panelDragging {
			for _, n := range e.Project.Graph.Nodes {
				if view.Intersects(nodeRect(n, e.Assets)) {
					if asset, ok := e.Assets[n.Asset]; ok {
						size := 512
						if e.View.Zoom > 2 {
							size = 1024
						}
						a.images.Request(asset, size)
					}
				}
			}
		}
		if a.dockVisible() {
			a.canvasMaterialDrop(c, box, e)
		}
		if files := box.DroppedFiles(); len(files) > 0 {
			a.importFiles(files, e.World(a.pointer))
		}
		if box.FileDragOver() {
			box.Border(2, c.Theme().Text)
		}
		switch {
		case a.gesture == "pan" || a.gesture == "drag":
			box.Cursor(ui.CursorGrabbing)
		case a.space:
			box.Cursor(ui.CursorGrab)
		case a.textPlacing:
			box.Cursor(ui.CursorText)
		case a.hover != "" && a.gesture == "":
			if n := e.Node(a.hover); n != nil {
				if e.positionLocked(n) {
					box.Cursor(ui.CursorPointer)
					hintAtPointer(c, box, n.Name+" · 位置已锁定")
				} else {
					box.Cursor(ui.CursorGrab)
					hintAtPointer(c, box, n.Name+" · 拖动移动")
				}
			}
		default:
			box.Cursor(ui.CursorDefault)
		}
		if !a.canvas.Focused(c) {
			a.space = false
		}
		box.ContextMenu(func(m *ui.Menu) {
			if m.Item("新建图片").Shortcut(ui.Cmd, ui.KeyN).Chosen() {
				a.addNode(e.World(a.pointer))
			}
			if m.Item("插入文字").Chosen() {
				a.beginCanvasText(e.World(a.pointer), nil)
			}
			if m.Item("导入图片…").Chosen() {
				a.chooseImages()
			}
			for _, source := range []string{dockPinterest, dockEagle} {
				if m.Item("从 " + source + " 导入…").Chosen() {
					a.openDock(source)
				}
			}
			if id := e.Hit(e.World(a.pointer)); id != "" {
				if n := e.Node(id); n.Kind == textNodeKind && m.Item("编辑文字").Chosen() {
					a.beginCanvasText(n.Position, n)
				}
				if m.Item(a.nodeLockLabel(e.Node(id))).Disabled(e.Project.LayoutLocked).Chosen() {
					e.toggleNodeLock(id)
				}
				var links []Edge
				for _, link := range e.Project.Graph.Edges {
					if link.Source == id || link.Target == id {
						links = append(links, link)
					}
				}
				if len(links) > 0 {
					m.Submenu("断开连线", func(sub *ui.Menu) {
						for _, link := range links {
							source, target := e.Node(link.Source), e.Node(link.Target)
							if source != nil && target != nil && sub.Item(source.Name+" → "+target.Name).Chosen() {
								e.Do(command{Label: "断开连线", RemovedEdges: []Edge{link}})
							}
						}
					})
				}
			}
			m.Separator()
			if m.Item("复制").Disabled(len(e.Selection) == 0).Shortcut(ui.Cmd, ui.KeyC).Chosen() {
				a.copySelection()
			}
			if m.Item("粘贴").Shortcut(ui.Cmd, ui.KeyV).Chosen() {
				a.paste()
			}
			if m.Item("删除").Disabled(len(e.Selection) == 0).Chosen() {
				a.moveSelectionToTrash()
			}
		})
		motion := a.generationMotion(c)
		a.syncImageReveals(c, view, motion)
		box.Draw(func(p *ui.Painter, r ui.Rect) { a.paintCanvas(p, r, motion) })
		a.canvasGenerationAccessibility(c)
		if len(e.Project.Graph.Nodes) == 0 && !a.panel.Open && a.gesture == "" && !a.textPlacing && a.textEdit == nil {
			a.emptyCanvasActions(c, box)
		}
		canvasHovered := box.Hovered()
		if n := e.Node(a.hover); n != nil && n.Kind != textNodeKind && a.gesture == "" && !a.panelDragging && a.nodeToolbar(*n).W > 0 {
			r := a.nodeToolbar(*n)
			bar := a.chromeSurface(c, ui.Row(c.Key("hover-actions")), 8).Absolute().Left(r.X).Top(r.Y).Size(r.W, r.H).Padding(5).Gap(4)
			toolsHovered := bar.Hovered()
			if !canvasHovered && !toolsHovered {
				// A sibling inspector/sidebar receives the pointer, so the custom
				// canvas no longer gets Move events. Clear its stale hover here.
				a.hover = ""
				bar.Hidden(true)
			}
			bar.Children(func() {
				asset, ok := e.Assets[n.Asset]
				disable(iconButton(c, "expand", "放大图片", func() { a.preview(asset) }), !ok)
				disable(iconButton(c, "download", "下载图片", func() { a.exportImage(asset) }), !ok)
				disable(layoutLockButton(c, e.positionLocked(n), a.nodeLockLabel(n), func() { e.toggleNodeLock(n.ID) }), e.Project.LayoutLocked)
				iconButton(c, "trash", "删除图片", func() { clear(e.Selection); e.Selection[n.ID] = true; a.draftKey = ""; a.moveSelectionToTrash() })
			})
		}
		undoX, zoomRight := float32(14), float32(14)
		if a.panel.Open {
			pr := a.panelRect()
			if pr.Y+pr.H > float32(a.canvasH)-58 {
				if pr.X < 100 {
					undoX = pr.X + pr.W + 10
				}
				if pr.X+pr.W > float32(a.canvasW)-252 {
					zoomRight = float32(a.canvasW) - pr.X + 10
				}
			}
		}
		undoBottom := float32(canvasToolsInset)
		viewGap, percentW := float32(2), float32(38)
		if a.canvasW < 350 {
			undoBottom += 40
			undoX, zoomRight = 14, 14
			viewGap, percentW = 0, 32
		}
		a.chromeSurface(c, ui.Row(c.Key("undo-redo-tools")).Absolute().Bottom(undoBottom).Left(undoX).Height(34).Padding(2, 4).Gap(2).AlignItems(ui.Center), 10).Children(func() {
			disable(iconButton(c, "undo", "撤销", func() { e.Undo(); a.draftKey = "" }), len(e.undo) == 0)
			disable(iconButton(c, "redo", "重做", func() { e.Redo(); a.draftKey = "" }), len(e.redo) == 0)
		})
		a.chromeSurface(c, ui.Row(c.Key("canvas-view-tools")).Absolute().Bottom(canvasToolsInset).Right(zoomRight).Height(34).Padding(2, 4).Gap(viewGap).AlignItems(ui.Center), 10).Children(func() {
			choiceIcon(c, "type", "文字工具", a.textPlacing || a.textEdit != nil, a.toggleCanvasTextTool).Description("按 T，在画布上点按放置文字")
			iconButton(c.Key("zoom-out"), "minus", "缩小画布", func() { e.ZoomAt(Point{a.canvasW / 2, a.canvasH / 2}, .8) })
			ui.Textf(c, "%.0f%%", e.View.Zoom*100).FontFeatures("tnum").Width(percentW).FontSize(10)
			iconButton(c.Key("zoom-in"), "plus", "放大画布", func() { e.ZoomAt(Point{a.canvasW / 2, a.canvasH / 2}, 1.25) })
			layoutLockButton(c, e.Project.LayoutLocked, lockAction("画布布局", e.Project.LayoutLocked), e.toggleCanvasLock)
			iconButton(c, "chat", "打开助手输入条", a.showAssistant, "打开助手 · ⌘ ⇧ Space")
			iconButton(c.Key("zoom-fit"), "expand", "适应画布", func() { e.Fit(a.canvasW, a.canvasH) })
		})
		if !a.textPlacing && a.textEdit == nil && (len(e.Selection) == 0 || len(a.draftSelection) > 0) {
			a.floatingInspector(c)
		}
		a.canvasTextEditor(c)
		if e.saveError != "" {
			toolSurface(c, ui.Row(c.Key("save-error")).Role(ui.RoleStatus).Label("画布未保存").Absolute().Bottom(56).Left(14).MaxWidth(float32(a.canvasW)-28).Padding(6, 10).Gap(8).AlignItems(ui.Center), 6).Children(func() {
				uiIcon(c, "warning", 16).TextColor(c.Theme().Danger)
				ui.Column(c).MaxWidth(366).Shrink(1).MinWidth(0).Children(func() {
					ui.Text(c, "画布未保存").FontSize(12).TextColor(c.Theme().Danger)
					flatTip(c, ui.Text(c, e.saveError).MaxLines(2).FontSize(11).TextColor(c.Theme().TextMuted), e.saveError)
				})
				activityIcon(c, "refresh", "重试保存", "正在重试保存", e.retryingSave, a.saves == nil, func() { e.retryingSave = true; a.saves.Put(e) })
			})
		}
	})
}
func lockAction(subject string, locked bool) string {
	if locked {
		return "解锁" + subject
	}
	return "锁定" + subject
}
func (a *desktop) nodeLockLabel(n *Node) string {
	if a.editor.Project.LayoutLocked {
		return "画布布局已锁定"
	}
	return lockAction("节点位置", n.PositionLocked)
}
func layoutLockButton(c *ui.Context, locked bool, label string, toggle func()) ui.Element {
	icon := "unlock"
	if locked {
		icon = "lock"
	}
	return choiceIcon(c, icon, label, locked, toggle)
}
func (a *desktop) nodeToolbar(n Node) ui.Rect {
	q, v := nodeRect(n, a.editor.Assets), a.editor.View
	// Four 30 pt buttons, three 4 pt gaps and 5 pt padding on each edge.
	w, h := float32(142), float32(40)
	x := float32((q.X+q.W/2)*v.Zoom+v.X) - w/2
	y := float32(q.Y*v.Zoom+v.Y) - h - 10
	if y < 36 {
		y = float32((q.Y+q.H)*v.Zoom+v.Y) + 10
	}
	if y < 36 || y+h > float32(a.canvasH)-8 || (q.X+q.W)*v.Zoom+v.X < 0 || q.X*v.Zoom+v.X > a.canvasW {
		// Never pin tools over the image when zoomed in. The inspector and
		// context menu remain available when both outside edges are offscreen.
		return ui.Rect{}
	}
	return ui.Rect{X: max(8, min(float32(a.canvasW)-w-8, x)), Y: y, W: w, H: h}
}

func (a *desktop) inNodeToolbar(n Node, p Point) bool {
	r := a.nodeToolbar(n)
	// Bridge the gap so tools stay reachable without crossing a dead zone.
	return r.W > 0 && p.X >= float64(r.X-4) && p.X <= float64(r.X+r.W+4) && p.Y >= float64(r.Y-11) && p.Y <= float64(r.Y+r.H+11)
}

func (a *desktop) canvasInput(ev ui.InputEvent) bool {
	if a.hotkeyEnabled && a.hotkey == nil && a.hotkeyStatus != "全局监听已开启" {
		kind := -1
		switch ev.Kind {
		case ui.InputKeyDown:
			kind = 0
			if ev.Key == ui.KeySpace && ev.Mods == 0 {
				kind = 1
			}
		case ui.InputKeyUp:
			kind = 0
			if ev.Key == ui.KeySpace && ev.Mods == 0 {
				kind = 2
			}
		case ui.InputPointerDown:
			kind = 0
		}
		if kind >= 0 && a.localSpace.feed(kind, time.Duration(time.Now().UnixNano())) {
			a.space = false
			a.showAssistant()
			return true
		}
	}

	if a.editor == nil {
		return false
	}
	e := a.editor
	p := Point{float64(ev.X), float64(ev.Y)}
	if a.motionTrace != nil {
		a.motionTrace.input(ev, a.gesture, "canvas")
	}
	switch ev.Kind {
	case ui.InputKeyDown:
		if ev.Mods == 0 && ev.Key == ui.KeyT && !ev.Repeat {
			a.toggleCanvasTextTool()
			return true
		}
		if ev.Key == ui.KeySpace {
			a.space = true
			return true
		}
		if ev.Mods == ui.Cmd {
			switch ev.Key {
			case ui.KeyA:
				for _, n := range e.Project.Graph.Nodes {
					e.Selection[n.ID] = true
				}
				a.draftKey = ""
				return true
			case ui.KeyC:
				a.copySelection()
				return true
			case ui.KeyV:
				a.paste()
				return true
			case ui.KeyZ:
				e.Undo()
				a.draftKey = ""
				return true
			case ui.KeyN:
				a.addNode(a.center())
				return true
			case ui.KeyO:
				a.chooseImages()
				return true
			case ui.KeyS:
				a.changed(e)
				return true
			}
		}
		if ev.Mods == ui.Cmd|ui.Shift && ev.Key == ui.KeyZ {
			e.Redo()
			a.draftKey = ""
			return true
		}
		if ev.Key == ui.KeyBackspace || ev.Key == ui.KeyDelete {
			if len(e.Selection) > 0 {
				a.moveSelectionToTrash()
			}
			return true
		}
		if ev.Key == ui.KeyEscape {
			if a.textPlacing || a.textEdit != nil {
				a.textPlacing, a.textEdit = false, nil
				return true
			}
			if a.gesture == "drag" {
				e.MoveDrag(0, 0)
				e.drag = nil
			}
			a.gesture = ""
			a.selectionRect = nil
			clear(e.Selection)
			a.draftKey = ""
			return true
		}
	case ui.InputKeyUp:
		if ev.Key == ui.KeySpace {
			a.space = false
			return true
		}
	case ui.InputCommand:
		switch ev.Text {
		case "copy":
			a.copySelection()
		case "paste":
			a.paste()
		case "undo":
			e.Undo()
			a.draftKey = ""
		case "redo":
			e.Redo()
			a.draftKey = ""
		case "delete":
			a.moveSelectionToTrash()
		case "selectAll":
			for _, n := range e.Project.Graph.Nodes {
				e.Selection[n.ID] = true
			}
		default:
			return false
		}
		return true
	case ui.InputZoom:
		e.ZoomAt(p, float64(ev.Scale))
		return true
	case ui.InputScroll:
		notches := canvasWheelNotches(float64(ev.DY), ev.Precise)
		e.ZoomAt(p, math.Pow(1.06, -max(-4, min(4, notches))))
		return true
	case ui.InputPointerDown:
		a.pointer = p
		a.start = p
		a.panStart = e.View
		if a.space || ev.Button == 2 {
			a.gesture = "pan"
			return true
		}
		if ev.Button == 0 {
			if a.textPlacing {
				a.beginCanvasText(e.World(p), nil)
				return true
			}
			a.commitCanvasText()
		}
		id := e.Hit(e.World(p))
		if ev.Button == 1 {
			if id != "" && !e.Selection[id] {
				clear(e.Selection)
				e.Selection[id] = true
				a.draftKey = ""
			}
			return false
		}
		if ev.Button != 0 {
			return false
		}
		if ev.Clicks == 2 && id != "" {
			if n := e.Node(id); n != nil {
				if n.Kind == textNodeKind {
					a.beginCanvasText(n.Position, n)
					return true
				}
				if asset, ok := e.Assets[n.Asset]; ok {
					a.preview(asset)
					return true
				}
			}
		}
		if id != "" {
			if e.Node(id).Kind != textNodeKind {
				a.panel.Open = true
			}
			if ev.Mods&(ui.Shift|ui.Cmd) != 0 {
				if e.Selection[id] {
					delete(e.Selection, id)
				} else {
					e.Selection[id] = true
				}
				a.draftKey = ""
			} else if !e.Selection[id] {
				clear(e.Selection)
				e.Selection[id] = true
				a.draftKey = ""
			}
			if !e.positionLocked(e.Node(id)) && e.Selection[id] {
				e.BeginDrag()
				a.gesture = "drag"
			}
		} else {
			a.gesture = "select"
			a.addSelection = ev.Mods&ui.Shift != 0
			if !a.addSelection {
				clear(e.Selection)
			}
			world := e.World(p)
			a.selectionRect = &Rect{X: world.X, Y: world.Y}
			a.draftKey = ""
		}
		return true
	case ui.InputPointerMove:
		a.pointer = p
		if a.textPlacing && a.gesture == "" {
			return false
		}
		switch a.gesture {
		case "pan":
			e.View.X = a.panStart.X + p.X - a.start.X
			e.View.Y = a.panStart.Y + p.Y - a.start.Y
		case "drag":
			e.MoveDrag((p.X-a.start.X)/e.View.Zoom, (p.Y-a.start.Y)/e.View.Zoom)
		case "select":
			s, t := e.World(a.start), e.World(p)
			a.selectionRect = &Rect{math.Min(s.X, t.X), math.Min(s.Y, t.Y), math.Abs(t.X - s.X), math.Abs(t.Y - s.Y)}
		default:
			hover := e.Hit(e.World(p))
			if hover == "" {
				if n := e.Node(a.hover); n != nil && a.inNodeToolbar(*n, p) {
					hover = a.hover
				}
			}
			if hover == a.hover {
				return false // Merely moving across the same surface needs no canvas frame.
			}
			a.hover = hover
		}
		return true
	case ui.InputPointerUp:
		switch a.gesture {
		case "drag":
			e.EndDrag()
		case "select":
			if a.selectionRect != nil && a.selectionRect.W+a.selectionRect.H > 4 {
				e.SelectRect(*a.selectionRect, a.addSelection)
			}
			a.draftKey = ""
		case "pan":
			a.changed(e)
		}
		a.gesture = ""
		a.selectionRect = nil
		return true
	}
	return false
}
func (a *desktop) paintCanvas(p *ui.Painter, r ui.Rect, motion bool) {
	start := time.Now()
	if a.motionTrace != nil {
		a.motionTrace.paint(start)
	}
	defer func() {
		a.frameTimes[a.frameHead%len(a.frameTimes)] = time.Since(start).Nanoseconds()
		a.frameHead++
		a.paintCount++
	}()
	if a.editor == nil {
		return
	}
	t := p.Theme()
	e := a.editor
	generations := a.canvasGenerations()
	z := float32(e.View.Zoom)
	ox, oy := float32(e.View.X), float32(e.View.Y)
	step := float32(32) * z
	for step < 22 {
		step *= 2
	}
	gx := float32(math.Mod(float64(ox), float64(step)))
	gy := float32(math.Mod(float64(oy), float64(step)))
	for x := r.X + gx; x < r.X+r.W; x += step {
		for y := r.Y + gy; y < r.Y+r.H; y += step {
			p.Fill(ui.Rect{X: x, Y: y, W: 1, H: 1}, t.Border, .5)
		}
	}
	view := Rect{-e.View.X / e.View.Zoom, -e.View.Y / e.View.Zoom, float64(r.W) / e.View.Zoom, float64(r.H) / e.View.Zoom}
	p.Clip(r, canvasCorner, func() {
		var points [257][2]float32
		for _, edge := range e.Project.Graph.Edges {
			source, target := e.Node(edge.Source), e.Node(edge.Target)
			if source == nil || target == nil {
				continue
			}
			sr, tr := nodeRect(*source, e.Assets), nodeRect(*target, e.Assets)
			route := imageConnection(
				ui.Rect{X: r.X + ox + float32(sr.X)*z, Y: r.Y + oy + float32(sr.Y)*z, W: float32(sr.W) * z, H: float32(sr.H) * z},
				ui.Rect{X: r.X + ox + float32(tr.X)*z, Y: r.Y + oy + float32(tr.Y)*z, W: float32(tr.W) * z, H: float32(tr.H) * z})
			if !route.visible(r) {
				continue
			}
			curve := route.points(points[:0])
			edgeColor, edgeWidth := t.TextMuted.Mix(t.Background, .63), float32(1)
			if len(e.Selection) > 0 || a.hover != "" {
				edgeColor = t.Border
			}
			related := e.Selection[source.ID] || e.Selection[target.ID]
			if a.hover != "" {
				related = a.hover == source.ID || a.hover == target.ID
			}
			if related {
				edgeColor, edgeWidth = t.TextMuted, 1.25
			}
			// Opaque strokes stay on the GPU polyline path, even when de-emphasized.
			p.StrokePolyline(curve, edgeWidth, edgeColor)
		}
		// Project axis-aligned cards into screen coordinates. A scaled rounded
		// clip forces MyGo to rasterize and upload a new CPU mask whenever a
		// card moves. Screen-space clips stay in the GPU's rounded-rect shader.
		for _, n := range e.Project.Graph.Nodes {
			q := nodeRect(n, e.Assets)
			if !view.Intersects(q) {
				continue
			}
			rect := ui.Rect{X: r.X + ox + float32(q.X)*z, Y: r.Y + oy + float32(q.Y)*z, W: float32(q.W) * z, H: float32(q.H) * z}
			radius := min(7*z, float32(4))
			if n.Adobe != nil {
				radius = 0
			}
			asset, hasAsset := e.Assets[n.Asset]
			state := generations[n.ID]
			if n.Kind == textNodeKind {
				a.paintCanvasText(p, n, rect, z)
			} else if hasAsset {
				bitmap := a.canvasBitmap(asset, e.View.Zoom)
				var checkSize float32
				if n.Adobe == nil && (e.Selection[n.ID] || a.hover == n.ID) {
					checkSize = 12 * z
				}
				reveal, revealing := a.imageReveals[n.ID]
				visible := canvasGenerationRect(rect, r)
				if revealing && reveal.start.IsZero() {
					paintImageTiles(p, rect, radius, nil, -1, motion)
					paintGenerationCaption(p, visible, "decoding", false)
				} else {
					drawCanvasPhoto(p, r, rect, radius, bitmap, checkSize)
					if revealing && bitmap != nil {
						progress := float32(p.Now().Sub(reveal.start)) / float32(imageRevealDuration)
						if progress < 1 {
							paintImageTiles(p, rect, radius, bitmap, max(0, progress), motion)
						}
					}
					if state.animates() {
						// Keep the first result visible while the remaining versions generate.
						paintGenerationCaption(p, visible, state.status, true)
					}
				}
			} else if state.animates() {
				paintImageTiles(p, rect, radius, nil, -1, motion)
				paintGenerationCaption(p, canvasGenerationRect(rect, r), state.status, false)
			} else {
				p.Fill(rect, t.Surface, radius)
				if state.status != "" && state.status != "succeeded" {
					paintCanvasGenerationTerminal(p, state, canvasGenerationRect(rect, r))
				} else {
					p.Icon(icons["spark"], ui.Rect{X: rect.X + rect.W/2 - 12*z, Y: rect.Y + rect.H/2 - 12*z, W: 24 * z, H: 24 * z}, t.TextMuted)
				}
			}
			editing := a.textEdit != nil && a.textEdit.editor == e && a.textEdit.id == n.ID
			if editing {
				continue
			}
			if e.Selection[n.ID] {
				p.Stroke(ui.Rect{X: rect.X - 1.5, Y: rect.Y - 1.5, W: rect.W + 3, H: rect.H + 3}, t.Text, radius+1.5, 1.15)
			} else if a.hover == n.ID {
				p.Stroke(ui.Rect{X: rect.X - 1.5, Y: rect.Y - 1.5, W: rect.W + 3, H: rect.H + 3}, t.TextMuted, radius+1.5, .8)
			}
		}
		if m := a.materials; m != nil {
			for _, drop := range m.importing {
				if drop.editor != e || drop.source != "pinterest" {
					continue
				}
				rect := ui.Rect{X: r.X + ox + float32(drop.at.X)*z, Y: r.Y + oy + float32(drop.at.Y)*z, W: 240 * z, H: 200 * z}
				p.Fill(rect, t.Surface, min(7*z, float32(4)))
				p.Icon(icons["download"], ui.Rect{X: rect.X + rect.W/2 - 9, Y: rect.Y + rect.H/2 - 9, W: 18, H: 18}, t.TextMuted)
			}
		}
		if q := a.selectionRect; q != nil {
			rect := ui.Rect{X: r.X + ox + float32(q.X)*z, Y: r.Y + oy + float32(q.Y)*z, W: float32(q.W) * z, H: float32(q.H) * z}
			p.Fill(rect, t.Text.Alpha(.06), 0)
			p.Stroke(rect, t.TextMuted, 0, 1)
		}
	})
}

func (a *desktop) canvasBitmap(asset Asset, zoom float64) *ui.Bitmap {
	if a.images == nil {
		return nil
	}
	if zoom > 2 {
		if b := a.images.Get(asset, 1024); b != nil {
			return b
		}
	}
	return a.images.Get(asset, 512)
}

// Metal keeps only the innermost rounded clip. When a photo meets a corner
// of the viewport, round the photo itself and leave the viewport clip active.
func drawCanvasPhoto(p *ui.Painter, view, photo ui.Rect, radius float32, bitmap *ui.Bitmap, checkSize float32) {
	nearSide := photo.X < view.X+canvasCorner || photo.X+photo.W > view.X+view.W-canvasCorner
	nearEnd := photo.Y < view.Y+canvasCorner || photo.Y+photo.H > view.Y+view.H-canvasCorner
	if nearSide && nearEnd {
		cornerPhoto.Draw(p, photo, shader.Options{
			Images: [4]*ui.Bitmap{bitmap, checker}, Radius: radius,
			Uniforms: map[string]any{"CheckSize": checkSize},
		})
		return
	}
	p.Clip(photo, radius, func() {
		if checkSize > 0 {
			drawCheckers(p, photo, checkSize)
		}
		if bitmap != nil {
			p.Image(bitmap, photo, ui.Contain)
		}
	})
}

var cornerPhoto = shader.MustCompile(`package main
var CheckSize float
func Fragment(dst vec4, pos vec2) vec4 {
	background := vec4(0)
	if CheckSize > 0 {
		background = image1At(mod(pos * (12 / CheckSize), image1Size()))
	}
	size := image0Size()
	if size.x < 1 || size.y < 1 { return background }
	box := rectSize()
	fit := min(box.x / size.x, box.y / size.y)
	texel := (pos - (box - size * fit) * 0.5) / fit
	if texel.x < 0 || texel.y < 0 || texel.x >= size.x || texel.y >= size.y { return background }
	pixel := image0Sample(clamp(texel, vec2(0.5), size - vec2(0.5)))
	return pixel + background * (1 - pixel.a)
}`)

var checker = func() *ui.Bitmap {
	im := image.NewRGBA(image.Rect(0, 0, 240, 240))
	for y := 0; y < 240; y++ {
		for x := 0; x < 240; x++ {
			v := uint8(255)
			if (x/12+y/12)%2 == 0 {
				v = 238
			}
			im.SetRGBA(x, y, color.RGBA{v, v, v, 255})
		}
	}
	return ui.NewBitmap(im)
}()

func drawCheckers(p *ui.Painter, r ui.Rect, step float32) {
	scale := max(step/12, .01)
	tile := 240 * scale
	for y := r.Y; y < r.Y+r.H; y += tile {
		for x := r.X; x < r.X+r.W; x += tile {
			w, h := min(tile, r.X+r.W-x), min(tile, r.Y+r.H-y)
			p.ImageRegion(checker, ui.Rect{W: w / scale, H: h / scale}, ui.Rect{X: x, Y: y, W: w, H: h}, ui.ImageOptions{})
		}
	}
}
func (a *desktop) previewModal(c *ui.Context) {
	w, h := c.Size()
	asset := a.previewAsset
	ui.DialogBase(c, &a.previewOpen, func(backdrop, panel ui.Element) {
		backdrop.Background(ui.RGBA(0, 0, 0, .6))
		panel.Size(w-40, h-40).Padding(0).Radius(10).Clip().Background(c.Theme().Background).Gap(0)
		ui.Row(c).Height(44).Padding(0, 16).Gap(8).AlignItems(ui.Center).Children(func() {
			name := asset.Name
			if name == "" {
				name = imageTitle(asset)
			}
			fittingLabel(c, name, 13, 500).Grow(1)
			ui.Textf(c, "%d × %d · %s", asset.Width, asset.Height, imageByteSize(asset.Bytes)).FontFeatures("tnum").FontSize(11).TextColor(c.Theme().TextMuted)
			iconButton(c, "download", "下载", func() { a.exportImage(asset) })
			iconButton(c, "close", "关闭", func() { a.previewOpen = false })
		})
		box := ui.Box(c.Key("large-preview")).Grow(1).FillWidth().Clip().Background(c.Theme().Background).Focusable().Label("原图预览")
		r := box.Bounds()
		if a.previewView.Zoom == 0 && r.W > 0 {
			z := math.Min(float64(r.W)/float64(asset.Width), float64(r.H)/float64(asset.Height))
			a.previewView = Viewport{(float64(r.W) - float64(asset.Width)*z) / 2, (float64(r.H) - float64(asset.Height)*z) / 2, z}
		}
		dragHint(c, box, a.previewPanning, "拖动平移 · 滚动缩放").HandleInput(func(ev ui.InputEvent) bool {
			v := &a.previewView
			if v.Zoom <= 0 {
				return false
			}
			zoom := func(f float64) {
				x, y := float64(ev.X), float64(ev.Y)
				wx, wy := (x-v.X)/v.Zoom, (y-v.Y)/v.Zoom
				v.Zoom = math.Max(.02, math.Min(4, v.Zoom*f))
				v.X = x - wx*v.Zoom
				v.Y = y - wy*v.Zoom
			}
			switch ev.Kind {
			case ui.InputScroll:
				if ev.Precise && ev.Mods&(ui.Ctrl|ui.Cmd) == 0 {
					v.X -= float64(ev.DX)
					v.Y -= float64(ev.DY)
				} else {
					zoom(math.Exp(-float64(ev.DY) * .005))
				}
			case ui.InputZoom:
				zoom(float64(ev.Scale))
			case ui.InputPointerDown:
				a.previewPanning = true
				a.previewDrag = Point{float64(ev.X) - v.X, float64(ev.Y) - v.Y}
			case ui.InputPointerUp:
				a.previewPanning = false
			case ui.InputPointerMove:
				if a.previewPanning {
					v.X = float64(ev.X) - a.previewDrag.X
					v.Y = float64(ev.Y) - a.previewDrag.Y
				}
			default:
				return false
			}
			return true
		}).Draw(func(p *ui.Painter, r ui.Rect) {
			if a.previewView.Zoom <= 0 && r.W > 0 && r.H > 0 {
				z := math.Min(float64(r.W)/float64(asset.Width), float64(r.H)/float64(asset.Height))
				a.previewView = Viewport{(float64(r.W) - float64(asset.Width)*z) / 2, (float64(r.H) - float64(asset.Height)*z) / 2, z}
			}
			p.Clip(r, 0, func() {
				if a.images != nil {
					b := a.images.Get(asset, 0)
					if b == nil {
						b = a.images.Get(asset, 1024)
					}
					if b == nil {
						b = a.images.Get(asset, 512)
					}
					if b != nil {
						v := a.previewView
						p.Image(b, ui.Rect{X: r.X + float32(v.X), Y: r.Y + float32(v.Y), W: float32(float64(asset.Width) * v.Zoom), H: float32(float64(asset.Height) * v.Zoom)}, ui.Contain)
					}
				}
			})
		})
	})
}

func (a *desktop) performanceReceipt() map[string]any {
	times := make([]int64, 0, len(a.frameTimes))
	for _, v := range a.frameTimes {
		if v > 0 {
			times = append(times, v)
		}
	}
	sortTimes(times)
	p95 := 0.0
	if len(times) > 0 {
		p95 = float64(times[int(float64(len(times)-1)*.95)]) / 1e6
	}
	return map[string]any{"paintSamples": len(times), "paintCPU_p95_ms": p95, "nativePaintCount": a.paintCount, "metric": "CPU command construction only; not end-to-end GPU FPS", "cacheBytes": func() int64 {
		if a.images == nil {
			return 0
		}
		return a.images.bytes
	}(), "canvasCards": len(a.editor.Project.Graph.Nodes), "zoom": a.editor.View.Zoom, "motion": a.motionTrace.receipt()}
}
func sortTimes(x []int64) {
	for i := 1; i < len(x); i++ {
		for j := i; j > 0 && x[j] < x[j-1]; j-- {
			x[j], x[j-1] = x[j-1], x[j]
		}
	}
}
