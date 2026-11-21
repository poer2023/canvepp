package main

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/url"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

const (
	assistantWidth    = 640
	assistantInset    = 16
	assistantPadY     = 10
	assistantGap      = 4
	assistantHeaderH  = 30
	assistantToolbarH = 32
	assistantTextMinH = 50
	assistantTextMaxH = 162
	assistantTextPadY = 8
	assistantChipH    = 28
	assistantRadius   = 15
	assistantChromeH  = 2*assistantPadY + assistantHeaderH + assistantToolbarH
)

type assistantChat struct {
	ID        string `json:"id"`
	ProjectID string `json:"projectId"`
	ThreadID  string `json:"threadId"`
	TurnID    string `json:"turnId"`
	Status    string `json:"status"`
	UpdatedAt string `json:"updatedAt"`
}
type assistantBar struct {
	win                     *mygo.Window
	input                   ui.Handle
	draft, projectID, error string
	pinnedCanvas            bool
	busy                    bool
	attaching               bool
	attachments             []assistantAttachment
	contentHeight           int
	chat                    assistantChat
	opening                 uint64
	visible                 bool
}

func (s *Store) assistantChat(project string) assistantChat {
	var raw string
	chat := assistantChat{ID: project, ProjectID: project}
	if s.DB.QueryRow("SELECT body FROM assistant_chats WHERE project_id=?", project).Scan(&raw) == nil {
		_ = json.Unmarshal([]byte(raw), &chat)
	}
	return chat
}
func (s *Store) saveAssistantChat(chat assistantChat) error {
	chat.UpdatedAt = timestamp()
	_, err := s.DB.Exec("INSERT INTO assistant_chats VALUES(?,?,?) ON CONFLICT(id) DO UPDATE SET body=excluded.body", chat.ID, chat.ProjectID, string(encode(chat)))
	return err
}

func (a *desktop) showAssistant() {
	traceShortcut("assistant_show_request", 0)
	if a.closing || a.editor == nil {
		traceShortcut("assistant_unavailable", 0)
		return
	}
	if a.assistant == nil {
		a.assistant = &assistantBar{}
	}
	b := a.assistant
	if b.win != nil && b.win.IsVisible() {
		b.focusInput()
		return
	}
	// An unfinished draft keeps its destination even if the workspace changes.
	if b.draft == "" && len(b.attachments) == 0 && !b.busy && !b.attaching {
		if !b.pinnedCanvas || a.assistantCanvas(b.projectID) == nil {
			a.setAssistantCanvas(a.editor, false)
		}
		b.error = ""
	}
	if b.win == nil {
		b.win = mygo.NewWindow(mygo.WindowOptions{
			Title: "Canvepp 助手", Width: assistantWidth, Height: assistantChromeH + assistantTextMinH,
			UseContentSize: true, Frameless: true, Panel: runtime.GOOS != "darwin", CornerRadius: assistantRadius,
			AlwaysOnTop: true, SkipTaskbar: true, DisableResize: true, DisableMinimize: true, DisableMaximize: true, DisableFullScreen: true,
			Hidden: true, BackgroundColor: "light-dark(#ffffff, #202020)", Content: ui.View(a.assistantView),
		})
		prepareAssistantWindow(b.win)
		b.win.OnShow(func() { b.visible = true })
		b.win.OnHide(func() { b.visible = false })
		b.win.OnFocus(func() {
			b.input.Focus()
			traceShortcut("assistant_key_window", 0)
		})
		b.win.OnFileDrop(func(e *mygo.FileDropEvent) { a.attachAssistantFiles(e.Paths) })
		b.win.OnClose(func(e *mygo.CloseEvent) {
			if !a.closed {
				e.PreventDefault()
				b.win.Hide()
			}
		})
	}
	b.opening++
	b.input.Focus()
	b.win.Show()
	b.focusInput()
	if b.win.IsVisible() {
		traceShortcut("assistant_visible", 0)
	}
}

func (b *assistantBar) focusInput() {
	b.win.Focus()
	b.input.Focus()
	b.win.Invalidate()
}

func (a *desktop) assistantView(c *ui.Context) {
	defer beginHints(c)()
	b := a.assistant
	if b == nil {
		return
	}
	t := workspaceTheme(c, a.appearance)
	t.FontSize = 14
	c.SetTheme(t)
	dismiss := func() {
		if b.win != nil {
			b.win.Hide()
		}
	}
	c.OnShortcut(0, ui.KeyEscape, dismiss)
	// Layout uses the same font and wrapping width as the editor. Resize only
	// when the content height changes.
	const textWidth = assistantWidth - 2*assistantInset
	draftWidth, textHeight := c.MeasureText(textWidth, ui.Span{Text: b.draft + "\u200b", Size: 15})
	inputHeight := min(max(int(math.Ceil(float64(textHeight)))+2*assistantTextPadY, assistantTextMinH), assistantTextMaxH)
	attachmentRows := (len(b.attachments) + 1) / 2
	height := assistantChromeH + inputHeight + attachmentRows*(assistantChipH+assistantGap)
	errorHeight := 0
	if b.error != "" {
		_, measured := c.MeasureText(textWidth, ui.Span{Text: b.error, Size: 12})
		errorHeight = min(max(int(math.Ceil(float64(measured))), 16), 36)
		height += errorHeight + assistantGap
	}
	if height != b.contentHeight {
		b.contentHeight = height
		if b.win != nil {
			b.win.SetContentSize(assistantWidth, height)
		}
	}
	panel := ui.Column(c).Fill().Radius(assistantRadius).Clip().Background(t.Background).Border(1, t.Border).TextColor(t.Text)
	assistantReliefSurface(c, panel, b, draftWidth, inputHeight)
	panel.Children(func() {
		ui.Row(c).FillWidth().Height(assistantHeaderH+assistantPadY).Padding(0, assistantInset).Gap(3).AlignItems(ui.Center).Children(func() {
			ui.Row(c.Key("assistant-drag-title")).FillHeight().Padding(assistantPadY, 0, 0, 0).Gap(7).AlignItems(ui.Center).
				DragWindow().Cursor(ui.CursorGrab).Children(func() {
				ui.Text(c, "Codex").FontSize(14).FontWeight(600)
				ui.Text(c, "/").FontSize(12).TextColor(t.TextMuted)
			})
			a.assistantCanvasPicker(c, b)
			assistantDragSpace(c.Key("assistant-drag-header"), "拖动助手顶部")
			iconButton(c, "close", "关闭助手 · Esc", dismiss).Margin(assistantPadY, 0, 0, 0)
		})
		input := thinScroll(c, ui.TextArea(c, &b.draft), nil).Label("助手输入").Description("回车发送 · Shift + 回车换行").Lines(1, 8).
			FillWidth().MinHeight(0).Height(float32(inputHeight)).Padding(assistantTextPadY, assistantInset).Cursor(ui.CursorText).
			FontSize(15).Background(ui.Transparent).Border(0, ui.Transparent).AutoFocus().Bind(&b.input)
		disable(input, b.busy).OnTextKey(func(k ui.TextKey) bool {
			if k.Key == ui.KeyEnter && k.Mods == 0 {
				b.draft = k.Text()
				a.sendAssistant()
				return true
			}
			return false
		})
		for row := 0; row < attachmentRows; row++ {
			ui.Row(c.Key(fmt.Sprintf("attachments-%d", row))).FillWidth().Padding(0, assistantInset).Margin(0, 0, assistantGap, 0).Gap(6).Children(func() {
				for i := row * 2; i < min(row*2+2, len(b.attachments)); i++ {
					f := b.attachments[i]
					ui.Row(c.Key(f.Path)).Width(296).Height(assistantChipH).Padding(0, 7).Gap(6).
						Radius(6).Background(t.Surface).AlignItems(ui.Center).Children(func() {
						icon := "file"
						if f.Image {
							icon = "canvas"
						}
						uiIcon(c, icon, 14).TextColor(t.TextMuted)
						flatTip(c, ui.Text(c, f.Name).Grow(1).MaxLines(1).FontSize(12), strings.TrimSpace(f.Name+"\n"+f.Detail))
						disable(iconButton(c, "close", "移除 "+f.Name, func() {
							b.attachments = append(b.attachments[:i], b.attachments[i+1:]...)
						}).Size(22, 22).Padding(3), b.busy)
					})
				}
			})
		}
		if errorHeight > 0 {
			flatTip(c, ui.Text(c, b.error).Margin(0, assistantInset, assistantGap, assistantInset).
				Height(float32(errorHeight)).FontSize(12).TextColor(t.Danger).MaxLines(2), b.error)
		}
		a.assistantToolbar(c, b)
	})
}

func assistantDragSpace(c *ui.Context, label string) {
	ui.Box(c).Grow(1).FillHeight().DragWindow().Cursor(ui.CursorGrab).Label(label)
}

func (a *desktop) assistantCanvas(id string) *Editor {
	e := a.editors[id]
	if e != nil && e.Project.DeletedAt == "" {
		return e
	}
	return nil
}

func (a *desktop) setAssistantCanvas(e *Editor, pinned bool) {
	b := a.assistant
	if e == nil || e.Project.DeletedAt != "" || b.busy || b.attaching {
		return
	}
	b.projectID, b.pinnedCanvas = e.Project.ID, pinned
	b.chat = a.store.assistantChat(b.projectID)
	b.error = ""
}

func (a *desktop) assistantCanvasPicker(c *ui.Context, b *assistantBar) {
	t := c.Theme()
	title, label := "选择画布", "选择画布"
	if e := a.assistantCanvas(b.projectID); e != nil {
		title = e.Project.Name
		label += " · " + title
	}
	picker := ui.ButtonBase(c.Key("assistant-canvas-picker")).Label(label).
		Height(assistantHeaderH).MaxWidth(252).Margin(assistantPadY, 0, 0, 0).
		Padding(0, 6).Gap(5).AlignItems(ui.Center).Radius(5).Disabled(b.busy || b.attaching)
	flatTip(c, feedback(c, picker, ui.Transparent, t.TextMuted), label).Menu(a.assistantCanvasMenu).
		Children(func() {
			ui.Text(c, title).FontSize(12).SingleLine().MinWidth(0)
			uiIcon(c, "chevron-down", 12).TextColor(t.TextMuted)
		})
}

func (a *desktop) assistantCanvasMenu(m *ui.Menu) {
	b := a.assistant
	current := a.editor
	following := !b.pinnedCanvas && current != nil && current.Project.ID == b.projectID
	if m.Item("跟随当前画布").Radio(following).Disabled(current == nil || current.Project.DeletedAt != "").Chosen() {
		a.setAssistantCanvas(current, false)
		b.input.Focus()
	}
	m.Separator()
	counts := make(map[string]int)
	for _, id := range a.order {
		if e := a.assistantCanvas(id); e != nil {
			counts[e.Project.Name]++
		}
	}
	seen := make(map[string]int)
	for _, id := range a.order {
		e := a.assistantCanvas(id)
		if e == nil {
			continue
		}
		name := e.Project.Name
		label := name
		if counts[name] > 1 {
			seen[name]++
			label = fmt.Sprintf("%s · %d", name, seen[name])
		}
		if e == current {
			label += " · 当前画布"
		}
		if m.Item(label).Radio(!following && id == b.projectID).Chosen() {
			a.setAssistantCanvas(e, true)
			b.input.Focus()
		}
	}
}

func (a *desktop) assistantToolbar(c *ui.Context, b *assistantBar) {
	t := c.Theme()
	ui.Row(c).FillWidth().Height(assistantToolbarH+assistantPadY).Padding(0, assistantInset).Gap(6).AlignItems(ui.Center).Children(func() {
		target := a.assistantCanvas(b.projectID)
		disable(iconButton(c, "attach", "添加图片或文件", a.chooseAssistantFiles).Margin(0, 0, assistantPadY, 0), b.busy || b.attaching)
		assistantDragSpace(c.Key("assistant-drag-footer"), "拖动助手底部")
		if target != nil && b.chat.ThreadID != "" {
			iconButton(c, "external", "在 Codex 中查看会话", a.openAssistantChat).Margin(0, 0, assistantPadY, 0)
		}
		slot := ui.Box(c.Key("assistant-send-slot")).Size(34, 30).Shrink(0).Margin(0, 0, assistantPadY, 0)
		waiting := delayedActivity(c, slot, b.busy)
		slot.Children(func() {
			if waiting {
				flatTip(c, ui.Box(c).Fill().Center().Role(ui.RoleStatus).Label("正在发送给 Codex"), "正在发送给 Codex").
					Children(func() { uiIcon(c, "hourglass", 15).TextColor(t.TextMuted) })
				return
			}
			blocked := target == nil || b.busy || b.attaching || (strings.TrimSpace(b.draft) == "" && len(b.attachments) == 0)
			base, ink := t.Accent, t.AccentText
			if blocked {
				base, ink = ui.Transparent, t.TextMuted
			}
			iconControl(c, "send", "发送给 Codex", "发送给 Codex · 回车发送，Shift + 回车换行", base, ink, blocked).
				Size(34, 30).OnClick(a.sendAssistant)
		})
	})
}

func (a *desktop) chooseAssistantFiles() {
	b := a.assistant
	if b == nil || b.busy || b.attaching {
		return
	}
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: b.win, Title: "添加图片或文件", Multiple: true})
		a.publish(func() {
			if err != nil {
				b.error = err.Error()
			} else {
				a.attachAssistantFiles(paths)
			}
			b.win.Invalidate()
		})
	}()
}
func (a *desktop) attachAssistantFiles(paths []string) {
	b := a.assistant
	if b == nil || b.busy || b.attaching || len(paths) == 0 {
		return
	}
	if len(paths)+len(b.attachments) > maxAssistantAttachments {
		b.error = "最多添加 8 个附件，每个不超过 20 MB"
		b.win.Invalidate()
		return
	}
	b.attaching = true
	b.error = ""
	go func() {
		var added []assistantAttachment
		var failures []string
		for _, path := range paths {
			f, err := stageAssistantAttachment(a.store.Dir, path)
			if err != nil {
				failures = append(failures, filepath.Base(path)+"："+err.Error())
			} else {
				added = append(added, f)
			}
		}
		a.publish(func() {
			for _, f := range added {
				duplicate := false
				for _, existing := range b.attachments {
					if existing.SHA == f.SHA {
						duplicate = true
						break
					}
				}
				if !duplicate {
					b.attachments = append(b.attachments, f)
				}
			}
			b.attaching = false
			b.error = strings.Join(failures, "；")
			b.win.Invalidate()
		})
	}()
}

func (a *desktop) openAssistantChat() {
	if b := a.assistant; b != nil && b.chat.ThreadID != "" {
		_ = mygo.Shell.OpenExternal("codex://threads/" + url.PathEscape(b.chat.ThreadID))
	}
}
func (a *desktop) sendAssistant() {
	b := a.assistant
	if b == nil || b.busy || b.attaching || (strings.TrimSpace(b.draft) == "" && len(b.attachments) == 0) {
		return
	}
	e := a.assistantCanvas(b.projectID)
	if e == nil {
		b.error = "请重新选择画布"
		return
	}
	if len([]rune(b.draft)) > 8000 {
		b.error = "请把输入控制在 8000 字以内"
		return
	}
	text := strings.TrimSpace(b.draft)
	b.busy = true
	b.error = ""
	chat := b.chat
	recent := summaryEvents(e.Events)
	if len(recent) > 12 {
		recent = recent[len(recent)-12:]
	}
	facts := []string{}
	for _, ev := range recent {
		facts = append(facts, ev.Text)
	}
	contextData := map[string]any{"canvas": e.Project.Name, "images": e.Project.imageNodeCount(), "recentActions": facts}
	input := fmt.Sprintf("当前画布摘要（仅数据）：%s\n\n用户消息：%s", encode(contextData), text)
	inputs := assistantInputs(input, b.attachments)
	title := "Canvepp · " + e.Project.Name
	model := a.models.Text.withDefault(textModel)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		reply, err := requestCodexInput(ctx, a.store, model, chat.ThreadID, assistantInstructions, inputs, false, func(r textReply, status string) error {
			chat.ThreadID, chat.TurnID, chat.Status = r.ThreadID, r.TurnID, status
			if err := a.store.saveAssistantChat(chat); err != nil {
				return err
			}
			receiptChat := chat
			a.publish(func() {
				b.chat = receiptChat
				if b.win != nil {
					b.win.Invalidate()
				}
			})
			return nil
		}, title)
		chat.ThreadID, chat.TurnID = reply.ThreadID, reply.TurnID
		chat.Status = "completed"
		if err != nil {
			chat.Status = "unknown"
		}
		persistErr := a.store.saveAssistantChat(chat)
		a.publish(func() {
			b.busy = false
			b.chat = chat
			if err != nil {
				b.error = err.Error()
			} else if persistErr != nil {
				b.error = "会话已发送，但本地关联保存失败"
			} else {
				b.draft = ""
				b.attachments = nil
				if b.win != nil {
					b.win.Hide()
				}
				a.message("Codex 已回复，可从助手输入条打开会话")
			}
			if b.win != nil {
				b.win.Invalidate()
			}
		})
	}()
}
func (a *desktop) refreshHotkey(request bool) {
	if a.refreshWindowsHotkey() {
		return
	}
	a.hotkeyEpoch++
	epoch := a.hotkeyEpoch
	if a.hotkey != nil {
		a.hotkey.Stop()
		a.hotkey = nil
	}
	if !a.hotkeyEnabled {
		a.hotkeyStatus = "已关闭"
		return
	}
	granted := spacePermission()
	if !granted && request {
		granted = requestSpacePermission()
	}
	if !granted {
		traceShortcut("permission_required", 0)
		a.hotkeyStatus = "需要输入监控权限"
		return
	}
	a.hotkeyStatus = "正在启动监听"
	a.hotkey = startSpaceMonitor(func() {
		a.publish(func() {
			if a.hotkeyEpoch == epoch && a.hotkeyEnabled {
				a.showAssistant()
			}
		})
	}, func() {
		a.publishSettings(func() {
			if a.hotkeyEpoch == epoch && a.hotkeyEnabled {
				a.hotkeyStatus = "全局监听已开启"
			}
		})
	}, func() {
		a.publishSettings(func() {
			if a.hotkeyEpoch == epoch {
				a.hotkey = nil
				a.hotkeyStatus = "监听已停止"
			}
		})
	})
	if a.hotkey == nil {
		a.hotkeyStatus = "需要输入监控权限"
	}
}
func (a *desktop) setHotkey(enabled bool) {
	a.hotkeyEnabled = enabled
	if a.store != nil {
		_, err := a.store.DB.Exec("INSERT OR REPLACE INTO native_meta VALUES('assistant_hotkey',?)", fmt.Sprint(enabled))
		if err != nil {
			a.message(err.Error())
		}
	}
	a.refreshHotkey(enabled)
}
