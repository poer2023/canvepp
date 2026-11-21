package main

import (
	"encoding/json"
	"errors"

	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"
)

func (s *Store) PurgedNodes(project string) (map[string]bool, error) {
	rows, err := s.DB.Query("SELECT id FROM purged_nodes WHERE project_id=?", project)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := map[string]bool{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			return nil, err
		}
		ids[id] = true
	}
	return ids, rows.Err()
}

type purgeResult struct {
	Project      Project
	Removed      []string
	Files        []string
	CleanupError string
}

// The database transaction removes the card and unshared versions together.
// Tombstones make stale saves, undo and task recovery unable to revive the card.
func (s *Store) PurgeCard(project, nodeID string) (purgeResult, error) {
	s.writes.Lock()
	defer s.writes.Unlock()
	result := purgeResult{}
	tx, err := s.DB.Begin()
	if err != nil {
		return result, err
	}
	defer tx.Rollback()
	var raw string
	if err = tx.QueryRow("SELECT body FROM projects WHERE id=?", project).Scan(&raw); err != nil {
		return result, err
	}
	if err = json.Unmarshal([]byte(raw), &result.Project); err != nil {
		return result, err
	}
	if slices.ContainsFunc(result.Project.Graph.Nodes, func(n Node) bool { return n.ID == nodeID }) {
		return result, errors.New("图片仍在画布中，请先移入回收站")
	}
	var count int
	if err = tx.QueryRow("SELECT COUNT(*) FROM canvas_trash WHERE project_id=? AND node_id=?", project, nodeID).Scan(&count); err != nil {
		return result, err
	}
	if count == 0 {
		return result, errors.New("回收站图片已不存在")
	}
	if err = tx.QueryRow("SELECT COUNT(*) FROM native_runs WHERE project_id=? AND json_extract(body,'$.nodeId')=? AND json_extract(body,'$.status') IN ('queued','preparing','submitted','running','unknown')", project, nodeID).Scan(&count); err != nil {
		return result, err
	}
	if count > 0 {
		return result, errors.New("这张图片仍有未结束的任务，请先取消或查询原任务结果")
	}
	candidates, protected := map[string]bool{}, map[string]bool{}
	assets := map[string]Asset{}
	rows, err := tx.Query("SELECT id,body,file FROM assets")
	if err != nil {
		return result, err
	}
	for rows.Next() {
		var id, body, file string
		var asset Asset
		if err = rows.Scan(&id, &body, &file); err == nil {
			err = json.Unmarshal([]byte(body), &asset)
		}
		if err != nil {
			rows.Close()
			return result, err
		}
		asset.File = file
		assets[id] = asset
		if asset.ProjectID == project && asset.NodeID == nodeID {
			candidates[id] = true
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return result, err
	}
	rows, err = tx.Query("SELECT node_id,body FROM canvas_trash")
	if err != nil {
		return result, err
	}
	isText := false
	for rows.Next() {
		var id, body string
		var tr Trash
		if err = rows.Scan(&id, &body); err == nil {
			err = json.Unmarshal([]byte(body), &tr)
		}
		if err != nil {
			rows.Close()
			return result, err
		}
		if id == nodeID && tr.Node.Kind == textNodeKind {
			isText = true
		}
		refs := append(slices.Clone(tr.Assets), tr.Node.Asset, tr.Display)
		for _, aid := range refs {
			if aid == "" {
				continue
			}
			if id == nodeID {
				candidates[aid] = true
			} else {
				protected[aid] = true
			}
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return result, err
	}
	rows, err = tx.Query("SELECT body FROM projects")
	if err != nil {
		return result, err
	}
	for rows.Next() {
		var body string
		var p Project
		if err = rows.Scan(&body); err == nil {
			err = json.Unmarshal([]byte(body), &p)
		}
		if err != nil {
			rows.Close()
			return result, err
		}
		for _, n := range p.Graph.Nodes {
			protected[n.Asset] = true
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return result, err
	}
	// Descendant provenance is retained even when its visible connection was cut.
	for id, asset := range assets {
		if !candidates[id] {
			for _, parent := range asset.Parents {
				protected[parent] = true
			}
		}
	}
	rows, err = tx.Query("SELECT body FROM native_runs")
	if err != nil {
		return result, err
	}
	for rows.Next() {
		var body string
		var r NativeRun
		if err = rows.Scan(&body); err == nil {
			err = json.Unmarshal([]byte(body), &r)
		}
		if err != nil {
			rows.Close()
			return result, err
		}
		if r.NodeID != nodeID {
			for _, src := range r.Sources {
				protected[src.ID] = true
			}
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return result, err
	}
	// Protect ancestors of any shared candidate as well.
	for changed := true; changed; {
		changed = false
		for id := range protected {
			for _, parent := range assets[id].Parents {
				if !protected[parent] {
					protected[parent] = true
					changed = true
				}
			}
		}
	}
	for id := range candidates {
		if protected[id] {
			continue
		}
		if _, err = tx.Exec("DELETE FROM assets WHERE id=?", id); err != nil {
			return result, err
		}
		if _, err = tx.Exec("INSERT INTO purged_assets VALUES(?,?,?) ON CONFLICT(id) DO NOTHING", id, project, string(encode(map[string]string{"deletedAt": timestamp()}))); err != nil {
			return result, err
		}
		result.Removed = append(result.Removed, id)
		if file := assets[id].File; file != "" {
			result.Files = append(result.Files, file)
		}
	}
	if _, err = tx.Exec("DELETE FROM canvas_trash WHERE project_id=? AND node_id=?", project, nodeID); err != nil {
		return result, err
	}
	if _, err = tx.Exec("INSERT INTO purged_nodes VALUES(?,?,?) ON CONFLICT(id) DO NOTHING", nodeID, project, string(encode(map[string]string{"deletedAt": timestamp()}))); err != nil {
		return result, err
	}
	result.Project.Revision++
	result.Project.UpdatedAt = timestamp()
	if _, err = tx.Exec("UPDATE projects SET body=? WHERE id=?", string(encode(result.Project)), project); err != nil {
		return result, err
	}
	title := "永久删除了 1 张回收站图片"
	if isText {
		title = "永久删除了 1 段回收站文字"
	}
	o := Operation{ID: uuid(), ProjectID: project, Action: "purge", Text: title, At: timestamp(), UpdatedAt: timestamp(), Nodes: []string{nodeID}}
	if err = saveOperations(tx, []Operation{o}); err != nil {
		return result, err
	}
	// Only unlink a content-addressed file when no project still references it.
	files := []string{}
	for _, file := range result.Files {
		if err = tx.QueryRow("SELECT COUNT(*) FROM assets WHERE file=?", file).Scan(&count); err != nil {
			return result, err
		}
		if count == 0 {
			files = append(files, file)
		}
	}
	if err = tx.Commit(); err != nil {
		return result, err
	}
	for _, file := range files {
		if filepath.Base(file) != file {
			continue
		}
		if err = os.Remove(filepath.Join(s.Dir, "assets", file)); err != nil && !os.IsNotExist(err) {
			result.CleanupError = "素材文件清理失败：" + err.Error()
		}
	}
	// Thumbnails share content hashes just like originals; shared versions keep them.
	for _, file := range files {
		sha := strings.TrimSuffix(file, filepath.Ext(file))
		if len(sha) != 64 {
			continue
		}
		matches, _ := filepath.Glob(filepath.Join(s.Dir, "thumbnails", sha+"-*.png"))
		for _, path := range matches {
			if err = os.Remove(path); err != nil && !os.IsNotExist(err) {
				result.CleanupError = "缩略图清理失败：" + err.Error()
			}
		}
	}

	return result, nil
}
func (e *Editor) forgetPurged(node string, result purgeResult) {
	if e.Purged == nil {
		e.Purged = map[string]bool{}
	}
	e.Purged[node] = true
	e.Trash = slices.DeleteFunc(e.Trash, func(t Trash) bool { return t.Node.ID == node })
	for _, id := range result.Removed {
		delete(e.Assets, id)
	}
	e.discardCardHistory(node, "")
	e.Project = result.Project
	e.reindex()
	delete(e.Selection, node)
}

// Preserve unrelated undo, including other deltas from a mixed action.
func (e *Editor) discardCardHistory(node, asset string) {
	matches := func(n *Node) bool {
		return n != nil && (node != "" && n.ID == node || asset != "" && n.Asset == asset)
	}
	prune := func(commands []command) []command {
		out := []command{}
		for _, cmd := range commands {
			cmd.Nodes = slices.DeleteFunc(slices.Clone(cmd.Nodes), func(d nodeDelta) bool {
				return matches(d.Before) || matches(d.After)
			})
			bad := func(edge Edge) bool { return node != "" && (edge.Source == node || edge.Target == node) }
			cmd.AddedEdges = slices.DeleteFunc(slices.Clone(cmd.AddedEdges), bad)
			cmd.RemovedEdges = slices.DeleteFunc(slices.Clone(cmd.RemovedEdges), bad)
			if cmd.CanvasLock != nil || len(cmd.Nodes)+len(cmd.AddedEdges)+len(cmd.RemovedEdges) > 0 {
				out = append(out, cmd)
			}
		}
		return out
	}
	e.undo, e.redo = prune(e.undo), prune(e.redo)
}
func (a *desktop) purgeCard(item libraryEntry) {
	if item.trash == nil || a.purgeBusy || a.store == nil {
		return
	}
	a.purgeBusy = true
	go func() {
		var err error
		if a.saves != nil {
			err = a.saves.Flush()
		}
		var result purgeResult
		if err == nil {
			result, err = a.store.PurgeCard(item.projectID, item.trash.Node.ID)
		}
		a.publish(func() {
			a.purgeBusy = false
			if err != nil {
				a.message(err.Error())
				return
			}
			e := a.editors[item.projectID]
			e.forgetPurged(item.trash.Node.ID, result)
			e.Events, _ = a.store.Operations(item.projectID)
			a.libraryKey = ""
			a.lineage = nil
			a.purgeOpen = false
			a.message("已永久删除")
			if result.CleanupError != "" {
				a.message(result.CleanupError)
			}
		})
	}()
}
func (a *desktop) confirmPurge(c *ui.Context, anchor ui.Element, item libraryEntry) {
	ui.PopoverBase(c, anchor, &a.purgeOpen, func(panel ui.Element) {
		toolSurface(c, panel, 8).Margin(3, 0, 0, 0)
		ui.Column(c).Width(242).Padding(12).Gap(8).Children(func() {
			flatTip(c, ui.Text(c, "永久删除「"+item.name+"」？").FontSize(14).Bold().MaxLines(2), item.name)
			ui.Text(c, "无法撤销。共用的素材仍会保留。").TextColor(c.Theme().TextMuted).FontSize(12)
			ui.Row(c).Justify(ui.End).Gap(8).Children(func() {
				disable(iconButton(c, "close", "取消", func() { a.purgeOpen = false }).AutoFocus(), a.purgeBusy)
				disable(commandHint(c, ui.Button(c, "永久删除").TextColor(c.Theme().Danger), "永久删除「"+item.name+"」，无法撤销").OnClick(func() {
					if a.purgeOpen && a.purgeKey == item.key {
						a.purgeCard(item)
					}
				}), a.purgeBusy)

			})
		})
	})
}
