//go:build darwin || linux

package media

import (
	"os"
	"syscall"
)

func identifyDir(path string) DirID {
	info, err := os.Stat(path)
	if err != nil {
		return DirID{}
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok || st == nil {
		return DirID{}
	}
	return DirID{dev: uint64(st.Dev), ino: uint64(st.Ino), set: true}
}
