package main

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
)

func logTime(raw string) string {
	t, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		return ""
	}
	return t.In(time.Local).Format("01/02 15:04")
}
func (a *desktop) loadOperations(e *Editor) {
	a.summaryError = ""
	a.logSummary = OperationSummary{}
	if a.store == nil {
		return
	}
	events, err := a.store.historyOperationPage(e.Project.ID, Operation{})
	if err != nil {
		a.message(err.Error())
		return
	}
	if a.logExhausted == nil {
		a.logExhausted = map[string]bool{}
	}
	a.logExhausted[e.Project.ID] = len(events) < 500
	seen := make(map[string]bool, len(e.Events))
	for _, o := range e.Events {
		seen[o.ID] = true
	}
	for _, o := range events {
		if !seen[o.ID] {
			e.Events = append(e.Events, o)
		}
	}
	slices.SortFunc(e.Events, compareOperations)
	a.logSummary = a.store.LoadSummary(e.Project.ID)
	if a.logSummary.Scope == "" && a.logSummary.Fingerprint != eventFingerprint(summaryEvents(e.Events)) {
		a.logSummary = OperationSummary{}
	}
}
func compareOperations(x, y Operation) int {
	if order := strings.Compare(strings.TrimSuffix(x.At, "Z"), strings.TrimSuffix(y.At, "Z")); order != 0 {
		return order
	}
	return strings.Compare(x.ID, y.ID)
}
func (a *desktop) olderOperations() {
	if a.editor == nil || a.store == nil || len(a.editor.Events) == 0 {
		return
	}
	e := a.editor
	items, err := a.store.historyOperationPage(e.Project.ID, e.Events[0])
	if err != nil {
		a.message(err.Error())
		return
	}
	e.Events = append(items, e.Events...)
	if a.logExhausted == nil {
		a.logExhausted = map[string]bool{}
	}
	a.logExhausted[e.Project.ID] = len(items) < 500
}

type operationHistoryRow struct {
	day   string
	event *Operation
}

func operationHistoryRows(events []Operation, now time.Time) []operationHistoryRow {
	var rows []operationHistoryRow
	last := ""
	for i := len(events) - 1; i >= 0; i-- {
		op := &events[i]
		_, day := galleryDate(op.At)
		if last != day {
			last = day
			label := day
			switch day {
			case now.Format("2006-01-02"):
				label = "今天"
			case now.AddDate(0, 0, -1).Format("2006-01-02"):
				label = "昨天"
			}
			rows = append(rows, operationHistoryRow{day: label})
		}
		rows = append(rows, operationHistoryRow{event: op})
	}
	return rows
}

func (a *desktop) operationHistoryView(c *ui.Context) {
	if a.editor == nil {
		return
	}
	e := a.editor
	panel := ui.Column(c.Key("operations-"+e.Project.ID)).Label("操作记录列表").Grow(1).MinHeight(0).FillWidth().Clip().Padding(7, 13, 8, 13).Gap(6)
	list := ui.Local(panel, "records", func() ui.ListState {
		a.loadOperations(e)
		return ui.ListState{}
	})
	panel.Children(func() {
		ui.Row(c).FillWidth().Height(43).AlignItems(ui.Center).Gap(8).Children(func() {
			ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
				ui.Text(c, "操作历史").Role(ui.RoleHeading).Level(1).FontSize(15).FontWeight(600).SingleLine()
				flatTip(c, ui.Text(c, e.Project.Name).FontSize(11).TextColor(c.Theme().TextMuted).SingleLine(), e.Project.Name)
			})
			activityIcon(c, "spark", "总结最近操作", "正在整理操作日志", a.summaryBusy, !slices.ContainsFunc(e.Events, Operation.significant), a.summarizeOperations, "GPT-6 Luna · "+a.funding.Summary.label())
		})
		if a.summaryBusy {
			ui.Text(c, "Luna 正在整理最近操作…").FontSize(12).TextColor(c.Theme().TextMuted)
		}
		if a.summaryError != "" {
			ui.Text(c, a.summaryError).FontSize(12).TextColor(c.Theme().Danger).MaxLines(3)
		}
		if summary := a.logSummary; summary.ProjectID == e.Project.ID && summary.Text != "" {
			ui.Column(c).FillWidth().Padding(12).Gap(8).Radius(8).Background(c.Theme().Surface).Children(func() {
				ui.Text(c, summary.Text).FontSize(13).MaxLines(9)
				label := fmt.Sprintf("Luna · %s · %s–%s · %d 条", summary.Funding.label(), logTime(summary.From), logTime(summary.To), summary.Count)
				if len(e.Events) > 0 && summary.To != e.Events[len(e.Events)-1].UpdatedAt {
					label += " · 有新操作"
				}
				ui.Text(c, label).FontFeatures("tnum").FontSize(11).TextColor(c.Theme().TextMuted)
			})
		}
		if len(e.Events) == 0 {
			ui.Column(c).FillWidth().Grow(1).Center().Gap(8).Children(func() {
				emptyIcon(c, "log")
				ui.Text(c, "暂无操作记录").FontSize(13)

			})
			return
		}
		rows := operationHistoryRows(e.Events, time.Now())
		list.Key = func(i int) any {
			if row := rows[i]; row.event != nil {
				return row.event.ID
			}
			return "day/" + rows[i].day
		}
		list.Label = func(i int) string {
			if row := rows[i]; row.event != nil {
				return row.event.Text
			}
			return rows[i].day
		}
		thinScroll(c, ui.List(c.Key(e.Project.ID+"-operations"), list, len(rows), func(i int) {
			if row := rows[i]; row.event != nil {
				a.operationRow(c, e, *row.event)
			} else {
				timelineHeading(c, row.day)
			}
		}), nil).FillWidth().Grow(1).MinHeight(0)
		if a.store != nil && !a.logExhausted[e.Project.ID] {
			iconButton(c, "chevron-down", "加载更早操作", a.olderOperations)
		}
	})
}

func operationIcon(o Operation) string {
	switch o.Action {
	case "generation", "generate":
		return "spark"
	case "import":
		return "upload"
	case "create", "canvas":
		return "plus"
	case "trash", "purge":
		return "trash"
	case "undo", "restore":
		return "undo"
	case "redo":
		return "redo"
	case "move":
		return "grip"
	case "edit-text":
		return "type"
	case "edit", "rename":
		return "edit"
	case "copy", "paste":
		return "copy"
	case "connect", "disconnect":
		return "branch"
	default:
		return "history"
	}
}

// Resolve targets against the current graph. A historic receipt must never
// resurrect a card or redirect by an ambiguous display name.
type operationTarget struct {
	id, name, trashID, assetID string
	kind                       string
}

func (e *Editor) operationTargets(o Operation) []operationTarget {
	var targets []operationTarget
	for _, id := range o.Nodes {
		if n := e.Node(id); n != nil {
			targets = append(targets, operationTarget{id: id, name: n.Name, assetID: n.Asset, kind: n.Kind})
			continue
		}
		if e.Purged[id] {
			continue
		}
		for i := len(e.Trash) - 1; i >= 0; i-- {
			tr := &e.Trash[i]
			if tr.Node.ID == id {
				targets = append(targets, operationTarget{id: id, name: tr.Node.Name, trashID: tr.ID, assetID: tr.Display, kind: tr.Node.Kind})
				break
			}
		}
	}
	return targets
}

func (a *desktop) locateOperationTarget(e *Editor, target operationTarget) {
	if a.editor != e {
		return
	}
	if e.Node(target.id) != nil {
		a.openWorkspace("画布")
		clear(e.Selection)
		e.Selection[target.id] = true
		a.focusNodeID, a.fitNext, a.draftKey = target.id, false, ""
		a.canvas.Focus()
		return
	}
	if target.trashID == "" {
		return
	}
	a.openWorkspace("回收站")
	a.selectLibraryGroup("")
	for i, item := range a.visibleLibraryItems() {
		if item.projectID == e.Project.ID && item.trash != nil && item.trash.Node.ID == target.id {
			a.locatedTrashKey = item.key
			a.trashView.locateItem = i + 1
			return
		}
	}
}

func (a *desktop) operationRow(c *ui.Context, e *Editor, o Operation) {
	targets := e.operationTargets(o)
	_, windowHeight := c.Size()
	subject, tip := "", ""
	if len(targets) > 0 {
		subject = targets[0].name
		kind := "图片"
		if targets[0].kind == textNodeKind {
			kind = "文字"
		}
		tip = "定位" + kind + " · " + subject
		if targets[0].trashID != "" {
			tip = "定位到回收站 · " + subject
		}
		if len(targets) > 1 {
			subject = fmt.Sprintf("%s 等 %d 项", subject, len(targets))
			tip = "选择要定位的内容"
		}
	} else if len(o.Nodes) > 0 && e.Purged[o.Nodes[0]] {
		subject = "图片已永久删除"
	}
	body := func() {
		icon := operationIcon(o)
		drawn := false
		if len(targets) == 1 {
			if targets[0].kind == textNodeKind {
				icon = "type"
			}
			if asset, ok := e.Assets[targets[0].assetID]; ok {
				a.imageThumbnail(c, asset, 28, 0, windowHeight)
				drawn = true
			}
		}
		if !drawn {
			ui.Box(c).Size(28, 28).Shrink(0).Center().Radius(14).Background(c.Theme().Surface).Children(func() {
				uiIcon(c, icon, 14).TextColor(c.Theme().TextMuted)
			})
		}
		ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
			flatTip(c, ui.Text(c, o.Text).FontSize(12).FontWeight(500).SingleLine(), o.Text)
			if o.Detail != "" {
				flatTip(c, ui.Text(c, o.Detail).FontSize(11).TextColor(c.Theme().TextMuted).SingleLine(), o.Detail)
			} else if subject != "" {
				ui.Text(c, subject).FontSize(11).TextColor(c.Theme().TextMuted).SingleLine()
			}
		})
		if at, err := time.Parse(time.RFC3339Nano, o.At); err == nil {
			ui.Text(c, at.Local().Format("15:04")).FontFeatures("tnum").FontSize(11).TextColor(c.Theme().TextMuted)
		}
	}
	if len(targets) == 0 {
		ui.Row(c.Key(o.ID)).FillWidth().Height(48).Padding(0, 7).Gap(8).AlignItems(ui.Center).Children(body)
		return
	}
	row := ui.ButtonBase(c.Key(o.ID)).Label("定位："+o.Text).FillWidth().Height(48).Padding(0, 7).Gap(8).AlignItems(ui.Center).Radius(7)
	open := ui.Local(row, "targets", func() bool { return false })
	commandHint(c, feedback(c, row, ui.Transparent, c.Theme().Text), tip).Children(body).OnClick(func() {
		if len(targets) == 1 {
			a.locateOperationTarget(e, targets[0])
			return
		}
		*open = !*open
	})
	if len(targets) < 2 {
		return
	}
	ui.PopoverBase(c, row, open, func(panel ui.Element) {
		toolSurface(c, panel, 7).Background(c.Theme().Background).Padding(4).Width(240)
		list := ui.Local(panel, "target-list", func() ui.ListState { return ui.ListState{} })
		list.Key = func(i int) any { return targets[i].id }
		thinScroll(c, ui.List(c, list, len(targets), func(i int) {
			target := targets[i]
			where := "画布"
			if target.trashID != "" {
				where = "回收站"
			}
			b := ui.ButtonBase(c.Key(target.id)).Label("定位图片："+target.name).FillWidth().Height(30).Padding(0, 8).Gap(6).Radius(4).AlignItems(ui.Center)
			commandHint(c, feedback(c, b, ui.Transparent, c.Theme().Text), where+" · "+target.name).Children(func() {
				if asset, ok := e.Assets[target.assetID]; ok {
					a.imageThumbnail(c, asset, 22, 0, windowHeight)
				}
				ui.Text(c, target.name).Grow(1).MinWidth(0).SingleLine().FontSize(12)
				if target.trashID != "" {
					uiIcon(c, "trash", 13)
				}
			}).OnClick(func() { *open = false; a.locateOperationTarget(e, target) })
		}), nil).FillWidth().Height(float32(30 * min(7, len(targets))))
	})
}
