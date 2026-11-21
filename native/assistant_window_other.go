//go:build !darwin || !cgo

package main

import "github.com/egoist/mygo"

func prepareAssistantWindow(*mygo.Window) {}
