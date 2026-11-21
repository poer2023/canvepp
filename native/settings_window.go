package main

import (
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

const (
	settingsWindowTitle = "Canvepp 设置"
	settingsMinWidth    = 640
	settingsMinHeight   = 430
)

func (a *desktop) toggleSettings() {
	if a.closing || a.closed {
		return
	}
	if a.settingsOpen {
		if w := a.settingsWin; w != nil && w.IsMinimized() {
			w.Restore()
			w.Show()
			return
		}
		a.closeSettings()
		return
	}
	a.settingsOpen = true
	if a.win == nil {
		return
	}
	bounds := a.settingsWindowBounds()
	opts := mygo.WindowOptions{
		Title: settingsWindowTitle, Parent: a.win, Hidden: true, AutoHideMenuBar: true,
		X: bounds.X, Y: bounds.Y, Width: bounds.Width, Height: bounds.Height,
		MinWidth: settingsMinWidth, MinHeight: settingsMinHeight,
		TitleBarStyle: mygo.TitleBarHidden, Vibrancy: a.windowMaterial(), Content: ui.View(a.settingsView),
	}
	w := mygo.NewWindow(opts)
	a.settingsWin = w
	if bounds == a.settingsBounds && bounds.X == 0 && bounds.Y == 0 {
		// The constructor interprets (0, 0) as centering, including on Windows.
		w.SetPosition(0, 0)
	}
	w.OnClose(func(_ *mygo.CloseEvent) {
		if !w.IsMinimized() {
			a.settingsBounds = w.Bounds()
		}
	})
	w.OnClosed(func() {
		a.settingsWin = nil
		a.closeSettings()
		if !a.closing && !a.closed && a.win.IsVisible() {
			a.win.Focus()
		}
	})
	a.checkAccount(false)
	w.Show()
	a.win.Invalidate()
}

func (a *desktop) settingsWindowBounds() mygo.Rectangle {
	owner := a.win.Bounds()
	b := a.settingsBounds
	if b.Width > 0 {
		work := mygo.Screen.DisplayMatching(b).WorkArea
		// A saved title bar may have been on a disconnected display.
		if b.X < work.X+work.Width-96 && b.X+b.Width > work.X+96 &&
			b.Y >= work.Y && b.Y < work.Y+work.Height-32 {
			return b
		}
	}
	work := mygo.Screen.DisplayMatching(owner).WorkArea
	b = mygo.Rectangle{Width: 850, Height: 704}
	if work.Width > 0 && work.Height > 0 {
		b.Width = min(b.Width, max(settingsMinWidth, work.Width))
		b.Height = min(b.Height, max(settingsMinHeight, work.Height))
		b.X = max(work.X, min(owner.X+(owner.Width-b.Width)/2, work.X+work.Width-b.Width))
		b.Y = max(work.Y, min(owner.Y+(owner.Height-b.Height)/2, work.Y+work.Height-b.Height))
	}
	return b
}

func (a *desktop) closeSettings() {
	if w := a.settingsWin; w != nil {
		w.Close()
		return
	}
	if !a.settingsOpen && a.funding.keyDraft == "" {
		return
	}
	a.settingsOpen = false
	a.funding.closeKeyEditor()
	if a.win != nil && !a.win.IsDestroyed() {
		a.win.Invalidate()
	}
}

func (a *desktop) publishSettings(fn func()) {
	a.publish(func() {
		fn()
		if a.settingsWin != nil {
			a.settingsWin.Invalidate()
		}
	})
}
