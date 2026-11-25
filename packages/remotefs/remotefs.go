// Package remotefs is how Kando reaches files kept on another machine: the Provider every place files live is behind, a folder of a NAS or a server over SMB, SFTP or WebDAV (Remote, OpenRemote), why opening one failed (Diagnose), and a loopback proxy (Proxy) that serves a provider's file, or any Reader's, to the system player. It imports no package of the apps, so the desktop's storage locations and media sources and the phone use the same protocol code.
package remotefs

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"
	"strings"
	"time"
)

// Reader is what serving and copying a file needs of where it lives (Handler, Proxy, and a copy to this computer): a Provider, or anything else that reads files by path the way a Provider does, as a media source does. Paths follow Provider's rules.
type Reader interface {
	// Stat is the file or folder at p, ErrNotExist when there is none.
	Stat(ctx context.Context, p string) (Info, error)
	// Open reads the file at p from offset, length bytes of it or, with length -1, to its end; an offset at or past the end reads nothing. The caller closes it.
	Open(ctx context.Context, p string, offset, length int64) (io.ReadCloser, error)
}

// Provider keeps files under its root. Every path is slash-separated and relative to the root, "" being the root itself; a path that leaves the root (an absolute path, one with a backslash, or one that climbs above the root with "..") is ErrInvalidPath.
type Provider interface {
	// Reader stats and opens files: Stat is ErrNotExist for a path that holds nothing.
	Reader
	// List is what lies directly in the folder dir, in no set order; ErrNotExist when there is no such folder.
	List(ctx context.Context, dir string) ([]Info, error)
	// Write stores size bytes read from r as the file at p, making the folders above it, and replaces what was there only once all of them are stored where the backend can (both providers here can). r giving fewer than size bytes is an error and stores nothing.
	Write(ctx context.Context, p string, r io.Reader, size int64) error
	// Remove removes the file or the empty folder at p; nothing there is no error, a folder that is not empty is ErrNotEmpty.
	Remove(ctx context.Context, p string) error
	// Move moves the file or folder at from to to, making the folders above to, and replaces a file at to.
	Move(ctx context.Context, from, to string) error
	// MkdirAll makes the folder at dir and those above it.
	MkdirAll(ctx context.Context, dir string) error
}

// Info is a file or folder as a provider tells it. ETag is the store's tag for a file's content when it has one; Modified may be zero for a folder.
type Info struct {
	Name     string
	Size     int64
	Modified time.Time
	Dir      bool
	ETag     string
}

// ErrNotExist is the error for a path that holds nothing. It is also fs.ErrNotExist.
var ErrNotExist = fmt.Errorf("storage: %w", fs.ErrNotExist)

// ErrInvalidPath is the error for a path that leaves the provider's root, or for the root itself where a file is needed. It is also fs.ErrInvalid.
var ErrInvalidPath = fmt.Errorf("storage: path outside the root: %w", fs.ErrInvalid)

// ErrIsDir is Open's error for a folder.
var ErrIsDir = errors.New("storage: is a folder")

// ErrNotEmpty is Remove's error for a folder that still holds something.
var ErrNotEmpty = errors.New("storage: folder is not empty")

// TempPrefix starts the name of the file a provider's Write, or a copy to this computer, writes into before it renames it into place; List does not show such files.
const TempPrefix = ".kando-write-"

// CleanPath is p made plain, "a/./b/../c" as "a/c" and the root as "", or ErrInvalidPath when it leaves the root.
func CleanPath(p string) (string, error) {
	if strings.HasPrefix(p, "/") || strings.ContainsAny(p, "\\\x00") {
		return "", fmt.Errorf("%q: %w", p, ErrInvalidPath)
	}
	p = path.Clean(p)
	if p == "." {
		return "", nil
	}
	if p == ".." || strings.HasPrefix(p, "../") {
		return "", fmt.Errorf("%q: %w", p, ErrInvalidPath)
	}
	return p, nil
}

// CleanFile is CleanPath for a path that must name something under the root rather than the root itself.
func CleanFile(p string) (string, error) {
	p, err := CleanPath(p)
	if err == nil && p == "" {
		return "", fmt.Errorf("the root: %w", ErrInvalidPath)
	}
	return p, err
}
