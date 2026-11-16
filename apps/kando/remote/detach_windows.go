//go:build windows

package remote

import (
	"os/exec"
	"syscall"
)

// createBreakawayFromJob is CREATE_BREAKAWAY_FROM_JOB, which the syscall package does not name.
const createBreakawayFromJob = 0x01000000

// detach starts cmd in a process group of its own, so the app outlives the console that started it, and out of the job its parent runs in, as Firefox puts the extension's host (package browserext) in one it kills when the extension disconnects (https://developer.mozilla.org/en-US/docs/Mozilla/Add-ons/WebExtensions/Native_messaging#closing_the_native_app).
func detach(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP | createBreakawayFromJob}
}

// detachInJob is detach without leaving the job, for a job that does not allow it, where CreateProcess refuses detach's flags.
var detachInJob = func(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP}
}
