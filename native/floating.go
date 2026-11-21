package main

import (
	"encoding/json"
	"math"

	"github.com/egoist/mygo/ui"
)

const (
	panelWidth                float32 = 320
	imagePanelHeaderHeight            = 44
	imageDrawerHeadingHeight          = 40
	creationTopPad                    = 3
	creationBottomPad                 = 12
	creationFormGap                   = 9
	creationPromptLabelHeight         = 18
	creationPromptGap                 = 5
	creationPromptHeight              = 84
	creationActionsHeight             = 42
	creationPanelHeight               = imagePanelHeaderHeight + 2 + creationTopPad + creationBottomPad + 2*creationFormGap +
		creationPromptLabelHeight + creationPromptGap + creationPromptHeight + 3*creationParameterHeight + creationActionsHeight
)

type panelPlacement struct {
	X, Y                    float32
	DockX, DockY            string
	Open, Collapsed, Placed bool
}

func (s *Store) loadPanel() panelPlacement {
	p := panelPlacement{DockX: "right", DockY: "top"}
	var raw string
	if s.DB.QueryRow("SELECT value FROM native_meta WHERE key='image_panel'").Scan(&raw) == nil {
		_ = json.Unmarshal([]byte(raw), &p)
	}
	return p
}
func (s *Store) savePanel(p panelPlacement) error {
	_, err := s.DB.Exec("INSERT OR REPLACE INTO native_meta VALUES('image_panel',?)", string(encode(p)))
	return err
}
func (a *desktop) saveLayout() {
	if a.saves != nil {
		a.saves.PutUI(a.panel)
	}
}

// Anchor all four sides to the same inset inside the canvas.
func panelBounds(w, h, ph float32) ui.Rect {
	return ui.Rect{X: 12, Y: 12, W: max(0, w-panelWidth-24), H: max(0, h-ph-24)}
}
func (p *panelPlacement) constrain(b ui.Rect) {
	if !p.Placed {
		p.DockX, p.DockY, p.Placed = "right", "top", true
	}
	if math.IsNaN(float64(p.X)) || math.IsInf(float64(p.X), 0) {
		p.X = b.X
	}
	if math.IsNaN(float64(p.Y)) || math.IsInf(float64(p.Y), 0) {
		p.Y = b.Y
	}
	switch p.DockX {
	case "left":
		p.X = b.X
	case "right":
		p.X = b.X + b.W
	}
	switch p.DockY {
	case "top":
		p.Y = b.Y
	case "bottom":
		p.Y = b.Y + b.H
	}
	p.X, p.Y = max(b.X, min(b.X+b.W, p.X)), max(b.Y, min(b.Y+b.H, p.Y))
}
func (p panelPlacement) snapped(b ui.Rect) panelPlacement {
	p.DockX, p.DockY = "", ""
	if p.X-b.X <= 28 {
		p.DockX = "left"
	} else if b.X+b.W-p.X <= 28 {
		p.DockX = "right"
	}
	if p.Y-b.Y <= 28 {
		p.DockY = "top"
	} else if b.Y+b.H-p.Y <= 28 {
		p.DockY = "bottom"
	}
	p.constrain(b)
	return p
}
func (a *desktop) panelHeight() float32 {
	if a.panel.Collapsed {
		return imagePanelHeaderHeight
	}
	if a.editor == nil || len(a.editor.Selection) == 0 {
		return 192
	}
	n, asset, hasImage := a.selectedImage()
	if hasImage && a.imageDrawer == "source" {
		height := float32(224)
		if _, recorded := a.sourceRecord(asset); recorded || n.Adobe != nil || asset.SourceURL != "" {
			height = 370
		}
		return min(height, max(200, float32(a.canvasH)-88))
	}
	if hasImage && !(a.imageDrawer == "creation" && a.creationEditing) {
		return min(500, max(200, float32(a.canvasH)-88))
	}
	height := float32(creationPanelHeight)
	if a.count > 1 {
		height += creationParameterHeight
	}
	if hasImage {
		height += 2 * imageDrawerHeadingHeight
	}
	if len(a.draftSelection) > 1 {
		height += imageDrawerHeadingHeight + float32((min(8, len(a.draftSelection))+3)/4)*59 + 7
	}
	if a.creationPixelsStacked() {
		height += creationStackedPixelHeight - creationParameterHeight
	}
	return min(height, max(200, float32(a.canvasH)-88))
}
func (a *desktop) panelRect() ui.Rect {
	h := a.panelHeight()
	a.panel.constrain(panelBounds(float32(a.canvasW), float32(a.canvasH), h))
	return ui.Rect{X: a.panel.X, Y: a.panel.Y, W: min(panelWidth, max(1, float32(a.canvasW)-24)), H: h}
}
func (a *desktop) panelInput(ev ui.InputEvent, header ui.Rect) bool {
	if a.motionTrace != nil {
		a.motionTrace.input(ev, "panel", "panel")
	}
	pointer := ui.Point{X: header.X + ev.X, Y: header.Y + ev.Y}
	switch ev.Kind {
	case ui.InputPointerDown:
		if ev.Button != 0 {
			return false
		}
		a.panelDragging = true
		a.hover = ""
		a.panelGrab = pointer
		a.panelDragStart = ui.Point{X: a.panel.X, Y: a.panel.Y}
		a.panel.DockX, a.panel.DockY = "", ""
		return true
	case ui.InputPointerMove:
		if !a.panelDragging {
			return false
		}
		// Several native pointer events can arrive before layout catches up.
		// Reconstruct absolute coordinates from the rendered header; never add
		// an old local delta to an already updated model position.
		a.panel.X = a.panelDragStart.X + pointer.X - a.panelGrab.X
		a.panel.Y = a.panelDragStart.Y + pointer.Y - a.panelGrab.Y
		a.panel.constrain(panelBounds(float32(a.canvasW), float32(a.canvasH), a.panelHeight()))
		return true
	case ui.InputPointerUp:
		if !a.panelDragging {
			return false
		}
		a.panelDragging = false
		a.panel = a.panel.snapped(panelBounds(float32(a.canvasW), float32(a.canvasH), a.panelHeight()))
		a.saveLayout()
		return true
	}
	return false
}

func (a *desktop) floatingInspector(c *ui.Context) {
	if !a.panel.Open {
		return
	}
	r := a.panelRect()
	panel := ui.Column(c.Key("floating-inspector")).Absolute().Size(r.W, r.H)
	if a.panelDragging {
		panel.Jump("height", r.H)
		panel.Left(a.panelDragStart.X).Top(a.panelDragStart.Y).Translate(r.X-a.panelDragStart.X, r.Y-a.panelDragStart.Y)
	} else {
		height := min(panel.AnimateWith("height", r.H, foldDuration, ui.EaseOut), max(200, float32(a.canvasH)-88))
		y := r.Y
		if a.panel.DockY == "bottom" {
			y += r.H - height
		} else {
			y = min(y, max(12, float32(a.canvasH)-height-12))
		}
		panel.Left(r.X).Top(y).Height(height)
	}
	creating := len(a.draftSelection) > 0 && a.imageDrawer == "creation" && a.creationEditing
	toolSurface(c, panel, 16)
	if creating {
		t := c.Theme()
		if !t.Dark {
			panel.Background(t.Background)
		}
		panel.Radius(13).Border(1, t.Border)
	} else {
		a.ratioMenuOpen = false
	}
	panel.Clip().Cache().Label("悬浮图片设置").Children(func() {
		ui.Row(c).Height(imagePanelHeaderHeight).Padding(0, 7, 0, 10).Gap(2).AlignItems(ui.Center).Children(func() {
			header := ui.Row(c.Key("panel-drag-handle")).Grow(1).FillHeight().Gap(7).Cursor(ui.CursorGrab).Label("拖动图片设置").Bind(&a.panelHandle)
			var dx, dy float32
			if a.panelDragging {
				dx, dy = r.X-a.panelDragStart.X, r.Y-a.panelDragStart.Y
			}
			dragHint(c, header, a.panelDragging, "拖动图片面板，靠边吸附").HandleInput(func(ev ui.InputEvent) bool {
				b := a.panelHandle.Bounds(c)
				b.X += dx
				b.Y += dy
				return a.panelInput(ev, b)
			}).Children(func() {
				uiIcon(c, "grip", 13).TextColor(c.Theme().TextMuted)
				title := "图片"
				if len(a.draftSelection) > 1 {
					title = "多图参考"
				} else if creating {
					title = "创作图片"
				}
				sectionTitle(c, title)
			})
			name, label := "minus", "收起图片设置"
			if a.panel.Collapsed {
				name, label = "plus", "展开图片设置"
			}
			iconButton(c, name, label, func() {
				a.panel.Collapsed = !a.panel.Collapsed
				a.ratioMenuOpen = false
				a.saveLayout()
			})
			iconButton(c, "close", "关闭图片设置", func() {
				a.panel.Open, a.ratioMenuOpen = false, false
				a.saveLayout()
			})
		})
		if !a.panel.Collapsed {
			body := ui.Column(c.Key("inspector-body")).FillWidth().Grow(1).MinHeight(0).Clip()
			paneMotion(c, body, 0, 5).Children(func() { a.inspector(c) })
		}
	})
}
