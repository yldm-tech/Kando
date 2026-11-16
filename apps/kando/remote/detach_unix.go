//go:build !windows

package remote

import (
	"os/exec"
	"syscall"
)

// detach starts cmd in a session of its own, so the app outlives the command that started it, and the signals a browser ends its extension's host with (package browserext) do not reach it.
func detach(cmd *exec.Cmd) { cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true} }

// detachInJob is nil: only Windows puts a program in a job.
var detachInJob func(*exec.Cmd)
