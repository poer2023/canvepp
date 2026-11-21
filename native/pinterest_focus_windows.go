package main

import (
	"strings"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	pinUser32        = windows.NewLazySystemDLL("user32.dll")
	pinWindowText    = pinUser32.NewProc("GetWindowTextW")
	pinAttachInput   = pinUser32.NewProc("AttachThreadInput")
	pinSetForeground = pinUser32.NewProc("SetForegroundWindow")
)

// Chrome can still own the foreground when its local callback has completed.
// Only hand focus back from the tab bearing this authorization's receipt.
func (a *desktop) focusPinterestCallback(title string) {
	win := a.win
	hwnd := windows.HWND(win.NativeHandle())
	var attempt func(int)
	attempt = func(left int) {
		foreground := windows.GetForegroundWindow()
		if foreground == hwnd || foreground == 0 || a.closing {
			return
		}
		var name [256]uint16
		pinWindowText.Call(uintptr(foreground), uintptr(unsafe.Pointer(&name[0])), uintptr(len(name)))
		if strings.HasPrefix(windows.UTF16ToString(name[:]), title) {
			thread, err := windows.GetWindowThreadProcessId(foreground, nil)
			current := windows.GetCurrentThreadId()
			if err == nil && current != thread {
				if ok, _, _ := pinAttachInput.Call(uintptr(current), uintptr(thread), 1); ok != 0 {
					defer pinAttachInput.Call(uintptr(current), uintptr(thread), 0)
					if windows.GetForegroundWindow() == foreground {
						pinSetForeground.Call(uintptr(hwnd))
					}
				}
			}
			return
		}
		// The browser may not have painted the 200 response yet.
		if left > 0 {
			time.AfterFunc(100*time.Millisecond, func() { win.Update(func() { attempt(left - 1) }) })
		}
	}
	attempt(12)
}
