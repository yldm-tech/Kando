package storage

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"path"
	"strings"
	"sync"
	"time"

	"kando/apps/kando/cloud"
)

// Handler serves the file at p of provider, whatever the request's path, as a file server does: GET and HEAD, a Range or several (206), If-Range, If-Modified-Since and If-None-Match against the file's time and the store's ETag, and a Content-Type from the extension. A missing file or a folder is 404.
func Handler(provider Provider, p string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		info, err := provider.Stat(r.Context(), p)
		if errors.Is(err, ErrNotExist) || errors.Is(err, ErrInvalidPath) || (err == nil && info.Dir) {
			http.NotFound(w, r)
			return
		}
		if err != nil {
			http.Error(w, "storage unavailable", http.StatusBadGateway)
			return
		}
		w.Header().Set("Content-Type", cloud.ContentType(p))
		if info.ETag != "" {
			w.Header().Set("ETag", info.ETag)
		}
		body := &rangeReader{ctx: r.Context(), provider: provider, path: p, size: info.Size}
		defer body.Close()
		http.ServeContent(w, r, path.Base(p), info.Modified, body)
	})
}

// rangeReader is the file at path of size bytes as the io.ReadSeeker http.ServeContent takes: a Seek only notes the offset, and the first Read after it opens the file there, to its end.
type rangeReader struct {
	ctx      context.Context
	provider Provider
	path     string
	size     int64
	offset   int64
	body     io.ReadCloser
}

func (r *rangeReader) Read(buf []byte) (int, error) {
	if r.offset >= r.size {
		return 0, io.EOF
	}
	if r.body == nil {
		body, err := r.provider.Open(r.ctx, r.path, r.offset, -1)
		if err != nil {
			return 0, err
		}
		r.body = body
	}
	n, err := r.body.Read(buf)
	r.offset += int64(n)
	return n, err
}

func (r *rangeReader) Seek(offset int64, whence int) (int64, error) {
	switch whence {
	case io.SeekCurrent:
		offset += r.offset
	case io.SeekEnd:
		offset += r.size
	}
	if offset < 0 {
		return 0, errors.New("seek before the start")
	}
	if offset != r.offset {
		r.Close()
		r.offset = offset
	}
	return offset, nil
}

// Close closes the open body, if any.
func (r *rangeReader) Close() error {
	if r.body == nil {
		return nil
	}
	err := r.body.Close()
	r.body = nil
	return err
}

// Proxy is an HTTP server on the loopback address that serves provider files to the system player, which opens a URL and not a provider. Each file it serves has a random token of its own in its URL, so another program on this computer can read only the files it was given a URL for, not any file of a provider.
type Proxy struct {
	listener net.Listener
	server   *http.Server

	mu     sync.Mutex
	files  map[string]proxied
	tokens map[proxied]string
}

// proxied is a file a Proxy serves.
type proxied struct {
	provider Provider
	path     string
}

// NewProxy starts a Proxy on a free port of 127.0.0.1.
func NewProxy() (*Proxy, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	p := &Proxy{listener: listener, files: map[string]proxied{}, tokens: map[proxied]string{}}
	p.server = &http.Server{Handler: p, ReadHeaderTimeout: 10 * time.Second, ErrorLog: log.New(io.Discard, "", 0)}
	go p.server.Serve(listener)
	return p, nil
}

// URL is the address the file at path of provider is served at, http://127.0.0.1:port/token/name, the name being the file's own so a player can tell its kind; asking again for the same file gives the same URL. provider must be comparable, as a pointer is.
func (p *Proxy) URL(provider Provider, filePath string) (string, error) {
	clean, err := cleanFile(filePath)
	if err != nil {
		return "", err
	}
	key := proxied{provider: provider, path: clean}
	p.mu.Lock()
	token, ok := p.tokens[key]
	if !ok {
		token = rand.Text()
		p.tokens[key] = token
		p.files[token] = key
	}
	p.mu.Unlock()
	return fmt.Sprintf("http://%s/%s/%s", p.listener.Addr(), token, url.PathEscape(path.Base(clean))), nil
}

// ServeHTTP serves the file of the token that starts the request's path, and 404 for a path without a known one.
func (p *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	token, _, _ := strings.Cut(strings.TrimPrefix(r.URL.Path, "/"), "/")
	p.mu.Lock()
	file, ok := p.files[token]
	p.mu.Unlock()
	if !ok || token == "" {
		http.NotFound(w, r)
		return
	}
	Handler(file.provider, file.path).ServeHTTP(w, r)
}

// Close stops the server and ends the requests it is serving.
func (p *Proxy) Close() error {
	return p.server.Close()
}
