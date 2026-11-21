package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

type saveSnapshot struct {
	Project Project
	Trash   []Trash
	Events  []Operation
}
type saveQueue struct {
	store     *Store
	mu        sync.Mutex
	cond      *sync.Cond
	pending   map[string]saveSnapshot
	pendingUI *panelPlacement
	active    bool
	errors    map[string]error
	wake      chan struct{}
	report    func(string, int, error)
}

func newSaveQueue(s *Store, report func(string, int, error)) *saveQueue {
	q := &saveQueue{store: s, pending: map[string]saveSnapshot{}, errors: map[string]error{}, wake: make(chan struct{}, 1), report: report}
	q.cond = sync.NewCond(&q.mu)
	go q.loop()
	return q
}
func (q *saveQueue) Put(e *Editor) {
	p := e.Project
	p.Graph = cloneGraph(p.Graph)
	p.Graph.View = e.View
	q.mu.Lock()
	q.pending[p.ID] = saveSnapshot{p, slices.Clone(e.Trash), slices.Clone(e.Events)}
	q.mu.Unlock()
	select {
	case q.wake <- struct{}{}:
	default:
	}
}
func (q *saveQueue) PutUI(p panelPlacement) {
	q.mu.Lock()
	q.pendingUI = &p
	q.mu.Unlock()
	select {
	case q.wake <- struct{}{}:
	default:
	}
}
func (q *saveQueue) loop() {
	for range q.wake {
		for {
			q.mu.Lock()
			var next saveSnapshot
			for id, s := range q.pending {
				next = s
				delete(q.pending, id)
				break
			}
			if next.Project.ID == "" {
				if q.pendingUI != nil {
					p := *q.pendingUI
					q.pendingUI, q.active = nil, true
					q.mu.Unlock()
					err := q.store.savePanel(p)
					q.mu.Lock()
					if err != nil {
						q.errors["image_panel"] = err
					} else {
						delete(q.errors, "image_panel")
					}
					q.mu.Unlock()
					if err != nil {
						q.report("", 0, err)
					}
					continue
				}
				q.active = false
				q.cond.Broadcast()
				q.mu.Unlock()
				break
			}
			q.active = true
			q.mu.Unlock()
			err := q.store.SaveWithOperations(next.Project, next.Trash, next.Events)
			q.mu.Lock()
			if err == nil {
				delete(q.errors, next.Project.ID)
			} else {
				q.errors[next.Project.ID] = err
			}
			q.mu.Unlock()
			q.report(next.Project.ID, next.Project.Revision, err)
		}
	}
}
func (q *saveQueue) Flush() error {
	q.mu.Lock()
	defer q.mu.Unlock()
	for len(q.pending) > 0 || q.pendingUI != nil || q.active {
		q.cond.Wait()
	}
	for _, err := range q.errors {
		return err
	}
	return nil
}

type desktop struct {
	store                  *Store
	win                    *mygo.Window
	images                 *ImageCache
	saves                  *saveQueue
	editors                map[string]*Editor
	order                  []string
	editor                 *Editor
	libraryKey             string
	libraryCache           []libraryEntry
	libraryGroups          []libraryGroup
	libraryList            ui.ListState
	groupList              ui.ListState
	historyView, trashView galleryViewport
	libraryVisible         []libraryEntry
	libraryFilterKey       string
	appearance             string
	frosted                bool
	rail                   sidebarLayout
	railVisualWidth        float32
	railHandle             ui.Handle
	railDragging           bool
	railGrabOffset         float32
	canvasSort             int
	canvasIndex            canvasDirectoryIndex
	groupOpen              bool
	canvasQuery            string
	galleryQuery           string
	canvasSearch           searchControl
	gallerySearch          searchControl
	galleryGrouping        int
	gallerySort            int
	trashSort              int
	galleryGroup           string
	trashGroup             string
	deleteSessionOpen      bool
	deleteSessionID        string
	searchResults          ui.ListState
	dock                   dockLayout
	adobeBusy              bool
	closeAfterAdobe        bool
	exitFromTray           bool
	adobeShortcutError     string
	renameOpen             bool
	renameProjectID        string
	renameDraft            string
	runRevision            int
	canvasGeneration       *canvasGenerationIndex
	imageReveals           map[string]imageReveal
	legacyPrompts          map[string]string
	legacyRecords          []imageRecord
	lineage                *imageLineage
	imageDrawer            string
	creationEditing        bool
	creationRecordID       string
	creationList           ui.ListState
	drawerContext          string
	reusePending           *imageRecord
	runs                   []NativeRun
	running                map[string]bool
	parallelGeneration     bool
	generationSlots        chan struct{}
	left                   string
	panel                  panelPlacement
	panelGrab              ui.Point
	panelDragStart         ui.Point
	panelHandle            ui.Handle
	panelDragging          bool
	settingsOpen           bool
	settingsWin            *mygo.Window
	settingsBounds         mygo.Rectangle
	backupSettings         settingsTask
	importSettings         settingsTask
	restoreSettings        settingsTask
	openLibrarySettings    settingsTask
	overview               overviewState
	noticeOverview         overviewState
	generationNotices      generationNoticeTracker
	textEdit               *canvasTextEdit
	textPlacing            bool
	textLayouts            map[string]*ui.TextLayout
	canvasTimeline         canvasTimeline
	explorerPreview        *explorerPreview
	logExhausted           map[string]bool
	logSummary             OperationSummary
	summaryBusy            bool
	summaryError           string
	settingsTab            int
	assistant              *assistantBar
	materials              *materialsPanel
	hotkey                 *spaceMonitor
	hotkeyEnabled          bool
	hotkeyStatus           string
	hotkeyEpoch            uint64
	localSpace             tripleSpace

	purgeOpen          bool
	purgeBusy          bool
	purgeKey           string
	purgeAnchorDrawn   bool
	status             string
	notice             string
	noticeUntil        time.Time
	prompt             string
	draftKey           string
	draftSelectedCount int
	draftEditor        *Editor
	draftSelection     []string
	width, height      float64
	count              int
	ratio              int
	ratioMenuOpen      bool
	canvas             ui.Handle
	hover              string
	pointer            Point
	start              Point
	panStart           Viewport
	gesture            string
	space              bool
	selectionRect      *Rect
	addSelection       bool
	canvasW, canvasH   float64
	fitNext            bool
	focusNodeID        string
	locatedTrashKey    string
	undoDeleteEditor   *Editor
	undoDeleteRevision int
	previewOpen        bool
	previewAsset       Asset
	previewView        Viewport
	previewDrag        Point
	previewPanning     bool
	funding            fundingSettings
	models             modelSettings
	chatGPTAccount     *chatGPTAccount
	loginError         string
	loginCode          string
	cancelLogin        context.CancelFunc
	accountBusy        bool
	closing            bool
	closed             bool
	frameTimes         []int64
	frameHead          int
	paintCount         uint64
	motionTrace        *motionTrace
}

func newDesktop(s *Store) (*desktop, error) {
	a := &desktop{store: s, editors: map[string]*Editor{}, running: map[string]bool{}, left: "画布", status: "已保存", width: 1024, height: 1024, count: 1, frameTimes: make([]int64, 2048)}
	if err := a.funding.load(s); err != nil {
		return nil, err
	}
	if err := a.models.load(s); err != nil {
		return nil, err
	}
	a.panel = s.loadPanel()
	a.rail = s.loadSidebar()
	_ = s.DB.QueryRow("SELECT value FROM native_meta WHERE key='dock_fraction'").Scan(&a.dock.fraction)
	_ = s.DB.QueryRow("SELECT value FROM native_meta WHERE key='appearance'").Scan(&a.appearance)
	_ = s.DB.QueryRow("SELECT value FROM native_meta WHERE key='frosted_appearance'").Scan(&a.frosted)
	_ = s.DB.QueryRow("SELECT value FROM native_meta WHERE key='parallel_generation'").Scan(&a.parallelGeneration)
	if a.appearance != "light" && a.appearance != "dark" {
		a.appearance = ""
	}
	if err := a.reload(); err != nil {
		return nil, err
	}
	if err := a.canvasTimeline.loadVisits(s); err != nil {
		return nil, err
	}
	for i := range a.runs {
		r := &a.runs[i]
		if r.Status == "queued" || r.Status == "preparing" || r.Status == "submitted" || r.Status == "running" {
			if r.Funding == apiFunding && r.Status == "submitted" || r.TemporaryThread {
				r.Status, r.Error = "unknown", apiUncertain
				if r.TemporaryThread {
					r.Error = "临时会话已停止，可查询已写入本机的结果"
				}
				if e := a.editors[r.ProjectID]; e != nil {
					for _, img := range e.Assets {
						if img.RunID == r.ID {
							r.Status, r.Error, r.Result = "succeeded", "", img.ID
							if reconcileResults(e, []NativeRun{*r}) {
								e.Project.Revision++
								if err := s.Save(e.Project, e.Trash); err != nil {
									return nil, err
								}
							}
							break
						}
					}
				}
			} else if r.ThreadID == "" {
				r.Status, r.Error = "failed", "上次提交未完成，尚未开始生图"
			} else {
				r.Status, r.Error = "unknown", "可查询原任务结果"
			}
			if err := s.SaveRun(*r); err != nil {
				return nil, err
			}
		}
	}
	var active string
	if s.DB.QueryRow("SELECT value FROM native_meta WHERE key='active_project'").Scan(&active) == nil && a.editors[active] != nil && a.editors[active].Project.DeletedAt == "" {
		a.editor = a.editors[active]
		a.fitNext = false
	} else {
		for _, e := range a.editors {
			if e.Project.DeletedAt == "" && (a.editor == nil || len(e.Project.Graph.Nodes) > len(a.editor.Project.Graph.Nodes)) {
				a.editor = e
			}
		}
	}
	return a, nil
}
func (a *desktop) reload() error {
	projects, err := a.store.Projects()
	if err != nil {
		return err
	}
	for _, p := range projects {
		if a.editors[p.ID] != nil {
			continue
		}
		assets, err := a.store.Assets(p.ID)
		if err != nil {
			return err
		}
		trash, err := a.store.Trashed(p.ID)
		if err != nil {
			return err
		}
		e := newEditor(p, assets, trash)
		e.Events, err = a.store.historyOperationPage(p.ID, Operation{})
		if err != nil {
			return err
		}
		e.Purged, err = a.store.PurgedNodes(p.ID)
		if err != nil {
			return err
		}
		e.OnChange = func() { a.changed(e) }
		a.editors[p.ID] = e
		a.order = append(a.order, p.ID)
	}
	if a.editor == nil {
		for _, id := range a.order {
			if a.editors[id].Project.DeletedAt == "" {
				a.editor = a.editors[id]
				a.fitNext = true
				break
			}
		}
	}
	a.runs, err = a.store.NativeRuns()
	if err != nil {
		return err
	}
	a.runRevision++
	a.legacyPrompts, err = a.store.LegacyPrompts()
	if err != nil {
		return err
	}
	a.legacyRecords, err = a.store.ImageRecords()
	if err != nil {
		return err
	}
	a.lineage = nil
	for _, e := range a.editors {
		if reconcileResults(e, a.runs) {
			e.Project.Revision++
			if err = a.store.Save(e.Project, e.Trash); err != nil {
				return err
			}
		}
	}
	return nil
}
func (a *desktop) attach(win *mygo.Window) {
	a.win = win
	a.images = newImageCache(a.store, a.publish)
	a.saves = newSaveQueue(a.store, func(id string, rev int, err error) {
		a.publish(func() {
			a.onCanvasSaved(id, rev, err)
		})
	})
	if a.editor == nil {
		a.createCanvas()
	}
	// NewWindow can construct its first view before the async image cache is
	// attached. Schedule a fresh frame so cold start needs no mouse interaction.
	var hotkey string
	_ = a.store.DB.QueryRow("SELECT value FROM native_meta WHERE key='assistant_hotkey'").Scan(&hotkey)
	a.hotkeyEnabled = hotkey != "false"
	a.refreshHotkey(false)
	win.Invalidate()
}
func (a *desktop) onCanvasSaved(id string, rev int, err error) {
	e := a.editors[id]
	if e == nil {
		if err != nil {
			a.message(err.Error())
		}
		return
	}
	e.retryingSave = false
	if err != nil {
		e.saveError = err.Error()
		if e != a.editor || a.left != "画布" || a.settingsOpen {
			a.message("「" + e.Project.Name + "」未保存")
		}
		return
	}
	if rev == e.Project.Revision {
		e.dirty = false
		e.saveError = ""
		a.status = "已保存"
	}
}

func (a *desktop) publish(fn func()) {
	if a.win != nil {
		a.win.Update(fn)
	}
}
func (a *desktop) changed(e *Editor) {
	a.status = "保存中…"
	if a.saves != nil {
		a.saves.Put(e)
	}
}
func (a *desktop) message(s string) {
	a.undoDeleteEditor = nil
	a.notice = s
	a.noticeUntil = time.Now().Add(7 * time.Second)
}

func (a *desktop) moveSelectionToTrash() {
	e := a.editor
	if e == nil {
		return
	}
	nodes := e.Selected()
	if len(nodes) == 0 {
		return
	}
	textCount := 0
	for _, n := range nodes {
		if n.Kind == textNodeKind {
			textCount++
		}
	}
	e.Delete()
	a.draftKey, a.hover = "", ""
	a.message("已将 " + nodeQuantity(len(nodes), textCount) + "移入回收站")
	a.undoDeleteEditor, a.undoDeleteRevision = e, e.Project.Revision
}

func (a *desktop) canUndoDelete() bool {
	e := a.undoDeleteEditor
	return e != nil && e == a.editor && e.Project.Revision == a.undoDeleteRevision && len(e.undo) > 0 && time.Now().Before(a.noticeUntil)
}
func (a *desktop) undoDelete() {
	if a.canUndoDelete() {
		a.editor.Undo()
		a.draftKey = ""
		a.message("已恢复选中内容")
	}
}
func (a *desktop) createCanvas() {
	a.commitCanvasText()
	a.textPlacing = false
	a.closeSettings()
	a.left = "画布"
	a.canvasQuery = ""
	a.fitNext, a.focusNodeID, a.hover = false, "", ""
	p := Project{ID: uuid(), Name: "未命名画布", SchemaVersion: 1, CreatedAt: timestamp(), UpdatedAt: timestamp(), Graph: Graph{Nodes: []Node{}, Edges: []Edge{}}}
	e := newEditor(p, nil, nil)
	e.OnChange = func() { a.changed(e) }
	a.editors[p.ID] = e
	a.order = append(a.order, p.ID)
	a.editor = e
	a.draftKey = ""
	a.changed(e)
	a.rememberCanvasOpen(e)
}
func (a *desktop) addNode(at Point) {
	a.closeSettings()
	if a.editor == nil {
		return
	}
	if len(a.editor.Project.Graph.Nodes) >= a.editor.Project.nodeLimit() {
		a.message(fmt.Sprintf("此画布最多容纳 %d 张图片", a.editor.Project.nodeLimit()))
		return
	}
	a.editor.Add([]Node{newNode(a.editor.Project.ID, at)}, nil)
	a.draftKey = ""
	a.canvas.Focus()
}
func (a *desktop) center() Point {
	if a.editor == nil {
		return Point{48, 48}
	}
	return a.editor.World(Point{a.canvasW * .45, a.canvasH * .4})
}
func (a *desktop) syncDraft() {
	if a.editor == nil {
		return
	}
	// A pointer frame must not clone hundreds of selected cards and concatenate
	// their IDs again. Selection identities change only at selection boundaries;
	// read the current node by ID when displaying its content.
	unchanged := a.draftKey != "" && a.draftEditor == a.editor && a.draftSelectedCount == len(a.editor.Selection)
	if unchanged {
		for _, id := range a.draftSelection {
			if !a.editor.Selection[id] {
				unchanged = false
				break
			}
		}
	}
	if unchanged {
		return
	}
	a.draftSelection = a.draftSelection[:0]
	for _, n := range a.editor.Project.Graph.Nodes {
		if a.editor.Selection[n.ID] && n.Kind != textNodeKind {
			a.draftSelection = append(a.draftSelection, n.ID)
		}
	}
	a.draftEditor, a.draftKey = a.editor, "synced"
	a.draftSelectedCount = len(a.editor.Selection)
	a.drawerContext = ""
	a.ratioMenuOpen = false
	if len(a.draftSelection) > 0 {
		a.panel.Open = true
	}
	a.prompt = ""
	a.width, a.height = 1024, 1024
	a.count = 1
	a.ratio = 0
	if len(a.draftSelection) == 1 {
		n := a.editor.Node(a.draftSelection[0])
		a.prompt = n.Config.Text
		fmt.Sscanf(n.Config.Params.Size, "%fx%f", &a.width, &a.height)
		a.count = max(1, min(4, n.Config.Params.N))
		a.ratio = ratioFor(a.width, a.height)
	}
}
func (a *desktop) updateDraft() {
	if a.editor == nil {
		return
	}
	nodes := a.editor.selectedImages()
	if len(nodes) != 1 {
		return
	}
	n := nodes[0]
	n.Config.Text = a.prompt
	n.Config.Params = Params{fmt.Sprintf("%.0fx%.0f", a.width, a.height), "high", a.count}
	n.Config.CreationStarted = true
	a.editor.Edit(n)
}
func (a *desktop) importFiles(paths []string, at Point) {
	if a.editor == nil || len(paths) == 0 {
		return
	}
	e := a.editor
	if len(e.Project.Graph.Nodes)+len(paths) > e.Project.nodeLimit() {
		a.message("画布图片数量超过限制")
		return
	}
	a.message("正在导入图片…")
	go func() {
		nodes := []Node{}
		assets := []Asset{}
		errs := []string{}
		for i, path := range paths {
			b, err := readBounded(path, maxImageBytes)
			if err != nil {
				errs = append(errs, filepath.Base(path))
				continue
			}
			n := newNode(e.Project.ID, Point{at.X + float64(i%4)*280, at.Y + float64(i/4)*320})
			asset, err := a.store.ImportImage(b, e.Project.ID, n.ID, filepath.Base(path), "import", "", []string{})
			if err != nil {
				errs = append(errs, filepath.Base(path))
				continue
			}
			n.Name = asset.Name
			n.Asset = asset.ID
			nodes = append(nodes, n)
			assets = append(assets, asset)
		}
		a.publish(func() {
			for _, asset := range assets {
				e.Assets[asset.ID] = asset
			}
			if len(nodes) > 0 {
				e.AddNamed(nodes, nil, "导入")
			}
			a.draftKey = ""
			if len(errs) > 0 {
				a.message(fmt.Sprintf("已导入 %d 张；无法读取 %s", len(nodes), strings.Join(errs, "、")))
			} else {
				a.message(fmt.Sprintf("已导入 %d 张图片", len(nodes)))
			}
		})
	}()
}
func (a *desktop) chooseImages() {
	at := a.center()
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: a.win, Title: "导入图片", Multiple: true, Filters: []mygo.FileFilter{{Name: "图片", Extensions: []string{"png", "jpg", "jpeg", "webp"}}}})
		a.publish(func() {
			if err != nil {
				a.message(err.Error())
			} else {
				a.importFiles(paths, at)
			}
		})
	}()
}
func (a *desktop) exportImage(asset Asset) {
	go func() {
		path, err := mygo.Dialog.Save(mygo.SaveDialogOptions{Parent: a.win, Title: "下载图片", DefaultPath: asset.Name})
		if path == "" || err != nil {
			return
		}
		src, err := a.store.AssetPath(asset)
		if err == nil {
			var b []byte
			b, err = readBounded(src, maxStoredImageBytes)
			if err == nil && digest(b) != asset.SHA {
				err = fmt.Errorf("图片校验失败")
			}
			if err == nil {
				err = atomicFile(path, b)
			}
		}
		a.publish(func() {
			if err != nil {
				a.message(err.Error())
			} else {
				a.message("图片已下载")
			}
		})
	}()
}
func (a *desktop) preview(asset Asset) {
	a.previewAsset = asset
	a.previewOpen = true
	a.previewView = Viewport{Zoom: 0}
	a.previewPanning = false
	if a.images != nil {
		a.images.Request(asset, 0)
	}
}
func (a *desktop) backup(owner *mygo.Window) {
	if a.closing || a.backupSettings.busy {
		return
	}
	a.backupSettings.busy = true
	a.commitCanvasText()
	if owner == nil {
		owner = a.win
	}
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: owner, Title: "选择备份位置", Directory: true, CreateDirectories: true})
		if err != nil || len(paths) == 0 {
			a.publishSettings(func() { a.finishSettingsTask(&a.backupSettings, "", err) })
			return
		}
		if err = a.saves.Flush(); err == nil {
			err = a.store.Backup(filepath.Join(paths[0], "Canvepp-"+time.Now().Format("20060102-150405")))
		}
		a.publishSettings(func() { a.finishSettingsTask(&a.backupSettings, "备份已保存", err) })
	}()
}
func (a *desktop) importLegacy() {
	if a.closing || a.importSettings.busy {
		return
	}
	a.importSettings.busy = true
	owner := a.settingsWin
	if owner == nil {
		owner = a.win
	}
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: owner, Title: "选择旧版数据或备份目录", Directory: true, ShowHiddenFiles: true})
		if err != nil || len(paths) == 0 {
			a.publishSettings(func() { a.finishSettingsTask(&a.importSettings, "", err) })
			return
		}
		report, err := a.store.mergeLibrary(paths[0], true)
		a.publishSettings(func() {
			if err == nil && report.Imported > 0 {
				err = a.reload()
			}
			message := fmt.Sprintf("已导入 %d 张画布", report.Imported)
			if report.Skipped > 0 {
				message += fmt.Sprintf("，跳过 %d 张已有画布", report.Skipped)
			}
			a.finishSettingsTask(&a.importSettings, message, err)
		})
	}()
}

func (a *desktop) finishSettingsTask(task *settingsTask, message string, err error) {
	task.busy = false
	if err != nil {
		message = err.Error()
	}
	if message != "" {
		task.message, task.failed = message, err != nil
		a.message(message)
	}
}

func (a *desktop) checkAccount(login bool) {
	if a.accountBusy {
		return
	}
	a.accountBusy = true
	a.loginError = ""
	timeout := 45 * time.Second
	if login {
		timeout = 15 * time.Minute
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	if login {
		a.cancelLogin = cancel
	}
	go func() {
		defer cancel()
		var account *chatGPTAccount
		c, err := startCodex(ctx)
		if err == nil {
			defer c.Close()
			if login {
				err = c.loginChatGPT(ctx, func(start chatLogin) error {
					a.publishSettings(func() { a.loginCode = start.UserCode })
					return mygo.Shell.OpenExternal(start.URL)
				})
			}
			if err == nil {
				account, err = c.readChatGPTAccount(ctx)
			}
		}
		cancelled := errors.Is(ctx.Err(), context.Canceled)
		a.publishSettings(func() {
			a.accountBusy = false
			a.cancelLogin = nil
			a.loginCode = ""
			switch {
			case err == nil:
				a.chatGPTAccount = account
			case errors.Is(err, errChatGPTSignInRequired) && !login:
				a.chatGPTAccount = nil
			case !cancelled:
				a.loginError = err.Error()
			}
		})
	}()
}
func (a *desktop) generate() {
	runs := a.prepareCanvasGeneration()
	if len(runs) == 0 {
		return
	}
	a.generationNotices.start(runs)
	parallel := a.parallelGeneration
	if a.generationSlots == nil {
		a.generationSlots = make(chan struct{}, 4)
	}
	slots := a.generationSlots
	go func() {
		if err := a.saves.Flush(); err != nil {
			a.failQueuedGeneration(runs, "画布尚未保存，未开始生成", false)
			return
		}
		if err := a.store.QueueRuns(runs); err != nil {
			a.failQueuedGeneration(runs, "未能保存生成任务："+err.Error(), true)
			return
		}
		dispatchGenerationBatch(runs, parallel, slots, func(r NativeRun) {
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
			defer cancel()
			executeNative(ctx, a.store, r, false, a.receiveRun)
		})
	}()
}
func (a *desktop) failQueuedGeneration(runs []NativeRun, reason string, canvasSaved bool) {
	for _, r := range runs {
		r.Status, r.Error, r.UpdatedAt = "failed", reason, timestamp()
		if canvasSaved {
			if err := a.store.SaveRun(r); err != nil {
				log.Printf("记录未提交的生成任务：%v", err)
			}
		}
		a.receiveRun(r, nil)
	}
	a.publish(func() { a.message(reason) })
}

func (a *desktop) receiveRun(r NativeRun, asset *Asset) {
	a.publish(func() {
		defer a.finishGenerationNotice(r, asset)
		if e := a.editors[r.ProjectID]; e != nil {
			if o := runOperation(r); o != nil {
				if !slices.ContainsFunc(e.Events, func(old Operation) bool { return old.ID == o.ID }) {
					e.Events = append(e.Events, *o)
				}
			}
		}
		a.runRevision++
		for i := range a.runs {
			if a.runs[i].ID == r.ID {
				a.runs[i] = r
				break
			}
		}
		if r.Status == "succeeded" || r.Status == "failed" || r.Status == "unknown" || r.Status == "cancelled" {
			delete(a.running, r.ID)
		}
		if asset != nil {
			e := a.editors[r.ProjectID]
			if e == nil {
				return
			}
			e.Assets[asset.ID] = *asset
			if adoptGeneratedResult(e, r.NodeID, asset.ID) && r.Status == "succeeded" {
				a.beginImageReveal(e, r.NodeID, asset.ID)
			}
			e.changed()
		}
		if r.Error != "" {
			a.message(r.Error)
		}
	})
}
func (a *desktop) recoverRun(r NativeRun) {
	if a.running[r.ID] {
		return
	}
	a.running[r.ID] = true
	a.runRevision++
	if r.Funding.subscription() && r.Status != "succeeded" {
		a.generationNotices.start([]NativeRun{r})
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
		defer cancel()
		executeNative(ctx, a.store, r, true, a.receiveRun)
	}()
}

func (a *desktop) view(c *ui.Context) {
	defer beginHints(c)()
	if m := a.materials; m != nil && m.web != nil && m.web.browser != nil && !a.wantsPinterestBrowser() {
		m.web.browser.hide()
	}
	a.purgeAnchorDrawn = false
	canvasPage := a.left == "画布"
	if !canvasPage {
		clear(a.imageReveals)
	}
	t := workspaceTheme(c, a.appearance)
	c.SetTheme(t)
	surround := a.workspaceSurface(c, t.Surface, .65)
	c.Root().Background(surround)
	// Do not mount new editors, lay out source text, or load reference previews
	// on the critical drag path. The canvas selection moves immediately; the
	// inspector catches up on release, including a simple click.
	if a.gesture == "" && !a.panelDragging && !a.dock.dragging {
		a.syncDraft()
		a.syncImageDrawer()
	}
	c.OnShortcut(ui.Cmd|ui.Shift, ui.KeySpace, a.showAssistant)
	c.OnShortcut(ui.Cmd, ui.KeyComma, a.toggleSettings)
	if a.dockVisible() && a.dock.active == dockPinterest && a.materials != nil && a.materials.showNativePins {
		// Register at the window scope: opening configuration removes the
		// focused launch button, so a content-subtree shortcut has no owner.
		c.OnShortcut(0, ui.KeyEscape, a.backInMaterials)
	}
	// Tint the whole window surround once so the rail and exposed corners meet seamlessly.
	disable(ui.Column(c).Fill(), a.closing || a.purgeBusy).Children(func() {
		a.workspaceTitleBar(c)
		ui.Row(c).Grow(1).MinHeight(0).FillWidth().AlignItems(ui.Stretch).Children(func() {
			a.controlPanel(c)
			ui.Box(c.Key("workspace-content")).Grow(1).MinWidth(0).FillHeight().Clip().Children(func() {
				key, inset, shift := a.left, float32(contentInset), float32(5)
				if canvasPage {
					inset, shift = canvasInset, 0
					if a.editor != nil {
						key += "/" + a.editor.Project.ID
					}
				}
				page := ui.Box(c.Key(key)).Fill().Padding(0, inset, inset, 0)
				paneMotion(c, page, 0, shift).Children(func() {
					switch a.left {
					case allCanvases:
						a.canvasTimelinePage(c)
					case allPictures, generatedPictures:
						a.overviewPage(c)
					case "画布":
						ui.Row(c).Fill().Children(func() {
							a.canvasView(c)
							a.rightDock(c)
						})
					default:
						a.galleryView(c)
					}
				})
			})
		})
	})
	if a.notice != "" && time.Now().Before(a.noticeUntil) {
		notice := ui.Row(c.Key("notice")).Absolute().Bottom(48).Left(80).MaxWidth(620).Padding(8, 14).Gap(12).AlignItems(ui.Center).Radius(12).Background(t.Inverse)
		paneMotion(c, notice, 0, 6).Children(func() {
			ui.Text(c, a.notice).TextColor(t.InverseText).MaxLines(3)
			if a.canUndoDelete() {
				iconButton(c, "undo", "撤销移入回收站", a.undoDelete).Background(t.Inverse).TextColor(t.InverseText)
			}
		})
		c.After(time.Until(a.noticeUntil))
	}
	if !a.purgeAnchorDrawn {
		a.purgeOpen = false
		a.purgeKey = ""
	}
	a.renameDialog(c)
	a.deleteSessionDialog(c)
	if a.previewOpen {
		a.previewModal(c)
	}
}

func (a *desktop) openWorkspace(page string) {
	a.commitCanvasText()
	a.textPlacing = false
	a.closeSettings()
	a.left, a.groupOpen = page, false
	a.locatedTrashKey = ""
	a.hover = ""
}

func statusLabel(status string) string {
	switch status {
	case "queued":
		return "等待生成"
	case "preparing", "submitted", "running":
		return "正在生成"
	case "recovering":
		return "正在查询结果"
	case "decoding":
		return "正在载入结果"
	case "cancelled":
		return "已取消"
	case "unknown":
		return "待查询结果"
	case "failed":
		return "生成未完成"
	case "succeeded":
		return "已完成"
	default:
		return status
	}
}
func (a *desktop) assetPrompt(asset Asset) string {
	for _, r := range a.runs {
		if r.ID == asset.RunID {
			return r.Prompt
		}
	}
	if prompt := a.legacyPrompts[asset.RunID]; prompt != "" {
		return prompt
	}
	if e := a.editors[asset.ProjectID]; e != nil {
		if n := e.Node(asset.NodeID); n != nil {
			return n.Config.Text
		}
		for _, tr := range e.Trash {
			if tr.Node.ID == asset.NodeID {
				return tr.Node.Config.Text
			}
		}
	}
	return ""
}

var ratios = []struct {
	label string
	w, h  int
}{{"1:1", 1, 1}, {"4:3", 4, 3}, {"3:4", 3, 4}, {"3:2", 3, 2}, {"2:3", 2, 3}, {"16:9", 16, 9}, {"9:16", 9, 16}, {"21:9", 21, 9}}

func ratioFor(w, h float64) int {
	for i, r := range ratios {
		if math.Abs(w/h-float64(r.w)/float64(r.h)) < .002 {
			return i
		}
	}
	return -1
}
func (a *desktop) creationForm(c *ui.Context) {
	ui.Column(c.Key("image-settings")).Grow(1).FillWidth().Children(func() {
		if a.editor == nil || len(a.editor.Selection) == 0 {
			ui.Column(c).Grow(1).Center().Gap(12).Children(func() {
				emptyIcon(c, "canvas")
				ui.Text(c, "未选中图片").TextColor(c.Theme().TextMuted)
			})
			return
		}
		thinScroll(c, ui.Scroll(c.Key("creation-scroll")), nil).Grow(1).Padding(creationTopPad, 13, creationBottomPad, 13).Gap(creationFormGap).Children(func() {
			ids := a.draftSelection
			if len(ids) > 1 {
				ui.Row(c).MaxWidth(231).Gap(5).Wrap().Children(func() {
					for _, id := range ids[:min(8, len(ids))] {
						n := a.editor.Node(id)
						if n == nil {
							continue
						}
						if asset, ok := a.editor.Assets[n.Asset]; ok && a.images != nil {
							a.images.Request(asset, 512)
							if b := a.images.Get(asset, 512); b != nil {
								ui.Image(c.Key(n.ID), b).Size(54, 54).Fit(ui.Cover).Radius(4)
							}
						}
					}
				})
			}
			ui.Column(c).FillWidth().Gap(creationPromptGap).Children(func() {
				ui.Text(c, "提示词").Height(creationPromptLabelHeight).FontSize(12).TextColor(c.Theme().TextMuted)
				input := editHint(c, thinScroll(c, ui.TextArea(c.Key("prompt"), &a.prompt), nil).Label("编辑提示词"), "编辑图片提示词").
					Placeholder("写下你的提示词…").FillWidth().Height(creationPromptHeight).Padding(8, 10).
					FontSize(13).Radius(8).Background(creationFieldColor(c.Theme())).OnChange(a.updateDraft)
				if input.Focused() {
					input.Border(1, c.Theme().Focus)
				}
			})
			a.creationParameters(c)
			_, asset, _ := a.selectedImage()
			a.imageActions(c, asset, true)
		})
	})
}
