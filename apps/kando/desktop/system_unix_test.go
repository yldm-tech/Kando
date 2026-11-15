//go:build !windows

package desktop

import "os/exec"

// sysCommandLine is "" outside Windows, where a command runs by its arguments alone.
func sysCommandLine(*exec.Cmd) string { return "" }
