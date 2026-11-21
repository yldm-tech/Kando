package phonesync

import (
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	gosync "sync"
	"testing"
	"time"

	psync "kando/packages/sync"
)

// makingSource is fakeSource's library with films being made besides: a link whose sound plays until its video comes, and one waiting with nothing of it in yet. A test moves them on.
type makingSource struct {
	fakeSource
	mu     gosync.Mutex
	making []Film
	done   []Film
}

func (m *makingSource) Films(ctx context.Context, lib Library) ([]Film, error) {
	films, err := m.fakeSource.Films(ctx, lib)
	m.mu.Lock()
	defer m.mu.Unlock()
	return append(films, m.done...), err
}

func (m *makingSource) Making(_ context.Context, lib Library) ([]Film, error) {
	if lib.ID != "lib1" {
		return nil, ErrNotFound
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return slices.Clone(m.making), nil
}

func (m *makingSource) change(fn func()) {
	m.mu.Lock()
	defer m.mu.Unlock()
	fn()
}

// watched is what the app was told of the phones watching.
type watched struct {
	mu  gosync.Mutex
	got []string
}

func (w *watched) add(caller psync.Caller, f Film, at time.Duration, claim bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.got = append(w.got, caller.DeviceID+" "+f.ID+" "+at.String()+map[bool]string{true: " claim", false: ""}[claim])
}

func (w *watched) list() []string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return slices.Clone(w.got)
}

// A film being made is listed to a phone that asks for such films alone, plays what is in of it so far, and its watching tells the app where it is watched and the phone how far it has come, until it is done and found as any other film under the same ID.
func TestAFilmBeingMadeIsWatched(t *testing.T) {
	lib := t.TempDir()
	for name, data := range map[string]string{"talk.mp4": "0123456789abcdefghij", "talk.bilingual.srt": "x", "link.m4a": "sound", "link.mp4": "the picture"} {
		if err := os.WriteFile(filepath.Join(lib, name), []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	link := Film{ID: "link", Title: "A Link", Source: "youtube", Making: true, ComingSound: filepath.Join(lib, "link.m4a"), Video: filepath.Join(lib, "link.mp4"), Subtitles: filepath.Join(lib, "link.bilingual.srt")}
	src := &makingSource{fakeSource: fakeSource{dir: lib}, making: []Film{link, {ID: "waiting", Title: "Waiting", Source: "web", Making: true}}}
	seen := &watched{}
	srv, err := Start(Config{Dir: t.TempDir(), Name: "Test Mac", Source: src, Watched: seen.add, Addrs: loopback, Logf: testLogf(t)})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { srv.Close() })
	ctx := context.Background()
	c := client(t, srv, "")
	paired, err := pair(t, c, srv.NewOffer().Code)
	if err != nil {
		t.Fatal(err)
	}

	films, err := c.Films(ctx, "lib1")
	if err != nil || len(films) != 1 || films[0].ID != "talk" {
		t.Fatalf("a phone from before films being made lists %+v, %v", films, err)
	}
	making, err := c.Making(ctx, "lib1")
	if err != nil || len(making) != 2 || !making[0].Making || making[0].VideoSize != 5 || making[1].ID != "waiting" || making[1].VideoSize != 0 {
		t.Fatalf("the films being made are %+v, %v", making, err)
	}
	video := func(id string) (string, error) {
		part, err := c.Video(ctx, id, 0, -1, "")
		if err != nil {
			return "", err
		}
		defer part.Body.Close()
		got, err := io.ReadAll(part.Body)
		return string(got), err
	}
	if got, err := video("link"); err != nil || got != "sound" {
		t.Fatalf("the link plays %q, %v", got, err)
	}
	_, err = video("waiting")
	wantCode(t, err, http.StatusNotFound, psync.CodeNotFound)

	w, err := c.WatchFilm(ctx, "link", 90*time.Second, true)
	if err != nil || !w.Making || w.Plays != psync.PlaysSound {
		t.Fatalf("watching the link answered %+v, %v", w, err)
	}
	if w, err := c.WatchFilm(ctx, "waiting", 0, true); err != nil || !w.Making || w.Plays != "" {
		t.Fatalf("watching the film waiting answered %+v, %v", w, err)
	}

	// Its video comes: the next watching says so, and the video plays where the sound did.
	src.change(func() { src.making[0].ComingSound = "" })
	if w, err := c.WatchFilm(ctx, "link", 95*time.Second, false); err != nil || !w.Making || w.Plays != psync.PlaysVideo {
		t.Fatalf("watching the link once its video came answered %+v, %v", w, err)
	}
	if got, err := video("link"); err != nil || got != "the picture" {
		t.Fatalf("the link plays %q, %v once its video came", got, err)
	}

	// Done, it is found among the films at once, under the same ID, and the app is no longer told of it.
	src.change(func() {
		done := src.making[0]
		done.Making = false
		src.done, src.making = []Film{done}, src.making[1:]
	})
	if w, err := c.WatchFilm(ctx, "link", 100*time.Second, false); err != nil || w.Making || w.Plays != psync.PlaysVideo {
		t.Fatalf("watching the link once done answered %+v, %v", w, err)
	}
	if got, err := video("link"); err != nil || got != "the picture" {
		t.Fatalf("the link plays %q, %v once done", got, err)
	}
	_, err = c.WatchFilm(ctx, "gone", 0, true)
	wantCode(t, err, http.StatusNotFound, psync.CodeNotFound)

	want := []string{paired.DeviceID + " link 1m30s claim", paired.DeviceID + " waiting 0s claim", paired.DeviceID + " link 1m35s"}
	if got := seen.list(); !slices.Equal(got, want) {
		t.Fatalf("the app was told %q, want %q", got, want)
	}
}

// Without the app taking the phones' watching, the call is not offered.
func TestWatchingIsAnAddition(t *testing.T) {
	srv, _, _ := startTest(t)
	c := client(t, srv, "")
	if _, err := pair(t, c, srv.NewOffer().Code); err != nil {
		t.Fatal(err)
	}
	_, err := c.WatchFilm(context.Background(), "talk", 0, true)
	wantCode(t, err, http.StatusNotImplemented, psync.CodeUnsupported)
}
