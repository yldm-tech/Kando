//go:build !darwin

package clitool

// readOnlyVolume reports whether the volume holding path is mounted read-only; only macOS has disk images under /Volumes, so elsewhere none counts.
func readOnlyVolume(string) bool { return false }
