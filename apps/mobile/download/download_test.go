package download

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"kando/apps/mobile/store"
	"kando/packages/sync"
	"kando/packages/sync/synctest"
)

// paired is a desktop with one film, video bytes long, and a client paired with it.
func paired(t *testing.T, video []byte) (*synctest.Desktop, *sync.Client) {
	t.Helper()
	d := synctest.New(t, "Studio")
	d.AddLibrary("lib", "Films")
	d.AddFilm(synctest.Film{Film: sync.Film{ID: "f1", Title: "Talk"}, Library: "lib", Video: video, Poster: []byte("jpeg"),
		Cues: []sync.Cue{{StartMS: 0, EndMS: 1000, Original: "hi", Translation: "你好"}}})
	d.Offer("123456")
	c := sync.NewClient(d.Addr, d.Fingerprint, "")
	t.Cleanup(c.Close)
	if _, err := c.Pair(context.Background(), sync.PairRequest{Code: "123456", Device: "Phone", Platform: sync.PlatformIOS}); err != nil {
		t.Fatal(err)
	}
	return d, c
}

func filmOf(t *testing.T, c *sync.Client) store.Film {
	t.Helper()
	films, err := c.Films(context.Background(), "lib")
	if err != nil || len(films) != 1 {
		t.Fatalf("films %v, %v", films, err)
	}
	return store.Film{Desktop: "desk", Library: "Films", Film: films[0]}
}

func TestADownloadCutOffGoesOnFromWhereItStopped(t *testing.T) {
	video := bytes.Repeat([]byte("0123456789"), 100_000)
	d, c := paired(t, video)
	st := store.Open(t.TempDir())
	f := filmOf(t, c)

	d.Stall(300_000)
	ctx, cancel := context.WithCancel(context.Background())
	var last int64
	got, err := Film(ctx, c, st, f, func(done, total int64) {
		last = done
		if done >= 300_000 {
			cancel()
		}
	})
	if !errors.Is(err, context.Canceled) || got.Done() || last != 300_000 {
		t.Fatalf("stopped: %+v, %v, at %d", got, err, last)
	}
	if got.ETag == "" || got.Poster != "poster.jpg" {
		t.Fatalf("the record after stopping: %+v", got)
	}
	dir := st.FilmDir("desk", "f1")
	if info, err := os.Stat(filepath.Join(dir, store.PartFile)); err != nil || info.Size() != 300_000 {
		t.Fatalf("part %v, %v", info, err)
	}

	d.Stall(0)
	var first int64 = -1
	got, err = Film(context.Background(), c, st, got, func(done, total int64) {
		if first < 0 {
			first = done
		}
	})
	if err != nil || !got.Done() || got.Video != "video.mp4" || first != 300_000 {
		t.Fatalf("resumed: %+v, %v, from %d", got, err, first)
	}
	if data, _ := os.ReadFile(filepath.Join(dir, got.Video)); !bytes.Equal(data, video) {
		t.Fatalf("the video kept is %d bytes, not the desktop's", len(data))
	}
	if r := d.Ranges(); len(r) != 2 || r[0] != "bytes=0-" || r[1] != "bytes=300000- "+got.ETag {
		t.Fatalf("ranges asked for %q", r)
	}
	if cues, ok, err := st.Cues("desk", "f1"); !ok || err != nil || len(cues) != 1 || cues[0].Translation != "你好" {
		t.Fatalf("cues %v %v %v", cues, ok, err)
	}
	if _, err := os.Stat(filepath.Join(dir, store.PartFile)); !os.IsNotExist(err) {
		t.Fatalf("the part file stayed: %v", err)
	}
}

func TestAVideoThatChangedOnTheDesktopStartsOver(t *testing.T) {
	d, c := paired(t, bytes.Repeat([]byte("a"), 200_000))
	st := store.Open(t.TempDir())
	f := filmOf(t, c)
	d.Stall(50_000)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	stopped, cancelStop := context.WithCancel(ctx)
	f, err := Film(stopped, c, st, f, func(done, _ int64) {
		if done >= 50_000 {
			cancelStop()
		}
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("stopping: %v", err)
	}
	old := f.ETag
	changed := bytes.Repeat([]byte("b"), 120_000)
	d.ReplaceVideo("f1", changed)
	d.Stall(0)
	f, err = Film(ctx, c, st, f, func(int64, int64) {})
	if err != nil || !f.Done() {
		t.Fatalf("after the change: %+v, %v", f, err)
	}
	if data, _ := os.ReadFile(filepath.Join(st.FilmDir("desk", "f1"), f.Video)); !bytes.Equal(data, changed) {
		t.Fatalf("kept %d bytes, not the changed video's %d", len(data), len(changed))
	}
	if r := d.Ranges(); len(r) != 2 || r[1] != "bytes=50000- "+old || f.ETag == old {
		t.Fatalf("ranges %q", r)
	}
}

func TestExtensionNamesTheVideoByItsType(t *testing.T) {
	for kind, want := range map[string]string{"video/mp4": ".mp4", "video/quicktime": ".mov", "video/x-matroska": ".mkv", "video/webm; codecs=vp9": ".webm", "application/octet-stream": ".mp4", "": ".mp4"} {
		if got := extension(kind, ".mp4"); got != want {
			t.Errorf("extension(%q) = %q, want %q", kind, got, want)
		}
	}
}
