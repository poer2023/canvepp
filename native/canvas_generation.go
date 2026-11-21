package main

import (
	"fmt"
	"math"
	"slices"
	"strings"
	"sync"

	"github.com/egoist/mygo/ui"
)

func dispatchGenerationBatch(runs []NativeRun, parallel bool, slots chan struct{}, execute func(NativeRun)) {
	run := func(r NativeRun) {
		slots <- struct{}{}
		defer func() { <-slots }()
		execute(r)
	}
	var batch sync.WaitGroup
	for _, r := range runs {
		if parallel {
			batch.Go(func() { run(r) })
		} else {
			run(r)
		}
	}
	batch.Wait()
}

type canvasGenerationState struct {
	status  string
	pending bool
}

type canvasGenerationIndex struct {
	project  string
	revision int
	nodes    map[string]canvasGenerationState
}

func (a *desktop) canvasGenerations() map[string]canvasGenerationState {
	project := a.editor.Project.ID
	if index := a.canvasGeneration; index != nil && index.project == project && index.revision == a.runRevision {
		return index.nodes
	}
	nodes := make(map[string]canvasGenerationState)
	for _, r := range a.runs {
		if r.ProjectID != project || r.NodeID == "" {
			continue
		}
		state := canvasGenerationState{status: r.Status, pending: a.running[r.ID]}
		switch {
		case r.Cancelled && r.Status != "succeeded":
			state.status = "cancelled"
		case state.pending && (r.Status == "unknown" || r.Status == "failed"):
			state.status = "recovering"
		case !state.pending && state.animates():
			// Previously interrupted runs stay available in history for recovery.
			state.status = "unknown"
		}
		old, hasOld := nodes[r.NodeID]
		if !hasOld || state.priority() > old.priority() {
			nodes[r.NodeID] = state
		}
	}
	a.canvasGeneration = &canvasGenerationIndex{project: project, revision: a.runRevision, nodes: nodes}
	return nodes
}

func (s canvasGenerationState) animates() bool {
	switch s.status {
	case "queued", "preparing", "submitted", "running", "recovering":
		return true
	}
	return false
}

func (s canvasGenerationState) priority() int {
	switch {
	case s.animates() && s.status != "queued":
		return 3
	case s.animates():
		return 2
	case s.pending:
		return 1
	default:
		return 0
	}
}

func (a *desktop) selectionGeneration() canvasGenerationState {
	states := a.canvasGenerations()
	var busiest canvasGenerationState
	for _, id := range a.draftSelection {
		state := states[id]
		if n := a.editor.Node(id); n != nil && (n.Asset != "" || !state.pending) {
			return canvasGenerationState{}
		}
		if state.priority() > busiest.priority() {
			busiest = state
		}
	}
	return busiest
}

func (a *desktop) prepareCanvasGeneration() []NativeRun {
	if a.editor == nil || strings.TrimSpace(a.prompt) == "" {
		return nil
	}
	e := a.editor
	selected := e.selectedImages()
	if len(selected) == 0 {
		return nil
	}
	var references, slots []Node
	var sources []Asset
	states := a.canvasGenerations()
	for _, n := range selected {
		if n.Asset == "" {
			if !states[n.ID].pending {
				slots = append(slots, n)
			}
			continue
		}
		asset, ok := e.Assets[n.Asset]
		if !ok {
			a.message("选中的参考图片未保存在本机")
			return nil
		}
		references = append(references, n)
		sources = append(sources, asset)
	}
	count := max(1, min(4, a.count))
	switch {
	case len(references) > 8:
		a.message("最多选择 8 张参考图片")
		return nil
	case len(references) > 0 && len(e.Project.Graph.Nodes)+count > e.Project.nodeLimit():
		a.message("画布图片数量超过限制")
		return nil
	case len(references) == 0 && len(slots) == 0:
		return nil
	}

	params := Params{fmt.Sprintf("%.0fx%.0f", a.width, a.height), "high", 1}
	ids := make([]string, 0, count)
	if len(references) == 0 {
		c := command{Label: "生成", GenerationCount: count}
		for _, before := range slots[:min(count, len(slots))] {
			after := before
			after.Config.Text, after.Config.Params = a.prompt, params
			after.Config.Params.N = count
			after.Config.CreationStarted = false
			c.Nodes = append(c.Nodes, nodeDelta{Before: &before, After: &after})
			ids = append(ids, after.ID)
		}
		e.Do(c)
	} else {
		a.updateDraft()
		right, top := math.Inf(-1), math.Inf(1)
		for _, n := range selected {
			r := nodeRect(n, e.Assets)
			right, top = max(right, r.X+r.W), min(top, r.Y)
		}
		var nodes []Node
		var edges []Edge
		for i := range count {
			n := newNode(e.Project.ID, Point{right + 120 + float64(i%2)*300, top + float64(i/2)*340})
			n.Name = fmt.Sprintf("生成结果 %d", i+1)
			n.Config = Config{Text: a.prompt, Params: params}
			nodes = append(nodes, n)
			ids = append(ids, n.ID)
			for _, src := range references {
				edges = append(edges, Edge{uuid(), src.ID, "image", n.ID, "image"})
			}
		}
		e.AddNamed(nodes, edges, "生成")
		a.draftKey = ""
	}

	groupID := uuid()
	runs := make([]NativeRun, 0, count)
	model := a.models.Image.withDefault(imageModelID)
	if a.funding.Image == apiFunding {
		model = modelChoice{Model: apiImageModel}
	}
	for i := range count {
		r := NativeRun{Funding: a.funding.Image, Model: model.Model, Effort: model.Effort, ID: uuid(), GenerationGroup: groupID, ProjectID: e.Project.ID, NodeID: ids[i%len(ids)], Prompt: a.prompt, Params: params, Sources: sources, Status: "queued", CreatedAt: timestamp()}
		runs = append(runs, r)
		a.runs = append([]NativeRun{r}, a.runs...)
		a.running[r.ID] = true
	}
	a.runRevision++
	return runs
}

func canvasGenerationRect(card, view ui.Rect) ui.Rect {
	x, y := max(card.X, view.X), max(card.Y, view.Y)
	return ui.Rect{X: x, Y: y, W: min(card.X+card.W, view.X+view.W) - x, H: min(card.Y+card.H, view.Y+view.H) - y}
}

func (a *desktop) canvasGenerationAccessibility(c *ui.Context) {
	e := a.editor
	view := ui.Rect{W: float32(a.canvasW), H: float32(a.canvasH)}
	for id, state := range a.canvasGenerations() {
		n := e.Node(id)
		if n == nil || n.Kind == textNodeKind {
			continue
		}
		if reveal, ok := a.imageReveals[id]; ok && reveal.start.IsZero() {
			state.status = "decoding"
		} else if state.status == "succeeded" || (n.Asset != "" && !state.animates()) {
			continue
		}
		q, v := nodeRect(*n, e.Assets), e.View
		card := ui.Rect{X: float32(q.X*v.Zoom + v.X), Y: float32(q.Y*v.Zoom + v.Y), W: float32(q.W * v.Zoom), H: float32(q.H * v.Zoom)}
		visible := canvasGenerationRect(card, view)
		if visible.W > 0 && visible.H > 0 {
			ui.Box(c.Key("generation-status-"+id)).Absolute().Left(visible.X).Top(visible.Y).Size(visible.W, visible.H).
				IgnoreInput(true).Role(ui.RoleStatus).Label(statusLabel(state.status))
		}
	}
}

func paintGenerationCaption(p *ui.Painter, rect ui.Rect, status string, bottom bool) {
	t := p.Theme()
	layout := ui.ShapeText(statusLabel(status), ui.Font{Size: 12, Weight: 500}, 0)
	textW, textH := layout.Size()
	w, h := textW+20, textH+8
	if rect.W < w+4 || rect.H < h+6 {
		return
	}
	x, y := rect.X+(rect.W-w)/2, rect.Y+(rect.H-h)/2
	if bottom {
		y = rect.Y + rect.H - h - 8
	}
	p.Fill(ui.Rect{X: x, Y: y, W: w, H: h}, t.Background, 5)
	p.TextLayout(layout, x+10, y+4, t.Text)
}

func paintCanvasGenerationTerminal(p *ui.Painter, state canvasGenerationState, rect ui.Rect) {
	t := p.Theme()
	cx, cy := rect.X+rect.W/2, rect.Y+rect.H/2
	label := statusLabel(state.status)
	layout := ui.ShapeText(label, ui.Font{Size: 12}, 0)
	tw, th := layout.Size()
	showLabel := rect.W >= tw+12 && rect.H >= th+19
	if showLabel {
		p.TextLayout(layout, cx-tw/2, cy+1, t.Text)
		cy -= th/2 + 4
	}
	if rect.W >= 20 && rect.H >= 20 {
		icon := "warning"
		switch state.status {
		case "unknown":
			icon = "history"
		case "cancelled":
			icon = "close"
		}
		p.Icon(icons[icon], ui.Rect{X: cx - 8, Y: cy - 8, W: 16, H: 16}, t.TextMuted)
	}
}

func (a *desktop) canvasImageVersions(n Node) []Asset {
	var versions []Asset
	archived := archivedPictureVersions(a.editor)
	for _, asset := range a.editor.Assets {
		if !archived[asset.ID] && (asset.NodeID == n.ID || asset.ID == n.Asset) {
			versions = append(versions, asset)
		}
	}
	slices.SortFunc(versions, func(x, y Asset) int {
		if order := strings.Compare(x.CreatedAt, y.CreatedAt); order != 0 {
			return order
		}
		return strings.Compare(x.ID, y.ID)
	})
	return versions
}
