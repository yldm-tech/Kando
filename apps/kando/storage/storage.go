// Package storage is where a finished film's files can live other than the library folder: a folder on this computer (Local), a folder on a NAS or a server (remotefs.Remote), or a bucket of an object store (ObjectStore), each behind the same remotefs.Provider, and a Pool that keeps them opened and serves their files to the system player through a remotefs.Proxy. Processing stays on this computer; a provider only keeps and hands back files.
package storage

import (
	"context"
	"time"
)

// Linker is a Provider that can hand out a link anyone reads the file at p with by a plain GET, without Kando in between, good for ttl: an object store presigns one.
type Linker interface {
	Link(ctx context.Context, p string, ttl time.Duration) (string, error)
}
