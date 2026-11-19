package storage

import (
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
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
			// If-Range takes the store's ETag when there is one, else the file's time.
			validator := resp.Header.Get("ETag")
			if validator == "" {
				validator = resp.Header.Get("Last-Modified")
			}
			if validator == "" || (kind == "objects") != strings.HasPrefix(validator, `"`) {
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
