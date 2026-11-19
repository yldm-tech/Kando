package phonesync

import (
	"context"
	"encoding/json"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	gosync "sync"
	"testing"
	"time"

	psync "kando/packages/sync"
)

// filmSource is a library whose films a test changes: "talk", with its video, lines and poster on disk, and "notes", lines alone, its video not on disk.
type filmSource struct {
	dir string

	mu    gosync.Mutex
	films []Film
}

func newFilmSource(t *testing.T) *filmSource {
	t.Helper()
	dir := t.TempDir()
	for name, data := range map[string]string{"talk.mp4": "0123456789abcdefghij", "talk.bilingual.srt": "1\n00:00:01,000 --> 00:00:02,000\n你好\nHello\n", "cover.jpg": "jpeg"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return &filmSource{dir: dir, films: []Film{
		{ID: "talk", Title: "Rust Talk", Source: "youtube", Duration: 90 * time.Second, Video: filepath.Join(dir, "talk.mp4"), Subtitles: filepath.Join(dir, "talk.bilingual.srt"), Poster: filepath.Join(dir, "cover.jpg")},
		{ID: "notes", Title: "Notes", Video: filepath.Join(dir, "notes.mp4")},
	}}
}

func (f *filmSource) Libraries(context.Context) ([]Library, error) {
	return []Library{{ID: "lib1", Name: "My Films", Dir: f.dir}}, nil
}

func (f *filmSource) Films(context.Context, Library) ([]Film, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.films), nil
}

func (f *filmSource) Cues(_ context.Context, film Film) ([]psync.Cue, error) {
	if film.ID == "mute" {
		return nil, ErrNotFound
	}
	return []psync.Cue{{StartMS: 1000, EndMS: 2000, Original: "Hello " + film.Title, Translation: "你好"}}, nil
}

func (f *filmSource) change(fn func([]Film) []Film) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.films = fn(f.films)
}

// filmsRig is a server whose relay shares src's films through bucket, with the statuses it told.
type filmsRig struct {
	srv    *Server
	dir    string
	bucket *memBucket
	src    *filmSource
	prefix string

	mu     gosync.Mutex
	status FilmsStatus
}

func startFilms(t *testing.T, dir string, bucket *memBucket, src *filmSource, cfg FilmsConfig) *filmsRig {
	t.Helper()
	every := relayEvery
	relayEvery = 50 * time.Millisecond
	t.Cleanup(func() { relayEvery = every })
	g := &filmsRig{dir: dir, bucket: bucket, src: src}
	cfg.Status = func(st FilmsStatus) {
		g.mu.Lock()
		defer g.mu.Unlock()
		g.status = st
	}
	srv, err := Start(Config{Dir: dir, Name: "Test Mac", Source: src, Jobs: &fakeJobs{}, Relay: bucket, Films: &cfg, Addrs: loopback, Logf: testLogf(t)})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { srv.Close() })
	g.srv, g.prefix = srv, psync.RelayPrefix(srv.Fingerprint())
	return g
}

func (g *filmsRig) told() FilmsStatus {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.status
}

// objects are the names below the relay's prefix of the films' objects in the bucket.
func (g *filmsRig) objects() []string {
	var out []string
	for _, k := range g.bucket.keys() {
		if name, ok := strings.CutPrefix(k, g.prefix); ok && strings.HasPrefix(name, psync.RelayFilmsFolder) {
			out = append(out, name)
		}
	}
	return out
}

// files are the films as the relay lists them in the phones' state.json, by ID.
func (g *filmsRig) files() map[string]psync.RelayFilmFiles {
	out := map[string]psync.RelayFilmFiles{}
	if f := g.srv.relay.Films(); f != nil {
		for _, film := range f.Films {
			out[film.Film.ID] = film
		}
	}
	return out
}

// The relay puts every film in the bucket: the lines and poster of each, the video of one whose file is on disk, and lists them for the phones with the library they are in. Started again, it puts nothing again; in another bucket, it puts them all there.
func TestFilmsGoToTheBucketForThePhones(t *testing.T) {
	dir := t.TempDir()
	bucket := &memBucket{}
	src := newFilmSource(t)
	g := startFilms(t, dir, bucket, src, FilmsConfig{Videos: true, Store: "r2/films"})
	waitFor(t, "the video in the bucket", func() bool { return g.files()["talk"].Video != "" && g.told().WithVideo == 1 })
	files := g.files()
	talk, notes := files["talk"], files["notes"]
	if talk.Library != "lib1" || talk.Subtitles == "" || !strings.HasSuffix(talk.Video, "/video.mp4") || !strings.HasSuffix(talk.Poster, "/poster.jpg") || talk.Film.VideoSize != 20 || talk.Film.Title != "Rust Talk" || talk.Film.DurationMS != 90_000 {
		t.Fatalf("talk %+v", talk)
	}
	if notes.Subtitles == "" || notes.Video != "" || notes.Poster != "" {
		t.Fatalf("a film without its video on disk: %+v", notes)
	}
	if libs := g.srv.relay.Films().Libraries; len(libs) != 1 || libs[0].Name != "My Films" {
		t.Fatalf("libraries %+v", libs)
	}
	if path.Dir(talk.Video) == path.Dir(notes.Subtitles) {
		t.Fatal("two films share a folder")
	}
	var lines psync.Subtitles
	body, _ := bucket.Get(context.Background(), g.prefix+talk.Subtitles)
	if err := json.Unmarshal(body, &lines); err != nil || len(lines.Cues) != 1 || lines.Cues[0].Original != "Hello Rust Talk" {
		t.Fatalf("the lines in the bucket: %+v, %v", lines, err)
	}
	st := g.told()
	if !st.Measured || st.Films != 2 || st.Videos != 20 || st.Lines == 0 || st.Waiting != 0 || st.Uploading != "" || st.Err != nil {
		t.Fatalf("status %+v", st)
	}
	saved, err := os.ReadFile(filepath.Join(dir, FilmsFile))
	if err != nil || !strings.Contains(string(saved), `"store": "r2/films"`) {
		t.Fatalf("%s: %s, %v", FilmsFile, saved, err)
	}
	g.srv.Close()

	puts := len(bucket.putFiles())
	g = startFilms(t, dir, bucket, src, FilmsConfig{Videos: true, Store: "r2/films"})
	waitFor(t, "the films listed again", func() bool { return len(g.files()) == 2 && g.told().Measured })
	if got := bucket.putFiles(); len(got) != puts {
		t.Fatalf("started again, it put %q", got[puts:])
	}
	g.srv.Close()

	moved := &memBucket{}
	g = startFilms(t, dir, moved, src, FilmsConfig{Videos: true, Store: "s3/elsewhere"})
	waitFor(t, "the films in the other bucket", func() bool { return g.files()["talk"].Video != "" })
	if got := moved.putFiles(); len(got) != 2 {
		t.Fatalf("in another bucket it put %q", got)
	}
}

// An upload that stopped goes on from where it got, as the store keeps the parts it took.
func TestAFilmsUploadGoesOnWhereItStopped(t *testing.T) {
	retry := filmsRetry
	filmsRetry = 50 * time.Millisecond
	t.Cleanup(func() { filmsRetry = retry })
	bucket := &memBucket{fail: map[string]bool{"talk.mp4": true}}
	g := startFilms(t, t.TempDir(), bucket, newFilmSource(t), FilmsConfig{Videos: true, Store: "r2/films"})
	waitFor(t, "the upload to fail", func() bool { return g.told().Err != nil })
	saved, _ := os.ReadFile(filepath.Join(g.dir, FilmsFile))
	if !strings.Contains(string(saved), `"uploadId": "u1"`) {
		t.Fatalf("what was kept of the upload: %s", saved)
	}
	bucket.mu.Lock()
	bucket.fail = nil
	bucket.mu.Unlock()
	waitFor(t, "the video in the bucket", func() bool { return g.files()["talk"].Video != "" })
	bucket.mu.Lock()
	defer bucket.mu.Unlock()
	if !slices.Contains(bucket.resumed, "u1") {
		t.Fatalf("the uploads went on from %q", bucket.resumed)
	}
}

// With videos off, the films go as their lines and posters alone.
func TestFilmsGoAsTheirLinesAloneWithVideosOff(t *testing.T) {
	g := startFilms(t, t.TempDir(), &memBucket{}, newFilmSource(t), FilmsConfig{Store: "r2/films"})
	waitFor(t, "the films listed", func() bool { return len(g.files()) == 2 && g.told().Measured })
	for _, name := range g.objects() {
		if strings.Contains(name, "/video") {
			t.Fatalf("a video went up: %q", g.objects())
		}
	}
	g.srv.SetFilms(true, 0)
	waitFor(t, "the video once videos go", func() bool { return g.files()["talk"].Video != "" })
}

// A video watched to its end is listed to clean up; cleaned up, it leaves the bucket while its lines stay, and is not put back. With the cleanup set to a while after watching, it goes once that while has passed. A film no library serves any more leaves the bucket whole.
func TestWatchedVideosAreCleanedUp(t *testing.T) {
	src := newFilmSource(t)
	watched := time.Now().Add(-10 * 24 * time.Hour)
	src.change(func(f []Film) []Film { f[0].Watched, f[0].WatchedAt = true, watched; return f })
	g := startFilms(t, t.TempDir(), &memBucket{}, src, FilmsConfig{Videos: true, Store: "r2/films"})
	waitFor(t, "the watched film listed", func() bool { return len(g.told().Watched) == 1 })
	if w := g.told().Watched[0]; w.ID != "talk" || w.Library != "lib1" || w.Title != "Rust Talk" || w.Size != 20 || !w.At.Equal(watched) {
		t.Fatalf("watched %+v", w)
	}
	done := make(chan error, 1)
	g.srv.CleanVideos([]FilmRef{{Library: "lib1", ID: "talk"}}, func(err error) { done <- err })
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if f := g.files()["talk"]; f.Video != "" || f.Subtitles == "" {
		t.Fatalf("cleaned up: %+v", f)
	}
	if st := g.told(); len(st.Watched) != 0 || st.Videos != 0 || st.WithVideo != 0 {
		t.Fatalf("status after the cleanup %+v", st)
	}
	g.srv.SetFilms(true, 0)
	time.Sleep(200 * time.Millisecond)
	if g.files()["talk"].Video != "" {
		t.Fatal("a video cleaned up went up again")
	}
	g.srv.Close()

	src = newFilmSource(t)
	src.change(func(f []Film) []Film { f[0].Watched, f[0].WatchedAt = true, watched; return f })
	g = startFilms(t, t.TempDir(), &memBucket{}, src, FilmsConfig{Videos: true, Store: "r2/films", CleanAfter: 30 * 24 * time.Hour})
	waitFor(t, "the video in the bucket", func() bool { return g.files()["talk"].Video != "" })
	g.srv.SetFilms(true, 7*24*time.Hour)
	waitFor(t, "the cleanup a week after watching", func() bool { return g.files()["talk"].Video == "" && g.files()["talk"].Subtitles != "" })

	folder := path.Dir(g.files()["talk"].Subtitles)
	src.change(func(f []Film) []Film { return f[1:] })
	g.srv.SetFilms(true, 7*24*time.Hour)
	waitFor(t, "the film gone from the bucket", func() bool {
		_, listed := g.files()["talk"]
		return !listed && !slices.ContainsFunc(g.objects(), func(name string) bool { return strings.HasPrefix(name, folder+"/") })
	})
}

// A film without lines yet is not shared, and is no failure of the turn.
func TestAFilmWithoutLinesIsNotShared(t *testing.T) {
	src := newFilmSource(t)
	src.change(func(f []Film) []Film { return append(f, Film{ID: "mute", Title: "Mute", Video: f[0].Video}) })
	g := startFilms(t, t.TempDir(), &memBucket{}, src, FilmsConfig{Videos: true, Store: "r2/films"})
	waitFor(t, "the films shared", func() bool { return g.files()["talk"].Video != "" && g.told().Measured })
	if _, shared := g.files()["mute"]; shared || g.told().Err != nil || g.told().Films != 2 {
		t.Fatalf("files %v, status %+v", g.files(), g.told())
	}
}
