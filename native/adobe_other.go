//go:build (!darwin || !cgo) && !windows

package main

import (
	"context"
	"errors"
)

func foregroundAdobe() string { return "" }
func exportAdobe(context.Context, string, string) ([]byte, error) {
	return nil, errors.New("当前平台不支持 Adobe 桌面快照")
}
