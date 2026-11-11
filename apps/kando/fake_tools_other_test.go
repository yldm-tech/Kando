//go:build !windows

package main

import "testing"

// fakeShell is only looked up on Windows (fake_tools_windows_test.go): elsewhere a script runs by its #!/bin/sh line.
const fakeShell = ""

// fakeScriptShim runs no script: outside Windows the fake tools' scripts run by their #! line (writeFakeScript).
func fakeScriptShim() (int, bool) { return 0, false }

// fakeScriptProgram is only used on Windows (fake_tools_windows_test.go).
func fakeScriptProgram(t *testing.T) string {
	t.Fatal("fake scripts run by their #! line outside Windows")
	return ""
}

// fakeToolsCleanup has nothing to remove outside Windows.
func fakeToolsCleanup() {}
