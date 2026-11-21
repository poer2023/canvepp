//go:build !windows

package main

import "github.com/egoist/mygo/ui"

type pinterestBrowser struct{ message string }

func pinterestBrowserAvailable() bool                       { return false }
func (*desktop) newPinterestBrowser(bool) *pinterestBrowser { return nil }
func (*pinterestBrowser) place(ui.Rect, float32)            {}
func (*pinterestBrowser) hide()                             {}
func (*pinterestBrowser) close()                            {}
func (*pinterestBrowser) command(int)                       {}

func (*pinterestBrowser) owns() bool { return false }

func (*pinterestBrowser) openIfNeeded(_ *desktop, login bool, open func(bool)) { open(login) }

func (*pinterestBrowser) scrollbar(*ui.Context) {}
