// Package mediasource is Kando's media sources (docs/media-sources.md): the NAS shares, servers and media servers the user adds to browse and import films from, either copied into the library or indexed and streamed from where they are. It holds the kinds of source a Registry knows, built in (SMB, SFTP) or contributed by a script plugin, the read-only Driver every kind reads a source with, a video's address on a source (Ref), whether a host is on the local network (Local), and where an indexed film's companion files go in the library (Placeholder). It imports no interface package.
package mediasource

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"io"
	"strings"
	"time"

	"kando/packages/remotefs"
)

// Driver reads one media source. Paths are slash-separated and relative to the source's root, "" being the root. It has no method that writes, removes or moves anything, so nothing Kando does through it changes the user's files on the source.
type Driver interface {
	// List is what lies directly in the folder dir, in no set order; ErrNotExist when there is no such folder.
	List(ctx context.Context, dir string) ([]Entry, error)
	// Stat is the file or folder at p, ErrNotExist when there is none.
	Stat(ctx context.Context, p string) (Entry, error)
	// Open reads p from offset, length bytes or to its end with -1, as remotefs.Provider.Open does.
	Open(ctx context.Context, p string, offset, length int64) (io.ReadCloser, error)
	// Close ends the driver's connection to the source.
	Close() error
}

// CoverDriver is a Driver that can read a video's own cover picture from its source, as a media server's artwork (Plex, Jellyfin, Emby). ErrNotExist when the video has none.
type CoverDriver interface {
	Cover(ctx context.Context, p string) ([]byte, error)
}

// Entry is a file or folder of a source. Version is what tells a changed file from the one imported: an ETag, or size and time.
type Entry struct {
	Name     string
	Size     int64
	Modified time.Time
	Dir      bool
	Version  string
}

// ErrNotExist is a Driver's error for a path that holds nothing, the same as remotefs's, so what serves and copies a storage location's files (remotefs.Handler, storage.Get) tells it apart alike. It is also fs.ErrNotExist.
var ErrNotExist = remotefs.ErrNotExist

// ErrInvalidPath is the error for a path that leaves the source's root, the same as remotefs's. It is also fs.ErrInvalid.
var ErrInvalidPath = remotefs.ErrInvalidPath

// Source is a media source the user added: a connection to one share, folder or server. ID is random (NewID), made as the source is added and kept after, which films imported from it know it by, so renaming the source or moving it to another address leaves them its own. Kind is the ID of its Kind in the Registry. Name is the user's name for it, as 客厅 NAS. Config holds the values of its kind's fields that are not secret (Field), by key; the secret ones are kept in the system's secure storage, never here.
type Source struct {
	ID     string
	Kind   string
	Name   string
	Config map[string]string
}

// idLength is the length of a source's ID: 16 hexadecimal digits, 64 random bits.
const idLength = 16

// NewID is a new source's ID, 16 random lowercase hexadecimal digits.
func NewID() string {
	var b [idLength / 2]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

// ValidID reports whether id has the shape NewID gives: 16 lowercase hexadecimal digits.
func ValidID(id string) bool {
	if len(id) != idLength {
		return false
	}
	for _, c := range []byte(id) {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

// addressKeys are the fields that say where a source's files are, whatever the user and the secrets: its server, port, share, folder, and a plugin's source's endpoint.
var addressKeys = []string{SMBHost, SMBPort, SMBShare, SMBRoot, EndpointKey}

// SameAddress reports whether a and b reach the same files: the same kind, server (ignoring case), port, share, folder and endpoint, whoever signs in. A source added back at the address of one removed while films were indexed from it takes its ID back by it, so those films play from it again.
func SameAddress(a, b Source) bool {
	if a.Kind != b.Kind {
		return false
	}
	for _, key := range addressKeys {
		x := strings.Trim(strings.TrimSpace(a.Config[key]), `/\`)
		y := strings.Trim(strings.TrimSpace(b.Config[key]), `/\`)
		if key == SMBHost {
			x, y = strings.ToLower(x), strings.ToLower(y)
		}
		if x != y {
			return false
		}
	}
	return true
}
