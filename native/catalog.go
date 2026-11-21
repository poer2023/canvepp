package main

import (
	"fmt"
	"math"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

type libraryEntry struct {
	key, projectID, name, date, day string
	deletedCanvas                   bool
	asset                           *Asset
	trash                           *Trash
	run                             *NativeRun
}
type libraryGroup struct {
	key, name string
	count     int
}

func galleryDate(raw string) (string, string) {
	t, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		return "", "未标注日期"
	}
	t = t.In(time.Local)
	return t.UTC().Format("2006-01-02T15:04:05.000000000Z"), t.Format("2006-01-02")
}
func imageByteSize(bytes int64) string {
	switch {
	case bytes < 1024:
		return fmt.Sprintf("%d B", bytes)
	case bytes < 1<<20:
		return fmt.Sprintf("%.1f KiB", float64(bytes)/1024)
	default:
		return fmt.Sprintf("%.1f MiB", float64(bytes)/(1<<20))
	}
}

func imageTitle(a Asset) string {
	if strings.HasPrefix(a.Name, "exec-") {
		return "生成图片"
	}
	if a.Name == "" {
		return "未命名图片"
	}
	return strings.TrimSuffix(a.Name, filepath.Ext(a.Name))
}

// Cache the catalog, not a canvas selection. Gallery filtering spans all
// projects and never changes graph content, active versions or task ownership.
func (a *desktop) libraryItems() []libraryEntry {
	query, order := strings.ToLower(strings.TrimSpace(a.galleryQuery)), a.gallerySort
	if a.left == "回收站" {
		query = ""
		order = a.trashSort
	}
	var key strings.Builder
	fmt.Fprintf(&key, "%s/%s/%d/%d/%d", a.left, query, order, a.galleryGrouping, a.runRevision)
	for _, id := range a.order {
		e := a.editors[id]
		fmt.Fprintf(&key, "/%s:%d:%d:%d:%s", id, e.Project.Revision, len(e.Assets), len(e.Trash), e.Project.Name)
	}
	stamp := key.String()
	if stamp == a.libraryKey {
		return a.libraryCache
	}
	filter := parseLibraryQuery(query)
	items := []libraryEntry{}
	for _, id := range a.order {
		e := a.editors[id]
		if e.Project.DeletedAt != "" {
			if a.left == "回收站" {
				date, day := galleryDate(e.Project.DeletedAt)
				items = append(items, libraryEntry{key: id + "/canvas", projectID: id, name: e.Project.Name, date: date, day: day, deletedCanvas: true})
			}
			continue
		}
		if a.left == "回收站" {
			seen := map[string]bool{}
			for i := len(e.Trash) - 1; i >= 0; i-- {
				tr := e.Trash[i]
				if e.Node(tr.Node.ID) != nil || seen[tr.Node.ID] {
					continue
				}
				seen[tr.Node.ID] = true
				date, day := galleryDate(tr.DeletedAt)
				item := libraryEntry{key: id + "/trash/" + tr.ID, projectID: id, name: tr.Node.Name, date: date, day: day, trash: &tr}
				if asset, ok := e.Assets[tr.Display]; ok {
					item.asset = &asset
				}
				items = append(items, item)
			}
			continue
		}
		for _, asset := range e.Assets {
			asset := asset
			if query != "" && !filter.matches(asset.Name+" "+imageTitle(asset)+" "+e.Project.Name+" "+a.assetPrompt(asset)) {
				continue
			}
			date, day := galleryDate(asset.CreatedAt)
			items = append(items, libraryEntry{key: id + "/asset/" + asset.ID, projectID: id, name: imageTitle(asset), date: date, day: day, asset: &asset})
		}
	}
	if a.left != "回收站" {
		for _, r := range a.runs {
			r := r
			e := a.editors[r.ProjectID]
			if e == nil || e.Project.DeletedAt != "" || e.Purged[r.NodeID] || r.Status == "succeeded" {
				continue
			}
			if query != "" && !filter.matches(r.Prompt+" "+e.Project.Name) {
				continue
			}
			date, day := galleryDate(r.CreatedAt)
			items = append(items, libraryEntry{key: r.ProjectID + "/run/" + r.ID, projectID: r.ProjectID, name: statusLabel(r.Status), date: date, day: day, run: &r})
		}
	}
	slices.SortFunc(items, func(x, y libraryEntry) int {
		var cmp int
		switch order {
		case 1:
			cmp = strings.Compare(x.date, y.date)
		case 2:
			cmp = strings.Compare(strings.ToLower(x.name), strings.ToLower(y.name))
		default:
			cmp = strings.Compare(y.date, x.date)
		}
		if cmp == 0 {
			return strings.Compare(x.key, y.key)
		}
		return cmp
	})
	groupMap := map[string]*libraryGroup{}
	for _, item := range items {
		k, name := item.day, item.day
		if a.left != "回收站" && a.galleryGrouping == 1 {
			k = item.projectID
			name = a.editors[k].Project.Name
		}
		if groupMap[k] == nil {
			groupMap[k] = &libraryGroup{key: k, name: name}
		}
		groupMap[k].count++
	}
	groups := make([]libraryGroup, 0, len(groupMap))
	for _, g := range groupMap {
		groups = append(groups, *g)
	}
	slices.SortFunc(groups, func(x, y libraryGroup) int {
		if x.key == y.key {
			return 0
		}
		if a.left != "回收站" && a.galleryGrouping == 1 {
			cmp := strings.Compare(x.name, y.name)
			if cmp == 0 {
				return strings.Compare(x.key, y.key)
			}
			return cmp
		}
		if x.key == "未标注日期" {
			return 1
		}
		if y.key == "未标注日期" {
			return -1
		}
		if order == 1 {
			return strings.Compare(x.key, y.key)
		}
		return strings.Compare(y.key, x.key)
	})
	a.libraryKey, a.libraryCache, a.libraryGroups = stamp, items, groups
	if selected := a.selectedLibraryGroup(); selected != "" && groupMap[selected] == nil {
		a.selectLibraryGroup("")
	}
	a.libraryFilterKey = ""
	return items
}
func (a *desktop) selectedLibraryGroup() string {
	if a.left == "回收站" {
		return a.trashGroup
	}
	return a.galleryGroup
}
func (a *desktop) selectLibraryGroup(key string) {
	if a.left == "回收站" {
		a.trashGroup = key
	} else {
		a.galleryGroup = key
	}
	a.resetGalleryScroll()
}
func (a *desktop) visibleLibraryItems() []libraryEntry {
	items := a.libraryItems()
	group := a.selectedLibraryGroup()
	key := a.libraryKey + "/filter/" + group
	if key == a.libraryFilterKey {
		return a.libraryVisible
	}
	visible := items
	if group != "" {
		visible = make([]libraryEntry, 0, len(items))
		for _, item := range items {
			k := item.day
			if a.left != "回收站" && a.galleryGrouping == 1 {
				k = item.projectID
			}
			if k == group {
				visible = append(visible, item)
			}
		}
	}
	a.libraryVisible, a.libraryFilterKey = visible, key
	return visible
}

// Names and creation order are independent of pointer and canvas geometry changes.
type canvasDirectoryIndex struct {
	query        string
	order, count int
	ids          []string
}

func (a *desktop) filteredCanvases() []string {
	index := &a.canvasIndex
	if index.ids != nil && index.query == a.canvasQuery && index.order == a.canvasSort && index.count == len(a.order) {
		return index.ids
	}
	query := parseLibraryQuery(a.canvasQuery)
	ids := make([]string, 0, len(a.order))
	for _, id := range a.order {
		p := a.editors[id].Project
		if p.DeletedAt == "" && (len(query) == 0 || query.matches(p.Name)) {
			ids = append(ids, id)
		}
	}
	switch a.canvasSort {
	case 1:
		slices.Reverse(ids)
	case 2:
		slices.SortStableFunc(ids, func(x, y string) int {
			return strings.Compare(strings.ToLower(a.editors[x].Project.Name), strings.ToLower(a.editors[y].Project.Name))
		})
	}
	slices.SortStableFunc(ids, func(x, y string) int {
		px, py := a.editors[x].Project.Pinned, a.editors[y].Project.Pinned
		if px == py {
			return 0
		}
		if px {
			return -1
		}
		return 1
	})
	*index = canvasDirectoryIndex{query: a.canvasQuery, order: a.canvasSort, count: len(a.order), ids: ids}
	return ids
}

func (a *desktop) returnToAsset(item libraryEntry) {
	e := a.editors[item.projectID]
	if e == nil {
		a.message("来源画布暂不可用")
		return
	}
	a.editor = e
	a.left = "画布"
	a.closeSettings()
	a.hover = ""
	a.fitNext = false
	clear(e.Selection)
	var node *Node
	if item.asset != nil {
		node = e.displayedPicture(*item.asset)
		if node == nil {
			node = e.Node(item.asset.NodeID)
		}
	}
	if node == nil && item.run != nil {
		node = e.Node(item.run.NodeID)
	}
	if node != nil {
		e.Selection[node.ID] = true
		a.focusNodeID = node.ID
		a.panel.Open = true
		if item.asset != nil && node.Asset != item.asset.ID {
			a.message("已定位来源图片，保留画布当前版本")
		}
	} else {
		a.fitNext = true
		a.panel.Open = false
		if item.asset != nil && !archivedPictureVersions(e)[item.asset.ID] {
			a.message("已打开所属画布")
		} else if item.asset == nil || item.asset.NodeID != "" || e.Project.AdobeBoard == nil {
			a.message("已打开来源画布，已删除的图片可在回收站恢复")
		}
	}
	a.draftKey = ""
	a.canvas.Focus()
	a.rememberCanvasOpen(e)
}
func (a *desktop) focusCanvasNode() {
	if a.focusNodeID == "" || a.editor == nil {
		return
	}
	n := a.editor.Node(a.focusNodeID)
	a.focusNodeID = ""
	if n == nil {
		return
	}
	q := nodeRect(*n, a.editor.Assets)
	w, h := a.canvasW, a.canvasH
	if a.panel.Open && (a.panel.DockX == "right" || !a.panel.Placed || a.panel.X > float32(w)/2) {
		w = max(280, w-float64(panelWidth)-36)
	}
	z := math.Min(1, math.Min((w-80)/q.W, (h-100)/q.H))
	a.editor.View = Viewport{X: w/2 - (q.X+q.W/2)*z, Y: h/2 - (q.Y+q.H/2)*z, Zoom: max(.05, z)}
}
func (a *desktop) restoreGalleryItem(item libraryEntry) {
	e := a.editors[item.projectID]
	if e == nil || item.trash == nil {
		return
	}
	if len(e.Project.Graph.Nodes) >= e.Project.nodeLimit() {
		a.message("画布已满，请先移出部分图片再恢复")
		return
	}
	e.Restore(item.trash.ID)
	a.editor = e
	a.draftKey = ""
	a.message("已恢复到「" + e.Project.Name + "」")
}
