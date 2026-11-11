//go:build darwin || linux || windows

package media

import (
	"os"
	"path/filepath"
)

// DirID is the identity of a directory that survives a rename on the same volume: its device and inode, or on Windows its volume serial number and file index (identifyDir).
type DirID struct {
	dev, ino uint64
	set      bool
}

// Parts returns the device and inode. ok is false for the zero identity.
func (id DirID) Parts() (dev, ino uint64, ok bool) { return id.dev, id.ino, id.set }

// MakeDirID rebuilds an identity saved earlier. A zero pair is not an identity.
func MakeDirID(dev, ino uint64) DirID {
	if dev == 0 && ino == 0 {
		return DirID{}
	}
	return DirID{dev: dev, ino: ino, set: true}
}

// IdentifyDir reads the identity of path. An unreadable path has the zero identity.
func IdentifyDir(path string) DirID { return identifyDir(path) }

// FindDir returns the directory under parent with id. A zero id matches nothing.
func FindDir(parent string, id DirID) (string, bool) { return findDirID(parent, id) }

func findDirID(parent string, id DirID) (string, bool) {
	if !id.set {
		return "", false
	}
	entries, err := os.ReadDir(parent)
	if err != nil {
		return "", false
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		path := filepath.Join(parent, entry.Name())
		got := identifyDir(path)
		if got.set && got.dev == id.dev && got.ino == id.ino {
			return path, true
		}
	}
	return "", false
}
