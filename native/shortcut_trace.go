package main

import (
	"encoding/json"
	"os"
	"sync"
	"time"
)

// Opt-in, bounded diagnostics. Never record key codes, characters or text.
var shortcutDiagnostics struct {
	sync.Mutex
	file  *os.File
	start time.Time
}

func startShortcutTrace(path string) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	shortcutDiagnostics.Lock()
	shortcutDiagnostics.file, shortcutDiagnostics.start = f, time.Now()
	shortcutDiagnostics.Unlock()
	time.AfterFunc(2*time.Minute, stopShortcutTrace)
	return nil
}

func stopShortcutTrace() {
	shortcutDiagnostics.Lock()
	defer shortcutDiagnostics.Unlock()
	if shortcutDiagnostics.file != nil {
		_ = shortcutDiagnostics.file.Close()
		shortcutDiagnostics.file = nil
	}
}

func traceShortcut(stage string, eventTime time.Duration) {
	shortcutDiagnostics.Lock()
	defer shortcutDiagnostics.Unlock()
	if shortcutDiagnostics.file == nil {
		return
	}
	_ = json.NewEncoder(shortcutDiagnostics.file).Encode(struct {
		Stage     string `json:"stage"`
		ElapsedMS int64  `json:"elapsedMs"`
		EventMS   int64  `json:"eventMs,omitempty"`
	}{stage, time.Since(shortcutDiagnostics.start).Milliseconds(), eventTime.Milliseconds()})
}
