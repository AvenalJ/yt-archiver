//go:build windows

package sysutil

import (
	"os/exec"
	"syscall"
)

// HideWindow ensures that child processes (such as python, yt-dlp, ffmpeg) execute silently in the background without spawning any console or terminal window.
func HideWindow(cmd *exec.Cmd) {
	if cmd == nil {
		return
	}
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.HideWindow = true
	cmd.SysProcAttr.CreationFlags |= 0x08000000 // CREATE_NO_WINDOW
}
