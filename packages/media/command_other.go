//go:build !windows

package media

import "os/exec"

// hideConsole does nothing outside Windows, where a program started from an app opens no window of its own.
func hideConsole(*exec.Cmd) {}
