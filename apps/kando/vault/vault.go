// Package vault keeps a library's files encrypted on disk while the library is locked. On macOS a library with a password keeps its files in an AES-256 encrypted disk image inside its own folder, ImageName, which Kando attaches over that folder while the library is unlocked, so every path inside it stays the same, and detaches when it locks: the folder then holds only the image, which nothing opens without the password. Elsewhere a library's password is the app lock alone (Supported).
//
// The image says by itself that the library is encrypted (Has), so a library brought from another Mac, whose library.json knows nothing of its password, still shows locked. Nothing here keeps a password: each one goes to hdiutil on its standard input and is dropped.
package vault

import (
	"errors"
	"os"
	"path/filepath"
)

const (
	// ImageName is the encrypted image inside a library's folder, hidden as its name starts with a dot.
	ImageName = ".kando-vault.sparseimage"
	// stagingName is the image Create fills before it becomes ImageName: until then the library's files are still where they were, so a Create cut short leaves the library as it was, and the next Create removes it.
	stagingName = ".kando-vault-new.sparseimage"
	// mountName is the hidden folder inside the library's where Create, Open and Remove attach the image while they copy files into or out of it, the library's folder itself then holding the files they copy.
	mountName = ".kando-vault-mount"
	// markerName is the file at the root of an image's volume that tells it from any other volume mounted over a folder (Mounted).
	markerName = ".kando-vault"
)

var (
	// ErrUnsupported is returned where there is no encrypted image to keep a library in: everywhere but macOS.
	ErrUnsupported = errors.New("vault: not supported on this system")
	// ErrExists is returned by Create for a library that is encrypted already.
	ErrExists = errors.New("vault: the library is encrypted already")
	// ErrNotVault is returned for a library that has no image.
	ErrNotVault = errors.New("vault: the library is not encrypted")
	// ErrOpen is returned by ChangePassword and Remove while the image is attached over the library's folder: its file is hidden under it then.
	ErrOpen = errors.New("vault: the library is open")
	// ErrNotOpened is returned by Open when hdiutil would not attach the image, as for a wrong password; hdiutil says which only in the system's language, so this does not tell them apart.
	ErrNotOpened = errors.New("vault: the image could not be opened")
	// ErrNoSpace is returned by Create and Remove when the disk has no room for a second copy of the library's files, which they make before removing the first.
	ErrNoSpace = errors.New("vault: not enough free space")
)

// Has reports whether the library at dir keeps its files in an encrypted image: the image is in its folder, or attached over it.
func Has(dir string) bool {
	if !Supported() || dir == "" {
		return false
	}
	return Mounted(dir) || isFile(filepath.Join(dir, ImageName))
}

func isFile(path string) bool {
	info, err := os.Lstat(path)
	return err == nil && info.Mode().IsRegular()
}

// Progress hears how far copying a library's files into or out of its image has gone, in bytes of total.
type Progress func(done, total int64)
