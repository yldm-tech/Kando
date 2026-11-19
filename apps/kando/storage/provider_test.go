package storage

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"slices"
	"strings"
	"testing"
)

// providers are a Local over a new folder, an ObjectStore over a fake bucket, rooted below its top so the root's prefix is part of every key, a WebDAV and an SFTP provider over servers in the test, and, when KANDO_TEST_SMB names a share, an SMB provider over a new folder of it.
func providers(t *testing.T) map[string]Provider {
	store, _ := fakeStore(t, "kando/films")
	dav, _ := webdavProvider(t)
	sf, _ := sftpProvider(t)
	out := map[string]Provider{"local": NewLocal(t.TempDir()), "objects": store, "webdav": dav, "sftp": sf}
	if smb := smbProvider(t); smb != nil {
		out["smb"] = smb
	}
	return out
}

func write(t *testing.T, p Provider, name, body string) {
	t.Helper()
	if err := p.Write(context.Background(), name, strings.NewReader(body), int64(len(body))); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

func read(t *testing.T, p Provider, name string, offset, length int64) string {
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

func names(t *testing.T, p Provider, dir string) []string {
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

func TestProvidersWriteStatListAndRead(t *testing.T) {
	ctx := context.Background()
	for kind, p := range providers(t) {
		t.Run(kind, func(t *testing.T) {
			write(t, p, "Heat (1995)/video.mp4", "0123456789")
			write(t, p, "Heat (1995)/video.en.srt", "subs")
			write(t, p, "Heat (1995)/extras/trailer.mp4", "trailer")
			write(t, p, "top.txt", "top")

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
			if got := names(t, p, ""); !slices.Equal(got, []string{"Heat (1995)/", "top.txt"}) {
				t.Fatalf("root lists %q", got)
			}
			if got := names(t, p, "Heat (1995)"); !slices.Equal(got, []string{"extras/", "video.en.srt", "video.mp4"}) {
				t.Fatalf("folder lists %q", got)
			}

			if got := read(t, p, "Heat (1995)/video.mp4", 0, -1); got != "0123456789" {
				t.Fatalf("whole = %q", got)
			}
			if got := read(t, p, "Heat (1995)/video.mp4", 3, 4); got != "3456" {
				t.Fatalf("range = %q", got)
			}
			if got := read(t, p, "Heat (1995)/video.mp4", 7, -1); got != "789" {
				t.Fatalf("tail = %q", got)
			}
			if got := read(t, p, "Heat (1995)/video.mp4", 10, -1); got != "" {
				t.Fatalf("past the end = %q", got)
			}
			if got := read(t, p, "Heat (1995)/video.mp4", 0, 0); got != "" {
				t.Fatalf("nothing = %q", got)
			}

			write(t, p, "top.txt", "replaced")
			if got := read(t, p, "top.txt", 0, -1); got != "replaced" {
				t.Fatalf("replaced = %q", got)
			}
		})
	}
}

func TestProvidersSayWhatIsMissing(t *testing.T) {
	ctx := context.Background()
	for kind, p := range providers(t) {
		t.Run(kind, func(t *testing.T) {
			write(t, p, "a/b.mp4", "b")
			_, err := p.Stat(ctx, "a/none.mp4")
			if !errors.Is(err, ErrNotExist) || !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("stat missing = %v", err)
			}
			if _, err := p.Open(ctx, "a/none.mp4", 0, -1); !errors.Is(err, ErrNotExist) || !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("open missing = %v", err)
			}
			if _, err := p.List(ctx, "nowhere"); !errors.Is(err, ErrNotExist) {
				t.Fatalf("list missing = %v", err)
			}
			if err := p.Move(ctx, "a/none.mp4", "a/c.mp4"); !errors.Is(err, ErrNotExist) {
				t.Fatalf("move missing = %v", err)
			}
			if err := p.Remove(ctx, "a/none.mp4"); err != nil {
				t.Fatalf("remove missing = %v", err)
			}
		})
	}
}

func TestProvidersRefusePathsOutsideTheRoot(t *testing.T) {
	ctx := context.Background()
	for kind, p := range providers(t) {
		t.Run(kind, func(t *testing.T) {
			for _, bad := range []string{"..", "../x", "a/../../x", "/etc/passwd", `a\..\..\x`, "a\x00b"} {
				if _, err := p.Stat(ctx, bad); !errors.Is(err, ErrInvalidPath) {
					t.Errorf("stat %q = %v", bad, err)
				}
				if err := p.Write(ctx, bad, strings.NewReader("x"), 1); !errors.Is(err, ErrInvalidPath) {
					t.Errorf("write %q = %v", bad, err)
				}
				if _, err := p.Open(ctx, bad, 0, -1); !errors.Is(err, ErrInvalidPath) {
					t.Errorf("open %q = %v", bad, err)
				}
				if err := p.Move(ctx, "a", bad); !errors.Is(err, ErrInvalidPath) {
					t.Errorf("move to %q = %v", bad, err)
				}
			}
			for _, root := range []string{"", ".", "a/.."} {
				if err := p.Remove(ctx, root); !errors.Is(err, ErrInvalidPath) {
					t.Errorf("remove %q = %v", root, err)
				}
				if err := p.Write(ctx, root, strings.NewReader("x"), 1); !errors.Is(err, ErrInvalidPath) {
					t.Errorf("write %q = %v", root, err)
				}
			}
			// A path that climbs and comes back stays inside.
			write(t, p, "a/../b.txt", "b")
			if got := read(t, p, "./b.txt", 0, -1); got != "b" {
				t.Fatalf("b.txt = %q", got)
			}
		})
	}
}

func TestProvidersMoveFilesAndFolders(t *testing.T) {
	ctx := context.Background()
	for kind, p := range providers(t) {
		t.Run(kind, func(t *testing.T) {
			write(t, p, "inbox/Heat/video.mp4", "video")
			write(t, p, "inbox/Heat/video.srt", "subs")
			write(t, p, "inbox/Heat/extras/trailer.mp4", "trailer")
			write(t, p, "inbox/note.txt", "note")

			if err := p.Move(ctx, "inbox/note.txt", "notes/2026/note.txt"); err != nil {
				t.Fatal(err)
			}
			if got := read(t, p, "notes/2026/note.txt", 0, -1); got != "note" {
				t.Fatalf("moved file = %q", got)
			}
			if _, err := p.Stat(ctx, "inbox/note.txt"); !errors.Is(err, ErrNotExist) {
				t.Fatalf("the file stayed: %v", err)
			}

			write(t, p, "library/Heat/video.srt", "old subs")
			if err := p.Move(ctx, "library/Heat/video.srt", "inbox/Heat/video.srt"); err != nil {
				t.Fatalf("move over a file: %v", err)
			}
			if got := read(t, p, "inbox/Heat/video.srt", 0, -1); got != "old subs" {
				t.Fatalf("replaced file = %q", got)
			}

			if err := p.Move(ctx, "inbox/Heat", "library/Heat (1995)"); err != nil {
				t.Fatal(err)
			}
			if got := names(t, p, "library/Heat (1995)"); !slices.Equal(got, []string{"extras/", "video.mp4", "video.srt"}) {
				t.Fatalf("moved folder lists %q", got)
			}
			if got := read(t, p, "library/Heat (1995)/extras/trailer.mp4", 0, -1); got != "trailer" {
				t.Fatalf("moved nested file = %q", got)
			}
			if _, err := p.Stat(ctx, "inbox/Heat"); !errors.Is(err, ErrNotExist) {
				t.Fatalf("the folder stayed: %v", err)
			}
		})
	}
}

func TestProvidersRemove(t *testing.T) {
	ctx := context.Background()
	for kind, p := range providers(t) {
		t.Run(kind, func(t *testing.T) {
			write(t, p, "Heat/video.mp4", "video")
			if err := p.Remove(ctx, "Heat"); !errors.Is(err, ErrNotEmpty) {
				t.Fatalf("remove a full folder = %v", err)
			}
			if err := p.Remove(ctx, "Heat/video.mp4"); err != nil {
				t.Fatal(err)
			}
			if _, err := p.Stat(ctx, "Heat/video.mp4"); !errors.Is(err, ErrNotExist) {
				t.Fatalf("the file stayed: %v", err)
			}
			if err := p.Remove(ctx, "Heat"); err != nil {
				t.Fatalf("remove the emptied folder = %v", err)
			}
			if _, err := p.Stat(ctx, "Heat"); !errors.Is(err, ErrNotExist) {
				t.Fatalf("the folder stayed: %v", err)
			}
		})
	}
}

// A body shorter than its size stores nothing, and leaves what was there.
func TestProvidersWriteNothingOfAShortBody(t *testing.T) {
	ctx := context.Background()
	for kind, p := range providers(t) {
		t.Run(kind, func(t *testing.T) {
			write(t, p, "a/video.mp4", "old")
			if err := p.Write(ctx, "a/video.mp4", strings.NewReader("new"), 10); err == nil {
				t.Fatal("a short body was stored")
			}
			if got := read(t, p, "a/video.mp4", 0, -1); got != "old" {
				t.Fatalf("after a short write = %q", got)
			}
			if got := names(t, p, "a"); !slices.Equal(got, []string{"video.mp4"}) {
				t.Fatalf("a short write left %q", got)
			}
		})
	}
}

func TestLocalWritesAtomicallyAndHidesItsTempFile(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	l := NewLocal(dir)
	write(t, l, "a/video.mp4", "old")
	// While a write is going, the file is still the old one and the temp file is not listed.
	r, w := io.Pipe()
	done := make(chan error, 1)
	go func() { done <- l.Write(ctx, "a/video.mp4", r, 6) }()
	if _, err := w.Write([]byte("new")); err != nil {
		t.Fatal(err)
	}
	if got := read(t, l, "a/video.mp4", 0, -1); got != "old" {
		t.Fatalf("during the write = %q", got)
	}
	entries, err := os.ReadDir(dir + "/a")
	if err != nil || len(entries) != 2 {
		t.Fatalf("the folder holds %v, %v", entries, err)
	}
	if got := names(t, l, "a"); !slices.Equal(got, []string{"video.mp4"}) {
		t.Fatalf("lists %q", got)
	}
	w.Write([]byte("abc"))
	w.Close()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if got := read(t, l, "a/video.mp4", 0, -1); got != "newabc" {
		t.Fatalf("after the write = %q", got)
	}
	if entries, _ := os.ReadDir(dir + "/a"); len(entries) != 1 {
		t.Fatalf("the temp file stayed: %v", entries)
	}
}

func TestLocalRefusesASymlinkOutOfTheRoot(t *testing.T) {
	outside := t.TempDir()
	if err := os.WriteFile(outside+"/secret", []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.Symlink(outside, dir+"/out"); err != nil {
		t.Skip("no symlinks here:", err)
	}
	l := NewLocal(dir)
	if r, err := l.Open(context.Background(), "out/secret", 0, -1); err == nil {
		r.Close()
		t.Fatal("read through a symlink out of the root")
	}
	if err := l.Write(context.Background(), "out/planted", strings.NewReader("x"), 1); err == nil {
		t.Fatal("wrote through a symlink out of the root")
	}
	if _, err := os.Stat(outside + "/planted"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("planted: %v", err)
	}
}

func TestLocalMkdirAllAndOpenAFolder(t *testing.T) {
	ctx := context.Background()
	l := NewLocal(t.TempDir())
	if err := l.MkdirAll(ctx, "a/b/c"); err != nil {
		t.Fatal(err)
	}
	if info, err := l.Stat(ctx, "a/b/c"); err != nil || !info.Dir {
		t.Fatalf("stat = %+v, %v", info, err)
	}
	if _, err := l.Open(ctx, "a/b", 0, -1); !errors.Is(err, ErrIsDir) {
		t.Fatalf("open a folder = %v", err)
	}
}

func TestObjectStoreKeysLiveUnderItsRoot(t *testing.T) {
	ctx := context.Background()
	store, fake := fakeStore(t, "kando/films/")
	write(t, store, "Heat/video.mp4", "video")
	if _, ok := fake.object("kando/films/Heat/video.mp4"); !ok {
		t.Fatalf("keys %v", fake.objects)
	}
	// An object outside the root is not seen.
	fake.mu.Lock()
	fake.store("kando/other.mp4", []byte("x"))
	fake.mu.Unlock()
	if got := names(t, store, ""); !slices.Equal(got, []string{"Heat/"}) {
		t.Fatalf("root lists %q", got)
	}
	if _, err := NewObjectStore(nil, "../x"); !errors.Is(err, ErrInvalidPath) {
		t.Fatalf("root outside = %v", err)
	}
	// The root of an empty bucket lists nothing rather than failing.
	empty, _ := fakeStore(t, "")
	if infos, err := empty.List(ctx, ""); err != nil || len(infos) != 0 {
		t.Fatalf("empty root = %v, %v", infos, err)
	}
}

// Listing a folder of many objects goes page after page, and a folder holding only folders is there.
func TestObjectStoreListsEveryPage(t *testing.T) {
	store, _ := fakeStore(t, "")
	var want []string
	for _, name := range []string{"a", "b", "c", "d", "e"} {
		write(t, store, "films/"+name+".mp4", name)
		want = append(want, name+".mp4")
		write(t, store, "films/"+name+"/video.mp4", name)
		want = append(want, name+"/")
	}
	slices.Sort(want)
	if got := names(t, store, "films"); !slices.Equal(got, want) {
		t.Fatalf("lists %q, want %q", got, want)
	}
}

func TestObjectStoreLinksAndReadsByRange(t *testing.T) {
	ctx := context.Background()
	store, fake := fakeStore(t, "kando")
	write(t, store, "Heat/video.mp4", "0123456789")
	if got := read(t, store, "Heat/video.mp4", 2, 3); got != "234" {
		t.Fatalf("range = %q", got)
	}
	if got := fake.ranges; !slices.Equal(got, []string{"bytes=2-4"}) {
		t.Fatalf("ranges asked %q", got)
	}
	var linker Linker = store
	link, err := linker.Link(ctx, "Heat/video.mp4", 0)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(link, "/films/kando/Heat/video.mp4?") || !strings.Contains(link, "X-Amz-Signature=") {
		t.Fatalf("link %s", link)
	}
}

// A body larger than one PUT is not read into memory whole: it is sent as an upload in parts by the cloud package, which its own tests cover with short parts; here a body over the threshold goes in parts and the object holds all of it.
func TestObjectStoreWritesALargeBodyInParts(t *testing.T) {
	if testing.Short() {
		t.Skip("writes 40 MiB")
	}
	store, fake := fakeStore(t, "")
	body := bytes.Repeat([]byte("kando-"), 40<<20/6)
	// A pipe cannot go back to its start, so only parts of it are kept in memory.
	r, w := io.Pipe()
	go func() {
		w.Write(body)
		w.Close()
	}()
	if err := store.Write(context.Background(), "big.mp4", r, int64(len(body))); err != nil {
		t.Fatal(err)
	}
	if got, _ := fake.object("big.mp4"); !bytes.Equal(got, body) {
		t.Fatalf("stored %d bytes of %d", len(got), len(body))
	}
	if fake.count("part") != 2 || fake.count("complete") != 1 || fake.count("put") != 0 {
		t.Fatalf("calls %v", fake.calls)
	}
}
