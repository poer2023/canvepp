package main

import (
	_ "embed"

	"github.com/egoist/mygo"
)

//go:embed assets/canvepp-icon.png
var workspaceTrayIcon []byte

func newWorkspaceTray(id, title string, open, quit func()) (*mygo.Tray, error) {
	menu := mygo.NewMenu([]*mygo.MenuItem{
		{Label: "打开 Canvepp", Click: func(*mygo.MenuItem, *mygo.Window) { open() }},
		{Label: "退出 Canvepp", Click: func(*mygo.MenuItem, *mygo.Window) { quit() }},
	})
	tray, err := mygo.NewTray(mygo.TrayOptions{ID: id, Icon: workspaceTrayIcon, ToolTip: title, Menu: menu})
	if err == nil {
		tray.OnClick(open)
	}
	return tray, err
}
