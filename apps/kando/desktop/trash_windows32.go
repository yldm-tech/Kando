//go:build windows && !(amd64 || arm64)

package desktop

import "errors"

// trashItem has no Recycle Bin here: Kando ships for 64-bit Windows only, and the IFileOperation calls of trash_windows.go are written for its calling convention.
func trashItem(string) error { return errors.ErrUnsupported }
