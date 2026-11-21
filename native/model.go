package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"slices"
	"time"
)

type Point struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}
type Viewport struct {
	X    float64 `json:"x"`
	Y    float64 `json:"y"`
	Zoom float64 `json:"zoom"`
}
type Params struct {
	Size    string `json:"size"`
	Quality string `json:"quality"`
	N       int    `json:"n"`
}
type Config struct {
	Text            string `json:"text,omitempty"`
	Params          Params `json:"params"`
	Note            string `json:"note,omitempty"`
	CreationStarted bool   `json:"creationStarted,omitempty"`
}
type Node struct {
	PositionLocked bool            `json:"positionLocked,omitempty"`
	Adobe          *adobePlacement `json:"adobeLayer,omitempty"`
	ID             string          `json:"id"`
	ProjectID      string          `json:"projectId"`
	Kind           string          `json:"kind"`
	Name           string          `json:"name"`
	Position       Point           `json:"position"`
	TextSize       Point           `json:"textSize,omitzero"`
	CanvasScale    float64         `json:"canvasScale,omitempty"` // Preserve 0.6.3 backup data when saving an existing canvas.
	ConfigVersion  int             `json:"configVersion"`
	Config         Config          `json:"config"`
	Asset          string          `json:"selectedAssetVersionId,omitempty"`
	Stale          bool            `json:"stale"`
}
type Edge struct {
	ID         string `json:"id"`
	Source     string `json:"sourceNodeId"`
	SourcePort string `json:"sourcePort"`
	Target     string `json:"targetNodeId"`
	TargetPort string `json:"targetPort"`
}
type Graph struct {
	Nodes []Node   `json:"nodes"`
	Edges []Edge   `json:"edges"`
	View  Viewport `json:"viewport"`
}
type Project struct {
	LayoutLocked   bool        `json:"layoutLocked,omitempty"`
	AdobeBoard     *adobeBoard `json:"adobeBoard,omitempty"`
	Pinned         bool        `json:"pinned,omitempty"`
	DeletedAt      string      `json:"deletedAt,omitempty"`
	ID             string      `json:"id"`
	Name           string      `json:"name"`
	SchemaVersion  int         `json:"schemaVersion"`
	Revision       int         `json:"graphRevision"`
	LayoutRevision int         `json:"layoutRevision"`
	CreatedAt      string      `json:"createdAt"`
	UpdatedAt      string      `json:"updatedAt"`
	Graph          Graph       `json:"graph"`
}

func (p *Project) nodeLimit() int {
	if p.AdobeBoard != nil {
		return max(500, p.AdobeBoard.LayerCount)
	}
	return 500
}

type Asset struct {
	ID         string   `json:"id"`
	ProjectID  string   `json:"projectId"`
	AssetID    string   `json:"assetId"`
	SHA        string   `json:"sha256"`
	Mime       string   `json:"mime"`
	Width      int      `json:"width"`
	Height     int      `json:"height"`
	Bytes      int64    `json:"byteSize"`
	CreatedAt  string   `json:"createdAt"`
	Name       string   `json:"name"`
	Parents    []string `json:"parentVersionIds"`
	RunID      string   `json:"runId,omitempty"`
	NodeID     string   `json:"nodeId,omitempty"`
	Source     string   `json:"source"`
	SourceURL  string   `json:"sourceUrl,omitempty"`
	ExternalID string   `json:"externalId,omitempty"`
	Favorite   bool     `json:"favorite,omitempty"`
	Assessment string   `json:"assessment,omitempty"`
	File       string   `json:"-"`
}
type Trash struct {
	ID        string   `json:"id"`
	ProjectID string   `json:"projectId"`
	Node      Node     `json:"node"`
	Edges     []Edge   `json:"edges"`
	Assets    []string `json:"assetVersionIds"`
	Display   string   `json:"displayAssetVersionId,omitempty"`
	DeletedAt string   `json:"deletedAt"`
}
type Rect struct{ X, Y, W, H float64 }

func (r Rect) Contains(p Point) bool {
	return p.X >= r.X && p.Y >= r.Y && p.X <= r.X+r.W && p.Y <= r.Y+r.H
}
func (r Rect) Intersects(b Rect) bool {
	return r.X <= b.X+b.W && r.X+r.W >= b.X && r.Y <= b.Y+b.H && r.Y+r.H >= b.Y
}
func uuid() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	b[6] = b[6]&15 | 64
	b[8] = b[8]&63 | 128
	s := hex.EncodeToString(b[:])
	return s[:8] + "-" + s[8:12] + "-" + s[12:16] + "-" + s[16:20] + "-" + s[20:]
}
func timestamp() string { return time.Now().UTC().Format(time.RFC3339Nano) }
func defaults() Params  { return Params{Size: "1024x1024", Quality: "high", N: 1} }
func newNode(project string, at Point) Node {
	return Node{ID: uuid(), ProjectID: project, Kind: "image", Name: "新建图片", Position: at, ConfigVersion: 1, Config: Config{Params: defaults()}}
}
func cloneGraph(g Graph) Graph {
	g.Nodes = slices.Clone(g.Nodes)
	g.Edges = slices.Clone(g.Edges)
	return g
}
func nodeRect(n Node, assets map[string]Asset) Rect {
	if n.Kind == textNodeKind {
		return Rect{n.Position.X, n.Position.Y, max(36, n.TextSize.X), max(34, n.TextSize.Y)}
	}
	if layer := n.Adobe; layer != nil && layer.W > 0 && layer.H > 0 {
		return Rect{n.Position.X, n.Position.Y, layer.W, layer.H}
	}
	w, h := 240.0, 200.0
	if a, ok := assets[n.Asset]; ok && a.Width > 0 && a.Height > 0 {
		h = w * float64(a.Height) / float64(a.Width)
		if h > 420 {
			w *= 420 / h
			h = 420
		}
	}
	return Rect{n.Position.X, n.Position.Y, w, h}
}

type nodeDelta struct{ Before, After *Node }
type command struct {
	CanvasLock               *bool
	Nodes                    []nodeDelta
	AddedEdges, RemovedEdges []Edge
	Label                    string
	GenerationCount          int
}
type Editor struct {
	Project      Project
	Assets       map[string]Asset
	Trash        []Trash
	Selection    map[string]bool
	View         Viewport
	undo, redo   []command
	index        map[string]int
	drag         map[string]Point
	OnChange     func()
	dirty        bool
	saveError    string
	retryingSave bool
	Events       []Operation
	Purged       map[string]bool
}

func newEditor(p Project, assets []Asset, trash []Trash) *Editor {
	e := &Editor{Project: p, Assets: map[string]Asset{}, Trash: trash, Selection: map[string]bool{}, View: p.Graph.View}
	if e.View.Zoom <= 0 {
		e.View = Viewport{X: 48, Y: 48, Zoom: 1}
	}
	for _, a := range assets {
		e.Assets[a.ID] = a
	}
	e.reindex()
	return e
}
func (e *Editor) reindex() {
	e.index = make(map[string]int, len(e.Project.Graph.Nodes))
	for i, n := range e.Project.Graph.Nodes {
		e.index[n.ID] = i
	}
}
func (e *Editor) Node(id string) *Node {
	if i, ok := e.index[id]; ok {
		return &e.Project.Graph.Nodes[i]
	}
	return nil
}
func (e *Editor) World(p Point) Point {
	return Point{(p.X - e.View.X) / e.View.Zoom, (p.Y - e.View.Y) / e.View.Zoom}
}
func (e *Editor) Hit(p Point) string {
	for i := len(e.Project.Graph.Nodes) - 1; i >= 0; i-- {
		n := e.Project.Graph.Nodes[i]
		if nodeRect(n, e.Assets).Contains(p) {
			return n.ID
		}
	}
	return ""
}
func (e *Editor) ZoomAt(p Point, factor float64) {
	world := e.World(p)
	if factor <= 0 || math.IsNaN(factor) || math.IsInf(factor, 0) {
		return
	}
	e.View.Zoom = math.Max(.05, math.Min(4, e.View.Zoom*factor))
	e.View.X = p.X - world.X*e.View.Zoom
	e.View.Y = p.Y - world.Y*e.View.Zoom
}
func (e *Editor) Fit(w, h float64) {
	if len(e.Project.Graph.Nodes) == 0 {
		e.View = Viewport{X: 48, Y: 48, Zoom: 1}
		return
	}
	l, t, r, b := math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)
	if board := e.Project.AdobeBoard; board != nil && board.Width > 0 && board.Height > 0 {
		l, t, r, b = 0, 0, board.Width, board.Height
	}
	for _, n := range e.Project.Graph.Nodes {
		q := nodeRect(n, e.Assets)
		l = math.Min(l, q.X)
		t = math.Min(t, q.Y)
		r = math.Max(r, q.X+q.W)
		b = math.Max(b, q.Y+q.H)
	}
	z := math.Max(.05, math.Min(1, math.Min((w-96)/(r-l), (h-96)/(b-t))))
	e.View = Viewport{(w-(r-l)*z)/2 - l*z, (h-(b-t)*z)/2 - t*z, z}
}
func (e *Editor) SelectRect(r Rect, add bool) {
	if !add {
		clear(e.Selection)
	}
	for _, n := range e.Project.Graph.Nodes {
		if r.Intersects(nodeRect(n, e.Assets)) {
			e.Selection[n.ID] = true
		}
	}
}
func (e *Editor) Selected() []Node {
	out := make([]Node, 0, len(e.Selection))
	for _, n := range e.Project.Graph.Nodes {
		if e.Selection[n.ID] {
			out = append(out, n)
		}
	}
	return out
}

func (e *Editor) selectedImages() []Node {
	return slices.DeleteFunc(e.Selected(), func(n Node) bool { return n.Kind == textNodeKind })
}

func (p *Project) imageNodeCount() int {
	count := 0
	for _, n := range p.Graph.Nodes {
		if n.Kind != textNodeKind {
			count++
		}
	}
	return count
}

func nodeQuantity(count, textCount int) string {
	switch {
	case textCount == 0:
		return fmt.Sprintf("%d 张图片", count)
	case textCount == count:
		return fmt.Sprintf("%d 段文字", count)
	default:
		return fmt.Sprintf("%d 项内容", count)
	}
}
func (e *Editor) changed() {
	e.dirty = true
	e.Project.Revision++
	e.Project.UpdatedAt = timestamp()
	if e.OnChange != nil {
		e.OnChange()
	}
}
func (e *Editor) Do(c command) {
	e.apply(c, true)
	e.recordCommand(c, "")
	e.undo = append(e.undo, c)
	if len(e.undo) > 100 {
		e.undo = e.undo[1:]
	}
	e.redo = nil
	e.changed()
}
func (e *Editor) Undo() {
	if len(e.drag) > 0 {
		e.EndDrag()
	}
	if len(e.undo) == 0 {
		return
	}
	c := e.undo[len(e.undo)-1]
	e.undo = e.undo[:len(e.undo)-1]
	e.apply(c, false)
	e.recordCommand(c, "undo")
	e.redo = append(e.redo, c)
	e.changed()
}
func (e *Editor) Redo() {
	if len(e.drag) > 0 {
		e.EndDrag()
	}
	if len(e.redo) == 0 {
		return
	}
	c := e.redo[len(e.redo)-1]
	e.redo = e.redo[:len(e.redo)-1]
	e.apply(c, true)
	e.recordCommand(c, "redo")
	e.undo = append(e.undo, c)
	e.changed()
}

// Apply only fields changed by the command, preserving independently returned images.
func mergeNode(cur *Node, from, to Node) {
	if from.PositionLocked != to.PositionLocked {
		cur.PositionLocked = to.PositionLocked
	}
	if from.CanvasScale != to.CanvasScale {
		cur.CanvasScale = to.CanvasScale
	}
	if from.Position != to.Position {
		cur.Position = to.Position
	}
	if from.TextSize != to.TextSize {
		cur.TextSize = to.TextSize
	}
	if from.Config != to.Config {
		cur.Config = to.Config
		cur.ConfigVersion++
	}
	if from.Asset != to.Asset {
		cur.Asset = to.Asset
	}
	if from.Name != to.Name {
		cur.Name = to.Name
	}
	if from.Stale != to.Stale {
		cur.Stale = to.Stale
	}
}
func (e *Editor) apply(c command, forward bool) {
	if c.CanvasLock != nil {
		e.Project.LayoutLocked = *c.CanvasLock == forward
	}
	for _, d := range c.Nodes {
		from, to := d.Before, d.After
		if (from != nil && e.Purged[from.ID]) || (to != nil && e.Purged[to.ID]) {
			continue
		}
		if !forward {
			from, to = to, from
		}
		if to == nil {
			if current := e.Node(from.ID); current != nil {
				n := *current
				edges := []Edge{}
				for _, edge := range e.Project.Graph.Edges {
					if edge.Source == n.ID || edge.Target == n.ID {
						edges = append(edges, edge)
					}
				}
				ids := []string{}
				for _, a := range e.Assets {
					if a.NodeID == n.ID || a.ID == n.Asset {
						ids = append(ids, a.ID)
					}
				}
				e.Trash = append(e.Trash, Trash{uuid(), e.Project.ID, n, edges, ids, n.Asset, timestamp()})
				e.Project.Graph.Nodes = slices.DeleteFunc(e.Project.Graph.Nodes, func(n Node) bool { return n.ID == from.ID })
				delete(e.Selection, from.ID)
				e.reindex()
			}
		} else if cur := e.Node(to.ID); cur != nil {
			if from != nil {
				mergeNode(cur, *from, *to)
			}
		} else {
			restored := *to
			// A result may arrive while the card is deleted or an insertion is
			// undone. Replaying an old command must keep that returned image.
			if restored.Asset == "" {
				for i := len(e.Trash) - 1; i >= 0; i-- {
					if e.Trash[i].Node.ID == restored.ID && e.Trash[i].Node.Asset != "" {
						restored.Asset = e.Trash[i].Node.Asset
						break
					}
				}
			}
			e.Project.Graph.Nodes = append(e.Project.Graph.Nodes, restored)
			e.reindex()
		}
	}
	remove, add := c.RemovedEdges, c.AddedEdges
	if !forward {
		remove, add = add, remove
	}
	for _, edge := range remove {
		e.Project.Graph.Edges = slices.DeleteFunc(e.Project.Graph.Edges, func(x Edge) bool { return x.ID == edge.ID })
	}
	for _, edge := range add {
		if e.Node(edge.Source) != nil && e.Node(edge.Target) != nil && !slices.ContainsFunc(e.Project.Graph.Edges, func(x Edge) bool { return x.ID == edge.ID }) {
			e.Project.Graph.Edges = append(e.Project.Graph.Edges, edge)
		}
	}
	e.Project.Graph.Edges = slices.DeleteFunc(e.Project.Graph.Edges, func(x Edge) bool { return e.Node(x.Source) == nil || e.Node(x.Target) == nil })
}
func (e *Editor) Add(nodes []Node, edges []Edge) { e.AddNamed(nodes, edges, "新建") }
func (e *Editor) AddNamed(nodes []Node, edges []Edge, label string) {
	c := command{Label: label, AddedEdges: edges}
	clear(e.Selection)
	for _, n := range nodes {
		n := n
		c.Nodes = append(c.Nodes, nodeDelta{After: &n})
		e.Selection[n.ID] = true
	}
	e.Do(c)
}
func (e *Editor) Delete() { e.deleteNodes(e.Selected()) }
func (e *Editor) deleteNodes(nodes []Node) {
	if len(nodes) == 0 {
		return
	}
	c := command{Label: "删除"}
	removed := make(map[string]bool, len(nodes))
	for _, n := range nodes {
		n := n
		removed[n.ID] = true
		c.Nodes = append(c.Nodes, nodeDelta{Before: &n})
	}
	for _, edge := range e.Project.Graph.Edges {
		if removed[edge.Source] || removed[edge.Target] {
			c.RemovedEdges = append(c.RemovedEdges, edge)
		}
	}
	e.Do(c)
}
func (e *Editor) Edit(n Node) {
	old := e.Node(n.ID)
	if old == nil || reflect.DeepEqual(*old, n) {
		return
	}
	before := *old
	label := "修改图片"
	if n.Kind == textNodeKind {
		label = "修改文字"
	}
	e.Do(command{Label: label, Nodes: []nodeDelta{{&before, &n}}})
}
func (e *Editor) BeginDrag() {
	e.drag = map[string]Point{}
	for id := range e.Selection {
		if n := e.Node(id); n != nil && !e.positionLocked(n) {
			e.drag[id] = n.Position
		}
	}
}
func (e *Editor) positionLocked(n *Node) bool {
	return e.Project.LayoutLocked || n.PositionLocked
}
func (e *Editor) toggleCanvasLock() {
	e.EndDrag()
	locked := !e.Project.LayoutLocked
	e.Do(command{Label: "画布布局锁", CanvasLock: &locked})
}
func (e *Editor) toggleNodeLock(id string) {
	if n := e.Node(id); n != nil && !e.Project.LayoutLocked {
		e.EndDrag()
		updated := *n
		updated.PositionLocked = !n.PositionLocked
		e.Edit(updated)
	}
}
func (e *Editor) MoveDrag(dx, dy float64) {
	for id, p := range e.drag {
		if n := e.Node(id); n != nil {
			n.Position = Point{p.X + dx, p.Y + dy}
		}
	}
}
func (e *Editor) EndDrag() {
	c := command{Label: "移动"}
	for id, p := range e.drag {
		if n := e.Node(id); n != nil && n.Position != p {
			before, after := *n, *n
			before.Position = p
			c.Nodes = append(c.Nodes, nodeDelta{&before, &after})
		}
	}
	e.drag = nil
	if len(c.Nodes) > 0 {
		e.undo = append(e.undo, c)
		e.redo = nil
		e.changed()
	}
}
func (e *Editor) Restore(id string) {
	var selected *Trash
	for i := len(e.Trash) - 1; i >= 0; i-- {
		if e.Trash[i].ID == id {
			selected = &e.Trash[i]
			break
		}
	}
	if selected == nil || e.Node(selected.Node.ID) != nil {
		return
	}
	n := selected.Node
	if n.Asset == "" {
		n.Asset = selected.Display
	}
	edges := []Edge{}
	for _, t := range e.Trash {
		for _, edge := range t.Edges {
			if (edge.Source == n.ID || e.Node(edge.Source) != nil) && (edge.Target == n.ID || e.Node(edge.Target) != nil) {
				edges = append(edges, edge)
			}
		}
	}
	e.AddNamed([]Node{n}, edges, "恢复")
}

type Clipboard struct {
	Format    string `json:"format"`
	ProjectID string `json:"projectId"`
	Nodes     []Node `json:"nodes"`
	Edges     []Edge `json:"edges"`
}

func (e *Editor) Copy() Clipboard {
	clip := Clipboard{Format: "CANVEPP_NATIVE_V1", ProjectID: e.Project.ID, Nodes: e.Selected()}
	for _, edge := range e.Project.Graph.Edges {
		if e.Selection[edge.Source] && e.Selection[edge.Target] {
			clip.Edges = append(clip.Edges, edge)
		}
	}
	return clip
}
func (e *Editor) Paste(clip Clipboard, at Point) error {
	if clip.Format != "CANVEPP_NATIVE_V1" || clip.ProjectID != e.Project.ID || len(clip.Nodes) == 0 || len(e.Project.Graph.Nodes)+len(clip.Nodes) > e.Project.nodeLimit() {
		return fmt.Errorf("无法粘贴该画布内容")
	}
	ids := map[string]string{}
	left, top := math.Inf(1), math.Inf(1)
	for _, n := range clip.Nodes {
		if n.ID == "" || ids[n.ID] != "" || math.IsNaN(n.Position.X) || math.IsInf(n.Position.X, 0) || math.IsNaN(n.Position.Y) || math.IsInf(n.Position.Y, 0) {
			return fmt.Errorf("无效卡片")
		}
		if n.Asset != "" {
			if _, ok := e.Assets[n.Asset]; !ok {
				return fmt.Errorf("原图已不可用")
			}
		}
		ids[n.ID] = uuid()
		left = math.Min(left, n.Position.X)
		top = math.Min(top, n.Position.Y)
	}
	nodes := slices.Clone(clip.Nodes)
	for i := range nodes {
		nodes[i].ID = ids[nodes[i].ID]
		nodes[i].ProjectID = e.Project.ID
		nodes[i].Position = Point{at.X + nodes[i].Position.X - left, at.Y + nodes[i].Position.Y - top}
	}
	edges := []Edge{}
	for _, edge := range clip.Edges {
		if ids[edge.Source] == "" || ids[edge.Target] == "" {
			return fmt.Errorf("无效连线")
		}
		edge.ID = uuid()
		edge.Source = ids[edge.Source]
		edge.Target = ids[edge.Target]
		edges = append(edges, edge)
	}
	e.AddNamed(nodes, edges, "粘贴")
	return nil
}
func encode(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}
