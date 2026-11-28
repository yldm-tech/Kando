package store

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io/fs"
	"path/filepath"
	"strings"
)

// Source is a NAS or a server the phone reaches itself, without the computer (docs/media-sources.md 10.2): its ID, random and kept for good, which the films downloaded from it are kept by (SourceOwner); its kind (SMB, SFTP or WebDAV, package nas); the name the user gave it; and the values of its fields that are not secret, by the same keys as the computer's media sources, so sources.json has the shape of the computer's. Its secrets are in the platform's secure store under SourceSecretAccount, never here.
type Source struct {
	ID     string            `json:"id"`
	Kind   string            `json:"kind"`
	Name   string            `json:"name"`
	Config map[string]string `json:"config,omitempty"`
}

// sourcesFile keeps the sources, beside desktops.json.
const sourcesFile = "sources.json"

// Sources are the sources the user added, in the order added.
func (s *Store) Sources() ([]Source, error) {
	var out []Source
	err := readJSON(filepath.Join(s.dir, sourcesFile), &out)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	return out, err
}

// SaveSources keeps sources, without their secrets, readable by the app alone.
func (s *Store) SaveSources(sources []Source) error {
	return writeJSON(filepath.Join(s.dir, sourcesFile), sources)
}

// SourceSecretAccount is the account the secret field key of source id is kept under in the platform's secure store.
func SourceSecretAccount(id, key string) string { return "source." + id + "." + key }

// sourcePrefix starts the owner of the films downloaded from a source (SourceOwner): a desktop's ID is hexadecimal (DesktopID), so it never starts so.
const sourcePrefix = "source-"

// SourceOwner is what the films downloaded from source id are kept under in place of a desktop's ID (Film.Desktop), and so the folder under films/ they are in, films/source-<id>/<film>: a desktop's films' layout, so the store, the player and the Library take such a film as one kept from a desktop no longer paired, which plays offline.
func SourceOwner(id string) string { return sourcePrefix + id }

// OwnerSource is the ID of the source owner names (SourceOwner), false for a desktop's ID.
func OwnerSource(owner string) (string, bool) { return strings.CutPrefix(owner, sourcePrefix) }

// SourceFilmID is the ID of the film of the file at path in a source: the first 16 hexadecimal digits of its path's SHA-256, so a file is downloaded once however often it is asked for.
func SourceFilmID(path string) string {
	sum := sha256.Sum256([]byte(path))
	return hex.EncodeToString(sum[:8])
}

// FromSource reports whether f was downloaded from a source rather than a desktop.
func (f Film) FromSource() bool { return f.Source != "" }
