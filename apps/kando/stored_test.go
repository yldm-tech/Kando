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
	"sync"
	"testing"
	"time"

	"kando/apps/kando/account"
	"kando/apps/kando/cloud"
	"kando/apps/kando/film"
	"kando/apps/kando/films"
	"kando/apps/kando/locales"
	"kando/apps/kando/locales/testlocale"
	"kando/apps/kando/remote"
	"kando/apps/kando/settings"
	"kando/apps/kando/shelf"
	"kando/apps/kando/sidebar"
	"kando/apps/kando/storage"
	"kando/packages/core"
	"kando/packages/media"
	"kando/packages/remotefs"

	ns "github.com/nsgo-dev/nsgo/core"
	nsdesktop "github.com/nsgo-dev/nsgo/desktop"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

// storedPlace is the name of the NAS location the tests move videos to.
const storedPlace = "Home NAS"

// useStoredLocation gives f's app a NAS location, Home NAS of ID nas, reaching remote, opened by open (nil for storage.OpenTarget), and library.json in a file of its own, so what is saved can be read again.
func useStoredLocation(t *testing.T, f *moveFixture, remote remotefs.Remote, open func(context.Context, storage.Target) (remotefs.Provider, io.Closer, error)) string {
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
func localOpen(dir string) func(context.Context, storage.Target) (remotefs.Provider, io.Closer, error) {
	return func(context.Context, storage.Target) (remotefs.Provider, io.Closer, error) {
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
	path := useStoredLocation(t, f, remotefs.Remote{Kind: remotefs.KindWebDAV, URL: "https://nas.local/dav"}, localOpen(nas))
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
	// Closing the film revokes the proxy's address of its video, and playing it again gives a new one.
	if resp, err := http.Get(source); err != nil || resp.StatusCode != http.StatusNotFound {
		t.Fatalf("after closing the film its address answers %v, %v", resp, err)
	} else {
		resp.Body.Close()
	}
	again := playStored(t, f, item)
	if again == source || !strings.HasPrefix(again, "http://127.0.0.1:") {
		t.Fatalf("playing again plays %q, was %q", again, source)
	}
	wantRange(t, again, "bytes=1-3", "ict")
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
type failingStore struct{ remotefs.Provider }

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
	open := func(ctx context.Context, _ storage.Target) (remotefs.Provider, io.Closer, error) {
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
	useStoredLocation(t, f, remotefs.Remote{Kind: remotefs.KindWebDAV, URL: "https://nas.local/dav"}, open)
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
	useStoredLocation(t, f, remotefs.Remote{Kind: remotefs.KindWebDAV, URL: url, Root: "films", User: "kando", Password: "secret"}, nil)
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
	open := func(context.Context, storage.Target) (remotefs.Provider, io.Closer, error) {
		if !reachable {
			return nil, nil, errors.New("dial tcp: connection refused")
		}
		return storage.NewLocal(nas), nil, nil
	}
	useStoredLocation(t, f, remotefs.Remote{Kind: remotefs.KindWebDAV, URL: "https://nas.local/dav"}, open)
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
	a.libraryForget(a.snap().LibraryDir, []films.Film{item})
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
	a.locations.list = []account.Location{{ID: "nas", Name: storedPlace, Remote: remotefs.Remote{Kind: remotefs.KindWebDAV, URL: "https://nas.local/dav"}}}
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

// wantStoredMark fails t unless the mark of a video on a storage location in the view at where is named want.
func wantStoredMark(t *testing.T, tt *nsgotest.App, where, want string) {
	t.Helper()
	sel := where + " >> id:film-stored"
	if !tt.Exists(sel) {
		t.Fatalf("no mark of a video on a storage location in %s", where)
	}
	if got := tt.Find(sel).Props.Label; got != want {
		t.Fatalf("the mark in %s is named %q, want %q", where, got, want)
	}
}

// A film whose video was moved to a storage location has a mark on its poster and its list row that says where the video is, which the poster's tooltip says too and which follows the interface language; a cloud for an object store. A location removed from the settings turns the mark into a warning that says so, without reaching any location. A film whose video is in the library's folder has none.
func TestStoredMarkOnPostersAndRows(t *testing.T) {
	f := newMoveFixture(t)
	a, tt := f.a, f.tt
	m := testlocale.Messages()
	poster, row := "id:film-film", "id:films-list"
	if tt.Exists(poster + " >> id:film-stored") {
		t.Fatal("a film whose video is in the library's folder has the mark")
	}

	a.locations.list = []account.Location{{ID: "nas", Name: storedPlace, Remote: remotefs.Remote{Kind: remotefs.KindWebDAV, URL: "https://nas.local/dav"}}}
	a.library.Stored = map[string]shelf.Stored{f.video: {Location: "nas", Path: "Kando/films/video.mp4", Size: 7}}
	refresh(tt, a)
	on := m.FilmsStoredOn(storedPlace)
	wantStoredMark(t, tt, poster, on)
	if place, cloud, gone := a.StoredMark(m, must(films.Find(a.films(), "film"))); place != storedPlace || cloud || gone {
		t.Fatalf("StoredMark = %q, cloud %v, gone %v", place, cloud, gone)
	}
	tt.Hover(poster)
	tt.Advance(time.Second)
	wantText(t, tt, moveName+"\n"+on)
	tt.HoverOut()

	a.cx.SetLocale("en")
	refresh(tt, a)
	wantStoredMark(t, tt, poster, locales.In("en").FilmsStoredOn(storedPlace))
	a.cx.SetLocale(testlocale.Current())
	refresh(tt, a)

	a.SetLibraryView(films.ViewList)
	refresh(tt, a)
	wantStoredMark(t, tt, row, on)

	// The location is removed in Settings: the mark stays, as a warning.
	a.locations.list = nil
	refresh(tt, a)
	wantStoredMark(t, tt, row, m.FilmsStoredGone())
	a.SetLibraryView(films.ViewGrid)
	refresh(tt, a)
	wantStoredMark(t, tt, poster, m.FilmsStoredGone())
	if _, _, gone := a.StoredMark(m, must(films.Find(a.films(), "film"))); !gone {
		t.Fatal("StoredMark does not say the location is gone")
	}

	// On the object store set up in 账号与上传, named by its brand, the mark is a cloud.
	a.cloud.r2Endpoint, a.cloud.r2Bucket, a.cloud.r2AccessKey, a.cloud.r2Secret = "acct", "clips", "key", "secret-value"
	a.library.Stored[f.video] = shelf.Stored{Location: storedStorePrefix + cloud.KindR2, Path: "Kando/films/video.mp4", Size: 7}
	refresh(tt, a)
	store := a.storedName(m, storedStorePrefix+cloud.KindR2)
	wantStoredMark(t, tt, poster, m.FilmsStoredOn(store))
	if place, isCloud, gone := a.StoredMark(m, must(films.Find(a.films(), "film"))); place != store || !isCloud || gone {
		t.Fatalf("StoredMark of the object store = %q, cloud %v, gone %v", place, isCloud, gone)
	}

	// Moved back, the film has no mark.
	delete(a.library.Stored, f.video)
	refresh(tt, a)
	if tt.Exists(poster + " >> id:film-stored") {
		t.Fatal("the mark stays on a film whose video is back in the library's folder")
	}
}

// pickyStore is a location whose writes of a path holding hold wait until they are stopped, and of one holding fail fail, as a NAS that fills up partway; it counts the writes begun.
type pickyStore struct {
	remotefs.Provider
	mu         sync.Mutex
	hold, fail string
	writes     int
}

// set makes the writes of paths holding hold wait and those holding fail fail from now on, and counts writes from 0 again.
func (s *pickyStore) set(hold, fail string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.hold, s.fail, s.writes = hold, fail, 0
}

// count is how many writes began since set.
func (s *pickyStore) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.writes
}

func (s *pickyStore) Write(ctx context.Context, p string, r io.Reader, size int64) error {
	s.mu.Lock()
	s.writes++
	hold, fail := s.hold, s.fail
	s.mu.Unlock()
	switch {
	case hold != "" && strings.Contains(p, hold):
		<-ctx.Done()
		return ctx.Err()
	case fail != "" && strings.Contains(p, fail):
		return errStoreFull
	}
	return s.Provider.Write(ctx, p, r, size)
}

// storedSeries is newSeriesFixture with Home NAS, a folder nas on this computer, as its storage location, opened by open (localOpen(nas) when nil), and the three films selected on the library page: the talk a job made and the two episodes of Course no job processed.
func storedSeries(t *testing.T, open func(context.Context, storage.Target) (remotefs.Provider, io.Closer, error)) (f *moveFixture, nas string, items []films.Film) {
	t.Helper()
	f, ep1, ep2 := newSeriesFixture(t)
	nas = t.TempDir()
	if open == nil {
		open = localOpen(nas)
	}
	useStoredLocation(t, f, remotefs.Remote{Kind: remotefs.KindWebDAV, URL: "https://nas.local/dav"}, open)
	a := f.a
	items = []films.Film{must(films.Find(a.films(), "film")), must(films.Find(a.films(), ep1)), must(films.Find(a.films(), ep2))}
	for _, item := range items {
		a.lib.selection.Toggle(item.Key)
	}
	refresh(f.tt, a)
	return f, nas, items
}

// TestMoveSeveralVideosAtOnce moves the videos of three films selected, a job's and two no job processed, to a NAS location from the selection's menu, one after another, and back from the File menu: each is copied whole, the films stay on the shelf marked as on the location, and the message bar says once how many moved.
func TestMoveSeveralVideosAtOnce(t *testing.T) {
	f, nas, items := storedSeries(t, nil)
	a, tt := f.a, f.tt
	m := testlocale.Messages()

	menu := films.SelectionMenu(a.cx, a, items)
	var to *nsdesktop.MenuItem
	for i := range menu {
		if menu[i].Label == m.StoredMoveTo() {
			to = &menu[i]
		}
		if menu[i].Label == m.StoredMoveBack() {
			t.Fatalf("the selection offers moving back videos in the library: %+v", menu)
		}
	}
	if to == nil || len(to.Items) != 1 || to.Items[0].Label != storedPlace || to.Items[0].Disabled {
		t.Fatalf("the selection's menu offers %+v", menu)
	}
	if row := menuRow(t, a, m.StoredMoveTo()); !row.When() || len(row.Items()) != 1 {
		t.Fatal("File › 移动视频到 is not offered for the films selected")
	}
	if row := menuRow(t, a, m.StoredMoveBack()); row.When() {
		t.Fatal("File › 将视频移回资料库 is offered for videos in the library")
	}
	to.Items[0].OnSelect()
	refresh(tt, a)
	wantText(t, tt, m.StoredMovingToMany(1, 3, storedPlace, 0))
	waitStored(t, f)
	wantText(t, tt, m.StoredMovedMany(3, storedPlace))
	for _, item := range items {
		if _, err := os.Stat(item.VideoPath); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("%s stayed in the library: %v", item.VideoPath, err)
		}
		s, ok := a.library.Stored[item.VideoPath]
		if !ok {
			t.Fatalf("%s is not noted as moved", item.VideoPath)
		}
		if _, err := os.Stat(filepath.Join(nas, filepath.FromSlash(s.Path))); err != nil {
			t.Fatalf("%s is not on the location: %v", item.VideoPath, err)
		}
		if got, ok := films.Find(a.films(), item.Key); !ok || got.Stored != "nas" {
			t.Fatalf("%s left the shelf or has no mark: %+v", item.Key, got)
		}
	}

	refresh(tt, a)
	back := menuRow(t, a, m.StoredMoveBack())
	if !back.When() {
		t.Fatal("File › 将视频移回资料库 is not offered for the moved videos")
	}
	back.Run()
	waitStored(t, f)
	wantText(t, tt, m.StoredMovedBackMany(3))
	for _, item := range items {
		if _, err := os.Stat(item.VideoPath); err != nil {
			t.Fatalf("%s is not back: %v", item.VideoPath, err)
		}
	}
	if len(a.library.Stored) != 0 {
		t.Fatalf("library.json still notes %v", a.library.Stored)
	}
}

// TestMoveSeveralVideosStopsOrFails checks that 取消 while the second of three videos is copied stops it and leaves it and the third in the library, the first staying moved; and that a copy that fails partway leaves that video and the ones after it in the library, the message bar saying how many moved and why the others did not.
func TestMoveSeveralVideosStopsOrFails(t *testing.T) {
	store := &pickyStore{hold: "ep1"}
	f, nas, items := storedSeries(t, func(context.Context, storage.Target) (remotefs.Provider, io.Closer, error) { return store, nil, nil })
	store.Provider = storage.NewLocal(nas)
	a, tt := f.a, f.tt
	m := testlocale.Messages()

	a.MoveVideosTo(items, "nas")
	waitUntil(t, tt, a, f.updates, func() bool { return a.stored.run != nil && a.stored.run.batch.at == 2 })
	refresh(tt, a)
	wantText(t, tt, m.StoredMovingToMany(2, 3, storedPlace, 0))
	tt.Press("id:notice-stop")
	waitStored(t, f)
	if n := a.lib.notice; n.Text != m.StoredStoppedMany(1, storedPlace) {
		t.Fatalf("after 取消 the bar says %+v", n)
	}
	if _, moved := a.library.Stored[items[0].VideoPath]; !moved || len(a.library.Stored) != 1 {
		t.Fatalf("after 取消 library.json notes %v", a.library.Stored)
	}
	for _, item := range items[1:] {
		if _, err := os.Stat(item.VideoPath); err != nil {
			t.Fatalf("a stopped move took %s: %v", item.VideoPath, err)
		}
	}
	if n := store.count(); n != 2 {
		t.Fatalf("%d copies began, want 2", n)
	}

	// The first is back; now the second's copy fails, as a full NAS does, and the third is not tried.
	a.MoveVideoBack(must(films.Find(a.films(), items[0].Key)))
	waitStored(t, f)
	store.set("", "ep1")
	items = []films.Film{must(films.Find(a.films(), items[0].Key)), must(films.Find(a.films(), items[1].Key)), must(films.Find(a.films(), items[2].Key))}
	a.MoveVideosTo(items, "nas")
	waitStored(t, f)
	reason := m.StoredMoveFailed(films.Name(items[1]), storedPlace, m.AccountLocationFailOther())
	if n := a.lib.notice; n.Kind != "warning" || n.Text != m.StoredMovedSome(1, 3, storedPlace, reason) {
		t.Fatalf("after a failed copy the bar says %+v", n)
	}
	if n := store.count(); n != 2 {
		t.Fatalf("%d copies began, want 2: the third should not be tried", n)
	}
	for _, item := range items[1:] {
		if _, err := os.Stat(item.VideoPath); err != nil {
			t.Fatalf("%s left the library: %v", item.VideoPath, err)
		}
		if _, moved := a.library.Stored[item.VideoPath]; moved {
			t.Fatalf("%s is noted as moved", item.VideoPath)
		}
	}
}

// TestMoveAVideoNoJobProcessed moves the video of a film found in the library's folder, which no job processed: it stays on the shelf by its path, with its title and its subtitles, after the folder is read again and after library.json is read again; it plays from the location; processing it refuses with why; and it comes back.
func TestMoveAVideoNoJobProcessed(t *testing.T) {
	f, ep1, _ := newSeriesFixture(t)
	a, tt := f.a, f.tt
	m := testlocale.Messages()
	nas := t.TempDir()
	a.library.Names[ep1] = "Episode One"
	path := useStoredLocation(t, f, remotefs.Remote{Kind: remotefs.KindWebDAV, URL: "https://nas.local/dav"}, localOpen(nas))
	item := must(films.Find(a.films(), ep1))
	if item.JobID != "" || len(a.StoredMenu(a.cx, item)) != 1 {
		t.Fatalf("the folder's film %+v offers %+v", item, a.StoredMenu(a.cx, item))
	}
	a.MoveVideoTo(item, "nas")
	waitStored(t, f)
	wantText(t, tt, m.StoredMoved("Episode One", storedPlace))
	if _, err := os.Stat(ep1); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the video stayed: %v", err)
	}
	check := func(when string) films.Film {
		t.Helper()
		got, ok := films.Find(a.films(), ep1)
		if !ok || got.Stored != "nas" || got.JobID != "" || films.Name(got) != "Episode One" || got.SubtitlePath != filepath.Join(filepath.Dir(ep1), "ep1.bilingual.srt") {
			t.Fatalf("%s the film is %+v (%v)", when, got, ok)
		}
		return got
	}
	check("after the move")
	a.refreshLibrary()
	check("after the folder is read again")
	a.openLibrary(path)
	refresh(tt, a)
	item = check("after library.json is read again")

	if source := playStored(t, f, item); a.player.playErr != "" || !strings.HasPrefix(source, "http://127.0.0.1:") {
		t.Fatalf("the player plays %q, error %q", source, a.player.playErr)
	}
	a.closeFilm()
	jobs := len(a.library.Jobs)
	a.Reprocess(item)
	refresh(tt, a)
	if len(a.library.Jobs) != jobs || a.lib.notice.Text != m.StoredReprocess("Episode One", storedPlace) {
		t.Fatalf("processing it: %d jobs, bar %q", len(a.library.Jobs), a.lib.notice.Text)
	}
	if msg, _ := a.libraryReprocessOne(item); msg != m.StoredReprocess("Episode One", storedPlace) {
		t.Fatalf("processing several says %q", msg)
	}

	a.MoveVideoBack(item)
	waitStored(t, f)
	if got, err := os.ReadFile(ep1); err != nil || string(got) != "one" {
		t.Fatalf("back in the library: %q, %v", got, err)
	}
	if got, ok := films.Find(a.films(), ep1); !ok || got.Stored != "" || films.Name(got) != "Episode One" {
		t.Fatalf("the film back is %+v (%v)", got, ok)
	}
}

// TestRemoveALocationVideosAreOn checks that removing a NAS location two films' videos are on says so in its confirmation, removes nothing on the location, and that adding the location back at the same address, under another user, finds those videos again.
func TestRemoveALocationVideosAreOn(t *testing.T) {
	f := newMoveFixture(t)
	a, tt := f.a, f.tt
	m := testlocale.Messages()
	a.settingsPath = filepath.Join(t.TempDir(), "settings.json")
	nas := t.TempDir()
	useStoredLocation(t, f, remotefs.Remote{Kind: remotefs.KindWebDAV, URL: "https://nas.local/dav", User: "kando", Password: "secret"}, localOpen(nas))
	a.MoveVideoTo(must(films.Find(a.films(), "film")), "nas")
	waitStored(t, f)
	a.library.Stored["/elsewhere/Other/video.mp4"] = shelf.Stored{Location: "nas", Path: "Other/video.mp4", Size: 5}
	dest := filepath.Join(nas, filepath.FromSlash(storedDest(f)))

	tt = settingsOpenAt(a, tt, settings.SectionAccount)
	tt.Press("id:location-remove-nas")
	if !tt.Exists("id:location-remove-confirm") {
		t.Fatal("no confirmation")
	}
	wantText(t, tt, m.AccountLocationRemoveStored(2))
	tt.Press("id:location-alert-remove")
	if len(a.locations.list) != 0 || len(a.locations.removed) != 1 {
		t.Fatalf("list %+v, removed %+v", a.locations.list, a.locations.removed)
	}
	if _, err := os.Stat(dest); err != nil {
		t.Fatalf("removing the location removed the video on it: %v", err)
	}
	raw, err := os.ReadFile(a.settingsPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "secret") || !strings.Contains(string(raw), "removedStorageLocations") {
		t.Fatalf("settings.json: %s", raw)
	}
	item := must(films.Find(a.films(), "film"))
	if _, _, gone := a.StoredMark(m, item); !gone {
		t.Fatal("the film's mark does not say its location is gone")
	}
	if loc := a.storedRemote(item.VideoPath); loc == nil || !loc.Missing || loc.Name != storedPlace || loc.Kind != "webdav" {
		t.Fatalf("the socket tells %+v", loc)
	}

	// Added back at the same address, as another user: the videos are found again.
	a.AddLocation()
	*a.LocationDraft() = account.DraftOf(account.Location{Name: "NAS again", Remote: remotefs.Remote{Kind: remotefs.KindWebDAV, URL: "https://nas.local/dav/", User: "other", Password: "pw"}})
	a.SaveLocation()
	if len(a.locations.list) != 1 || a.locations.list[0].ID != "nas" || len(a.locations.removed) != 0 {
		t.Fatalf("added back: list %+v, removed %+v", a.locations.list, a.locations.removed)
	}
	if _, _, gone := a.StoredMark(m, item); gone {
		t.Fatal("the film's location is still gone after it was added back")
	}

	// A location at another address is a new one.
	a.AddLocation()
	*a.LocationDraft() = account.DraftOf(account.Location{Name: "Other", Remote: remotefs.Remote{Kind: remotefs.KindWebDAV, URL: "https://other.local/dav"}})
	a.SaveLocation()
	if len(a.locations.list) != 2 || a.locations.list[1].ID == "nas" {
		t.Fatalf("a new location: %+v", a.locations.list)
	}
}

// TestRemuxAVideoFromALocation plays a video moved to a NAS location that the system's player cannot open: Kando copies it for the player with the fake ffmpeg, which reads it through the proxy, into the cache of such copies, saying how far it got, and plays the copy; when not even the copies play, or there is no ffmpeg, the page says it cannot be played, with no button, and the film's menu still moves the video back.
func TestRemuxAVideoFromALocation(t *testing.T) {
	f := newMoveFixture(t)
	a, tt := f.a, f.tt
	m := testlocale.Messages()
	a.pipeline = media.Tools{FFmpeg: fakeTool(t, filepath.Join(t.TempDir(), "bin"), "ffmpeg")}
	a.player.remuxDir = filepath.Join(t.TempDir(), "playback")
	nas := t.TempDir()
	useStoredLocation(t, f, remotefs.Remote{Kind: remotefs.KindWebDAV, URL: "https://nas.local/dav"}, localOpen(nas))
	a.MoveVideoTo(must(films.Find(a.films(), "film")), "nas")
	waitStored(t, f)
	item := must(films.Find(a.films(), "film"))
	a.libraryLearnDuration(f.video, 2*time.Second)

	url := playStored(t, f, item)
	p := a.player.play
	if !strings.HasPrefix(url, "http://127.0.0.1:") || p.storedURL != url {
		t.Fatalf("the player plays %q", url)
	}
	apply(tt, a, func() { a.VideoFailed(errors.New("Cannot Open")) })
	if p.remux != film.RemuxCopy || p.source != "" {
		t.Fatalf("no copy is made: remux %v source %q", p.remux, p.source)
	}
	waitUntil(t, tt, a, f.updates, func() bool { return p.source != "" || a.player.playErr != "" })
	if a.player.playErr != "" || filepath.Dir(p.source) != a.player.remuxDir {
		t.Fatalf("the copy: source %q, error %q", p.source, a.player.playErr)
	}
	if got, err := os.ReadFile(p.source); err != nil || string(got) != "picture" {
		t.Fatalf("the copy holds %q, %v", got, err)
	}
	if v := a.playerVideo(); v.Source != p.source {
		t.Fatalf("the system's player is given %q", v.Source)
	}
	// ffmpeg said it got 0.5 s and then 1 s into the 2 s video: the stage says 50% while the copy is not yet played.
	if p.remuxAt != time.Second {
		t.Fatalf("the copy got %v far", p.remuxAt)
	}
	source := p.source
	p.source = ""
	if got := a.playerRemuxText(m); got != m.PlayerRemuxingFrom(storedPlace, 50) {
		t.Fatalf("while copying the stage says %q", got)
	}
	p.source = source

	// Neither copy plays.
	apply(tt, a, func() { a.VideoFailed(errors.New("Cannot Open")) })
	waitUntil(t, tt, a, f.updates, func() bool { return p.source != "" || a.player.playErr != "" })
	if p.remux != film.RemuxEncode {
		t.Fatalf("the copy encoded anew is not made: %v", p.remux)
	}
	apply(tt, a, func() { a.VideoFailed(errors.New("Cannot Open")) })
	if a.player.playErr != playerErrStoredUnplayable {
		t.Fatalf("when no copy plays the player says %q", a.player.playErr)
	}
	refresh(tt, a)
	wantText(t, tt, m.PlayerStoredUnplayable())
	if tt.Exists("id:film-stage-reveal") || tt.Exists("id:film-stage-stored-settings") {
		t.Fatalf("an unplayable video on a location offers %q", tt.Texts())
	}
	if menu := a.StoredOpenMenu(a.cx); len(menu) != 1 || menu[0].Label != m.StoredMoveBack() || menu[0].Disabled {
		t.Fatalf("the film's ⋯ offers %+v", menu)
	}
	a.closeFilm()

	// Without ffmpeg there is no copy.
	a.pipeline = &fakePipeline{}
	playStored(t, f, item)
	apply(tt, a, func() { a.VideoFailed(errors.New("Cannot Open")) })
	if a.player.playErr != playerErrStoredUnplayable {
		t.Fatalf("without ffmpeg the player says %q", a.player.playErr)
	}
}

// TestRemoteSaysWhereAMovedVideoIs checks that films.list and film.get over the control socket say where a moved video is: the location's name and kind and the video's path there, and that the location is gone once it is removed from the settings; a video in the library's folder has none.
func TestRemoteSaysWhereAMovedVideoIs(t *testing.T) {
	f, _, items := storedSeries(t, nil)
	a := f.a
	a.MoveVideoTo(items[0], "nas")
	waitStored(t, f)
	remoteServe(t, a)
	all, err := remoteCall[[]remote.Film](t, f.updates, remote.MethodFilms, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := remote.VideoLocation{Name: storedPlace, Kind: "webdav", Path: storedDest(f)}
	for _, film := range all {
		switch {
		case film.ID == "film" && (film.VideoLocation == nil || *film.VideoLocation != want):
			t.Fatalf("films.list tells %+v of the moved video", film.VideoLocation)
		case film.ID != "film" && film.VideoLocation != nil:
			t.Fatalf("films.list tells %+v of %s, in the library", film.VideoLocation, film.ID)
		}
	}
	one, err := remoteCall[remote.Film](t, f.updates, remote.MethodFilm, remote.IDParams{ID: "film"})
	if err != nil || one.VideoLocation == nil || *one.VideoLocation != want || one.VideoPath != f.video {
		t.Fatalf("film.get = %+v, %v", one, err)
	}

	a.locations.list = nil
	one, err = remoteCall[remote.Film](t, f.updates, remote.MethodFilm, remote.IDParams{ID: "film"})
	if err != nil || one.VideoLocation == nil || !one.VideoLocation.Missing || one.VideoLocation.Path != want.Path {
		t.Fatalf("film.get once the location is gone = %+v, %v", one.VideoLocation, err)
	}
}

// TestMovedVideoKeepsItsPathInAFolderOfTwo moves one of two videos in a folder to a location, for a film a job made and for videos found in the folder: reading the library again leaves the other video's name alone, as the folder still holds two films, rather than renaming it to video.<ext>, which could be the moved video's path; both come back to their own paths. A file found where a video goes back is not written over, and the move back says why.
func TestMovedVideoKeepsItsPathInAFolderOfTwo(t *testing.T) {
	m := testlocale.Messages()
	read := func(t *testing.T, path, want string) {
		t.Helper()
		if got, err := os.ReadFile(path); err != nil || string(got) != want {
			t.Fatalf("%s holds %q, %v; want %q", path, got, err, want)
		}
	}

	t.Run("a job's film", func(t *testing.T) {
		f := newMoveFixture(t)
		a, tt := f.a, f.tt
		extra := filepath.Join(f.from, "Talk", "extra.mp4")
		writeFiles(t, map[string]string{extra: "extra"})
		a.refreshLibrary()
		useStoredLocation(t, f, remotefs.Remote{Kind: remotefs.KindWebDAV, URL: "https://nas.local/dav"}, localOpen(t.TempDir()))
		a.MoveVideoTo(must(films.Find(a.films(), "film")), "nas")
		waitStored(t, f)
		a.refreshLibrary()
		refresh(tt, a)
		read(t, extra, "extra")
		if _, err := os.Stat(f.video); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("a file took the moved video's path: %v", err)
		}
		if _, ok := films.Find(a.films(), extra); !ok {
			t.Fatal("the video left in the folder is not on the shelf by its name")
		}
		a.MoveVideoBack(must(films.Find(a.films(), "film")))
		waitStored(t, f)
		wantText(t, tt, m.StoredMovedBack(moveName))
		read(t, f.video, "picture")
		read(t, extra, "extra")
	})

	t.Run("videos found in the folder", func(t *testing.T) {
		f, ep1, ep2 := newSeriesFixture(t)
		a, tt := f.a, f.tt
		useStoredLocation(t, f, remotefs.Remote{Kind: remotefs.KindWebDAV, URL: "https://nas.local/dav"}, localOpen(t.TempDir()))
		a.MoveVideoTo(must(films.Find(a.films(), ep1)), "nas")
		waitStored(t, f)
		read(t, ep2, "two")
		a.MoveVideoTo(must(films.Find(a.films(), ep2)), "nas")
		waitStored(t, f)
		// The first comes back while the second is away: its folder holds two films still, so it keeps its name.
		a.MoveVideoBack(must(films.Find(a.films(), ep1)))
		waitStored(t, f)
		a.refreshLibrary()
		read(t, ep1, "one")
		if _, err := os.Stat(filepath.Join(filepath.Dir(ep1), "video.mp4")); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("a video was renamed in the folder: %v", err)
		}
		// A file where the second goes back, as one copied in by hand, stays, and the move back says why.
		writeFiles(t, map[string]string{ep2: "someone else's"})
		a.MoveVideoBack(films.Film{Key: ep2, VideoPath: ep2, Name: "ep2"})
		refresh(tt, a)
		if a.stored.run != nil || a.lib.notice.Text != m.StoredMoveBackFailed("ep2", storedPlace, m.StoredLocalTaken()) {
			t.Fatalf("moving back over a file: run %v, bar %q", a.stored.run != nil, a.lib.notice.Text)
		}
		read(t, ep2, "someone else's")
		if err := os.Remove(ep2); err != nil {
			t.Fatal(err)
		}
		a.refreshLibrary()
		a.MoveVideoBack(must(films.Find(a.films(), ep2)))
		waitStored(t, f)
		read(t, ep1, "one")
		read(t, ep2, "two")
		if len(a.library.Stored) != 0 {
			t.Fatalf("library.json still notes %v", a.library.Stored)
		}
	})
}
