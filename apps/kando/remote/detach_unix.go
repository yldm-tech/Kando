//go:build !windows

package remote

import (
	"os/exec"
	"syscall"
)

// detach starts cmd in a session of its own, so the app outlives the command that started it.
func detach(cmd *exec.Cmd) { cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true} }
