//go:build !windows

package main

// fakeShell is only looked up on Windows (fake_tools_windows_test.go): elsewhere a script runs by its #!/bin/sh line.
const fakeShell = ""

// fakeScriptShim runs no script: outside Windows the fake tools' scripts run by their #! line (writeFakeScript).
func fakeScriptShim() (int, bool) { return 0, false }
