//go:build windows && (amd64 || arm64)

package main

import (
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	explorerUser32  = windows.NewLazySystemDLL("user32.dll")
	explorerOle32   = windows.NewLazySystemDLL("ole32.dll")
	explorerShell   = windows.NewLazySystemDLL("shlwapi.dll")
	comCreate       = explorerOle32.NewProc("CoCreateInstance")
	comInitialize   = explorerOle32.NewProc("CoInitializeEx")
	getClientRect   = explorerUser32.NewProc("GetClientRect")
	getWindowDPI    = explorerUser32.NewProc("GetDpiForWindow")
	isChildWindow   = explorerUser32.NewProc("IsChild")
	queryAssoc      = explorerShell.NewProc("AssocQueryStringW")
	shellFileStream = explorerShell.NewProc("SHCreateStreamOnFileEx")
	previewCreate   = explorerUser32.NewProc("CreateWindowExW")
	previewPlace    = explorerUser32.NewProc("SetWindowPos")
	previewDestroy  = explorerUser32.NewProc("DestroyWindow")
)

const shellPreviewHandlerID = "{8895b1c6-b41f-4c1c-a562-0d564250836f}"

func explorerGUID(value string) windows.GUID {
	guid, err := windows.GUIDFromString(value)
	if err != nil {
		panic(err)
	}
	return guid
}

var (
	clsidShellWindows      = explorerGUID("{9BA05972-F6A8-11CF-A442-00A0C90A8F39}")
	iidShellWindows        = explorerGUID("{85CB6900-4D95-11CF-960C-0080C7F4EE85}")
	iidShellService        = explorerGUID("{6D5140C1-7436-11CE-8034-00AA006009FA}")
	sidTopLevelBrowser     = explorerGUID("{4C96BE40-915C-11CF-99D3-00AA004AE837}")
	iidShellBrowser        = explorerGUID("{000214E2-0000-0000-C000-000000000046}")
	iidFolderView2         = explorerGUID("{1AF3A467-214F-4298-908E-06B03E0B39F9}")
	iidShellPreviewHandler = explorerGUID(shellPreviewHandlerID)
	iidStreamInitialize    = explorerGUID("{B824B49D-22AC-4161-AC8A-9916E8FA3F7F}")
	iidFileInitialize      = explorerGUID("{B7D14566-0509-4CCE-A71F-0A554233BD9B}")
)

type explorerCOM struct {
	vtable *[64]uintptr
}

func (o *explorerCOM) ptr() uintptr { return uintptr(unsafe.Pointer(o)) }
func (o *explorerCOM) release() {
	if o != nil {
		syscall.SyscallN(o.vtable[2], o.ptr())
	}
}
func (o *explorerCOM) query(iid *windows.GUID) *explorerCOM {
	var out *explorerCOM
	hr, _, _ := syscall.SyscallN(o.vtable[0], o.ptr(), uintptr(unsafe.Pointer(iid)), uintptr(unsafe.Pointer(&out)))
	if int32(hr) < 0 {
		return nil
	}
	return out
}

func explorerCOMServer(clsid, iid *windows.GUID) *explorerCOM {
	const localServer = 0x4
	var out *explorerCOM
	hr, _, _ := comCreate.Call(uintptr(unsafe.Pointer(clsid)), 0, localServer, uintptr(unsafe.Pointer(iid)), uintptr(unsafe.Pointer(&out)))
	if int32(hr) < 0 {
		return nil
	}
	return out
}

func selectedExplorerPath(focus windows.HWND) string {
	// Called by a worker with its own STA. IShellWindows can expose multiple
	// tabs for one top-level HWND. Match the actual focused Shell view.
	win := explorerCOMServer(&clsidShellWindows, &iidShellWindows)
	if win == nil {
		return ""
	}
	defer win.release()
	var count int32
	hr, _, _ := syscall.SyscallN(win.vtable[7], win.ptr(), uintptr(unsafe.Pointer(&count))) // get_Count
	if int32(hr) < 0 {
		return ""
	}
	for i := int32(0); i < count; i++ {
		var dispatch *explorerCOM
		// VARIANT is a 24-byte aggregate passed by reference on Windows x64/ARM64.
		index := struct {
			vt       uint16
			reserved [3]uint16
			value    [2]uint64
		}{vt: 3, value: [2]uint64{uint64(i)}} // VT_I4
		hr, _, _ = syscall.SyscallN(win.vtable[8], win.ptr(), uintptr(unsafe.Pointer(&index)), uintptr(unsafe.Pointer(&dispatch))) // Item
		if int32(hr) < 0 || dispatch == nil {
			continue
		}
		name := selectedInShellView(dispatch, focus)
		dispatch.release()
		if name != "" {
			return name
		}
	}
	return ""
}

func selectedInShellView(dispatch *explorerCOM, focus windows.HWND) string {
	services := dispatch.query(&iidShellService)
	if services == nil {
		return ""
	}
	defer services.release()
	var browser *explorerCOM
	hr, _, _ := syscall.SyscallN(services.vtable[3], services.ptr(), uintptr(unsafe.Pointer(&sidTopLevelBrowser)),
		uintptr(unsafe.Pointer(&iidShellBrowser)), uintptr(unsafe.Pointer(&browser)))
	if int32(hr) < 0 || browser == nil {
		return ""
	}
	defer browser.release()
	var view *explorerCOM
	hr, _, _ = syscall.SyscallN(browser.vtable[15], browser.ptr(), uintptr(unsafe.Pointer(&view))) // QueryActiveShellView
	if int32(hr) < 0 || view == nil {
		return ""
	}
	defer view.release()
	var hwnd windows.HWND
	hr, _, _ = syscall.SyscallN(view.vtable[3], view.ptr(), uintptr(unsafe.Pointer(&hwnd))) // GetWindow
	if int32(hr) < 0 || hwnd == 0 || !windows.IsWindowVisible(hwnd) {
		return ""
	}
	if child, _, _ := isChildWindow.Call(uintptr(hwnd), uintptr(focus)); focus != hwnd && child == 0 {
		return ""
	}
	folder := view.query(&iidFolderView2)
	if folder == nil {
		return ""
	}
	defer folder.release()
	var items *explorerCOM
	hr, _, _ = syscall.SyscallN(folder.vtable[32], folder.ptr(), 0, uintptr(unsafe.Pointer(&items))) // GetSelection(FALSE)
	if int32(hr) < 0 || items == nil {
		return ""
	}
	defer items.release()
	var item *explorerCOM
	hr, _, _ = syscall.SyscallN(items.vtable[8], items.ptr(), 0, uintptr(unsafe.Pointer(&item))) // GetItemAt(0)
	if int32(hr) < 0 || item == nil {
		return ""
	}
	defer item.release()
	var path *uint16
	const fileSystemPath = 0x80058000
	hr, _, _ = syscall.SyscallN(item.vtable[5], item.ptr(), fileSystemPath, uintptr(unsafe.Pointer(&path)))
	if int32(hr) < 0 || path == nil {
		return ""
	}
	defer windows.CoTaskMemFree(unsafe.Pointer(path))
	return windows.UTF16PtrToString(path)
}

func explorerPreviewRect(hwnd windows.HWND) windows.Rect {
	var r windows.Rect
	getClientRect.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&r)))
	dpi, _, _ := getWindowDPI.Call(uintptr(hwnd))
	if dpi == 0 {
		dpi = 96
	}
	r.Top = min(r.Bottom, int32((filePreviewHeaderHeight+1)*dpi/96))
	return r
}

type shellFilePreview struct {
	parent windows.HWND
	host   uintptr
	view   *explorerCOM
}

func openShellFilePreview(hwnd windows.HWND, path string) *shellFilePreview {
	ext, err := windows.UTF16PtrFromString(strings.ToLower(filepath.Ext(path)))
	if err != nil || ext == nil || *ext == 0 {
		return nil
	}
	iid, _ := windows.UTF16PtrFromString(shellPreviewHandlerID)
	var name [80]uint16
	length := uint32(len(name))
	const assocShellExtension = 16
	hr, _, _ := queryAssoc.Call(0, assocShellExtension, uintptr(unsafe.Pointer(ext)), uintptr(unsafe.Pointer(iid)),
		uintptr(unsafe.Pointer(&name[0])), uintptr(unsafe.Pointer(&length)))
	if int32(hr) != 0 {
		return nil
	}
	clsid, err := windows.GUIDFromString(windows.UTF16ToString(name[:]))
	if err != nil {
		return nil
	}
	handler := explorerCOMServer(&clsid, &iidShellPreviewHandler)
	if handler == nil {
		return nil
	}
	if !initializeShellPreview(handler, path) {
		handler.release()
		return nil
	}
	// Preview HWNDs need their own clipped region below the file header.
	class := windows.StringToUTF16Ptr("STATIC")
	const clipChild = 0x40000000 | 0x04000000 | 0x02000000
	host, _, _ := previewCreate.Call(0, uintptr(unsafe.Pointer(class)), 0, clipChild, 0, 0, 1, 1, uintptr(hwnd), 0, 0, 0)
	if host == 0 {
		handler.release()
		return nil
	}
	p := &shellFilePreview{parent: hwnd, host: host, view: handler}
	r := p.place()
	hr, _, _ = syscall.SyscallN(handler.vtable[3], handler.ptr(), host, uintptr(unsafe.Pointer(&r))) // SetWindow
	if int32(hr) >= 0 {
		hr, _, _ = syscall.SyscallN(handler.vtable[5], handler.ptr()) // DoPreview
		if int32(hr) >= 0 {
			return p
		}
	}
	p.close()
	return nil
}

func initializeShellPreview(handler *explorerCOM, path string) bool {
	file, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return false
	}
	if initializer := handler.query(&iidStreamInitialize); initializer != nil {
		defer initializer.release()
		var stream *explorerCOM
		const readShared = 0x40 // STGM_READ | STGM_SHARE_DENY_NONE
		hr, _, _ := shellFileStream.Call(uintptr(unsafe.Pointer(file)), readShared, 0, 0, 0, uintptr(unsafe.Pointer(&stream)))
		if int32(hr) >= 0 && stream != nil {
			defer stream.release()
			hr, _, _ = syscall.SyscallN(initializer.vtable[3], initializer.ptr(), stream.ptr(), 0)
			return int32(hr) >= 0
		}
		return false
	}
	if initializer := handler.query(&iidFileInitialize); initializer != nil {
		defer initializer.release()
		hr, _, _ := syscall.SyscallN(initializer.vtable[3], initializer.ptr(), uintptr(unsafe.Pointer(file)), 0)
		return int32(hr) >= 0
	}
	return false
}

func (p *shellFilePreview) place() windows.Rect {
	r := explorerPreviewRect(p.parent)
	width, height := r.Right-r.Left, r.Bottom-r.Top
	const showWithoutActivation = 0x40 | 0x10
	previewPlace.Call(p.host, 0, uintptr(r.Left), uintptr(r.Top), uintptr(width), uintptr(height), showWithoutActivation)
	return windows.Rect{Right: width, Bottom: height}
}

func (p *shellFilePreview) resize() {
	r := p.place()
	syscall.SyscallN(p.view.vtable[4], p.view.ptr(), uintptr(unsafe.Pointer(&r)))
}

func (p *shellFilePreview) close() {
	syscall.SyscallN(p.view.vtable[6], p.view.ptr()) // Unload
	p.view.release()
	previewDestroy.Call(p.host)
}
