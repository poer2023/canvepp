package main

import (
	"context"
	"errors"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unsafe"

	"github.com/egoist/mygo/ui"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

var chromeProgram = sync.OnceValue(func() string {
	for _, root := range []registry.Key{registry.CURRENT_USER, registry.LOCAL_MACHINE} {
		k, err := registry.OpenKey(root, `Software\Microsoft\Windows\CurrentVersion\App Paths\chrome.exe`, registry.QUERY_VALUE)
		if err != nil {
			continue
		}
		p, _, err := k.GetStringValue("")
		k.Close()
		if err == nil && validChrome(p) {
			return p
		}
	}
	for _, v := range []string{"ProgramFiles", "ProgramFiles(x86)", "LOCALAPPDATA"} {
		if dir := os.Getenv(v); dir != "" {
			p := filepath.Join(dir, "Google", "Chrome", "Application", "chrome.exe")
			if validChrome(p) {
				return p
			}
		}
	}
	return ""
})

func validChrome(p string) bool {
	i, err := os.Stat(p)
	return filepath.IsAbs(p) && strings.EqualFold(filepath.Base(p), "chrome.exe") && err == nil && !i.IsDir()
}
func pinterestBrowserAvailable() bool { return chromeProgram() != "" }

var (
	pinSystemMetric  = pinUser32.NewProc("GetSystemMetricsForDpi")
	pinDwmAttribute  = windows.NewLazySystemDLL("dwmapi.dll").NewProc("DwmSetWindowAttribute")
	pinFindWindow    = pinUser32.NewProc("FindWindowExW")
	pinSetLong       = pinUser32.NewProc("SetWindowLongPtrW")
	pinSetPosition   = pinUser32.NewProc("SetWindowPos")
	pinShowWindow    = pinUser32.NewProc("ShowWindowAsync")
	pinGetLong       = pinUser32.NewProc("GetWindowLongPtrW")
	pinCreate        = pinUser32.NewProc("CreateWindowExW")
	pinDestroy       = pinUser32.NewProc("DestroyWindow")
	pinSetParent     = pinUser32.NewProc("SetParent")
	pinGetParent     = pinUser32.NewProc("GetParent")
	pinDpiContext    = pinUser32.NewProc("GetWindowDpiAwarenessContext")
	pinDpiEqual      = pinUser32.NewProc("AreDpiAwarenessContextsEqual")
	pinStatic        = windows.StringToUTF16Ptr("STATIC")
	pinPostMessage   = pinUser32.NewProc("PostMessageW")
	pinSetProperty   = pinUser32.NewProc("SetPropW")
	pinGetProperty   = pinUser32.NewProc("GetPropW")
	pinOwnerProperty = windows.StringToUTF16Ptr("Canvepp.Pinterest.Owner")
	chromeClass      = windows.StringToUTF16Ptr("Chrome_WidgetWin_1")
)

type pinterestBrowser struct {
	webScroll
	scrollWake         chan struct{}
	mailbox            *scrollMailbox
	invalidate         func()
	parent, host, hwnd uintptr
	cancel             context.CancelFunc
	stopWatch          context.CancelFunc
	stopProbe          context.CancelFunc
	probeID            uint64
	probeLogin         bool
	fromSaved          bool
	deviceScale        float32
	checkOwnerAt       time.Time
	trim               [4]int
	message            string
	bounds             ui.Rect
	last               [4]int
	visible, closed    bool
}

func (a *desktop) newPinterestBrowser(login bool) *pinterestBrowser {
	b := &pinterestBrowser{fromSaved: !login, invalidate: a.win.Invalidate, parent: a.win.NativeHandle(), message: "正在打开 Pinterest…"}
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	b.cancel = cancel
	go func() {
		defer cancel()
		err := openPinterestLanding(ctx, login, func(link string) error {
			cmd := exec.Command(chromeProgram(), "--app="+link)
			if err := cmd.Start(); err != nil {
				return err
			}
			go cmd.Wait()
			return nil
		}, func(title string) error {
			tick := time.NewTicker(25 * time.Millisecond)
			defer tick.Stop()
			for {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-tick.C:
				}
				h := pinterestChromeWindow(title)
				if h == 0 {
					continue
				}
				result := make(chan error, 1)
				a.publish(func() {
					if b.closed || a.closing || ctx.Err() != nil {
						result <- context.Canceled
						return
					}
					if err := b.mount(h); err != nil {
						pinPostMessage.Call(h, 0x10, 0, 0)
						result <- err
						return
					}
					b.message = ""
					if a.wantsPinterestBrowser() {
						b.place(b.bounds, b.deviceScale)
					} else {
						b.hide()
					}
					a.win.Invalidate()
					b.watchPage(a, login)
					result <- nil
				})
				select {
				case err := <-result:
					return err
				case <-ctx.Done():
					return ctx.Err()
				}
			}
		})
		if err != nil {
			a.publish(func() {
				if !b.closed {
					b.message = "Pinterest 窗口未能打开"
					a.win.Invalidate()
				}
			})
		}
	}()
	return b
}

func pinterestChromeWindow(title string) uintptr {
	var last uintptr
	for {
		h, _, _ := pinFindWindow.Call(0, last, uintptr(unsafe.Pointer(chromeClass)), 0)
		if h == 0 {
			return 0
		}
		last = h
		var name [160]uint16
		pinWindowText.Call(h, uintptr(unsafe.Pointer(&name[0])), uintptr(len(name)))
		if !strings.HasPrefix(windows.UTF16ToString(name[:]), title) {
			continue
		}
		var pid uint32
		if _, err := windows.GetWindowThreadProcessId(windows.HWND(h), &pid); err != nil {
			continue
		}
		ph, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
		if err != nil {
			continue
		}
		var path [1024]uint16
		n := uint32(len(path))
		err = windows.QueryFullProcessImageName(ph, 0, &path[0], &n)
		windows.CloseHandle(ph)
		if err == nil && strings.EqualFold(windows.UTF16ToString(path[:n]), chromeProgram()) {
			return h
		}
	}
}

func (b *pinterestBrowser) owns() bool {
	if b.hwnd == 0 {
		return false
	}
	owner, _, _ := pinGetProperty.Call(b.hwnd, uintptr(unsafe.Pointer(pinOwnerProperty)))
	return owner == b.parent
}

// The browser lives inside a clipping child HWND of the actual canvas. Windows
// moves and clips both descendants together, including on maximize and DPI changes.
func (b *pinterestBrowser) mount(h uintptr) error {
	ownDPI, _, _ := pinDpiContext.Call(b.parent)
	browserDPI, _, _ := pinDpiContext.Call(h)
	if same, _, _ := pinDpiEqual.Call(ownDPI, browserDPI); same == 0 {
		return errors.New("incompatible browser DPI context")
	}
	host, _, err := pinCreate.Call(0, uintptr(unsafe.Pointer(pinStatic)), 0, 0x40000000|0x02000000|0x04000000, 0, 0, 1, 1, b.parent, 0, 0, 0)
	if host == 0 {
		return err
	}
	style, _, _ := pinGetLong.Call(h, ^uintptr(15))
	// WS_CHILD, with caption, resize frame, popup and taskbar buttons removed.
	style = (style &^ uintptr(0x80000000|0x00cf0000|0x21000000)) | 0x40000000 | 0x04000000 | 0x02000000
	pinSetLong.Call(h, ^uintptr(15), style)
	ex, _, _ := pinGetLong.Call(h, ^uintptr(19))
	pinSetLong.Call(h, ^uintptr(19), ex&^uintptr(0x00000200|0x00020000|0x00000001))
	noBorder := uint32(0xfffffffe) // DWMWA_COLOR_NONE, Windows 11
	pinDwmAttribute.Call(h, 34, uintptr(unsafe.Pointer(&noBorder)), 4)
	pinSetParent.Call(h, host)
	parent, _, _ := pinGetParent.Call(h)
	if parent != host {
		pinDestroy.Call(host)
		return errors.New("browser could not be mounted")
	}
	ok, _, _ := pinSetProperty.Call(h, uintptr(unsafe.Pointer(pinOwnerProperty)), b.parent)
	if ok == 0 {
		pinDestroy.Call(host)
		return errors.New("window ownership unavailable")
	}
	b.host, b.hwnd = host, h
	pinSetPosition.Call(h, 0, 0, 0, 1, 1, 0x20|0x10|0x40)
	return nil
}

func (b *pinterestBrowser) place(r ui.Rect, deviceScale float32) {
	b.bounds, b.deviceScale = r, deviceScale
	if b.closed || b.hwnd == 0 || r.W < 1 || r.H < 1 || deviceScale <= 0 {
		return
	}
	scale := float64(deviceScale)
	pix := func(x float32) int { return int(math.Round(float64(x) * scale)) }
	next := [4]int{pix(r.X), pix(r.Y), pix(r.W), pix(r.H)}
	// Pointer movement repaints the canvas. An unchanged dock needs no
	// cross-process geometry calls; still notice a user-closed Chrome window.
	now := time.Now()
	if b.visible && b.last == next && now.Before(b.checkOwnerAt) {
		return
	}
	b.checkOwnerAt = now.Add(500 * time.Millisecond)
	if !b.owns() {
		b.hwnd = 0
		b.message = "Pinterest 网页已关闭"
		b.invalidate()
		return
	}
	if !b.visible || b.last != next {
		pinSetPosition.Call(b.host, 0, uintptr(next[0]), uintptr(next[1]), uintptr(next[2]), uintptr(next[3]), 0x0010|0x0040)
		t := b.trim
		// Chrome's accessibility bounds can include the last physical pixel
		// of its app frame. The clip host exposes only the web viewport.
		t[0] += pix(1)
		t[2] += pix(2)
		t[3] += pix(2)
		if b.view < 100 {
			gutter := pix(17)
			if pinSystemMetric.Find() == nil {
				if n, _, _ := pinSystemMetric.Call(2, uintptr(math.Round(96*scale))); n > 0 {
					gutter = int(n)
				}
			}
			t[2] += gutter
		}
		if !b.visible {
			b.scrollCommand("show")
		}
		pinSetPosition.Call(b.hwnd, 0, uintptr(-t[0]), uintptr(-t[1]), uintptr(next[2]+t[0]+t[2]), uintptr(next[3]+t[1]+t[3]), 0x14|0x40|0x4000)
		b.visible = true
		b.last = next
	}
}
func (b *pinterestBrowser) hide() {
	b.dragging = false
	if b.visible && b.host != 0 {
		b.scrollCommand("hide")
		pinShowWindow.Call(b.host, 0)
	}
	b.visible = false
}
func (b *pinterestBrowser) close() {
	b.closed = true
	if b.cancel != nil {
		b.cancel()
	}
	if b.stopProbe != nil {
		b.stopProbe()
	}
	if b.stopWatch != nil {
		b.stopWatch()
	}
	if b.owns() {
		pinShowWindow.Call(b.hwnd, 0)
		pinSetParent.Call(b.hwnd, 0)
		pinPostMessage.Call(b.hwnd, 0x10, 0, 0)
	}
	if b.host != 0 {
		pinDestroy.Call(b.host)
	}
	b.hwnd, b.host = 0, 0
}
func (b *pinterestBrowser) command(n int) {
	if b.owns() {
		pinPostMessage.Call(b.hwnd, 0x0319, b.hwnd, uintptr(n)<<16)
	}
}
