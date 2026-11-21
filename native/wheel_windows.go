package main

import (
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

// MyGo delivers wheel travel in DIPs, scaled by the Windows "lines per notch"
// setting. Undo that conversion for zoom, including fractional wheel messages.
var wheelDIPsPerNotch = sync.OnceValue(func() float64 {
	var lines uint32
	spi := windows.NewLazySystemDLL("user32.dll").NewProc("SystemParametersInfoW")
	ok, _, _ := spi.Call(0x0068, 0, uintptr(unsafe.Pointer(&lines)), 0)
	if ok == 0 || lines == 0 || lines >= 100 {
		lines = 3
	}
	return 33 * float64(lines)
})

func canvasWheelNotches(dy float64, precise bool) float64 { return dy / wheelDIPsPerNotch() }
