package main

import "syscall"

// readOnlyVolume reports whether the volume holding path is mounted read-only, as a disk image is.
func readOnlyVolume(path string) bool {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return false
	}
	return st.Flags&0x1 != 0 // MNT_RDONLY
}
