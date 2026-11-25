package storage

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"kando/packages/remotefs"
	"kando/packages/remotefs/providertest"
)

// TestPutAndGetCopyAFileWhole checks, against every provider, that Put copies a file up byte for byte and tells its progress up to the whole size, and that Get copies it back down whole, leaving no hidden file beside it, and does not write over a file already there.
func TestPutAndGetCopyAFileWhole(t *testing.T) {
	ctx := context.Background()
	body := bytes.Repeat([]byte("0123456789abcdef"), 70_000)
	for kind, p := range copyProviders(t) {
		t.Run(kind, func(t *testing.T) {
			dir := t.TempDir()
			local := filepath.Join(dir, "video.mp4")
			if err := os.WriteFile(local, body, 0o644); err != nil {
				t.Fatal(err)
			}
			var last, total int64
			size, err := Put(ctx, p, "Kando/Talk/video.mp4", local, func(done, all int64) { last, total = done, all })
			if err != nil {
				t.Fatal(err)
			}
			if size != int64(len(body)) || last != size || total != size {
				t.Fatalf("Put = %d, progress %d of %d, want %d", size, last, total, len(body))
			}
			if got := providertest.Read(t, p, "Kando/Talk/video.mp4", 0, -1); got != string(body) {
				t.Fatalf("the provider holds %d bytes that differ", len(got))
			}
			if _, err := os.Stat(local); err != nil {
				t.Fatalf("Put took the file: %v", err)
			}

			back := filepath.Join(dir, "back", "video.mp4")
			last = 0
			if err := Get(ctx, p, "Kando/Talk/video.mp4", back, func(done, _ int64) { last = done }); err != nil {
				t.Fatal(err)
			}
			got, err := os.ReadFile(back)
			if err != nil || !bytes.Equal(got, body) || last != int64(len(body)) {
				t.Fatalf("Get wrote %d bytes (%v), progress %d", len(got), err, last)
			}
			entries, _ := os.ReadDir(filepath.Dir(back))
			if len(entries) != 1 {
				t.Fatalf("Get left %v beside the file", entries)
			}
			if err := Get(ctx, p, "Kando/Talk/video.mp4", back, nil); !errors.Is(err, os.ErrExist) {
				t.Fatalf("Get over a file = %v", err)
			}
			if err := Get(ctx, p, "Kando/Talk/missing.mp4", filepath.Join(dir, "missing.mp4"), nil); !errors.Is(err, remotefs.ErrNotExist) {
				t.Fatalf("Get of a missing file = %v", err)
			}
		})
	}
}

// TestPutAndGetStopWithTheirContext checks that a copy stopped partway fails and leaves nothing: Put stores nothing at the path, Get writes nothing beside the file.
func TestPutAndGetStopWithTheirContext(t *testing.T) {
	body := strings.Repeat("x", 1<<20)
	p := NewLocal(t.TempDir())
	dir := t.TempDir()
	local := filepath.Join(dir, "video.mp4")
	if err := os.WriteFile(local, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	_, err := Put(ctx, p, "video.mp4", local, func(done, _ int64) {
		if done > 0 {
			cancel()
		}
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("a stopped Put = %v", err)
	}
	if _, err := p.Stat(context.Background(), "video.mp4"); !errors.Is(err, remotefs.ErrNotExist) {
		t.Fatalf("a stopped Put stored something: %v", err)
	}
	if names := providertest.Names(t, p, ""); len(names) != 0 {
		t.Fatalf("a stopped Put left %v", names)
	}

	providertest.Write(t, p, "video.mp4", body)
	ctx, cancel = context.WithCancel(context.Background())
	back := filepath.Join(dir, "back", "video.mp4")
	err = Get(ctx, p, "video.mp4", back, func(done, _ int64) {
		if done > 0 {
			cancel()
		}
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("a stopped Get = %v", err)
	}
	if entries, _ := os.ReadDir(filepath.Dir(back)); len(entries) != 0 {
		t.Fatalf("a stopped Get left %v", entries)
	}
}

// shortStore is a provider that stores one byte fewer than it is given, as a store that loses data would, so the size check of a copy fails.
type shortStore struct{ remotefs.Provider }

func (s shortStore) Write(ctx context.Context, p string, r io.Reader, size int64) error {
	data, err := io.ReadAll(io.LimitReader(r, size))
	if err != nil {
		return err
	}
	return s.Provider.Write(ctx, p, bytes.NewReader(data[:len(data)-1]), size-1)
}

// TestPutRemovesACopyOfTheWrongSize checks that a copy the provider kept fewer bytes of is refused as ErrSizeMismatch and removed, and the file stays.
func TestPutRemovesACopyOfTheWrongSize(t *testing.T) {
	inner := NewLocal(t.TempDir())
	local := filepath.Join(t.TempDir(), "video.mp4")
	if err := os.WriteFile(local, []byte("picture"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Put(context.Background(), shortStore{inner}, "video.mp4", local, nil); !errors.Is(err, ErrSizeMismatch) {
		t.Fatalf("Put to a short store = %v", err)
	}
	if _, err := inner.Stat(context.Background(), "video.mp4"); !errors.Is(err, remotefs.ErrNotExist) {
		t.Fatalf("the short copy stayed: %v", err)
	}
	if _, err := os.Stat(local); err != nil {
		t.Fatal(err)
	}
}

// TestGetCopiesFromAReaderThatIsNotAProvider copies down a file of a Reader alone, as a media source is, which has Stat and Open and nothing else of a Provider.
func TestGetCopiesFromAReaderThatIsNotAProvider(t *testing.T) {
	inner := NewLocal(t.TempDir())
	providertest.Write(t, inner, "Films/Heat.mkv", "0123456789")
	src := struct{ remotefs.Reader }{inner}
	local := filepath.Join(t.TempDir(), "Heat.mkv")
	if err := Get(t.Context(), src, "Films/Heat.mkv", local, nil); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(local); err != nil || string(got) != "0123456789" {
		t.Fatalf("Get copied %q, %v", got, err)
	}
}

func TestFilmPathKeepsTheLibrarysLayout(t *testing.T) {
	for _, c := range []struct{ library, rel, want string }{
		{"我的影片", filepath.Join("Talk", "video.mp4"), "Kando/我的影片/Talk/video.mp4"},
		{"a/b", "video.mp4", "Kando/a-b/video.mp4"},
		{"", "video.mp4", "Kando/Library/video.mp4"},
		{"Films", filepath.Join("..", "video.mp4"), ""},
		{"Films", "", ""},
	} {
		if got := FilmPath(c.library, c.rel); got != c.want {
			t.Errorf("FilmPath(%q, %q) = %q, want %q", c.library, c.rel, got, c.want)
		}
	}
}
