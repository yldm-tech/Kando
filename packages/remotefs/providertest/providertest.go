// Package providertest checks that a remotefs.Provider keeps the contract every provider shares, and that a remotefs.Proxy serves its files, for the tests of the packages that have providers: remotefs's own over SMB, SFTP and WebDAV, and the storage locations' over a local folder and an object store. It imports no package of the apps.
package providertest

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"slices"
	"strings"
	"testing"

	"kando/packages/remotefs"
)

// Write stores body as the file name of p, failing the test when it cannot.
func Write(t *testing.T, p remotefs.Provider, name, body string) {
	t.Helper()
	if err := p.Write(context.Background(), name, strings.NewReader(body), int64(len(body))); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

// Read is length bytes of the file name of p from offset, to its end with length -1, failing the test when it cannot read them.
func Read(t *testing.T, p remotefs.Provider, name string, offset, length int64) string {
	t.Helper()
	r, err := p.Open(context.Background(), name, offset, length)
	if err != nil {
		t.Fatalf("open %s: %v", name, err)
	}
	defer r.Close()
	body, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return string(body)
}

// Names is what the folder dir of p holds, sorted, a folder with a slash after its name, failing the test when it cannot list it.
func Names(t *testing.T, p remotefs.Provider, dir string) []string {
	t.Helper()
	infos, err := p.List(context.Background(), dir)
	if err != nil {
		t.Fatalf("list %q: %v", dir, err)
	}
	var out []string
	for _, info := range infos {
		name := info.Name
		if info.Dir {
			name += "/"
		}
		out = append(out, name)
	}
	slices.Sort(out)
	return out
}

// Run checks, against each provider providers gives by kind, every rule a Provider keeps, and that a Proxy serves its files whole and by ranges and answers 404 for one that went. A provider of the kind "objects" or "webdav" is one that tags its files with an ETag, and only "objects" sees a file it wrote again as changed for If-Range.
func Run(t *testing.T, providers func(t *testing.T) map[string]remotefs.Provider) {
	t.Run("WriteStatListAndRead", func(t *testing.T) {
		ctx := context.Background()
		for kind, p := range providers(t) {
			t.Run(kind, func(t *testing.T) {
				Write(t, p, "Heat (1995)/video.mp4", "0123456789")
				Write(t, p, "Heat (1995)/video.en.srt", "subs")
				Write(t, p, "Heat (1995)/extras/trailer.mp4", "trailer")
				Write(t, p, "top.txt", "top")

				info, err := p.Stat(ctx, "Heat (1995)/video.mp4")
				if err != nil || info.Name != "video.mp4" || info.Size != 10 || info.Dir || info.Modified.IsZero() {
					t.Fatalf("stat file = %+v, %v", info, err)
				}
				info, err = p.Stat(ctx, "Heat (1995)")
				if err != nil || !info.Dir || info.Name != "Heat (1995)" {
					t.Fatalf("stat folder = %+v, %v", info, err)
				}
				if info, err := p.Stat(ctx, ""); err != nil || !info.Dir {
					t.Fatalf("stat root = %+v, %v", info, err)
				}
				if got := Names(t, p, ""); !slices.Equal(got, []string{"Heat (1995)/", "top.txt"}) {
					t.Fatalf("root lists %q", got)
				}
				if got := Names(t, p, "Heat (1995)"); !slices.Equal(got, []string{"extras/", "video.en.srt", "video.mp4"}) {
					t.Fatalf("folder lists %q", got)
				}

				if got := Read(t, p, "Heat (1995)/video.mp4", 0, -1); got != "0123456789" {
					t.Fatalf("whole = %q", got)
				}
				if got := Read(t, p, "Heat (1995)/video.mp4", 3, 4); got != "3456" {
					t.Fatalf("range = %q", got)
				}
				if got := Read(t, p, "Heat (1995)/video.mp4", 7, -1); got != "789" {
					t.Fatalf("tail = %q", got)
				}
				if got := Read(t, p, "Heat (1995)/video.mp4", 10, -1); got != "" {
					t.Fatalf("past the end = %q", got)
				}
				if got := Read(t, p, "Heat (1995)/video.mp4", 0, 0); got != "" {
					t.Fatalf("nothing = %q", got)
				}

				Write(t, p, "top.txt", "replaced")
				if got := Read(t, p, "top.txt", 0, -1); got != "replaced" {
					t.Fatalf("replaced = %q", got)
				}
			})
		}
	})

	t.Run("SayWhatIsMissing", func(t *testing.T) {
		ctx := context.Background()
		for kind, p := range providers(t) {
			t.Run(kind, func(t *testing.T) {
				Write(t, p, "a/b.mp4", "b")
				_, err := p.Stat(ctx, "a/none.mp4")
				if !errors.Is(err, remotefs.ErrNotExist) || !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("stat missing = %v", err)
				}
				if _, err := p.Open(ctx, "a/none.mp4", 0, -1); !errors.Is(err, remotefs.ErrNotExist) || !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("open missing = %v", err)
				}
				if _, err := p.List(ctx, "nowhere"); !errors.Is(err, remotefs.ErrNotExist) {
					t.Fatalf("list missing = %v", err)
				}
				if err := p.Move(ctx, "a/none.mp4", "a/c.mp4"); !errors.Is(err, remotefs.ErrNotExist) {
					t.Fatalf("move missing = %v", err)
				}
				if err := p.Remove(ctx, "a/none.mp4"); err != nil {
					t.Fatalf("remove missing = %v", err)
				}
			})
		}
	})

	t.Run("RefusePathsOutsideTheRoot", func(t *testing.T) {
		ctx := context.Background()
		for kind, p := range providers(t) {
			t.Run(kind, func(t *testing.T) {
				for _, bad := range []string{"..", "../x", "a/../../x", "/etc/passwd", `a\..\..\x`, "a\x00b"} {
					if _, err := p.Stat(ctx, bad); !errors.Is(err, remotefs.ErrInvalidPath) {
						t.Errorf("stat %q = %v", bad, err)
					}
					if err := p.Write(ctx, bad, strings.NewReader("x"), 1); !errors.Is(err, remotefs.ErrInvalidPath) {
						t.Errorf("write %q = %v", bad, err)
					}
					if _, err := p.Open(ctx, bad, 0, -1); !errors.Is(err, remotefs.ErrInvalidPath) {
						t.Errorf("open %q = %v", bad, err)
					}
					if err := p.Move(ctx, "a", bad); !errors.Is(err, remotefs.ErrInvalidPath) {
						t.Errorf("move to %q = %v", bad, err)
					}
				}
				for _, root := range []string{"", ".", "a/.."} {
					if err := p.Remove(ctx, root); !errors.Is(err, remotefs.ErrInvalidPath) {
						t.Errorf("remove %q = %v", root, err)
					}
					if err := p.Write(ctx, root, strings.NewReader("x"), 1); !errors.Is(err, remotefs.ErrInvalidPath) {
						t.Errorf("write %q = %v", root, err)
					}
				}
				// A path that climbs and comes back stays inside.
				Write(t, p, "a/../b.txt", "b")
				if got := Read(t, p, "./b.txt", 0, -1); got != "b" {
					t.Fatalf("b.txt = %q", got)
				}
			})
		}
	})

	t.Run("MoveFilesAndFolders", func(t *testing.T) {
		ctx := context.Background()
		for kind, p := range providers(t) {
			t.Run(kind, func(t *testing.T) {
				Write(t, p, "inbox/Heat/video.mp4", "video")
				Write(t, p, "inbox/Heat/video.srt", "subs")
				Write(t, p, "inbox/Heat/extras/trailer.mp4", "trailer")
				Write(t, p, "inbox/note.txt", "note")

				if err := p.Move(ctx, "inbox/note.txt", "notes/2026/note.txt"); err != nil {
					t.Fatal(err)
				}
				if got := Read(t, p, "notes/2026/note.txt", 0, -1); got != "note" {
					t.Fatalf("moved file = %q", got)
				}
				if _, err := p.Stat(ctx, "inbox/note.txt"); !errors.Is(err, remotefs.ErrNotExist) {
					t.Fatalf("the file stayed: %v", err)
				}

				Write(t, p, "library/Heat/video.srt", "old subs")
				if err := p.Move(ctx, "library/Heat/video.srt", "inbox/Heat/video.srt"); err != nil {
					t.Fatalf("move over a file: %v", err)
				}
				if got := Read(t, p, "inbox/Heat/video.srt", 0, -1); got != "old subs" {
					t.Fatalf("replaced file = %q", got)
				}

				if err := p.Move(ctx, "inbox/Heat", "library/Heat (1995)"); err != nil {
					t.Fatal(err)
				}
				if got := Names(t, p, "library/Heat (1995)"); !slices.Equal(got, []string{"extras/", "video.mp4", "video.srt"}) {
					t.Fatalf("moved folder lists %q", got)
				}
				if got := Read(t, p, "library/Heat (1995)/extras/trailer.mp4", 0, -1); got != "trailer" {
					t.Fatalf("moved nested file = %q", got)
				}
				if _, err := p.Stat(ctx, "inbox/Heat"); !errors.Is(err, remotefs.ErrNotExist) {
					t.Fatalf("the folder stayed: %v", err)
				}
			})
		}
	})

	t.Run("Remove", func(t *testing.T) {
		ctx := context.Background()
		for kind, p := range providers(t) {
			t.Run(kind, func(t *testing.T) {
				Write(t, p, "Heat/video.mp4", "video")
				if err := p.Remove(ctx, "Heat"); !errors.Is(err, remotefs.ErrNotEmpty) {
					t.Fatalf("remove a full folder = %v", err)
				}
				if err := p.Remove(ctx, "Heat/video.mp4"); err != nil {
					t.Fatal(err)
				}
				if _, err := p.Stat(ctx, "Heat/video.mp4"); !errors.Is(err, remotefs.ErrNotExist) {
					t.Fatalf("the file stayed: %v", err)
				}
				if err := p.Remove(ctx, "Heat"); err != nil {
					t.Fatalf("remove the emptied folder = %v", err)
				}
				if _, err := p.Stat(ctx, "Heat"); !errors.Is(err, remotefs.ErrNotExist) {
					t.Fatalf("the folder stayed: %v", err)
				}
			})
		}
	})

	// A body shorter than its size stores nothing, and leaves what was there.
	t.Run("WriteNothingOfAShortBody", func(t *testing.T) {
		ctx := context.Background()
		for kind, p := range providers(t) {
			t.Run(kind, func(t *testing.T) {
				Write(t, p, "a/video.mp4", "old")
				if err := p.Write(ctx, "a/video.mp4", strings.NewReader("new"), 10); err == nil {
					t.Fatal("a short body was stored")
				}
				if got := Read(t, p, "a/video.mp4", 0, -1); got != "old" {
					t.Fatalf("after a short write = %q", got)
				}
				if got := Names(t, p, "a"); !slices.Equal(got, []string{"video.mp4"}) {
					t.Fatalf("a short write left %q", got)
				}
			})
		}
	})

	t.Run("ProxyServesWholeFilesAndRanges", func(t *testing.T) {
		for kind, p := range providers(t) {
			t.Run(kind, func(t *testing.T) {
				proxy := newProxy(t)
				Write(t, p, "Heat (1995)/video.mp4", "0123456789")
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
					Write(t, p, "Heat (1995)/video.mp4", "abcdefghijkl")
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
	})

	t.Run("ProxyAnswersAMissingFileWith404", func(t *testing.T) {
		for kind, p := range providers(t) {
			t.Run(kind, func(t *testing.T) {
				proxy := newProxy(t)
				Write(t, p, "a/video.mp4", "video")
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
				Write(t, p, "a/other.mp4", "other")
				if resp, _ := get(t, folder, nil); resp.StatusCode != http.StatusNotFound {
					t.Fatalf("a folder: %d", resp.StatusCode)
				}
			})
		}
	})
}

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

func newProxy(t *testing.T) *remotefs.Proxy {
	t.Helper()
	proxy, err := remotefs.NewProxy()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { proxy.Close() })
	return proxy
}
