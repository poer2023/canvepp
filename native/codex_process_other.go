//go:build !windows

package main

import "os/exec"

func configureCodexProcess(*exec.Cmd) {}
