//go:build windows

package remote

import (
	"os/exec"
	"syscall"
)

// detach starts cmd in a process group of its own, so the app outlives the console that started it.
func detach(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP}
}
