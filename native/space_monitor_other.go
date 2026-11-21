//go:build !darwin || !cgo

package main

type spaceMonitor struct{}

func spacePermission() bool                                                        { return false }
func requestSpacePermission() bool                                                 { return false }
func startSpaceMonitor(trigger func(), ready func(), stopped func()) *spaceMonitor { return nil }
func (m *spaceMonitor) Stop()                                                      {}
