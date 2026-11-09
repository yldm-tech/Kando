//go:build !windows

package cli

// attachConsole does nothing outside Windows, where the app is started with the terminal's standard streams.
func attachConsole(args []string) {}
