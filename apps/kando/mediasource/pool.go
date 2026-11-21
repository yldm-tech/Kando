package mediasource

import (
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"sync"
	"time"

	"kando/apps/kando/storage"
)

// ErrSourceGone is the Pool's error for a source it does not hold: removed from the settings, or of a kind not registered.
var ErrSourceGone = errors.New("mediasource: the source is not in the settings")

// PoolOpenTimeout bounds connecting to a source for a Pool's Driver when ctx sets no deadline of its own, as storage.PoolOpenTimeout does for a storage location.
const PoolOpenTimeout = 30 * time.Second

// Opener connects to s, a source of kind k: it reads the source's secrets where they are kept and opens the kind with them.
type Opener func(ctx context.Context, k Kind, s Source) (Driver, error)

// Pool keeps one connection to each source in use, so the files a browse lists and the videos a job copies share it. Set gives it the sources as the settings hold them and how to connect to them, which drops the connection of one that changed or went; Driver connects on first use. It is safe for any goroutine: the engine's sets it, a job's reads through it. The zero Pool holds nothing.
type Pool struct {
	mu      sync.Mutex
	open    Opener
	sources map[string]poolSource
	conns   map[string]Driver
}

// poolSource is a source the Pool holds, with its kind.
type poolSource struct {
	source Source
	kind   Kind
}

// Set makes the Pool hold sources, each with its kind as kinds gives it, and connect to them with open; a source whose kind is not registered is left out. A connection is closed when its source is gone or changed, as a new share needs a new one.
func (p *Pool) Set(sources []Source, kinds func(id string) (Kind, bool), open Opener) {
	held := map[string]poolSource{}
	for _, s := range sources {
		if k, ok := kinds(s.Kind); ok {
			held[s.ID] = poolSource{source: s, kind: k}
		}
	}
	p.mu.Lock()
	p.open = open
	var stale []Driver
	for id, d := range p.conns {
		now, ok := held[id]
		if before := p.sources[id]; !ok || !sameSource(before.source, now.source) {
			stale = append(stale, d)
			delete(p.conns, id)
		}
	}
	p.sources = held
	p.mu.Unlock()
	for _, d := range stale {
		_ = d.Close()
	}
}

// sameSource reports whether a and b connect the same way.
func sameSource(a, b Source) bool {
	return a.ID == b.ID && a.Kind == b.Kind && maps.Equal(a.Config, b.Config)
}

// Forget closes the connection to the source id, so the next Driver connects again, as after a failure that a new connection may not have, or once its secrets changed.
func (p *Pool) Forget(id string) {
	p.mu.Lock()
	d := p.conns[id]
	delete(p.conns, id)
	p.mu.Unlock()
	if d != nil {
		_ = d.Close()
	}
}

// Source is the source of ID id the Pool holds, false when it holds none.
func (p *Pool) Source(id string) (Source, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	s, ok := p.sources[id]
	return s.source, ok
}

// Driver is the connection to the source id, made the first time it is asked for, within PoolOpenTimeout; ErrSourceGone when the Pool holds no such source. Two callers that ask at once may both connect; the second connection is then closed and the first kept.
func (p *Pool) Driver(ctx context.Context, id string) (Driver, error) {
	p.mu.Lock()
	held, ok := p.sources[id]
	d, open := p.conns[id], p.open
	p.mu.Unlock()
	if !ok || open == nil {
		return nil, fmt.Errorf("%w: %s", ErrSourceGone, id)
	}
	if d != nil {
		return d, nil
	}
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, PoolOpenTimeout)
		defer cancel()
	}
	d, err := open(ctx, held.kind, held.source)
	if err != nil {
		return nil, err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if now, ok := p.sources[id]; !ok || !sameSource(now.source, held.source) {
		// The source changed or went while it connected: the connection is to what it was.
		_ = d.Close()
		return nil, fmt.Errorf("%w: %s", ErrSourceGone, id)
	}
	if have := p.conns[id]; have != nil {
		_ = d.Close()
		return have, nil
	}
	if p.conns == nil {
		p.conns = map[string]Driver{}
	}
	p.conns[id] = d
	return d, nil
}

// Close closes every connection; Driver connects again after it.
func (p *Pool) Close() {
	p.mu.Lock()
	conns := p.conns
	p.conns = nil
	p.mu.Unlock()
	for _, d := range conns {
		_ = d.Close()
	}
}

// StorageReader is d as storage reads a file (storage.Reader), so storage.Get copies a source's file into the library and storage.Handler serves one.
func StorageReader(d Driver) storage.Reader { return storageReader{d} }

type storageReader struct{ d Driver }

func (r storageReader) Stat(ctx context.Context, p string) (storage.Info, error) {
	e, err := r.d.Stat(ctx, p)
	if err != nil {
		return storage.Info{}, err
	}
	return storage.Info{Name: e.Name, Size: e.Size, Modified: e.Modified, Dir: e.Dir, ETag: e.Version}, nil
}

func (r storageReader) Open(ctx context.Context, p string, offset, length int64) (io.ReadCloser, error) {
	return r.d.Open(ctx, p, offset, length)
}
