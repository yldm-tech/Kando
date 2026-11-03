//go:build !darwin && !linux

package media

// DirID is the identity of a directory. This platform does not follow renames.
type DirID struct {
	set bool
}

// Parts returns the device and inode. This platform has no identity.
func (DirID) Parts() (dev, ino uint64, ok bool) { return 0, 0, false }

// MakeDirID rebuilds a saved identity. This platform always returns the zero identity.
func MakeDirID(uint64, uint64) DirID { return DirID{} }

// IdentifyDir reads the identity of path. This platform always returns the zero identity.
func IdentifyDir(string) DirID { return DirID{} }

// FindDir looks for id under parent. This platform does not follow renames.
func FindDir(string, DirID) (string, bool) { return "", false }

func identifyDir(string) DirID { return DirID{} }

func findDirID(string, DirID) (string, bool) { return "", false }
