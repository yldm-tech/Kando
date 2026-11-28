package remotefs

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
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

	psync "kando/packages/sync"
)

// firstByteTimeout is how long Handler waits for a file's store to find the file and give its first bytes before it gives up: 504 when it has not found the file by then, a body cut off when it has but gives no bytes.
var firstByteTimeout = 30 * time.Second

// Handler serves the file at p of reader, whatever the request's path, as a file server does: GET and HEAD, a Range or several (206), If-Range, If-Modified-Since and If-None-Match against the file's time and the store's ETag, and a Content-Type from the extension. A missing file or a folder is 404, a reader that cannot be reached 502, and one that does not answer within firstByteTimeout 504.
func Handler(reader Reader, p string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		// The store has firstByteTimeout to find the file and give its first bytes: one that does not answer, as a NAS whose connection hangs, fails then rather than holding the request open, and the reads it cuts off leave that connection to be made anew.
		ctx, cancel := context.WithCancel(r.Context())
		defer cancel()
		waiting := time.AfterFunc(firstByteTimeout, cancel)
		defer waiting.Stop()
		info, err := reader.Stat(ctx, p)
		if errors.Is(err, ErrNotExist) || errors.Is(err, ErrInvalidPath) || (err == nil && info.Dir) {
			http.NotFound(w, r)
			return
		}
		if err != nil && ctx.Err() != nil && r.Context().Err() == nil {
			http.Error(w, "storage did not answer in time", http.StatusGatewayTimeout)
			return
		}
		if err != nil {
			http.Error(w, "storage unavailable", http.StatusBadGateway)
			return
		}
		w.Header().Set("Content-Type", psync.MediaType(p))
		if etag := entityTag(info.ETag); etag != "" {
			w.Header().Set("ETag", etag)
		}
		body := &rangeReader{ctx: ctx, reader: reader, path: p, size: info.Size, waiting: waiting}
		defer body.Close()
		http.ServeContent(w, r, path.Base(p), info.Modified, body)
	})
}

// entityTag is tag, the store's tag of a file's content, as the ETag header carries it: as it is when it is a quoted entity tag already, as an object store's or a WebDAV server's; else, as a media source's version of a file (mediasource.Entry.Version: a plugin's, as Plex's part key and size, or a size and time), a quoted one made of it, the same for the same tag. A bare tag would never match the If-Range a client sends it back in, which http.ServeContent reads as a date, so each resumed download would get the whole file again.
func entityTag(tag string) string {
	if tag == "" || strings.HasPrefix(tag, `"`) || strings.HasPrefix(tag, `W/"`) {
		return tag
	}
	sum := sha256.Sum256([]byte(tag))
	return `"` + hex.EncodeToString(sum[:16]) + `"`
}

// rangeReader is the file at path of size bytes as the io.ReadSeeker http.ServeContent takes: a Seek only notes the offset, and the first Read after it opens the file there, to its end.
type rangeReader struct {
	ctx    context.Context
	reader Reader
	path   string
	size   int64
	offset int64
	body   io.ReadCloser
	// waiting ends the request's context unless the first bytes come in time (Handler), stopped once they do; nil after.
	waiting *time.Timer
}

func (r *rangeReader) Read(buf []byte) (int, error) {
	if r.offset >= r.size {
		return 0, io.EOF
	}
	if r.body == nil {
		body, err := r.reader.Open(r.ctx, r.path, r.offset, -1)
		if err != nil {
			return 0, err
		}
		r.body = body
	}
	n, err := r.body.Read(buf)
	r.offset += int64(n)
	if n > 0 && r.waiting != nil {
		r.waiting.Stop()
		r.waiting = nil
	}
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

// Proxy is an HTTP server on the loopback address that serves the files of a provider, or of any Reader, to the system player and ffmpeg, which open a URL and not a provider. Each file it serves has a random token of its own in its URL, so another program on this computer can read only the files it was given a URL for, and only until the URL is revoked (Revoke), not any file of a provider; there is no route that lists anything. It answers only a request whose Host is its own address, 127.0.0.1:port, so a web page whose name a DNS rebinding points at 127.0.0.1 cannot talk to it, and it sends no CORS header, so a page's script that has a URL still cannot read what it answers.
type Proxy struct {
	listener net.Listener
	server   *http.Server
	// host is the address requests must name in their Host header: the listener's, 127.0.0.1:port.
	host string

	mu     sync.Mutex
	files  map[string]proxied
	tokens map[proxied]string
	// holds counts, by token, the URL calls that gave it and were not revoked yet: the token is served until the last of them is.
	holds map[string]int
}

// proxied is a file a Proxy serves.
type proxied struct {
	reader Reader
	path   string
}

// NewProxy starts a Proxy on a free port of 127.0.0.1.
func NewProxy() (*Proxy, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	p := &Proxy{listener: listener, host: listener.Addr().String(), files: map[string]proxied{}, tokens: map[proxied]string{}, holds: map[string]int{}}
	p.server = &http.Server{Handler: p, ReadHeaderTimeout: 10 * time.Second, ErrorLog: log.New(io.Discard, "", 0)}
	go p.server.Serve(listener)
	return p, nil
}

// URL is the address the file at path of reader is served at, http://127.0.0.1:port/token/name, the name being the file's own so a player can tell its kind; asking again for the same file gives the same URL until it is revoked (Revoke) as many times as it was given, and a new one after. reader must be comparable, as a pointer is.
func (p *Proxy) URL(reader Reader, filePath string) (string, error) {
	clean, err := CleanFile(filePath)
	if err != nil {
		return "", err
	}
	key := proxied{reader: reader, path: clean}
	p.mu.Lock()
	token, ok := p.tokens[key]
	if !ok {
		token = rand.Text()
		p.tokens[key] = token
		p.files[token] = key
	}
	p.holds[token]++
	p.mu.Unlock()
	return fmt.Sprintf("http://%s/%s/%s", p.host, token, url.PathEscape(path.Base(clean))), nil
}

// Revoke gives back the URL link, as URL gave it, and stops serving its file once every URL call that gave that link has been given back, so one holder moving on from a file does not cut off another that asked for the same file meanwhile, as two openings of the same film whose lookups overlap do: a request with its token is 404 from then on, a request being answered goes on, and asking URL for the file again gives a new token. A link the Proxy did not give, or one revoked already, revokes nothing.
func (p *Proxy) Revoke(link string) {
	token := tokenOf(link)
	if token == "" {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if file, ok := p.files[token]; ok {
		if p.holds[token]--; p.holds[token] > 0 {
			return
		}
		delete(p.holds, token)
		delete(p.files, token)
		delete(p.tokens, file)
	}
}

// tokenOf is the token of a Proxy's URL, the first segment of its path, "" for a link that is not one.
func tokenOf(link string) string {
	u, err := url.Parse(link)
	if err != nil {
		return ""
	}
	token, _, _ := strings.Cut(strings.TrimPrefix(u.Path, "/"), "/")
	return token
}

// ServeHTTP serves the file of the token that starts the request's path: 421 for a request whose Host is not the Proxy's own address, and 404 for a path without a known token, whether there never was one or it was revoked.
func (p *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Host != p.host {
		http.Error(w, "misdirected request", http.StatusMisdirectedRequest)
		return
	}
	token, _, _ := strings.Cut(strings.TrimPrefix(r.URL.Path, "/"), "/")
	p.mu.Lock()
	file, ok := p.files[token]
	p.mu.Unlock()
	if !ok || token == "" {
		http.NotFound(w, r)
		return
	}
	// What the player reads is not kept by anything on the way, so a revoked URL serves nothing from a cache either.
	w.Header().Set("Cache-Control", "no-store")
	Handler(file.reader, file.path).ServeHTTP(w, r)
}

// Close stops the server and ends the requests it is serving.
func (p *Proxy) Close() error {
	return p.server.Close()
}
