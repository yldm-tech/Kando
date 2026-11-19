package main

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"kando/apps/kando/account"
	"kando/apps/kando/film"
	"kando/apps/kando/films"
	"kando/apps/kando/locales/testlocale"
	"kando/apps/kando/settings"
	"kando/apps/kando/shelf"
	"kando/apps/kando/sidebar"
	"kando/apps/kando/storage"
	"kando/packages/core"

	ns "github.com/nsgo-dev/nsgo/core"
)

// storedPlace is the name of the NAS location the tests move videos to.
const storedPlace = "Home NAS"

// useStoredLocation gives f's app a NAS location, Home NAS of ID nas, reaching remote, opened by open (nil for storage.OpenTarget), and library.json in a file of its own, so what is saved can be read again.
func useStoredLocation(t *testing.T, f *moveFixture, remote storage.Remote, open func(context.Context, storage.Target) (storage.Provider, io.Closer, error)) string {
	t.Helper()
	a := f.a
	a.locations.list = []account.Location{{ID: "nas", Name: storedPlace, Remote: remote}}
	a.stored.pool = storage.NewPool(open)
	t.Cleanup(a.stored.pool.Close)
	path := filepath.Join(t.TempDir(), "library.json")
	lib, err := shelf.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	lib.Jobs, lib.Positions, lib.Furthest, lib.Played, lib.Delays, lib.Lifts, lib.Names = a.library.Jobs, a.library.Positions, a.library.Furthest, a.library.Played, a.library.Delays, a.library.Lifts, a.library.Names
	a.library = lib
	if err := a.library.Save(); err != nil {
		t.Fatal(err)
	}
	refresh(f.tt, a)
	return path
}

// localOpen opens every location as a folder on this computer, dir.
func localOpen(dir string) func(context.Context, storage.Target) (storage.Provider, io.Closer, error) {
	return func(context.Context, storage.Target) (storage.Provider, io.Closer, error) {
		return storage.NewLocal(dir), nil, nil
	}
}

// storedDest is where the fixture's video goes on a location.
func storedDest(f *moveFixture) string {
	return storage.FilmPath(sidebar.Title(f.from), filepath.Join("Talk", "video.mp4"))
}

// waitStored applies the background updates until the move going on ended.
func waitStored(t *testing.T, f *moveFixture) {
	t.Helper()
	waitUntil(t, f.tt, f.a, f.updates, func() bool { return f.a.stored.run == nil })
}

// playStored opens item and waits until the player has its source, or why it has none, and returns the source.
func playStored(t *testing.T, f *moveFixture, item films.Film) string {
	t.Helper()
	a := f.a
	a.Open(item)
	waitUntil(t, f.tt, a, f.updates, func() bool {
		return a.player.playErr != "" || (a.player.play != nil && a.player.play.source != "")
	})
	if a.player.play == nil {
		return ""
	}
	return a.player.play.source
}

// wantRange fails t unless url serves want for the Range header rng, as 206.
func wantRange(t *testing.T, url, rng, want string) {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Range", rng)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusPartialContent || string(body) != want {
		t.Fatalf("%s %s = %d %q, want 206 %q", url, rng, resp.StatusCode, body, want)
	}
}

// TestMoveVideoToALocationAndBack moves a done film's video to a NAS location, here a folder on this computer: the menu offers the location; the copy is the video byte for byte, and only then is the video removed from the library's folder, its subtitles and the files beside it staying; library.json keeps where it went, which a new read of it finds; the film stays on the shelf, plays through the proxy with ranges, shows its folder for 在访达中显示, and refuses processing again, uploading and moving to another library with why. Moving it back brings the same bytes back and removes the copy.
func TestMoveVideoToALocationAndBack(t *testing.T) {
	f := newMoveFixture(t)
	a, tt := f.a, f.tt
	m := testlocale.Messages()
	nas := t.TempDir()
	path := useStoredLocation(t, f, storage.Remote{Kind: storage.KindWebDAV, URL: "https://nas.local/dav"}, localOpen(nas))
	item := must(films.Find(a.films(), "film"))

	menu := a.StoredMenu(a.cx, item)
	if len(menu) != 1 || menu[0].Label != m.StoredMoveTo() || len(menu[0].Items) != 1 || menu[0].Items[0].Label != storedPlace || menu[0].Items[0].Disabled {
		t.Fatalf("the film's menu offers %+v", menu)
	}
	a.MoveVideoTo(item, "nas")
	refresh(tt, a)
	if a.stored.run == nil {
		t.Fatal("no move runs")
	}
	waitStored(t, f)
	wantText(t, tt, m.StoredMoved(moveName, storedPlace))
	dest := storedDest(f)
	got, err := os.ReadFile(filepath.Join(nas, filepath.FromSlash(dest)))
	if err != nil || string(got) != "picture" {
		t.Fatalf("the location holds %q, %v", got, err)
	}
	if _, err := os.Stat(f.video); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the video stayed in the library: %v", err)
	}
	for _, kept := range []string{"video.bilingual.srt", "video.srt", ".video.translation.json"} {
		if _, err := os.Stat(filepath.Join(f.from, "Talk", kept)); err != nil {
			t.Fatalf("%s left the library: %v", kept, err)
		}
	}
	want := shelf.Stored{Location: "nas", Path: dest, Size: int64(len("picture"))}
	if s := a.library.Stored[f.video]; s.Location != want.Location || s.Path != want.Path || s.Size != want.Size || s.Moved.IsZero() {
		t.Fatalf("library.json keeps %+v", s)
	}
	f.wantKept(t, f.video)

	// The film stays on the shelf, its video now on the location, after the folder is read again and after library.json is read again, as at the next start.
	a.refreshLibrary()
	a.openLibrary(path)
	refresh(tt, a)
	item, ok := films.Find(a.films(), "film")
	if !ok || item.Stored != "nas" || a.StoredPlace(item) != storedPlace {
		t.Fatalf("after reading library.json again the film is %+v (%v)", item, ok)
	}
	if menu := a.StoredMenu(a.cx, item); len(menu) != 1 || menu[0].Label != m.StoredMoveBack() || menu[0].Disabled {
		t.Fatalf("the moved film's menu offers %+v", menu)
	}

	// It plays from the location through the proxy, by ranges.
	source := playStored(t, f, item)
	if a.player.playErr != "" || !strings.HasPrefix(source, "http://127.0.0.1:") {
		t.Fatalf("the player plays %q, error %q", source, a.player.playErr)
	}
	wantRange(t, source, "bytes=1-3", "ict")
	if v := a.playerVideo(); v.Source != source {
		t.Fatalf("the system's player is given %q", v.Source)
	}
	a.closeFilm()

	// What needs the file on disk says why it cannot.
	a.Reprocess(item)
	refresh(tt, a)
	if job, _ := a.library.Find("film"); job.Stage != core.StageComplete || a.lib.notice.Text != m.StoredReprocess(moveName, storedPlace) {
		t.Fatalf("processing again: job %s, bar %q", job.Stage, a.lib.notice.Text)
	}
	if msg, _ := a.libraryReprocessOne(item); msg != m.StoredReprocess(moveName, storedPlace) {
		t.Fatalf("processing several again says %q", msg)
	}
	if v := a.UploadView(item); v.Status != film.UploadBlocked || v.Detail != m.StoredUpload(storedPlace) {
		t.Fatalf("the upload button shows %+v", v)
	}
	if err := a.libraryMoveFilm(m, item, f.from, f.to, false, nil); err == nil || err.Error() != m.StoredLibraryMove(moveName, storedPlace) {
		t.Fatalf("moving to another library = %v", err)
	}
	var revealed string
	prev := revealInFolder
	revealInFolder = func(_ *ns.Scope, path string, done func(error)) { revealed = path; done(nil) }
	t.Cleanup(func() { revealInFolder = prev })
	a.Reveal(item.VideoPath)
	if revealed != filepath.Dir(f.video) {
		t.Fatalf("在访达中显示 showed %q", revealed)
	}

	// Back: the same bytes in the library's folder, the record gone, the copy removed.
	a.MoveVideoBack(item)
	waitStored(t, f)
	wantText(t, tt, m.StoredMovedBack(moveName))
	if got, err := os.ReadFile(f.video); err != nil || string(got) != "picture" {
		t.Fatalf("back in the library: %q, %v", got, err)
	}
	if _, ok := a.library.Stored[f.video]; ok {
		t.Fatal("library.json still keeps the video as moved")
	}
	if back, _ := shelf.Load(path); len(back.Stored) != 0 {
		t.Fatalf("library.json on disk keeps %v", back.Stored)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := os.Stat(filepath.Join(nas, filepath.FromSlash(dest))); errors.Is(err, os.ErrNotExist) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the copy on the location was not removed")
		}
		time.Sleep(20 * time.Millisecond)
	}
	item = must(films.Find(a.films(), "film"))
	if item.Stored != "" || len(a.StoredMenu(a.cx, item)) != 1 || a.StoredMenu(a.cx, item)[0].Label != m.StoredMoveTo() {
		t.Fatalf("the film back in the library is %+v", item)
	}
}

// failingStore is a location whose writes fail, as a NAS that is full.
type failingStore struct{ storage.Provider }

var errStoreFull = errors.New("no space left on device")

func (failingStore) Write(context.Context, string, io.Reader, int64) error { return errStoreFull }

// TestMoveVideoFailsOrStopsAndKeepsIt checks that a move whose copy fails, or that is stopped with the message bar's 取消, leaves the video in the library, notes nothing in library.json, and says so; and that a second move is refused while one runs.
func TestMoveVideoFailsOrStopsAndKeepsIt(t *testing.T) {
	f := newMoveFixture(t)
	a, tt := f.a, f.tt
	m := testlocale.Messages()
	nas := t.TempDir()
	hold := make(chan struct{})
	t.Cleanup(func() {
		select {
		case <-hold:
		default:
			close(hold)
		}
	})
	failing := true
	open := func(ctx context.Context, _ storage.Target) (storage.Provider, io.Closer, error) {
		if failing {
			return failingStore{storage.NewLocal(nas)}, nil, nil
		}
		// Held until the test stops the move.
		select {
		case <-hold:
		case <-ctx.Done():
			return nil, nil, ctx.Err()
		}
		return storage.NewLocal(nas), nil, nil
	}
	useStoredLocation(t, f, storage.Remote{Kind: storage.KindWebDAV, URL: "https://nas.local/dav"}, open)
	item := must(films.Find(a.films(), "film"))

	a.MoveVideoTo(item, "nas")
	waitStored(t, f)
	// The library's own error text is not shown for a NAS location: the catalog's words for an error it does not know are.
	if n := a.lib.notice; n.Kind != "warning" || n.Text != m.StoredMoveFailed(moveName, storedPlace, m.AccountLocationFailOther()) {
		t.Fatalf("after a failed copy the bar says %+v", n)
	}
	if _, err := os.Stat(f.video); err != nil {
		t.Fatalf("a failed copy took the video: %v", err)
	}
	if len(a.library.Stored) != 0 {
		t.Fatalf("a failed copy was noted: %v", a.library.Stored)
	}

	failing = false
	// The location was opened, failing, once: the pool keeps it, so a new pool opens it again the held way.
	a.stored.pool = storage.NewPool(open)
	t.Cleanup(a.stored.pool.Close)
	a.MoveVideoTo(item, "nas")
	refresh(tt, a)
	a.MoveVideoTo(item, "nas")
	refresh(tt, a)
	if a.lib.notice.Text != m.StoredBusy() {
		t.Fatalf("a second move says %q", a.lib.notice.Text)
	}
	a.CancelMove()
	waitStored(t, f)
	wantText(t, tt, m.StoredStopped(moveName))
	if _, err := os.Stat(f.video); err != nil {
		t.Fatalf("a stopped move took the video: %v", err)
	}
	if len(a.library.Stored) != 0 {
		t.Fatalf("a stopped move was noted: %v", a.library.Stored)
	}
}

// TestMoveVideoToWebDAVAndBack moves the video to a WebDAV location served in the test, plays it from there through the proxy with ranges, and moves it back.
func TestMoveVideoToWebDAVAndBack(t *testing.T) {
	url, dir := fakeLocationDAV(t)
	if err := os.Mkdir(filepath.Join(dir, "films"), 0o755); err != nil {
		t.Fatal(err)
	}
	f := newMoveFixture(t)
	a := f.a
	m := testlocale.Messages()
	useStoredLocation(t, f, storage.Remote{Kind: storage.KindWebDAV, URL: url, Root: "films", User: "kando", Password: "secret"}, nil)
	item := must(films.Find(a.films(), "film"))

	a.MoveVideoTo(item, "nas")
	waitStored(t, f)
	wantText(t, f.tt, m.StoredMoved(moveName, storedPlace))
	dest := storedDest(f)
	if got, err := os.ReadFile(filepath.Join(dir, "films", filepath.FromSlash(dest))); err != nil || string(got) != "picture" {
		t.Fatalf("the WebDAV server holds %q, %v", got, err)
	}
	if _, err := os.Stat(f.video); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the video stayed in the library: %v", err)
	}

	item = must(films.Find(a.films(), "film"))
	source := playStored(t, f, item)
	if a.player.playErr != "" || source == "" {
		t.Fatalf("the player plays %q, error %q", source, a.player.playErr)
	}
	wantRange(t, source, "bytes=2-", "cture")
	a.closeFilm()

	a.MoveVideoBack(item)
	waitStored(t, f)
	if got, err := os.ReadFile(f.video); err != nil || string(got) != "picture" {
		t.Fatalf("back in the library: %q, %v", got, err)
	}
}

// TestStoredFilmWhoseLocationIsGoneOrUnreachable checks that a film whose video is on a location no longer in the settings, or one that cannot be reached, or one that no longer has the video, stays on the shelf, says why on the player and still reads its lines; and that removing it from the library forgets where its video was.
func TestStoredFilmWhoseLocationIsGoneOrUnreachable(t *testing.T) {
	f := newMoveFixture(t)
	a, tt := f.a, f.tt
	m := testlocale.Messages()
	nas := t.TempDir()
	reachable := true
	open := func(context.Context, storage.Target) (storage.Provider, io.Closer, error) {
		if !reachable {
			return nil, nil, errors.New("dial tcp: connection refused")
		}
		return storage.NewLocal(nas), nil, nil
	}
	useStoredLocation(t, f, storage.Remote{Kind: storage.KindWebDAV, URL: "https://nas.local/dav"}, open)
	if err := os.Remove(f.video); err != nil {
		t.Fatal(err)
	}
	a.library.Stored[f.video] = shelf.Stored{Location: "nas", Path: storedDest(f), Size: 7}
	a.refreshLibrary()
	refresh(tt, a)
	item, ok := films.Find(a.films(), "film")
	if !ok || item.Stored != "nas" {
		t.Fatalf("the film left the shelf: %+v", item)
	}

	// Reachable, but without the video.
	playStored(t, f, item)
	if a.player.playErr != playerErrStoredMissing {
		t.Fatalf("a location without the video: error %q", a.player.playErr)
	}
	a.closeFilm()

	// Not reachable: the player says so, and the lines are read as ever. The location opened before is opened again, as after a restart.
	reachable = false
	a.stored.pool = storage.NewPool(open)
	t.Cleanup(a.stored.pool.Close)
	playStored(t, f, item)
	if a.player.playErr != playerErrStoredUnreachable {
		t.Fatalf("an unreachable location: error %q", a.player.playErr)
	}
	waitUntil(t, tt, a, f.updates, func() bool { return a.player.subs.ready })
	if len(a.player.subs.cues) != 1 {
		t.Fatalf("the lines read are %+v", a.player.subs.cues)
	}
	wantText(t, tt, m.PlayerStoredUnreachable())
	// The page offers the settings, where the location is, not a file manager on a video that is not on this computer.
	refresh(tt, a)
	if tt.Exists("id:film-stage-reveal") || !tt.Exists("id:film-stage-stored-settings") {
		t.Fatalf("an unreachable location offers %q", tt.Texts())
	}
	tt.Press("id:film-stage-stored-settings")
	wantSettings(t, a, settings.SectionAccount)
	a.closeFilm()

	// Gone from the settings.
	a.locations.list = nil
	playStored(t, f, item)
	if a.player.playErr != playerErrStoredGone {
		t.Fatalf("a location gone: error %q", a.player.playErr)
	}
	if menu := a.StoredMenu(a.cx, item); len(menu) != 1 || !menu[0].Disabled {
		t.Fatalf("a film on a location gone offers %+v", menu)
	}
	a.closeFilm()

	// Removed from the library: library.json forgets where its video was.
	a.libraryForget([]films.Film{item})
	if len(a.library.Stored) != 0 {
		t.Fatalf("a removed film's video is still noted: %v", a.library.Stored)
	}
}

// TestPhoneSyncServesAMovedVideo checks that Phone Sync serves a film whose video was moved from its location, with the size noted, and one whose location is gone by its lines alone.
func TestPhoneSyncServesAMovedVideo(t *testing.T) {
	a, _, _, dir, _ := phoneSyncApp(t)
	talk := filepath.Join(dir, "Rust Talk", "talk.mp4")
	nas := t.TempDir()
	if err := os.WriteFile(filepath.Join(nas, "talk.mp4"), []byte("moved video"), 0o644); err != nil {
		t.Fatal(err)
	}
	a.locations.list = []account.Location{{ID: "nas", Name: storedPlace, Remote: storage.Remote{Kind: storage.KindWebDAV, URL: "https://nas.local/dav"}}}
	a.stored.pool = storage.NewPool(localOpen(nas))
	t.Cleanup(a.stored.pool.Close)
	moved := time.Date(2026, 11, 18, 9, 0, 0, 0, time.UTC)
	stored := map[string]shelf.Stored{talk: {Location: "nas", Path: "talk.mp4", Size: 11, Moved: moved}}
	item := films.Film{Key: "talk", JobID: "talk", VideoPath: talk, SubtitlePath: filepath.Join(dir, "Rust Talk", "talk.bilingual.srt")}

	ps := phoneShelf{stored: stored, targets: map[string]storage.Target{"nas": {ID: "nas", Remote: &a.locations.list[0].Remote}}, pool: a.stored.pool}
	got := phoneFilmOf(item, ps)
	if got.Remote == nil || got.RemoteSize != 11 || !got.RemoteAt.Equal(moved) || got.SubtitlesOnly {
		t.Fatalf("the moved film is served as %+v", got)
	}
	req, _ := http.NewRequest(http.MethodGet, "/video", nil)
	req.Header.Set("Range", "bytes=6-")
	rec := httptest.NewRecorder()
	got.Remote.ServeHTTP(rec, req)
	if rec.Code != http.StatusPartialContent || rec.Body.String() != "video" {
		t.Fatalf("the location serves %d %q", rec.Code, rec.Body.String())
	}

	ps.targets = nil
	if got := phoneFilmOf(item, ps); got.Remote != nil || !got.SubtitlesOnly {
		t.Fatalf("a film on a location gone is served as %+v", got)
	}
}
