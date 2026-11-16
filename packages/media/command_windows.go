//go:build windows

package media

import (
	"os/exec"
	"syscall"
)

// createNoWindow is the process creation flag CREATE_NO_WINDOW, which package syscall does not name: the console program starts without a console.
const createNoWindow = 0x08000000

// hideConsole makes cmd start with no console window, and its window, should it make one, hidden.
func hideConsole(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.HideWindow = true
	cmd.SysProcAttr.CreationFlags |= createNoWindow
}
