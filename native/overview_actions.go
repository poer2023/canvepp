package main

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"runtime"
	"slices"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

func (a *desktop) overviewPictureMenu(m *ui.Menu, item libraryEntry) {
	if m.Item("进入画布").Chosen() {
		a.pictureOverview().selected = item.key
		a.returnToAsset(item)
	}
	var targets []*Editor
	for _, id := range a.order {
		e := a.editors[id]
		if id != item.projectID && e.Project.DeletedAt == "" {
			targets = append(targets, e)
		}
	}
	m.Submenu("移动到画布", func(sub *ui.Menu) {
		for _, target := range targets {
			if sub.Item(target.Project.Name).Disabled(len(target.Project.Graph.Nodes) >= target.Project.nodeLimit()).Chosen() {
				if err := a.moveOverviewPicture(item, target); err != nil {
					a.message("移动图片失败：" + err.Error())
				}
			}
		}
	}).Disabled(len(targets) == 0)
	reveal := "在文件管理器中显示"
	switch runtime.GOOS {
	case "darwin":
		reveal = "在 Finder 中显示"
	case "windows":
		reveal = "在文件资源管理器中显示"
	}
	if m.Item(reveal).Chosen() && a.store != nil {
		path, err := a.store.AssetPath(*item.asset)
		if err == nil {
			_, err = os.Stat(path)
		}
		if err != nil {
			a.message("原图文件不可用：" + err.Error())
		} else {
			mygo.Shell.ShowItemInFolder(path)
		}
	}
	m.Separator()
	if m.Item("移入回收站").Chosen() {
		a.archiveOverviewPicture(item)
	}
}

// Prefer the originating card when the same version is displayed on multiple cards.
func (e *Editor) displayedPicture(asset Asset) *Node {
	if node := e.Node(asset.NodeID); node != nil && node.Asset == asset.ID {
		return node
	}
	for i := range e.Project.Graph.Nodes {
		if e.Project.Graph.Nodes[i].Asset == asset.ID {
			return &e.Project.Graph.Nodes[i]
		}
	}
	return nil
}

func (a *desktop) archiveOverviewPicture(item libraryEntry) {
	e := a.editors[item.projectID]
	if e == nil || e.Project.DeletedAt != "" {
		return
	}
	asset, ok := e.Assets[item.asset.ID]
	if !ok || archivedPictureVersions(e)[asset.ID] {
		return
	}
	a.pictureOverview().selected = ""
	placement := e.displayedPicture(asset)
	if placement != nil {
		replacement := e.otherPictureVersion(placement.ID, asset.ID)
		if replacement == "" {
			e.deleteNodes([]Node{*placement})
			a.draftKey, a.hover = "", ""
			a.message("已将图片移入回收站")
			a.undoDeleteEditor, a.undoDeleteRevision = e, e.Project.Revision
			return
		}
		placement.Asset = replacement
		e.discardCardHistory(placement.ID, "")
		a.draftKey = ""
	}
	// Archive the selected version on its own so other images on the card
	// stay in the library, including the version that takes its place.
	n := newNode(e.Project.ID, Point{48, 48})
	n.Asset, n.Name = asset.ID, item.name
	if placement != nil {
		n.Config = placement.Config
	}
	e.Trash = append(e.Trash, Trash{ID: uuid(), ProjectID: e.Project.ID, Node: n, Assets: []string{asset.ID}, Display: asset.ID, DeletedAt: timestamp()})
	e.recordCommand(command{Label: "删除", Nodes: []nodeDelta{{Before: &n}}}, "")
	e.changed()
	a.message("已将图片移入回收站")
}

func (a *desktop) moveOverviewPicture(item libraryEntry, target *Editor) error {
	source := a.editors[item.projectID]
	if a.store == nil || source == nil || target == nil || source == target || source.Project.DeletedAt != "" || target.Project.DeletedAt != "" {
		return errors.New("画布暂不可用")
	}
	if len(target.Project.Graph.Nodes) >= target.Project.nodeLimit() {
		return errors.New("目标画布的图片数量已到上限")
	}
	asset, ok := source.Assets[item.asset.ID]
	if !ok {
		return errors.New("原图已不在来源画布")
	}
	from, to := source.Project, target.Project
	from.Graph, to.Graph = cloneGraph(from.Graph), cloneGraph(to.Graph)
	from.Graph.View, to.Graph.View = source.View, target.View

	nodeID := ""
	at := Point{48, 48}
	for _, existing := range to.Graph.Nodes {
		b := nodeRect(existing, target.Assets)
		at.X = max(at.X, b.X+b.W+36)
		at.Y = min(at.Y, b.Y)
	}
	card := newNode(to.ID, at)
	card.Name = item.name
	if n := source.displayedPicture(asset); n != nil {
		nodeID = n.ID
		card.Name, card.Config = n.Name, n.Config
		replacement := source.otherPictureVersion(n.ID, asset.ID)
		connected := slices.ContainsFunc(from.Graph.Edges, func(edge Edge) bool {
			return edge.Source == n.ID || edge.Target == n.ID
		})
		if replacement != "" || connected {
			from.Graph.Nodes[source.index[n.ID]].Asset = replacement
		} else {
			from.Graph.Nodes = slices.DeleteFunc(from.Graph.Nodes, func(node Node) bool { return node.ID == n.ID })
		}
	}
	for _, run := range a.runs {
		if run.ProjectID == from.ID && (run.ID == asset.RunID || nodeID != "" && run.NodeID == nodeID) {
			switch run.Status {
			case "queued", "preparing", "submitted", "running", "unknown":
				return errors.New("请先处理这张图片尚未结束的生成任务")
			}
		}
	}

	// A version shared by other placements or by trash snapshots stays available
	// there. A new placement can refer to the existing file without duplicating it.
	shared := slices.ContainsFunc(from.Graph.Nodes, func(n Node) bool { return n.Asset == asset.ID }) ||
		slices.ContainsFunc(source.Trash, func(tr Trash) bool {
			return tr.Node.Asset == asset.ID || tr.Display == asset.ID || slices.Contains(tr.Assets, asset.ID)
		})
	moved := asset
	moved.ProjectID, moved.NodeID = to.ID, card.ID
	if shared {
		moved.ID, moved.AssetID = uuid(), uuid()
		moved.Parents = append(slices.Clone(asset.Parents), asset.ID)
		// If a connection still needs the source card and there is no alternative
		// version, that card remains one of the shared placements.
		for i, n := range from.Graph.Nodes {
			if n.ID == nodeID && n.Asset == "" {
				from.Graph.Nodes[i].Asset = asset.ID
				break
			}
		}
	}
	card.Asset = moved.ID
	to.Graph.Nodes = append(to.Graph.Nodes, card)
	from.Revision++
	to.Revision++
	from.UpdatedAt, to.UpdatedAt = timestamp(), timestamp()
	if a.saves != nil {
		if err := a.saves.Flush(); err != nil {
			return err
		}
	}
	if err := a.store.transferPicture(asset, moved, from, to); err != nil {
		return err
	}
	if !shared {
		delete(source.Assets, asset.ID)
		source.discardCardHistory(nodeID, asset.ID)
	} else if nodeID != "" {
		source.discardCardHistory(nodeID, "")
	}
	delete(source.Selection, nodeID)
	target.Assets[moved.ID] = moved
	source.Project, target.Project = from, to
	source.reindex()
	target.reindex()
	source.dirty, target.dirty = false, false
	source.saveError, target.saveError = "", ""
	a.draftKey, a.libraryKey = "", ""
	a.lineage = nil
	a.pictureOverview().selected = to.ID + "/" + moved.ID
	a.status = "已保存"
	a.message("已移到「" + to.Name + "」")
	return nil
}

func (e *Editor) otherPictureVersion(node, excluded string) string {
	archived := archivedPictureVersions(e)
	var id, latest string
	for _, a := range e.Assets {
		if a.NodeID != node || a.ID == excluded || archived[a.ID] {
			continue
		}
		date, _ := galleryDate(a.CreatedAt)
		if id == "" || date > latest || date == latest && a.ID < id {
			id, latest = a.ID, date
		}
	}
	return id
}

// Save both owners and the version in one transaction. Reject stale snapshots
// before changing the in-memory catalog, so failure leaves both canvases intact.
func (s *Store) transferPicture(original, moved Asset, source, target Project) error {
	path, err := s.AssetPath(original)
	if err != nil {
		return err
	}
	if _, err := os.Stat(path); err != nil {
		return err
	}
	s.writes.Lock()
	defer s.writes.Unlock()
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	saved := func(result sql.Result, err error) error {
		if err != nil {
			return err
		}
		n, err := result.RowsAffected()
		if err == nil && n != 1 {
			err = errors.New("画布内容已变化，请刷新后重试")
		}
		return err
	}
	for _, p := range []Project{source, target} {
		err = saved(tx.Exec("UPDATE projects SET body=? WHERE id=? AND COALESCE(json_extract(body,'$.graphRevision'),0)=?",
			string(encode(p)), p.ID, p.Revision-1))
		if err != nil {
			return err
		}
	}
	if moved.ID == original.ID {
		err = saved(tx.Exec("UPDATE assets SET project_id=?,body=? WHERE id=? AND project_id=?",
			moved.ProjectID, string(encode(moved)), original.ID, source.ID))
	} else {
		err = saved(tx.Exec("INSERT INTO assets SELECT ?,?,?,file FROM assets WHERE id=? AND project_id=?",
			moved.ID, moved.ProjectID, string(encode(moved)), original.ID, source.ID))
	}
	if err != nil {
		return fmt.Errorf("无法保存图片位置：%w", err)
	}
	return tx.Commit()
}
