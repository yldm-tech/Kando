//go:build !windows

package remote

import (
	"errors"
	"fmt"
	"io/fs"
	"log"
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

// keepToUser takes the write access of the user's group and of others away from dir when it is the user's, as Kando's own socket folder made under a umask such as 002 is writable by the group. A folder that is not there yet, which Listen makes for the user alone, or that is another user's, which Listen refuses, is left as it is.
func keepToUser(dir string) error {
	info, err := os.Stat(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if st, ok := info.Sys().(*syscall.Stat_t); ok && int(st.Uid) != os.Getuid() {
		return nil
	}
	if perm := info.Mode().Perm(); perm&0o022 != 0 {
		if err := os.Chmod(dir, perm&^0o022); err != nil {
			return fmt.Errorf("the socket's folder %s is writable by other users (mode %v) and could not be made yours alone: %w", dir, perm, err)
		}
		log.Printf("remote: made the socket's folder %s writable by you alone (it was %v)", dir, perm)
	}
	return nil
}
