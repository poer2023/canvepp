//go:build darwin && cgo

package main

/*
#cgo LDFLAGS: -framework AppKit
int canveppActiveDesignHost(void);
*/
import "C"

import (
	"context"
	"fmt"
	"os/exec"
)

func foregroundAdobe() string {
	switch C.canveppActiveDesignHost() {
	case 1:
		return "com.adobe.Photoshop"
	case 2:
		return "com.adobe.illustrator"
	}
	return ""
}

func exportAdobe(ctx context.Context, host, jsx string) ([]byte, error) {
	// Resolve each host's scripting dictionary and pass the JSX as UTF-8 text.
	return exec.CommandContext(ctx, "/usr/bin/osascript", "-e", "on run argv", "-e",
		"set src to (POSIX file (item 1 of argv)) as alias", "-e", "set scriptText to read src as «class utf8»",
		"-e", "with timeout of 870 seconds", "-e", fmt.Sprintf(`tell application id "%s" to do javascript scriptText`, host),
		"-e", "end timeout", "-e", "end run", jsx).CombinedOutput()
}
