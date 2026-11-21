package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"
	"time"

	"kando/apps/kando/cloud"
)

// Target is a place files can be kept that a Pool opens, by an ID of the caller's: a folder on a NAS or a server (Remote), or an object store (Store).
type Target struct {
	ID     string
	Remote *Remote
	Store  *cloud.Store
}

// key tells two settings of a target apart, so a target whose settings changed is opened again.
func (t Target) key() string {
	switch {
	case t.Remote != nil:
		return fmt.Sprintf("remote %+v", *t.Remote)
	case t.Store != nil:
		return fmt.Sprintf("store %+v", *t.Store)
	}
	return ""
}

// OpenTarget opens t: a NAS location connects and checks its folder (OpenRemote), an object store only makes its client, and has nothing to close.
func OpenTarget(ctx context.Context, t Target) (Provider, io.Closer, error) {
	switch {
	case t.Remote != nil:
		return OpenRemote(ctx, *t.Remote)
	case t.Store != nil:
		store, err := OpenObjectStore(*t.Store, "")
		return store, nil, err
	}
	return nil, nil, errors.New("storage: a target without a location")
}

// ErrPoolClosed is a Pool's error once it was closed.
var ErrPoolClosed = errors.New("storage: pool closed")

// PoolOpenTimeout bounds opening a target for a request the Pool's Handler serves.
const PoolOpenTimeout = 30 * time.Second

// Pool keeps targets opened, by ID, and the Proxy that serves their files to the system player. Its methods run on any goroutine. A target whose settings changed is opened again; one that failed to open is tried again next time.
type Pool struct {
	mu     sync.Mutex
	conns  map[string]poolConn
	proxy  *Proxy
	open   func(ctx context.Context, t Target) (Provider, io.Closer, error)
	closed bool
}

// poolConn is a target opened: by settings key, its provider, and what closes it, nil for none.
type poolConn struct {
	key      string
	provider Provider
	closer   io.Closer
}

// NewPool is a Pool that opens targets with open, OpenTarget when it is nil.
func NewPool(open func(ctx context.Context, t Target) (Provider, io.Closer, error)) *Pool {
	if open == nil {
		open = OpenTarget
	}
	return &Pool{conns: map[string]poolConn{}, open: open}
}

// Provider is t opened, opening it now when it is not, or when its settings changed since.
func (p *Pool) Provider(ctx context.Context, t Target) (Provider, error) {
	key := t.key()
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return nil, ErrPoolClosed
	}
	if c, ok := p.conns[t.ID]; ok && c.key == key {
		p.mu.Unlock()
		return c.provider, nil
	}
	p.mu.Unlock()
	provider, closer, err := p.open(ctx, t)
	if err != nil {
		return nil, err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		closeQuietly(closer)
		return nil, ErrPoolClosed
	}
	if c, ok := p.conns[t.ID]; ok {
		// Another goroutine opened it meanwhile: its connection is kept, this one closed.
		if c.key == key {
			closeQuietly(closer)
			return c.provider, nil
		}
		closeQuietly(c.closer)
	}
	p.conns[t.ID] = poolConn{key: key, provider: provider, closer: closer}
	return provider, nil
}

// URL is the address the system player plays the file at path of t at, through the pool's Proxy, started the first time, once the file was found there: ErrNotExist when it is not.
func (p *Pool) URL(ctx context.Context, t Target, path string) (string, error) {
	provider, err := p.Provider(ctx, t)
	if err != nil {
		return "", err
	}
	if _, err := provider.Stat(ctx, path); err != nil {
		return "", err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return "", ErrPoolClosed
	}
	if p.proxy == nil {
		proxy, err := NewProxy()
		if err != nil {
			return "", err
		}
		p.proxy = proxy
	}
	return p.proxy.URL(provider, path)
}

// Revoke stops the pool's Proxy serving the file link is the URL of (Proxy.Revoke), as when the player moves on from it; nothing when there is no Proxy yet.
func (p *Pool) Revoke(link string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.proxy != nil {
		p.proxy.Revoke(link)
	}
}

// Handler serves the file at path of t over HTTP as Handler does, opening t as a request comes, within PoolOpenTimeout; 502 when t cannot be opened.
func (p *Pool) Handler(t Target, path string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), PoolOpenTimeout)
		provider, err := p.Provider(ctx, t)
		cancel()
		if err != nil {
			log.Printf("storage: could not open a location to serve a file: %v", err)
			http.Error(w, "the storage location cannot be reached", http.StatusBadGateway)
			return
		}
		Handler(provider, path).ServeHTTP(w, r)
	})
}

// Close closes every target opened and the Proxy, for good: the Pool opens nothing after.
func (p *Pool) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.closed = true
	for id, c := range p.conns {
		closeQuietly(c.closer)
		delete(p.conns, id)
	}
	if p.proxy != nil {
		_ = p.proxy.Close()
		p.proxy = nil
	}
}

func closeQuietly(c io.Closer) {
	if c != nil {
		_ = c.Close()
	}
}
