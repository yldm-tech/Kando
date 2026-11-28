package store

import (
	"errors"
	"io/fs"
	"path/filepath"
)

// lockedFile keeps the password-protected libraries the paired computers last listed as locked for the phone, beside desktops.json.
const lockedFile = "locked.json"

// LockedLibrary is a library a computer listed as locked for the phone (sync.Library.Locked): its ID, and its name, which a film kept before the phone kept library IDs (Film.LibraryID) is matched by.
type LockedLibrary struct {
	ID   string `json:"id"`
	Name string `json:"name,omitempty"`
}

// Locked are the libraries each computer, by its ID on the phone, last listed as locked. They are kept apart from desktops.json, so a computer unpaired still hides the films kept from its locked libraries, which stay on the phone (session.Unpair).
func (s *Store) Locked() (map[string][]LockedLibrary, error) {
	var out map[string][]LockedLibrary
	err := readJSON(filepath.Join(s.dir, lockedFile), &out)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	return out, err
}

// SaveLocked keeps the libraries each computer last listed as locked, readable by the app alone.
func (s *Store) SaveLocked(locked map[string][]LockedLibrary) error {
	return writeJSON(filepath.Join(s.dir, lockedFile), locked)
}
