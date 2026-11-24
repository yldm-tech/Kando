//go:build !windows

package media

import "os"

// replace moves tmp over path: the systems other than Windows replace a file whoever has it open.
func replace(tmp, path string) error { return os.Rename(tmp, path) }
