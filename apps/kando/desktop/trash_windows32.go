//go:build windows && !(amd64 || arm64)

package desktop

import "errors"

// trashItem has no Recycle Bin here: Kando ships for 64-bit Windows only, and SHFileOperationW's structure is packed differently on 32-bit Windows (trash_windows.go).
func trashItem(string) error { return errors.ErrUnsupported }
