package plugins

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"hash"
	"path/filepath"
)

// hashFiles is the content hash of a pack: SHA-256 over its files in names' order (sorted), each as the length of its name, its name, its length and its bytes, the lengths as 8 bytes big-endian, so no two packs hash alike by moving bytes from one file or name to the next. The bytes read come out of b. A file that is not as long as it was when it was listed changed while it was read, and is an error.
func hashFiles(dir string, names []string, b *budget) (string, error) {
	h := sha256.New()
	for _, name := range names {
		if err := hashFile(h, filepath.Join(dir, name), name, b); err != nil {
			return "", err
		}
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func hashFile(h hash.Hash, path, name string, b *budget) error {
	f, err := openRegular(path)
	if err != nil {
		return err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return err
	}
	var n [8]byte
	binary.BigEndian.PutUint64(n[:], uint64(len(name)))
	h.Write(n[:])
	h.Write([]byte(name))
	binary.BigEndian.PutUint64(n[:], uint64(info.Size()))
	h.Write(n[:])
	read, err := b.copy(h, f)
	if err != nil {
		return err
	}
	if read != info.Size() {
		return fmt.Errorf("%w: %s changed while it was read", ErrUnsafePath, name)
	}
	return nil
}
