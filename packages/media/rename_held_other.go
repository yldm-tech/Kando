//go:build !windows

package media

import (
	"os"
	"time"
)

// renameWithin renames from to to at once: the systems other than Windows rename a file or a folder whoever has it open.
func renameWithin(from, to string, _ time.Duration) error { return os.Rename(from, to) }
