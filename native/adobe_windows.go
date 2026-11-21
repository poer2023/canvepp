package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sys/windows"
)

func foregroundAdobe() string {
	var pid uint32
	if _, err := windows.GetWindowThreadProcessId(windows.GetForegroundWindow(), &pid); err != nil {
		return ""
	}
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return ""
	}
	defer windows.CloseHandle(handle)
	path := make([]uint16, 32768)
	length := uint32(len(path))
	if windows.QueryFullProcessImageName(handle, 0, &path[0], &length) != nil {
		return ""
	}
	exe := windows.UTF16ToString(path[:length])
	switch strings.ToLower(filepath.Base(exe)) {
	case "photoshop.exe":
		return exe
	case "illustrator.exe":
		return "Illustrator.Application"
	}
	return ""
}

func exportAdobe(ctx context.Context, host, jsx string) ([]byte, error) {
	if strings.EqualFold(filepath.Base(host), "Photoshop.exe") {
		return nil, exportPhotoshop(ctx, host, jsx)
	}
	const script = `$ErrorActionPreference='Stop'
[Console]::OutputEncoding=[Text.UTF8Encoding]::new($false)
try {
$app=[Runtime.InteropServices.Marshal]::GetActiveObject($env:CANVEPP_ADOBE_HOST)
[void]$app.DoJavaScript([IO.File]::ReadAllText($env:CANVEPP_ADOBE_JSX))
} catch { [Console]::Error.WriteLine($_.Exception.GetBaseException().Message); exit 1 }`
	cmd := exec.CommandContext(ctx, "powershell.exe", "-NoProfile", "-NonInteractive", "-Command", script)
	cmd.Env = append(os.Environ(), "CANVEPP_ADOBE_HOST="+host, "CANVEPP_ADOBE_JSX="+jsx)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW}
	return cmd.CombinedOutput()
}

// Photoshop forwards a JSX file to its running editor, even on systems with
// an unusable COM type library. The launcher can exit before the export ends.
func exportPhotoshop(ctx context.Context, exe, jsx string) error {
	cmd := exec.Command(exe, jsx)
	if err := cmd.Start(); err != nil {
		return err
	}
	// The forwarding process can return exit code 1 even after Photoshop
	// accepts the script. Its lifetime is independent of the editor's.
	_ = cmd.Process.Release()
	ready := filepath.Join(filepath.Dir(jsx), "finished")
	poll := time.NewTicker(150 * time.Millisecond)
	defer poll.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-poll.C:
			if _, err := os.Stat(ready); !os.IsNotExist(err) {
				return err
			}
		}
	}
}
