//go:build !windows

package remote

import (
	"fmt"
	"os"
	"syscall"
)

// privateFolder fails unless dir is the user's and no one else may write to it, so no other user can put a socket of their own where the app or a client expects Kando's.
func privateFolder(dir string) error {
	info, err := os.Stat(dir)
	if err != nil {
		return err
	}
	if st, ok := info.Sys().(*syscall.Stat_t); ok && int(st.Uid) != os.Getuid() {
		return fmt.Errorf("the socket's folder %s belongs to another user; set KANDO_SOCKET to a path in a folder of your own", dir)
	}
	if info.Mode().Perm()&0o022 != 0 {
		return fmt.Errorf("the socket's folder %s is writable by other users (mode %v); run chmod go-w on it, or set KANDO_SOCKET to a path in a folder only you can write to", dir, info.Mode().Perm())
	}
	return nil
}
