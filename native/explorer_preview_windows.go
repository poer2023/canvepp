//go:build windows && (amd64 || arm64)

package main

import (
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"unsafe"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"golang.org/x/sys/windows"
)

const (
	explorerSpace   = 0x20
	explorerEsc     = 0x1b
	explorerDown    = 0x100
	explorerUp      = 0x101
	explorerSysDown = 0x104
	explorerSysUp   = 0x105
	explorerQuit    = 0x12
	explorerWake    = 0x8000 + 113
)

var (
	setKeyboardHook    = explorerUser32.NewProc("SetWindowsHookExW")
	unhookKeyboard     = explorerUser32.NewProc("UnhookWindowsHookEx")
	nextKeyboardHook   = explorerUser32.NewProc("CallNextHookEx")
	explorerGetMsg     = explorerUser32.NewProc("GetMessageW")
	explorerPeekMsg    = explorerUser32.NewProc("PeekMessageW")
	explorerDispatch   = explorerUser32.NewProc("DispatchMessageW")
	explorerPostThread = explorerUser32.NewProc("PostThreadMessageW")
	explorerPost       = explorerUser32.NewProc("PostMessageW")
	explorerParent     = explorerUser32.NewProc("GetParent")
	explorerKeyStatus  = explorerUser32.NewProc("GetAsyncKeyState")
	explorerForeground = explorerUser32.NewProc("SetForegroundWindow")
	explorerMonitor    = explorerUser32.NewProc("MonitorFromWindow")
	setForegroundHook  = explorerUser32.NewProc("SetWinEventHook")
	unhookForeground   = explorerUser32.NewProc("UnhookWinEvent")
	explorerModule     = windows.NewLazySystemDLL("kernel32.dll").NewProc("GetModuleHandleW")

	activeExplorerPreview atomic.Pointer[explorerPreview]
	explorerKeyCallback   = windows.NewCallback(explorerPreviewKey)
	explorerFocusCallback = windows.NewCallback(explorerFocusChanged)
)

type explorerOrigin struct {
	window, focus windows.HWND
}
type explorerRequest struct {
	origin     explorerOrigin
	scan       uint32
	generation uint64
}
type explorerKeyboardData struct {
	key, scan, flags, time uint32
	extra                  uintptr
}
type explorerMessage struct {
	window         windows.HWND
	message        uint32
	wParam, lParam uintptr
	time           uint32
	x, y           int32
	private        uint32
}

type explorerPreview struct {
	app           *desktop
	on            atomic.Bool
	thread        atomic.Uint32
	stopped       atomic.Bool
	generation    atomic.Uint64
	requests      chan explorerRequest
	done          chan struct{}
	popup, origin atomic.Uintptr

	// Key state lives on the hook thread; window state lives on the UI thread.
	down, consumed [2]bool
	window         *mygo.Window
	err            string
	assistant      windowsSpaceShortcut
}

func (a *desktop) startExplorerPreview() {
	p := &explorerPreview{app: a, requests: make(chan explorerRequest, 1), done: make(chan struct{}),
		assistant: windowsSpaceShortcut{requests: make(chan windowsSpaceRequest, 1)}}
	var enabled bool
	if err := a.store.DB.QueryRow("SELECT value FROM native_meta WHERE key='explorer_preview'").Scan(&enabled); err != nil {
		enabled = true
	}
	p.on.Store(enabled)
	a.explorerPreview = p
	a.refreshWindowsHotkey()
	go p.listen()
	go p.resolve()
	go p.resolveAssistantShortcut()
}

func (a *desktop) stopExplorerPreview() {
	if p := a.explorerPreview; p != nil {
		p.stopped.Store(true)
		p.on.Store(false)
		p.assistant.enabled.Store(false)
		close(p.done)
		p.closeWindow()
		if thread := p.thread.Load(); thread != 0 {
			explorerPostThread.Call(uintptr(thread), explorerQuit, 0, 0)
		}
		a.explorerPreview = nil
	}
}

func (a *desktop) explorerPreviewSettings(c *ui.Context) {
	settingsGroup(c, "文件资源管理器", func() {
		settingsRow(c, "空格预览文件", "file", func() {
			p := a.explorerPreview
			enabled := p != nil && p.on.Load()
			switcher := flatSwitch(c, &enabled, "在文件资源管理器按空格预览")
			if commandHint(c, switcher, "选中文件后按空格预览").Changed() && p != nil {
				if _, err := a.store.DB.Exec("INSERT OR REPLACE INTO native_meta VALUES('explorer_preview',?)", enabled); err != nil {
					p.err = "保存快速查看设置失败"
					return
				}
				p.generation.Add(1)
				p.on.Store(enabled)
				if !enabled {
					p.closeWindow()
				}
				p.wakeListener()
			}
		})
		if p := a.explorerPreview; p != nil {
			settingsMessage(c, p.err, settingsError)
		}
	})
}

func (p *explorerPreview) wakeListener() {
	if thread := p.thread.Load(); thread != 0 {
		explorerPostThread.Call(uintptr(thread), explorerWake, 0, 0)
	}
}

func (p *explorerPreview) listen() {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	var msg explorerMessage
	explorerPeekMsg.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0, 0) // Create this thread's message queue.
	p.thread.Store(windows.GetCurrentThreadId())
	defer p.thread.Store(0)
	if p.stopped.Load() {
		return
	}
	activeExplorerPreview.Store(p)
	defer activeExplorerPreview.CompareAndSwap(p, nil)

	var keys, focus uintptr
	syncHooks := func() {
		wanted := p.on.Load() || p.assistant.enabled.Load()
		if wanted && keys == 0 {
			const lowLevelKeyboard = 13
			module, _, _ := explorerModule.Call(0)
			keys, _, _ = setKeyboardHook.Call(lowLevelKeyboard, explorerKeyCallback, module, 0)
			if keys != 0 {
				const foregroundEvent = 3
				focus, _, _ = setForegroundHook.Call(foregroundEvent, foregroundEvent, 0, explorerFocusCallback, 0, 0, 0)
			}
		} else if !wanted && keys != 0 {
			unhookKeyboard.Call(keys)
			if focus != 0 {
				unhookForeground.Call(focus)
			}
			keys, focus = 0, 0
			p.down, p.consumed = [2]bool{}, [2]bool{}
		}
		ready := keys != 0
		if ready {
			traceShortcut("listener_ready", 0)
		} else if wanted {
			traceShortcut("listener_start_failed", 0)
		}
		p.app.publishSettings(func() {
			if p.app.explorerPreview == p {
				p.err = ""
				if p.on.Load() && !ready {
					p.err = "资源管理器快捷键启动失败"
				}
				switch {
				case !p.app.hotkeyEnabled:
					p.app.hotkeyStatus = "已关闭"
				case ready:
					p.app.hotkeyStatus = "全局监听已开启"
				default:
					p.app.hotkeyStatus = "监听启动失败"
				}
			}
		})
	}
	defer func() {
		if keys != 0 {
			unhookKeyboard.Call(keys)
		}
		if focus != 0 {
			unhookForeground.Call(focus)
		}
	}()
	syncHooks()
	for !p.stopped.Load() {
		result, _, _ := explorerGetMsg.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if int32(result) <= 0 {
			break
		}
		if msg.message == explorerWake {
			syncHooks()
		} else {
			explorerDispatch.Call(uintptr(unsafe.Pointer(&msg)))
		}
	}
}

func explorerPreviewKey(code int32, message uintptr, key *explorerKeyboardData) uintptr {
	p := activeExplorerPreview.Load()
	if code == 0 && p != nil && key != nil {
		if p.assistant.enabled.Load() {
			p.feedAssistantShortcut(message, key)
		}
	}
	if code == 0 && p != nil && p.on.Load() && key != nil && (message == explorerDown || message == explorerUp) {
		index := 0
		if key.key == explorerEsc {
			index = 1
		}
		if key.key == explorerSpace || key.key == explorerEsc {
			if message == explorerDown {
				if !p.down[index] {
					p.down[index] = true
					p.consumed[index] = p.handleKey(key)
				}
				if p.consumed[index] {
					return 1
				}
			} else {
				taken := p.consumed[index]
				p.down[index], p.consumed[index] = false, false
				if taken {
					return 1
				}
			}
		}
	}
	out, _, _ := nextKeyboardHook.Call(0, uintptr(code), message, uintptr(unsafe.Pointer(key)))
	return out
}

func (p *explorerPreview) handleKey(key *explorerKeyboardData) bool {
	// Alt is also present in the keyboard packet. Other modifiers keep their
	// normal Windows shortcuts, including Ctrl+Space and Shift+Space.
	if key.flags&0x20 != 0 || explorerModifiersDown() {
		return false
	}
	foreground := windows.GetForegroundWindow()
	if popup := p.popup.Load(); popup != 0 && uintptr(foreground) == popup {
		p.requestClose(popup)
		return true
	}
	where := currentExplorerOrigin(foreground)
	if where.focus == 0 {
		return false
	}
	if popup := p.popup.Load(); popup != 0 && uintptr(foreground) == p.origin.Load() {
		p.requestClose(popup)
		return true
	}
	if key.key != explorerSpace {
		return false
	}
	generation := p.generation.Add(1)
	select {
	case p.requests <- explorerRequest{origin: where, scan: key.scan, generation: generation}:
		return true
	default:
		return false
	}
}

func explorerModifiersDown() bool {
	for _, key := range []uintptr{0x10, 0x11, 0x12, 0x5b, 0x5c} {
		value, _, _ := explorerKeyStatus.Call(key)
		if value&0x8000 != 0 {
			return true
		}
	}
	return false
}

func explorerClass(window windows.HWND) string {
	var name [64]uint16
	n, _ := windows.GetClassName(window, &name[0], int32(len(name)))
	return windows.UTF16ToString(name[:max(0, n)])
}

func currentExplorerOrigin(window windows.HWND) explorerOrigin {
	if window == 0 {
		return explorerOrigin{}
	}
	switch explorerClass(window) {
	case "CabinetWClass", "ExploreWClass":
	default:
		return explorerOrigin{}
	}
	thread, err := windows.GetWindowThreadProcessId(window, nil)
	if err != nil {
		return explorerOrigin{}
	}
	info := windows.GUIThreadInfo{Size: uint32(unsafe.Sizeof(windows.GUIThreadInfo{}))}
	if err := windows.GetGUIThreadInfo(thread, &info); err != nil || info.Focus == 0 || info.Active != window ||
		info.Flags&0x1e != 0 || info.CaretHandle != 0 {
		return explorerOrigin{}
	}
	switch explorerClass(info.Focus) {
	case "DirectUIHWND", "SysListView32", "SHELLDLL_DefView":
	default:
		return explorerOrigin{}
	}
	for child := info.Focus; child != 0 && child != window; {
		if explorerClass(child) == "SHELLDLL_DefView" {
			return explorerOrigin{window, info.Focus}
		}
		parent, _, _ := explorerParent.Call(uintptr(child))
		child = windows.HWND(parent)
	}
	return explorerOrigin{}
}

func explorerFocusChanged(_ uintptr, _ uint32, hwnd uintptr, _ int32, _ int32, _ uint32, _ uint32) uintptr {
	if p := activeExplorerPreview.Load(); p != nil {
		if origin := p.assistantOrigin(windows.HWND(hwnd)); origin != p.assistant.origin {
			p.assistant.sequence, p.assistant.origin = tripleSpace{}, origin
		}
		if popup := p.popup.Load(); popup != 0 && hwnd != 0 && hwnd != popup && hwnd != p.origin.Load() {
			p.requestClose(popup)
		}
	}
	return 0
}

func (p *explorerPreview) requestClose(popup uintptr) {
	p.app.publish(func() {
		if p.popup.Load() == popup {
			p.closeWindow()
		}
	})
}

func (p *explorerPreview) resolve() {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	hr, _, _ := comInitialize.Call(0, 2) // COINIT_APARTMENTTHREADED
	comReady := int32(hr) >= 0
	if comReady {
		defer windows.CoUninitialize()
	}
	for {
		select {
		case <-p.done:
			return
		case req := <-p.requests:
			if currentExplorerOrigin(windows.GetForegroundWindow()) != req.origin {
				continue
			}
			path := ""
			if comReady && p.on.Load() {
				path = selectedExplorerPath(req.origin.focus)
			}
			if path == "" {
				if currentExplorerOrigin(windows.GetForegroundWindow()) == req.origin {
					// Let the list carry out its usual Space action when it has no
					// selected file. Post only to the same focused control.
					parameter := uintptr(1 | (req.scan&0xff)<<16)
					explorerPost.Call(uintptr(req.origin.focus), explorerDown, explorerSpace, parameter)
					explorerPost.Call(uintptr(req.origin.focus), explorerUp, explorerSpace, parameter|3<<30)
				}
				continue
			}
			data := loadFilePreview(path)
			p.app.publish(func() {
				if p.on.Load() && !p.app.closing && !p.app.closed && p.generation.Load() == req.generation &&
					currentExplorerOrigin(windows.GetForegroundWindow()) == req.origin {
					p.showWindow(req.origin.window, data)
				} else if data.pdf != nil {
					data.pdf.close()
				}
			})
		}
	}
}

func (p *explorerPreview) showWindow(from windows.HWND, data filePreview) {
	if p.window != nil {
		if data.pdf != nil {
			data.pdf.close()
		}
		return
	}
	display := mygo.Screen.PrimaryDisplay()
	monitor, _, _ := explorerMonitor.Call(uintptr(from), 2) // MONITOR_DEFAULTTONEAREST
	for _, screen := range mygo.Screen.Displays() {
		if screen.ID == int64(monitor) {
			display = screen
			break
		}
	}
	work := display.WorkArea
	width, height := filePreviewSize(data, work.Width, work.Height, display.ScaleFactor)
	window := mygo.NewWindow(mygo.WindowOptions{
		Title: data.name + " · 预览", Hidden: true, AlwaysOnTop: true, SkipTaskbar: true,
		X: work.X + (work.Width-width)/2, Y: work.Y + (work.Height-height)/2,
		Width: width, Height: height, MinWidth: min(width, 148), MinHeight: min(height, 120),
		TitleBarStyle: mygo.TitleBarHidden, HideWindowButtons: true, AutoHideMenuBar: true,
		Content: ui.View(func(c *ui.Context) { filePreviewView(c, &data, p.app.appearance, p.closeWindow) }),
	})
	if data.pdf != nil {
		data.pdf.start(width-2*filePreviewPadding, height-filePreviewHeaderHeight-1-2*filePreviewPadding, display.ScaleFactor, window.Update)
	}
	p.window = window
	hwnd := windows.HWND(window.NativeHandle())
	p.origin.Store(uintptr(from))
	p.popup.Store(uintptr(hwnd))
	var handler *shellFilePreview
	returnFocus := false
	window.OnClose(func(_ *mygo.CloseEvent) {
		returnFocus = windows.GetForegroundWindow() == hwnd
		if data.pdf != nil {
			data.pdf.close()
		}
		if handler != nil {
			handler.close()
			handler = nil
		}
	})
	window.OnClosed(func() {
		p.window = nil
		p.popup.Store(0)
		p.origin.Store(0)
		if returnFocus && from != 0 {
			explorerForeground.Call(uintptr(from))
		}
	})
	window.OnResize(func() {
		if handler != nil {
			handler.resize()
		}
	})
	window.Show()
	window.Focus()
	focusShortcutWindow(hwnd, from)
	if data.useShell {
		handler = openShellFilePreview(hwnd, data.path)
		if handler == nil {
			p.showDocumentFallback(window, &data)
		}
	}
}

func (p *explorerPreview) showDocumentFallback(window *mygo.Window, data *filePreview) {
	if data.notice == "" {
		data.notice = "此文件格式暂无可用预览"
	}
	window.Invalidate()
	switch strings.ToLower(filepath.Ext(data.path)) {
	case ".docx", ".odt", ".rtf":
		go func(path string) {
			text, err := extractRichDocumentText(path)
			if err == nil && strings.TrimSpace(text) != "" {
				window.Update(func() {
					data.notice = "文本预览"
					data.lines = strings.Split(text, "\n")
				})
			}
		}(data.path)
	}
}

func (p *explorerPreview) closeWindow() {
	if p.window != nil {
		p.window.Close()
	}
}
