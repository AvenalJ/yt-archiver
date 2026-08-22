//go:build !windows

package sysutil

import "os/exec"

// HideWindow is a no-op on non-Windows platforms.
func HideWindow(cmd *exec.Cmd) {}
