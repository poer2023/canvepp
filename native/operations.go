package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
)

type Operation struct {
	ID        string   `json:"id"`
	ProjectID string   `json:"projectId"`
	Action    string   `json:"action"`
	Base      string   `json:"base,omitempty"`
	Text      string   `json:"text"`
	Detail    string   `json:"detail,omitempty"`
	Nodes     []string `json:"nodes,omitempty"`
	At        string   `json:"at"`
	UpdatedAt string   `json:"updatedAt"`
}

// Older undo receipts stored the command only in their localized title.
func (o Operation) significant() bool {
	action := o.Action
	if action == "undo" || action == "redo" {
		action = o.Base
		if action == "" {
			_, title, _ := strings.Cut(o.Text, "：")
			for _, prefix := range []string{"新建了", "导入了", "从 Pinterest", "从 Eagle", "粘贴了", "恢复了", "发起生成", "将 ", "连接了", "断开了", "修改了采用版本"} {
				if strings.HasPrefix(title, prefix) {
					return true
				}
			}
			return false
		}
	}
	switch action {
	case "create", "import", "paste", "restore", "generate", "generation", "trash", "purge", "connect", "disconnect", "version", "edit-text":
		return true
	case "edit":
		return strings.Contains(o.Text, "采用版本")
	}
	return false
}

func (e *Editor) recordCommand(c command, prefix string) {
	var action, title string
	var ids []string
	textCount := 0
	for _, d := range c.Nodes {
		n := d.After
		if n == nil {
			n = d.Before
		}
		if n != nil {
			ids = append(ids, n.ID)
			if n.Kind == textNodeKind {
				textCount++
			}
		}
	}
	switch c.Label {
	case "新建":
		action, title = "create", "新建了 "+nodeQuantity(len(ids), textCount)
	case "导入", "从 Pinterest 导入", "从 Pinterest 收藏 导入", "从 Eagle 素材库 导入":
		action, title = "import", fmt.Sprintf("导入了 %d 张图片", len(ids))
		if c.Label != "导入" {
			title = fmt.Sprintf("%s %d 张图片", c.Label, len(ids))
		}
	case "粘贴":
		action, title = "paste", "粘贴了 "+nodeQuantity(len(ids), textCount)
	case "恢复":
		action, title = "restore", "恢复了 "+nodeQuantity(len(ids), textCount)
	case "生成":
		action, title = "generate", fmt.Sprintf("发起生成 %d 张图片", max(len(ids), c.GenerationCount))
	case "删除":
		action, title = "trash", "将 "+nodeQuantity(len(ids), textCount)+"移入回收站"
	case "修改文字":
		if len(c.Nodes) != 1 || c.Nodes[0].Before == nil || c.Nodes[0].After == nil || c.Nodes[0].Before.Config.Text == c.Nodes[0].After.Config.Text {
			return
		}
		action, title = "edit-text", "编辑了文字"
	default:
		switch {
		case len(c.RemovedEdges) > 0 || len(c.AddedEdges) > 0:
			edges := c.RemovedEdges
			action, title = "disconnect", fmt.Sprintf("断开了 %d 条连线", len(edges))
			if len(edges) == 0 {
				edges = c.AddedEdges
				action, title = "connect", fmt.Sprintf("连接了 %d 条连线", len(edges))
			}
			for _, edge := range edges {
				ids = append(ids, edge.Source, edge.Target)
			}
		case len(c.Nodes) == 1 && c.Nodes[0].Before != nil && c.Nodes[0].After != nil && c.Nodes[0].Before.Asset != c.Nodes[0].After.Asset:
			action, title = "version", "切换了图片版本"
		default:
			return
		}
	}
	slices.Sort(ids)
	now := timestamp()
	o := Operation{ID: uuid(), ProjectID: e.Project.ID, Action: action, Text: title, Nodes: slices.Compact(ids), At: now, UpdatedAt: now}
	if prefix != "" {
		o.Base, o.Action = action, prefix
		if prefix == "undo" {
			o.Text = "撤销：" + title
		} else {
			o.Text = "重做：" + title
		}
	}
	e.Events = append(e.Events, o)
}

func (s *Store) Operations(project string) ([]Operation, error) {
	return s.OperationPage(project, Operation{})
}
func (s *Store) OperationPage(project string, before Operation) ([]Operation, error) {
	return s.operationPage(project, before, true)
}

func (s *Store) historyOperationPage(project string, before Operation) ([]Operation, error) {
	return s.operationPage(project, before, false)
}

func (s *Store) operationPage(project string, before Operation, importantOnly bool) ([]Operation, error) {
	query := "SELECT body FROM operation_events WHERE project_id=?"
	if importantOnly {
		query += " AND json_extract(body,'$.action') NOT IN ('move','copy','export','assistant','canvas','rename')"
	}
	args := []any{project}
	if before.ID != "" {
		query += " AND (rtrim(json_extract(body,'$.at'),'Z') < rtrim(?,'Z') OR (rtrim(json_extract(body,'$.at'),'Z') = rtrim(?,'Z') AND rowid < (SELECT rowid FROM operation_events WHERE id=?)))"
		args = append(args, before.At, before.At, before.ID)
	}
	// Several native events can share a clock tick. Their insert order
	// preserves the commands the user actually made.
	query += " ORDER BY rtrim(json_extract(body,'$.at'),'Z') DESC, rowid DESC"
	rows, err := s.DB.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Operation{}
	for len(out) < 500 && rows.Next() {
		var raw string
		var o Operation
		if err = rows.Scan(&raw); err != nil {
			return nil, err
		}
		if err = json.Unmarshal([]byte(raw), &o); err != nil {
			return nil, err
		}
		if !importantOnly || o.significant() {
			out = append(out, o)
		}
	}
	slices.Reverse(out)
	return out, rows.Err()
}
func cancelOperation(r NativeRun) Operation {
	if r.Status == "cancelled" {
		return *runOperation(r)
	}
	return Operation{ID: "run/" + r.ID + "/cancel-request", ProjectID: r.ProjectID, Action: "generation", Text: "请求取消生成，已返回的图片仍会保留", Nodes: []string{r.NodeID}, At: r.UpdatedAt, UpdatedAt: r.UpdatedAt}
}
func saveOperations(tx *sql.Tx, events []Operation) error {
	for _, o := range events {
		if _, err := tx.Exec("INSERT INTO operation_events VALUES(?,?,?) ON CONFLICT(id) DO UPDATE SET body=excluded.body WHERE rtrim(json_extract(excluded.body,'$.updatedAt'),'Z') >= rtrim(json_extract(operation_events.body,'$.updatedAt'),'Z')", o.ID, o.ProjectID, string(encode(o))); err != nil {
			return err
		}
	}
	return nil
}
func runOperation(r NativeRun) *Operation {
	text := ""
	switch r.Status {
	case "succeeded":
		text = "生成完成，已保存 1 张图片"
	case "failed":
		text = "图片生成失败"
	case "unknown":
		text = "生成结果待查询"
	case "cancelled":
		text = "已取消等待生成"
	default:
		return nil
	}
	if r.UpdatedAt == "" {
		r.UpdatedAt = timestamp()
	}
	// One durable receipt per run status, including recovery after a UI restart.
	o := Operation{ID: "run/" + r.ID + "/" + r.Status, ProjectID: r.ProjectID, Action: "generation", Text: text, Detail: r.Error, Nodes: []string{r.NodeID}, At: r.UpdatedAt, UpdatedAt: r.UpdatedAt}
	return &o
}
