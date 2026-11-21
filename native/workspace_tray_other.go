//go:build !windows

package main

import "github.com/egoist/mygo"

func newWorkspaceTray(string, string, func(), func()) (*mygo.Tray, error) {
	return nil, nil
}
