//go:build windows

package remote

// privateFolder accepts any folder on Windows, where the socket's default folder, under %AppData%, is the user's own by its access list, which Go's os package does not read.
func privateFolder(string) error { return nil }
