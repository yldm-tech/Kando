package storage

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func get(t *testing.T, link string, header http.Header) (*http.Response, string) {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, link, nil)
	if err != nil {
		t.Fatal(err)
	}
	for k, v := range header {
		req.Header[k] = v
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return resp, string(body)
}

func newProxy(t *testing.T) *Proxy {
	t.Helper()
	proxy, err := NewProxy()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { proxy.Close() })
	return proxy
}

func TestProxyServesWholeFilesAndRanges(t *testing.T) {
	for kind, p := range providers(t) {
		t.Run(kind, func(t *testing.T) {
			proxy := newProxy(t)
			write(t, p, "Heat (1995)/video.mp4", "0123456789")
			link, err := proxy.URL(p, "Heat (1995)/video.mp4")
			if err != nil {
				t.Fatal(err)
			}
			u, _ := url.Parse(link)
			if u.Hostname() != "127.0.0.1" || !strings.HasSuffix(u.Path, "/video.mp4") {
				t.Fatalf("link %s", link)
			}
			if again, _ := proxy.URL(p, "./Heat (1995)/video.mp4"); again != link {
				t.Fatalf("asked again: %s, was %s", again, link)
			}

			resp, body := get(t, link, nil)
			if resp.StatusCode != http.StatusOK || body != "0123456789" || resp.Header.Get("Content-Type") != "video/mp4" || resp.Header.Get("Accept-Ranges") != "bytes" {
				t.Fatalf("whole: %d %q %v", resp.StatusCode, body, resp.Header)
			}
			// If-Range takes the store's ETag when there is one (an object store's or a WebDAV server's), else the file's time.
			validator := resp.Header.Get("ETag")
			if validator == "" {
				validator = resp.Header.Get("Last-Modified")
			}
			if validator == "" || (kind == "objects" || kind == "webdav") != strings.HasPrefix(validator, `"`) {
				t.Fatalf("validator %q", validator)
			}

			resp, body = get(t, link, http.Header{"Range": {"bytes=2-5"}})
			if resp.StatusCode != http.StatusPartialContent || body != "2345" || resp.Header.Get("Content-Range") != "bytes 2-5/10" {
				t.Fatalf("range: %d %q %v", resp.StatusCode, body, resp.Header)
			}
			resp, body = get(t, link, http.Header{"Range": {"bytes=7-"}})
			if resp.StatusCode != http.StatusPartialContent || body != "789" {
				t.Fatalf("open range: %d %q", resp.StatusCode, body)
			}
			resp, body = get(t, link, http.Header{"Range": {"bytes=-3"}})
			if resp.StatusCode != http.StatusPartialContent || body != "789" {
				t.Fatalf("suffix range: %d %q", resp.StatusCode, body)
			}
			resp, _ = get(t, link, http.Header{"Range": {"bytes=0-1,4-5"}})
			if resp.StatusCode != http.StatusPartialContent || !strings.HasPrefix(resp.Header.Get("Content-Type"), "multipart/byteranges") {
				t.Fatalf("two ranges: %d %v", resp.StatusCode, resp.Header)
			}
			resp, _ = get(t, link, http.Header{"Range": {"bytes=20-"}})
			if resp.StatusCode != http.StatusRequestedRangeNotSatisfiable {
				t.Fatalf("past the end: %d", resp.StatusCode)
			}

			// A range against the file as it was is served; once the file changed, If-Range gets the whole new file.
			resp, body = get(t, link, http.Header{"Range": {"bytes=0-1"}, "If-Range": {validator}})
			if resp.StatusCode != http.StatusPartialContent || body != "01" {
				t.Fatalf("if-range of the same file: %d %q", resp.StatusCode, body)
			}
			if kind == "objects" {
				write(t, p, "Heat (1995)/video.mp4", "abcdefghijkl")
				resp, body = get(t, link, http.Header{"Range": {"bytes=0-1"}, "If-Range": {validator}})
				if resp.StatusCode != http.StatusOK || body != "abcdefghijkl" {
					t.Fatalf("if-range of a changed file: %d %q", resp.StatusCode, body)
				}
			}

			req, _ := http.NewRequest(http.MethodHead, link, nil)
			head, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			head.Body.Close()
			if head.StatusCode != http.StatusOK || head.ContentLength <= 0 {
				t.Fatalf("head: %d %d", head.StatusCode, head.ContentLength)
			}
		})
	}
}

func TestProxyRefusesWhatItWasNotGiven(t *testing.T) {
	p := NewLocal(t.TempDir())
	write(t, p, "a/video.mp4", "video")
	write(t, p, "a/secret.txt", "secret")
	proxy := newProxy(t)
	link, err := proxy.URL(p, "a/video.mp4")
	if err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(link)
	token := strings.Split(strings.TrimPrefix(u.Path, "/"), "/")[0]
	base := "http://" + u.Host
	for _, path := range []string{"/", "/a/secret.txt", "/" + token[:len(token)-1] + "x/video.mp4", "/x" + token + "/video.mp4", "//video.mp4"} {
		resp, body := get(t, base+path, nil)
		if resp.StatusCode != http.StatusNotFound || strings.Contains(body, "video") || strings.Contains(body, "secret") {
			t.Fatalf("%s: %d %q", path, resp.StatusCode, body)
		}
	}
	// The name after the token is only for the player: the token alone says which file.
	if resp, body := get(t, base+"/"+token+"/secret.txt", nil); resp.StatusCode != http.StatusOK || body != "video" {
		t.Fatalf("another name: %d %q", resp.StatusCode, body)
	}
	resp, _ := get(t, base+"/"+token+"/video.mp4", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("the token: %d", resp.StatusCode)
	}
	req, _ := http.NewRequest(http.MethodPost, link, nil)
	post, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	post.Body.Close()
	if post.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("post: %d", post.StatusCode)
	}
	if _, err := proxy.URL(p, "../x"); err == nil {
		t.Fatal("a link outside the root")
	}
}

func TestProxyAnswersAMissingFileWith404(t *testing.T) {
	for kind, p := range providers(t) {
		t.Run(kind, func(t *testing.T) {
			proxy := newProxy(t)
			write(t, p, "a/video.mp4", "video")
			link, err := proxy.URL(p, "a/video.mp4")
			if err != nil {
				t.Fatal(err)
			}
			if err := p.Remove(t.Context(), "a/video.mp4"); err != nil {
				t.Fatal(err)
			}
			if resp, _ := get(t, link, nil); resp.StatusCode != http.StatusNotFound {
				t.Fatalf("removed: %d", resp.StatusCode)
			}
			folder, err := proxy.URL(p, "a")
			if err != nil {
				t.Fatal(err)
			}
			write(t, p, "a/other.mp4", "other")
			if resp, _ := get(t, folder, nil); resp.StatusCode != http.StatusNotFound {
				t.Fatalf("a folder: %d", resp.StatusCode)
			}
		})
	}
}

func TestProxyCloses(t *testing.T) {
	proxy, err := NewProxy()
	if err != nil {
		t.Fatal(err)
	}
	p := NewLocal(t.TempDir())
	write(t, p, "v.mp4", "v")
	link, _ := proxy.URL(p, "v.mp4")
	if err := proxy.Close(); err != nil {
		t.Fatal(err)
	}
	if resp, err := http.Get(link); err == nil {
		resp.Body.Close()
		t.Fatal("served after Close")
	}
}

// memReader is a Reader that is not a Provider, as a media source is: files by path, read from memory, or none at all while down is set.
type memReader struct {
	files map[string]string
	at    time.Time
	down  bool
}

func (m *memReader) Stat(_ context.Context, p string) (Info, error) {
	if m.down {
		return Info{}, errors.New("connection refused")
	}
	body, ok := m.files[p]
	if !ok {
		return Info{}, ErrNotExist
	}
	return Info{Name: path.Base(p), Size: int64(len(body)), Modified: m.at}, nil
}

func (m *memReader) Open(_ context.Context, p string, offset, length int64) (io.ReadCloser, error) {
	if m.down {
		return nil, errors.New("connection refused")
	}
	body, ok := m.files[p]
	if !ok {
		return nil, ErrNotExist
	}
	body = body[min(offset, int64(len(body))):]
	if length >= 0 {
		body = body[:min(length, int64(len(body)))]
	}
	return io.NopCloser(strings.NewReader(body)), nil
}

// TestProxyServesAReaderThatIsNotAProvider serves a file of a Reader alone, by ranges, If-Range, HEAD and several ranges, with Cache-Control no-store and no CORS header, even to a request from a page's origin; Get copies from such a Reader too. A Reader that cannot be reached answers 502.
func TestProxyServesAReaderThatIsNotAProvider(t *testing.T) {
	src := &memReader{files: map[string]string{"Films/Heat.mkv": "0123456789"}, at: time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)}
	proxy := newProxy(t)
	link, err := proxy.URL(src, "Films/Heat.mkv")
	if err != nil {
		t.Fatal(err)
	}
	resp, body := get(t, link, http.Header{"Origin": {"https://example.com"}})
	if resp.StatusCode != http.StatusOK || body != "0123456789" || resp.Header.Get("Content-Type") != "video/x-matroska" || resp.Header.Get("Cache-Control") != "no-store" {
		t.Fatalf("whole: %d %q %v", resp.StatusCode, body, resp.Header)
	}
	for name := range resp.Header {
		if strings.HasPrefix(strings.ToLower(name), "access-control-") {
			t.Fatalf("a CORS header %s: %v", name, resp.Header)
		}
	}
	if resp.Header.Get("Content-Disposition") != "" {
		t.Fatalf("a Content-Disposition: %v", resp.Header)
	}
	resp, body = get(t, link, http.Header{"Range": {"bytes=3-5"}})
	if resp.StatusCode != http.StatusPartialContent || body != "345" || resp.Header.Get("Content-Range") != "bytes 3-5/10" {
		t.Fatalf("range: %d %q %v", resp.StatusCode, body, resp.Header)
	}
	modified := src.at.Format(http.TimeFormat)
	if resp, body = get(t, link, http.Header{"Range": {"bytes=0-1"}, "If-Range": {modified}}); resp.StatusCode != http.StatusPartialContent || body != "01" {
		t.Fatalf("if-range of the same file: %d %q", resp.StatusCode, body)
	}
	if resp, body = get(t, link, http.Header{"Range": {"bytes=0-1"}, "If-Range": {src.at.Add(-time.Hour).Format(http.TimeFormat)}}); resp.StatusCode != http.StatusOK || body != "0123456789" {
		t.Fatalf("if-range of another time: %d %q", resp.StatusCode, body)
	}
	if resp, _ = get(t, link, http.Header{"Range": {"bytes=0-1,4-5"}}); resp.StatusCode != http.StatusPartialContent || !strings.HasPrefix(resp.Header.Get("Content-Type"), "multipart/byteranges") {
		t.Fatalf("two ranges: %d %v", resp.StatusCode, resp.Header)
	}
	req, _ := http.NewRequest(http.MethodHead, link, nil)
	head, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	head.Body.Close()
	if head.StatusCode != http.StatusOK || head.ContentLength != 10 {
		t.Fatalf("head: %d %d", head.StatusCode, head.ContentLength)
	}

	local := filepath.Join(t.TempDir(), "Heat.mkv")
	if err := Get(t.Context(), src, "Films/Heat.mkv", local, nil); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(local); err != nil || string(got) != "0123456789" {
		t.Fatalf("Get copied %q, %v", got, err)
	}

	src.down = true
	if resp, _ = get(t, link, nil); resp.StatusCode != http.StatusBadGateway {
		t.Fatalf("a reader that cannot be reached: %d", resp.StatusCode)
	}
}

// TestProxyListensOnTheLoopbackAlone checks that a Proxy listens on 127.0.0.1 and nowhere else, and that its URLs name that address.
func TestProxyListensOnTheLoopbackAlone(t *testing.T) {
	proxy := newProxy(t)
	addr, ok := proxy.listener.Addr().(*net.TCPAddr)
	if !ok || !addr.IP.Equal(net.IPv4(127, 0, 0, 1)) || addr.Port == 0 {
		t.Fatalf("listens on %v", proxy.listener.Addr())
	}
	link, err := proxy.URL(&memReader{files: map[string]string{"v.mp4": "v"}}, "v.mp4")
	if err != nil {
		t.Fatal(err)
	}
	if u, _ := url.Parse(link); u.Host != addr.String() {
		t.Fatalf("link %s, listening on %s", link, addr)
	}
}

// TestProxyAnswersOnlyItsOwnHost refuses with 421 a request whose Host header is not the Proxy's 127.0.0.1:port, as a page's request after a DNS rebinding names the page's own host, even with a token the Proxy gave; localhost and another port are refused too.
func TestProxyAnswersOnlyItsOwnHost(t *testing.T) {
	proxy := newProxy(t)
	link, err := proxy.URL(&memReader{files: map[string]string{"v.mp4": "video"}}, "v.mp4")
	if err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(link)
	_, port, _ := net.SplitHostPort(u.Host)
	for _, host := range []string{"evil.example:" + port, "evil.example", "localhost:" + port, "127.0.0.1", "127.0.0.1:1", "[::1]:" + port} {
		req, _ := http.NewRequest(http.MethodGet, link, nil)
		req.Host = host
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusMisdirectedRequest || strings.Contains(string(body), "video") {
			t.Fatalf("Host %s: %d %q", host, resp.StatusCode, body)
		}
	}
	if resp, body := get(t, link, nil); resp.StatusCode != http.StatusOK || body != "video" {
		t.Fatalf("its own host: %d %q", resp.StatusCode, body)
	}
}

// TestProxyRevokesAURL stops serving a file once its URL is revoked, with 404 as for a token it never gave, leaves the other files' URLs as they were, and gives the file a new URL when asked again; revoking a link it did not give, or one revoked already, does nothing.
func TestProxyRevokesAURL(t *testing.T) {
	src := &memReader{files: map[string]string{"a.mp4": "aaa", "b.mp4": "bbb"}}
	proxy := newProxy(t)
	a, _ := proxy.URL(src, "a.mp4")
	b, _ := proxy.URL(src, "b.mp4")
	proxy.Revoke(a)
	if resp, body := get(t, a, nil); resp.StatusCode != http.StatusNotFound || body == "aaa" {
		t.Fatalf("revoked: %d %q", resp.StatusCode, body)
	}
	if resp, body := get(t, b, nil); resp.StatusCode != http.StatusOK || body != "bbb" {
		t.Fatalf("another file: %d %q", resp.StatusCode, body)
	}
	again, err := proxy.URL(src, "a.mp4")
	if err != nil || again == a {
		t.Fatalf("asked again: %s, was %s, %v", again, a, err)
	}
	if resp, body := get(t, again, nil); resp.StatusCode != http.StatusOK || body != "aaa" {
		t.Fatalf("the new URL: %d %q", resp.StatusCode, body)
	}
	for _, link := range []string{a, "", "http://127.0.0.1:1/", "not a url\x7f", "http://127.0.0.1:1/unknown/a.mp4"} {
		proxy.Revoke(link)
	}
	if resp, _ := get(t, again, nil); resp.StatusCode != http.StatusOK {
		t.Fatalf("revoking what is not the new URL revoked it: %d", resp.StatusCode)
	}
}

// TestProxyRevokesAURLOnceEveryHolderGaveItBack checks that a URL asked for twice, as by two openings of the same film whose lookups overlap, stays served when the first is revoked, so the stale opening's revoke does not cut off the one now playing, and is 404 once the second is revoked too.
func TestProxyRevokesAURLOnceEveryHolderGaveItBack(t *testing.T) {
	src := &memReader{files: map[string]string{"a.mp4": "aaa"}}
	proxy := newProxy(t)
	first, _ := proxy.URL(src, "a.mp4")
	second, _ := proxy.URL(src, "a.mp4")
	if first != second {
		t.Fatalf("the same file got %s and %s", first, second)
	}
	proxy.Revoke(first)
	if resp, body := get(t, second, nil); resp.StatusCode != http.StatusOK || body != "aaa" {
		t.Fatalf("revoked by one of two holders: %d %q", resp.StatusCode, body)
	}
	proxy.Revoke(second)
	if resp, _ := get(t, second, nil); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("revoked by both holders: %d", resp.StatusCode)
	}
	proxy.Revoke(second)
	again, _ := proxy.URL(src, "a.mp4")
	proxy.Revoke(again)
	if resp, _ := get(t, again, nil); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("a revoke after the last one carried over to the new URL: %d", resp.StatusCode)
	}
}
