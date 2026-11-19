package storage

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"

	"golang.org/x/net/webdav"
)

// fakeDAV is a WebDAV server over a folder, under the path /dav and behind basic authentication as kando with the password secret, that notes the Range of every GET.
type fakeDAV struct {
	url string
	dir string

	mu     sync.Mutex
	ranges []string
}

func newFakeDAV(t *testing.T) *fakeDAV {
	t.Helper()
	f := &fakeDAV{dir: t.TempDir()}
	dav := &webdav.Handler{Prefix: "/dav", FileSystem: webdav.Dir(f.dir), LockSystem: webdav.NewMemLS()}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if user, password, ok := r.BasicAuth(); !ok || user != "kando" || password != "secret" {
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
	f.url = srv.URL + "/dav"
	return f
}

// webdavProvider is a provider over the folder films of a new fakeDAV.
func webdavProvider(t *testing.T) (Provider, *fakeDAV) {
	t.Helper()
	f := newFakeDAV(t)
	if err := os.Mkdir(f.dir+"/films", 0o755); err != nil {
		t.Fatal(err)
	}
	p, closer, err := OpenRemote(context.Background(), Remote{Kind: KindWebDAV, URL: f.url, Root: "/films/", User: "kando", Password: "secret"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { closer.Close() })
	return p, f
}

func TestWebDAVKeepsFilesUnderItsRootAndReadsByRange(t *testing.T) {
	p, f := webdavProvider(t)
	write(t, p, "Heat (1995)/video.mp4", "0123456789")
	if body, err := os.ReadFile(f.dir + "/films/Heat (1995)/video.mp4"); err != nil || string(body) != "0123456789" {
		t.Fatalf("on the server: %q, %v", body, err)
	}
	f.mu.Lock()
	f.ranges = nil
	f.mu.Unlock()
	if got := read(t, p, "Heat (1995)/video.mp4", 2, 3); got != "234" {
		t.Fatalf("range = %q", got)
	}
	if got := read(t, p, "Heat (1995)/video.mp4", 7, -1); got != "789" {
		t.Fatalf("tail = %q", got)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if !slices.Equal(f.ranges, []string{"bytes=2-4", "bytes=7-9"}) {
		t.Fatalf("ranges asked %q", f.ranges)
	}
	info, err := p.Stat(context.Background(), "Heat (1995)/video.mp4")
	if err != nil || info.ETag == "" {
		t.Fatalf("stat = %+v, %v", info, err)
	}
}

func TestWebDAVRefusesAWrongPasswordAndABadAddress(t *testing.T) {
	ctx := context.Background()
	f := newFakeDAV(t)
	if _, _, err := OpenRemote(ctx, Remote{Kind: KindWebDAV, URL: f.url, User: "kando", Password: "wrong"}); err == nil || !strings.Contains(err.Error(), "401") {
		t.Fatalf("wrong password = %v", err)
	}
	if _, _, err := OpenRemote(ctx, Remote{Kind: KindWebDAV, URL: f.url, Root: "missing", User: "kando", Password: "secret"}); err == nil {
		t.Fatal("a missing root opened")
	}
	for _, bad := range []string{"ftp://nas/films", "nas/films", "https://user:pw@nas/films", "https://nas/../x"} {
		if _, _, err := OpenRemote(ctx, Remote{Kind: KindWebDAV, URL: bad}); err == nil {
			t.Errorf("%s opened", bad)
		}
	}
	if _, _, err := OpenRemote(ctx, Remote{Kind: KindWebDAV, URL: f.url, Root: "../x"}); err == nil {
		t.Error("a root above the address opened")
	}
}
