package mediasource

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"maps"
	"net/http"
	"sync"
	"time"

	"kando/packages/remotefs"
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
	// connecting is the connection being made to each source (connect), which every Driver that asks meanwhile waits for, as long as its own context lets it.
	connecting map[string]*poolConnect
	// proxy serves the sources' videos on the loopback address to the system's player and ffmpeg (URL), started the first time one is asked for.
	proxy *remotefs.Proxy
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

// Driver is the connection to the source id, made the first time it is asked for; ErrSourceGone when the Pool holds no such source. The connection is made once however many ask for it meanwhile, apart from any of them (connect), within PoolOpenTimeout, and each waits for it only as long as ctx lets it: connecting may take longer than a request can wait, as while macOS asks the user whether Kando may read the source's secrets from the keychain, and a request that gives up neither cuts the connecting short nor starts another, so the next one finds the connection made.
func (p *Pool) Driver(ctx context.Context, id string) (Driver, error) {
	p.mu.Lock()
	held, ok := p.sources[id]
	d, open := p.conns[id], p.open
	if !ok || open == nil {
		p.mu.Unlock()
		return nil, fmt.Errorf("%w: %s", ErrSourceGone, id)
	}
	if d != nil {
		p.mu.Unlock()
		return d, nil
	}
	c := p.connecting[id]
	if c == nil || !sameSource(c.held.source, held.source) {
		c = &poolConnect{held: held, done: make(chan struct{})}
		if p.connecting == nil {
			p.connecting = map[string]*poolConnect{}
		}
		p.connecting[id] = c
		go p.connect(id, open, c)
	}
	p.mu.Unlock()
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, PoolOpenTimeout)
		defer cancel()
	}
	select {
	case <-c.done:
		return c.d, c.err
	case <-ctx.Done():
		return nil, fmt.Errorf("mediasource: still connecting to %s: %w", id, ctx.Err())
	}
}

// poolConnect is a connection being made to the source held: the driver, or why there is none, once done is closed.
type poolConnect struct {
	held poolSource
	done chan struct{}
	d    Driver
	err  error
}

// connect makes the connection c to the source id with open, within PoolOpenTimeout, and keeps it as the source's while the source is still as it was; on its own goroutine, so no Driver waiting for it holds it up or cuts it short.
func (p *Pool) connect(id string, open Opener, c *poolConnect) {
	ctx, cancel := context.WithTimeout(context.Background(), PoolOpenTimeout)
	defer cancel()
	d, err := open(ctx, c.held.kind, c.held.source)
	p.mu.Lock()
	if p.connecting[id] == c {
		delete(p.connecting, id)
	}
	now, ok := p.sources[id]
	switch {
	case err != nil:
	case !ok || !sameSource(now.source, c.held.source):
		// The source changed or went while it connected: the connection is to what it was.
		_ = d.Close()
		d, err = nil, fmt.Errorf("%w: %s", ErrSourceGone, id)
	case p.conns[id] != nil:
		_ = d.Close()
		d = p.conns[id]
	default:
		if p.conns == nil {
			p.conns = map[string]Driver{}
		}
		p.conns[id] = d
	}
	p.mu.Unlock()
	c.d, c.err = d, err
	close(c.done)
}

// Close closes every connection and the loopback server; Driver connects again after it, and URL starts a new server.
func (p *Pool) Close() {
	p.mu.Lock()
	conns, proxy := p.conns, p.proxy
	p.conns, p.proxy = nil, nil
	p.mu.Unlock()
	for _, d := range conns {
		_ = d.Close()
	}
	if proxy != nil {
		_ = proxy.Close()
	}
}

// URL is the address the system's player and ffmpeg read the video at path of the source id at, http://127.0.0.1:port/token/name, served by the Pool's loopback server (remotefs.Proxy) through its connection to the source, made anew when the one it had failed; once the file was found there (ErrNotExist when it is not). Each call is given back with Revoke once its holder moved on, as the player does when the page moves on from the film and a job when it ends.
func (p *Pool) URL(ctx context.Context, id, path string) (string, error) {
	r := p.Reader(id)
	info, err := r.Stat(ctx, path)
	if err != nil {
		return "", err
	}
	if info.Dir {
		return "", fmt.Errorf("%s: %w", path, ErrNotExist)
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.proxy == nil {
		proxy, err := remotefs.NewProxy()
		if err != nil {
			return "", err
		}
		p.proxy = proxy
	}
	return p.proxy.URL(r, path)
}

// Revoke gives back link, an address URL gave, and the loopback server stops serving its video once every call that gave it was given back (remotefs.Proxy.Revoke).
func (p *Pool) Revoke(link string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.proxy != nil {
		p.proxy.Revoke(link)
	}
}

// Handler serves the video at path of the source id over HTTP as remotefs.Handler does, through the Pool's connection to it, as Phone Sync serves an indexed film's video to a phone: 502 when the source cannot be reached, 404 when it lacks the file.
func (p *Pool) Handler(id, path string) http.Handler {
	r := p.Reader(id)
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		ctx, cancel := context.WithTimeout(req.Context(), PoolOpenTimeout)
		_, err := p.Driver(ctx, id)
		cancel()
		if err != nil {
			log.Printf("media source: could not reach a source to serve a file: %v", err)
			http.Error(w, "the media source cannot be reached", http.StatusBadGateway)
			return
		}
		remotefs.Handler(r, path).ServeHTTP(w, req)
	})
}

// Reader is the source id as storage reads a file (remotefs.Reader), through the Pool's connection to it, made anew on each read after the one it had failed. It is comparable, so the loopback server gives one address to one file of it.
func (p *Pool) Reader(id string) remotefs.Reader { return poolReader{pool: p, id: id} }

// poolReader reads the source id through pool.
type poolReader struct {
	pool *Pool
	id   string
}

func (r poolReader) Stat(ctx context.Context, path string) (remotefs.Info, error) {
	d, err := r.pool.Driver(ctx, r.id)
	if err != nil {
		return remotefs.Info{}, err
	}
	info, err := StorageReader(d).Stat(ctx, path)
	r.forgetAfter(err)
	return info, err
}

func (r poolReader) Open(ctx context.Context, path string, offset, length int64) (io.ReadCloser, error) {
	d, err := r.pool.Driver(ctx, r.id)
	if err != nil {
		return nil, err
	}
	rc, err := d.Open(ctx, path, offset, length)
	r.forgetAfter(err)
	return rc, err
}

// forgetAfter drops the connection after err, unless err says the file is not there or the read was stopped, so the next read connects again.
func (r poolReader) forgetAfter(err error) {
	if err != nil && !errors.Is(err, ErrNotExist) && !errors.Is(err, ErrInvalidPath) && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
		r.pool.Forget(r.id)
	}
}

// StorageReader is d as storage reads a file (remotefs.Reader), so storage.Get copies a source's file into the library and remotefs.Handler serves one.
func StorageReader(d Driver) remotefs.Reader { return storageReader{d} }

type storageReader struct{ d Driver }

func (r storageReader) Stat(ctx context.Context, p string) (remotefs.Info, error) {
	e, err := r.d.Stat(ctx, p)
	if err != nil {
		return remotefs.Info{}, err
	}
	return remotefs.Info{Name: e.Name, Size: e.Size, Modified: e.Modified, Dir: e.Dir, ETag: e.Version}, nil
}

func (r storageReader) Open(ctx context.Context, p string, offset, length int64) (io.ReadCloser, error) {
	return r.d.Open(ctx, p, offset, length)
}
