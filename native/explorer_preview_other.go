//go:build !windows || (!amd64 && !arm64)

package main

import "github.com/egoist/mygo/ui"

type explorerPreview struct{}

func (a *desktop) startExplorerPreview()               {}
func (a *desktop) stopExplorerPreview()                {}
func (a *desktop) explorerPreviewSettings(*ui.Context) {}
func (a *desktop) refreshWindowsHotkey() bool          { return false }
