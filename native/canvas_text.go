package main

import (
	"math"
	"strings"

	"github.com/egoist/mygo/ui"
)

const (
	textNodeKind      = "text"
	canvasTextW       = 272
	canvasTextPt      = 21
	canvasTextPad     = 7
	canvasTextLeading = 1.4
)

type canvasTextEdit struct {
	editor   *Editor
	id       string
	at       Point
	value    string
	measured string
	focused  bool
	zoom     float32
	wrap     float32
	width    float32
	height   float32
}

func (a *desktop) toggleCanvasTextTool() {
	a.commitCanvasText()
	a.textPlacing = !a.textPlacing
	a.hover, a.space = "", false
	a.canvas.Focus()
}

func (a *desktop) beginCanvasText(at Point, node *Node) {
	a.commitCanvasText()
	if node == nil && len(a.editor.Project.Graph.Nodes) >= a.editor.Project.nodeLimit() {
		a.textPlacing = false
		a.message("画布项目数量超过限制")
		return
	}
	edit := &canvasTextEdit{editor: a.editor, at: at}
	if node != nil {
		edit.id, edit.at, edit.value = node.ID, node.Position, node.Config.Text
		clear(a.editor.Selection)
		a.editor.Selection[node.ID] = true
	}
	a.textEdit, a.textPlacing = edit, false
	a.hover, a.draftKey = "", ""
}

func (a *desktop) canvasTextLayout(value string) *ui.TextLayout {
	if layout := a.textLayouts[value]; layout != nil {
		return layout
	}
	if a.textLayouts == nil || len(a.textLayouts) >= 512 {
		a.textLayouts = make(map[string]*ui.TextLayout)
	}
	layout := ui.ShapeText(value, ui.Font{Size: canvasTextPt, LineHeight: canvasTextLeading}, canvasTextW)
	a.textLayouts[value] = layout
	return layout
}

func (a *desktop) commitCanvasText() {
	edit := a.textEdit
	a.textEdit = nil
	if edit == nil || strings.TrimSpace(edit.value) == "" || edit.editor.Project.DeletedAt != "" {
		return
	}
	e := edit.editor
	var node Node
	if edit.id == "" {
		if len(e.Project.Graph.Nodes) >= e.Project.nodeLimit() {
			a.textEdit = edit
			a.message("画布项目数量超过限制")
			return
		}
		node = newNode(e.Project.ID, edit.at)
	} else {
		old := e.Node(edit.id)
		if old == nil || old.Config.Text == edit.value {
			return
		}
		node = *old
	}
	node.Kind, node.Config.Text = textNodeKind, edit.value
	line, _, _ := strings.Cut(strings.TrimSpace(edit.value), "\n")
	name := []rune(strings.TrimSpace(line))
	if len(name) > 42 {
		name = append(name[:42], '…')
	}
	node.Name = string(name)
	w, h := a.canvasTextLayout(edit.value).Size()
	node.TextSize = Point{math.Ceil(float64(w)) + 2*canvasTextPad, math.Ceil(float64(h)) + 2*canvasTextPad}
	if edit.id == "" {
		e.Add([]Node{node}, nil)
	} else {
		e.Edit(node)
	}
	a.draftKey = ""
}

func (a *desktop) canvasTextEditor(c *ui.Context) {
	edit := a.textEdit
	if edit == nil || edit.editor != a.editor {
		return
	}
	field := ui.TextArea(c.Key(edit), &edit.value)
	view := &edit.editor.View
	z := float32(view.Zoom)
	pad := canvasTextPad * z
	availableW := float32(a.canvasW)
	availableH := float32(a.canvasH) - canvasToolsInset - 36
	wrap := max(1, min(canvasTextW*z, availableW-2*pad))
	changed := edit.width == 0 || edit.measured != edit.value
	if changed || z != edit.zoom || wrap != edit.wrap {
		size := canvasTextPt * z
		layout := ui.ShapeText(edit.value, ui.Font{Size: size, LineHeight: canvasTextLeading}, wrap)
		w, h := layout.Size()
		edit.width = min(wrap, max(94*z, float32(math.Ceil(float64(w)))+6)) + 2*pad
		edit.height = max(size*canvasTextLeading, float32(math.Ceil(float64(h)))) + 2*pad + 2
		edit.measured, edit.zoom, edit.wrap = edit.value, z, wrap
	}
	width, height := edit.width, max(1, min(edit.height, availableH))
	x, y := float32(edit.at.X*view.Zoom+view.X), float32(edit.at.Y*view.Zoom+view.Y)
	if changed {
		// Reveal the editor near an edge by panning the canvas. The text
		// layer keeps its original world position through editing and save.
		fitX := min(max(0, x), max(0, availableW-width))
		fitY := min(max(0, y), max(0, availableH-height))
		view.X += float64(fitX - x)
		view.Y += float64(fitY - y)
		x, y = fitX, fitY
	}
	finish := func() { a.commitCanvasText(); a.canvas.Focus() }
	cancel := func() { a.textEdit = nil; a.canvas.Focus() }
	t := c.Theme()
	field.Label("编辑画布文字").
		Description("点击画布其他位置或按 Command／Ctrl + Enter 保存，Esc 取消").
		Absolute().Left(x).Top(y).Size(width, height).MinHeight(0).Padding(pad).
		Font("").FontSize(canvasTextPt*z).LineHeight(canvasTextLeading).Placeholder("输入文字").
		Background(ui.Transparent).TextColor(t.Text).Border(0, ui.Transparent).Radius(0).
		FocusRing(false).AutoFocus().OnTextKey(func(k ui.TextKey) bool {
		switch {
		case k.Key == ui.KeyEscape:
			cancel()
		case k.Key == ui.KeyEnter && k.Mods == ui.Cmd:
			edit.value = k.Text()
			finish()
		default:
			return false
		}
		return true
	}).DrawOver(func(p *ui.Painter, r ui.Rect) {
		p.Stroke(r, t.TextMuted, 0, 1)
	})
	thinScroll(c, field, nil)
	if field.FocusWithin() {
		edit.focused = true
	} else if edit.focused {
		a.commitCanvasText()
		c.After(0)
	}
}

func (a *desktop) paintCanvasText(p *ui.Painter, n Node, rect ui.Rect, zoom float32) {
	if a.textEdit != nil && a.textEdit.id == n.ID && a.textEdit.editor == a.editor {
		return
	}
	layout := a.canvasTextLayout(n.Config.Text)
	p.Transform(ui.Scaling(zoom, zoom), rect.X, rect.Y, func() {
		p.TextLayout(layout, rect.X+canvasTextPad, rect.Y+canvasTextPad, p.Theme().Text)
	})
}
