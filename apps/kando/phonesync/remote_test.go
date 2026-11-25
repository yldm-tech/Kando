package phonesync

import (
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// remoteSource is fakeSource's library with a film whose video was moved off the computer to a storage location besides: its subtitles on disk, nothing where its video was, and its video served by Remote.
type remoteSource struct {
	fakeSource
	remote http.Handler
	moved  time.Time
}

func (f remoteSource) Films(ctx context.Context, lib Library) ([]Film, error) {
	films, err := f.fakeSource.Films(ctx, lib)
	if err != nil {
		return nil, err
	}
	return append(films, Film{ID: "moved", Title: "Moved", Source: "youtube",
		Video: filepath.Join(f.dir, "moved.mp4"), Subtitles: filepath.Join(f.dir, "moved.bilingual.srt"),
		Remote: f.remote, RemoteSize: 26, RemoteAt: f.moved, SourceKey: "6aTKFGnkWxhnQ8vrl6Wj-G"}), nil
}

// A film whose video was moved to a storage location is listed with the size the app noted, and its video is served by ranges from there, as one on disk is. The key of the file of a NAS it was made of goes with it to the phone (psync.Film.SourceKey), and a film of none has none.
func TestServesAVideoMovedToAStorageLocation(t *testing.T) {
	lib := t.TempDir()
	for name, data := range map[string]string{"talk.mp4": "0123456789abcdefghij", "talk.bilingual.srt": "x", "moved.bilingual.srt": "1\n00:00:01,000 --> 00:00:02,000\n你好\nHello\n"} {
		if err := os.WriteFile(filepath.Join(lib, name), []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	moved := time.Date(2026, 11, 18, 9, 0, 0, 0, time.UTC)
	body := "abcdefghijklmnopqrstuvwxyz"
	remote := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.ServeContent(w, r, "moved.mp4", moved, strings.NewReader(body))
	})
	srv, err := Start(Config{Dir: t.TempDir(), Name: "Test Mac", Source: remoteSource{fakeSource{dir: lib}, remote, moved}, Addrs: loopback, Logf: testLogf(t)})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { srv.Close() })
	ctx := context.Background()
	c := client(t, srv, "")
	if _, err := pair(t, c, srv.NewOffer().Code); err != nil {
		t.Fatal(err)
	}
	films, err := c.Films(ctx, "lib1")
	if err != nil || len(films) != 2 {
		t.Fatalf("films %+v, %v", films, err)
	}
	if films[0].SourceKey != "" {
		t.Fatalf("a film of no source has the key %q", films[0].SourceKey)
	}
	if f := films[1]; f.ID != "moved" || f.SubtitlesOnly || f.VideoSize != 26 || f.Updated.Before(moved) || f.SourceKey != "6aTKFGnkWxhnQ8vrl6Wj-G" {
		t.Fatalf("film %+v", f)
	}
	part, err := c.Video(ctx, "moved", 2, 4, "")
	if err != nil {
		t.Fatal(err)
	}
	defer part.Body.Close()
	got, err := io.ReadAll(part.Body)
	if err != nil || string(got) != "cde" || part.Start != 2 || part.End != 4 || part.Total != 26 {
		t.Fatalf("part %q %d-%d of %d, %v", got, part.Start, part.End, part.Total, err)
	}
}
