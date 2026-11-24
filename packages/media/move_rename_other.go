//go:build !windows

package media

import "os"

// renameHeld renames from to to: the systems other than Windows rename a file or a folder whoever has it open.
func renameHeld(from, to string) error { return os.Rename(from, to) }
