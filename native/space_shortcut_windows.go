//go:build windows && (amd64 || arm64)

package main

import (
	"strings"
	"sync/atomic"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

type windowsSpaceRequest struct {
	window windows.HWND
	epoch  uint64
}

type windowsSpaceShortcut struct {
	enabled  atomic.Bool
	epoch    atomic.Uint64
	requests chan windowsSpaceRequest

	// Owned by the Win32 hook thread.
	sequence tripleSpace
	seen     uint64
	origin   windows.HWND
	held     bool
}

var (
	attachShortcutInput = explorerUser32.NewProc("AttachThreadInput")
	shortcutSetFocus    = explorerUser32.NewProc("SetFocus")
	shortcutNotifyState = windows.NewLazySystemDLL("shell32.dll").NewProc("SHQueryUserNotificationState")
)

func (a *desktop) refreshWindowsHotkey() bool {
	if p := a.explorerPreview; p != nil {
		p.assistant.epoch.Add(1)
		p.assistant.enabled.Store(a.hotkeyEnabled)
		a.hotkeyStatus = "已关闭"
		if a.hotkeyEnabled {
			a.hotkeyStatus = "正在启动监听"
		}
		p.wakeListener()
	}
	return true
}

func (p *explorerPreview) assistantOrigin(window windows.HWND) windows.HWND {
	if popup := p.popup.Load(); popup != 0 && popup == uintptr(window) {
		return windows.HWND(p.origin.Load())
	}
	return window
}

// Observe before the Explorer preview handles Space so the same three taps
// can start in a file list even when the single-tap preview opens in between.
func (p *explorerPreview) feedAssistantShortcut(message uintptr, key *explorerKeyboardData) {
	down := message == explorerDown || message == explorerSysDown
	if !down && message != explorerUp && message != explorerSysUp {
		return
	}
	s := &p.assistant
	epoch := s.epoch.Load()
	if epoch != s.seen {
		s.sequence, s.origin, s.held, s.seen = tripleSpace{}, 0, false, epoch
	}
	if key.key != explorerSpace {
		if down {
			s.sequence = tripleSpace{}
		}
		return
	}
	held := s.held
	s.held = down
	if down == held || key.flags&0x20 != 0 || explorerModifiersDown() {
		s.sequence = tripleSpace{}
		return
	}
	current := p.assistantOrigin(windows.GetForegroundWindow())
	if current == 0 {
		s.sequence = tripleSpace{}
		return
	}
	if s.origin != current {
		s.sequence, s.origin = tripleSpace{}, current
	}
	kind := 2
	if down {
		kind = 1
	}
	stamp := time.Duration(key.time) * time.Millisecond
	if s.sequence.feed(kind, stamp) {
		p.generation.Add(1)
		select {
		case s.requests <- windowsSpaceRequest{window: current, epoch: epoch}:
		default:
		}
	}
}

func (p *explorerPreview) resolveAssistantShortcut() {
	for {
		select {
		case <-p.done:
			return
		case request := <-p.assistant.requests:
			traceShortcut("sequence_match", 0)
			if isGameWindow(request.window) {
				traceShortcut("game_excluded", 0)
				continue
			}
			p.app.publish(func() {
				foreground := windows.GetForegroundWindow()
				if !p.assistant.enabled.Load() || p.assistant.epoch.Load() != request.epoch ||
					p.assistantOrigin(foreground) != request.window {
					return
				}
				if uintptr(foreground) == p.popup.Load() {
					p.closeWindow()
				}
				p.app.showAssistant()
				bar := p.app.assistant
				if bar != nil && bar.win != nil && bar.win.IsVisible() {
					focusShortcutWindow(windows.HWND(bar.win.NativeHandle()), request.window)
				}
			})
		}
	}
}

func focusShortcutWindow(window, origin windows.HWND) {
	if windows.GetForegroundWindow() != origin {
		return
	}
	// Windows can refuse SetForegroundWindow from a background process. Share
	// the current input queue only for the explicit shortcut, then detach.
	source, err := windows.GetWindowThreadProcessId(origin, nil)
	thread := windows.GetCurrentThreadId()
	if err != nil || source == thread {
		return
	}
	attached, _, _ := attachShortcutInput.Call(uintptr(thread), uintptr(source), 1)
	if attached != 0 {
		defer attachShortcutInput.Call(uintptr(thread), uintptr(source), 0)
		if windows.GetForegroundWindow() == origin {
			explorerForeground.Call(uintptr(window))
			shortcutSetFocus.Call(uintptr(window))
		}
	}
}

func isGameWindow(window windows.HWND) bool {
	var state int32
	if hr, _, _ := shortcutNotifyState.Call(uintptr(unsafe.Pointer(&state))); int32(hr) >= 0 && state == 3 {
		return true // QUNS_RUNNING_D3D_FULL_SCREEN
	}
	class := explorerClass(window)
	switch strings.ToLower(class) {
	case "unitywndclass", "unrealwindow", "sourcerenderwindow", "valve001":
		return true
	}
	var pid uint32
	if _, err := windows.GetWindowThreadProcessId(window, &pid); err != nil {
		return false
	}
	process, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return false
	}
	defer windows.CloseHandle(process)
	var path [1024]uint16
	size := uint32(len(path))
	if err := windows.QueryFullProcessImageName(process, 0, &path[0], &size); err != nil {
		return false
	}
	name := strings.ToLower(windows.UTF16ToString(path[:size]))
	return strings.Contains(name, `\steamapps\common\`) ||
		strings.Contains(name, `\xboxgames\`) ||
		strings.Contains(name, `\.minecraft\runtime\`) ||
		strings.Contains(name, `\windowsapps\microsoft.minecraft`) ||
		strings.HasSuffix(name, `\robloxplayerbeta.exe`) ||
		strings.Contains(name, `\epic games\`) && !strings.Contains(name, `\epic games\launcher\`)
}
