//go:build darwin && cgo

package main

/*
#cgo LDFLAGS: -framework AppKit
#include <stdint.h>
void canveppPrepareAssistantWindow(uintptr_t handle);
*/
import "C"

import "github.com/egoist/mygo"

func prepareAssistantWindow(win *mygo.Window) {
	C.canveppPrepareAssistantWindow(C.uintptr_t(win.NativeHandle()))
}
