//go:build !windows

package store

import "os"

// replace moves tmp over path: the systems other than Windows replace a file whoever has it open.
func replace(tmp, path string) error { return os.Rename(tmp, path) }

// removeAll removes path and everything in it: the systems other than Windows remove a file whoever has it open.
func removeAll(path string) error { return os.RemoveAll(path) }
