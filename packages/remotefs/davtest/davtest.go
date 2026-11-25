// Package davtest is a WebDAV server on loopback for tests: the storage locations' and the media sources' (docs/media-sources.md 19) connect to it as to a NAS. It imports no package of Kando's.
package davtest

import (
	"net/http"
	"net/http/httptest"
	"slices"
	"sync"
	"testing"

	"golang.org/x/net/webdav"
)

// The user Server lets in, with Password, by HTTP basic authentication.
const (
	User     = "kando"
	Password = "secret"
)

// Server is a WebDAV server over the folder Dir, at URL (whose path is /dav) and behind basic authentication as User with Password, that notes the Range of every GET.
type Server struct {
	URL string
	Dir string

	srv *httptest.Server

	mu     sync.Mutex
	ranges []string
}

// New starts a Server over a new folder, which stops as the test ends.
func New(t testing.TB) *Server {
	t.Helper()
	return start(t, httptest.NewServer)
}

// NewTLS is New over HTTPS, with httptest's own certificate, which no system trusts, as a NAS's self-signed one: its URL starts with https://.
func NewTLS(t testing.TB) *Server {
	t.Helper()
	return start(t, httptest.NewTLSServer)
}

// start is New with serve starting the server.
func start(t testing.TB, serve func(http.Handler) *httptest.Server) *Server {
	t.Helper()
	f := &Server{Dir: t.TempDir()}
	dav := &webdav.Handler{Prefix: "/dav", FileSystem: webdav.Dir(f.Dir), LockSystem: webdav.NewMemLS()}
	srv := serve(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if user, password, ok := r.BasicAuth(); !ok || user != User || password != Password {
			w.Header().Set("WWW-Authenticate", `Basic realm="nas"`)
			http.Error(w, "who are you", http.StatusUnauthorized)
			return
		}
		if r.Method == http.MethodGet {
			f.mu.Lock()
			f.ranges = append(f.ranges, r.Header.Get("Range"))
			f.mu.Unlock()
		}
		dav.ServeHTTP(w, r)
	}))
	t.Cleanup(srv.Close)
	f.srv = srv
	f.URL = srv.URL + "/dav"
	return f
}

// Close stops the server before the test ends, as a NAS switched off: its address answers nothing from then on.
func (f *Server) Close() {
	f.srv.CloseClientConnections()
	f.srv.Close()
}

// Ranges are the Range headers of the GETs since the last ForgetRanges, "" for a GET of a whole file.
func (f *Server) Ranges() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.ranges)
}

// ForgetRanges forgets the GETs noted so far.
func (f *Server) ForgetRanges() {
	f.mu.Lock()
	f.ranges = nil
	f.mu.Unlock()
}
