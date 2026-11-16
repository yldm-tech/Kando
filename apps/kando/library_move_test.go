package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"kando/apps/kando/films"
	"kando/apps/kando/locales/testlocale"
	"kando/apps/kando/remote"
	"kando/apps/kando/saved"
	"kando/apps/kando/shelf"
	"kando/packages/core"
	"kando/packages/media"

	"github.com/nsgo-dev/nsgo/nsgotest"
)

// moveFixture is two libraries Kando knows, 我的影片 (open) and 英语听力, and a done film of 我的影片 in a folder of its own, Talk, with its job, its subtitles and a translation checkpoint beside its video, where it stopped, how far and when it last played, its subtitle delay and place, a title given in Kando and a saved line.
type moveFixture struct {
	a        *app
	tt       *nsgotest.App
	updates  chan func()
	from, to string
	video    string
}

const (
	movePosition = 42 * time.Second
	moveDelay    = 1500 * time.Millisecond
	moveLift     = float32(0.25)
	moveName     = "My Talk"
)

var movePlayed = time.Date(2026, 11, 10, 20, 0, 0, 0, time.UTC)

func newMoveFixture(t *testing.T) *moveFixture {
	t.Helper()
	root := t.TempDir()
	from, to := filepath.Join(root, "我的影片.kando"), filepath.Join(root, "英语听力.kando")
	video := filepath.Join(from, "Talk", "video.mp4")
	srt := filepath.Join(from, "Talk", "video.bilingual.srt")
	for path, body := range map[string]string{
		video:                                    "picture",
		srt:                                      "1\n00:00:01,000 --> 00:00:02,000\n你好。\nHello.\n\n",
		filepath.Join(from, "Talk", "video.srt"): "1\n00:00:01,000 --> 00:00:02,000\nHello.\n\n",
		filepath.Join(from, "Talk", ".video.translation.json"): "{}",
		filepath.Join(to, "Other", "video.mp4"):                "other",
	} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	book, _ := saved.Load(from)
	book.Toggle(saved.Line{Video: book.Key(video), Start: time.Second, End: 2 * time.Second, Original: "Hello.", Translation: "你好。", Film: "Talk", SavedAt: movePlayed})
	if err := book.Save(); err != nil {
		t.Fatal(err)
	}
	a, tt, updates := newTestApp(t, &fakePipeline{})
	a.edit(func(s *session) {
		s.LibraryDir = from
		s.Libraries = []string{from, to}
	})
	a.library, _ = shelf.Load("")
	a.library.Jobs = []core.Job{{ID: "film", Source: "https://youtu.be/TRWgBxVAb1M", Language: "简体中文", Stage: core.StageComplete, VideoPath: video, SubtitlePath: srt, LibraryDir: from}}
	a.library.Positions[video] = movePosition
	a.library.Furthest[video] = movePosition
	a.library.Played[video] = movePlayed
	a.library.Delays[video] = moveDelay
	a.library.Lifts[video] = moveLift
	a.library.Names[video] = moveName
	a.areasShelfLoaded()
	a.refreshLibrary()
	a.savedSync()
	refresh(tt, a)
	return &moveFixture{a: a, tt: tt, updates: updates, from: from, to: to, video: video}
}

// wantKept checks that everything Kando keeps of the film by its video's path is kept by video, and nothing by the path it had.
func (f *moveFixture) wantKept(t *testing.T, video string) {
	t.Helper()
	a := f.a
	lib := a.library
	if lib.Positions[video] != movePosition || lib.Furthest[video] != movePosition || !lib.Played[video].Equal(movePlayed) || lib.Delays[video] != moveDelay || lib.Lifts[video] != moveLift || lib.Names[video] != moveName {
		t.Fatalf("library.json keeps of %s: position %v, furthest %v, played %v, delay %v, lift %v, name %q", video, lib.Positions[video], lib.Furthest[video], lib.Played[video], lib.Delays[video], lib.Lifts[video], lib.Names[video])
	}
	if a.ResumeAt(video) != movePosition || a.Furthest(video) != movePosition || a.player.delays[video] != moveDelay || a.player.lifts[video] != moveLift {
		t.Fatalf("the player keeps of %s: position %v, furthest %v, delay %v, lift %v", video, a.ResumeAt(video), a.Furthest(video), a.player.delays[video], a.player.lifts[video])
	}
	if video != f.video {
		if _, ok := lib.Positions[f.video]; ok {
			t.Fatalf("library.json still keeps a position by the old path %s", f.video)
		}
		if _, ok := lib.Names[f.video]; ok {
			t.Fatalf("library.json still keeps a title by the old path %s", f.video)
		}
	}
}

// wantLines checks that the saved line of the film is in the book of the library at dir, by video, and in no other.
func wantLines(t *testing.T, dir, video string, others ...string) {
	t.Helper()
	book, err := saved.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if book.Find(book.Key(video), time.Second) < 0 {
		t.Fatalf("the saved lines of %s are %+v, want the film's line by %s", dir, book.Lines, book.Key(video))
	}
	for _, other := range others {
		if b, _ := saved.Load(other); len(b.Lines) != 0 {
			t.Fatalf("the saved lines of %s still hold %+v", other, b.Lines)
		}
	}
}

// TestMoveFilmToAnotherLibrary checks 移到资料库 from the film's ⋯ within one volume: the folder moves whole into the other library, the film leaves this one and is on the other's shelf with its job, everything kept of it, and its saved line; the message bar says so, opens the other library and undoes the move, which brings everything back.
func TestMoveFilmToAnotherLibrary(t *testing.T) {
	f := newMoveFixture(t)
	a, tt := f.a, f.tt
	m := testlocale.Messages()
	tt.Press("id:film-more >> nth=0")
	tt.ChoosePopup(m.FilmsMoveToLibrary(), "英语听力")
	refresh(tt, a)

	next := filepath.Join(f.to, "Talk", "video.mp4")
	for _, name := range []string{"video.mp4", "video.bilingual.srt", "video.srt", ".video.translation.json"} {
		if _, err := os.Stat(filepath.Join(f.to, "Talk", name)); err != nil {
			t.Fatalf("%s did not move: %v", name, err)
		}
	}
	if _, err := os.Stat(filepath.Join(f.from, "Talk")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the film's folder is still in the library it left: %v", err)
	}
	if len(a.films()) != 0 {
		t.Fatalf("the library it left still lists %+v", a.films())
	}
	job, _ := a.library.Find("film")
	if job.VideoPath != next || job.SubtitlePath != filepath.Join(f.to, "Talk", "video.bilingual.srt") || job.LibraryDir != f.to {
		t.Fatalf("the job is %+v", job)
	}
	f.wantKept(t, next)
	wantLines(t, f.to, next, f.from)
	wantText(t, tt, m.LibraryMoved(moveName, "英语听力"))
	if !tt.Exists("id:notice-undo") || !tt.Exists("id:notice-library") {
		t.Fatalf("the message bar offers no Undo or Open: %q", tt.Texts())
	}

	tt.Press("id:notice-undo")
	refresh(tt, a)
	if _, err := os.Stat(f.video); err != nil {
		t.Fatalf("Undo did not bring the video back: %v", err)
	}
	if _, err := os.Stat(filepath.Join(f.to, "Talk")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Undo left the film in the other library: %v", err)
	}
	if got, ok := films.Find(a.films(), "film"); !ok || got.VideoPath != f.video || got.Name != moveName {
		t.Fatalf("after Undo the library has %+v", a.films())
	}
	job, _ = a.library.Find("film")
	if job.LibraryDir != f.from {
		t.Fatalf("after Undo the job's library is %q", job.LibraryDir)
	}
	f.wantKept(t, f.video)
	wantLines(t, f.from, f.video, f.to)
	if tt.Exists("id:notice-undo") {
		t.Fatal("the bar after Undo offers Undo again")
	}

	// Moved again and opened from the message bar, the other library shows the film with its title.
	a.MoveFilm(must(films.Find(a.films(), "film")), f.to)
	refresh(tt, a)
	tt.Press("id:notice-library")
	refresh(tt, a)
	if a.snap().LibraryDir != f.to {
		t.Fatalf("打开 opened %q", a.snap().LibraryDir)
	}
	if got, ok := films.Find(a.films(), "film"); !ok || got.Name != moveName || a.ResumeAt(got.VideoPath) != movePosition {
		t.Fatalf("the other library shows %+v", a.films())
	}
	if lines := a.savedBook().Lines; len(lines) != 1 || lines[0].Video != "Talk/video.mp4" {
		t.Fatalf("the other library's saved lines are %+v", lines)
	}
}

func must[T any](v T, ok bool) T {
	if !ok {
		panic("not found")
	}
	return v
}

// TestMoveFilmAcrossVolumes checks the move to a library on another volume, which the copy stands in for when renaming fails: the message bar says how far the copy has gone and offers to stop it, the film moves once the copy is whole, and a copy stopped or failed leaves the film where it was, with nothing left in the other library.
func TestMoveFilmAcrossVolumes(t *testing.T) {
	prev := libraryMoveRename
	libraryMoveRename = func(media.FilmMove) error { return errors.New("cross-device link") }
	prevCopy := libraryMoveCopyFiles
	t.Cleanup(func() { libraryMoveRename, libraryMoveCopyFiles = prev, prevCopy })
	f := newMoveFixture(t)
	a, tt := f.a, f.tt
	m := testlocale.Messages()
	item := must(films.Find(a.films(), "film"))

	// Stopped while it copies: the film stays.
	copyFiles := libraryMoveCopyFiles
	libraryMoveCopyFiles = func(m media.FilmMove, ctx context.Context, progress func(done, total int64)) error {
		<-ctx.Done()
		return ctx.Err()
	}
	a.MoveFilm(item, f.to)
	refresh(tt, a)
	if a.lib.move.copying == nil || !tt.Exists("id:notice-stop") {
		t.Fatalf("no copy shows: %q", tt.Texts())
	}
	wantText(t, tt, m.LibraryMoving(moveName, "英语听力", 0))
	if again := a.libraryMoveFilm(m, item, f.from, f.to, true, nil); again == nil || again.Error() != m.LibraryMoveBusy() {
		t.Fatalf("a second move while copying = %v", again)
	}
	tt.Press("id:notice-stop")
	waitUntil(t, tt, a, f.updates, func() bool { return a.lib.move.copying == nil })
	wantText(t, tt, m.LibraryMoveStopped(moveName))
	if _, err := os.Stat(f.video); err != nil {
		t.Fatalf("a stopped copy took the video: %v", err)
	}
	if entries, _ := os.ReadDir(f.to); len(entries) != 1 {
		t.Fatalf("a stopped copy left %v in the other library", entries)
	}
	f.wantKept(t, f.video)

	// A copy that fails partway, as on a drive that fills up, leaves the film where it was and nothing in the other library: the real copy runs and is stopped after its first bytes, and reports the failure the disk would.
	full := errors.New("no space left on device")
	libraryMoveCopyFiles = func(m media.FilmMove, ctx context.Context, progress func(done, total int64)) error {
		ctx, stop := context.WithCancel(ctx)
		defer stop()
		if err := copyFiles(m, ctx, func(done, total int64) { stop() }); err == nil {
			t.Error("the copy stopped partway did not fail")
		}
		return full
	}
	a.MoveFilm(item, f.to)
	waitUntil(t, tt, a, f.updates, func() bool { return a.lib.move.copying == nil })
	libraryMoveCopyFiles = copyFiles
	if n := a.lib.notice; n.Kind != "warning" || n.Undo || n.Text != m.LibraryMoveFailed(moveName, "英语听力", full.Error()) {
		t.Fatalf("after a failed copy the bar says %+v", n)
	}
	if _, err := os.Stat(f.video); err != nil {
		t.Fatalf("a failed copy took the video: %v", err)
	}
	if entries, _ := os.ReadDir(f.to); len(entries) != 1 {
		t.Fatalf("a failed copy left %v in the other library", entries)
	}
	f.wantKept(t, f.video)

	// A whole copy moves the film.
	a.MoveFilm(item, f.to)
	waitUntil(t, tt, a, f.updates, func() bool { return a.lib.move.copying == nil })
	next := filepath.Join(f.to, "Talk", "video.mp4")
	if _, err := os.Stat(next); err != nil {
		t.Fatalf("the copy is not in the other library: %v", err)
	}
	if _, err := os.Stat(filepath.Join(f.from, "Talk")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the film's folder stayed after its copy: %v", err)
	}
	f.wantKept(t, next)
	wantLines(t, f.to, next, f.from)
	wantText(t, tt, m.LibraryMoved(moveName, "英语听力"))
}

// TestMoveFilmClashRefusalAndOutside checks that a film moved to a library with a folder of its name gets a folder named after it with a number; that a move is refused with why, nothing moved, when the other library's saved lines or library.json could not be read, as they would be written over, when the film is being processed, or when the other library's drive is not plugged in, which is listed but disabled; and that a film whose video is kept outside the library moves without its files.
func TestMoveFilmClashRefusalAndOutside(t *testing.T) {
	f := newMoveFixture(t)
	a, tt := f.a, f.tt
	m := testlocale.Messages()
	if err := os.MkdirAll(filepath.Join(f.to, "Talk"), 0o755); err != nil {
		t.Fatal(err)
	}

	// The other library's saved lines could not be read: nothing moves, so they are not written over.
	item := must(films.Find(a.films(), "film"))
	broken := filepath.Join(f.to, saved.FileName)
	if err := os.WriteFile(broken, []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}
	a.MoveFilm(item, f.to)
	refresh(tt, a)
	wantText(t, tt, m.LibraryMoveLinesUnread("英语听力"))
	if data, _ := os.ReadFile(broken); string(data) != "{" {
		t.Fatalf("the unreadable saved lines were written over: %q", data)
	}
	os.Remove(broken)

	// library.json could not be read in full: nothing moves, as it is not saved.
	kept := a.library
	unread, _ := shelf.Load(t.TempDir())
	unread.Jobs = kept.Jobs
	a.library = unread
	a.MoveFilm(item, f.to)
	refresh(tt, a)
	wantText(t, tt, m.LibraryShelfNotSaved())
	a.library = kept
	if _, err := os.Stat(f.video); err != nil {
		t.Fatal("a refused film moved")
	}

	// Being processed: refused.
	a.library.Jobs[0].Stage = core.StageQueued
	a.library.Jobs[0].Redo = core.RedoAll
	a.MoveFilm(item, f.to)
	refresh(tt, a)
	wantText(t, tt, m.LibraryMoveProcessing(moveName))
	if _, err := os.Stat(f.video); err != nil {
		t.Fatal("a film being processed moved")
	}
	a.library.Jobs[0].Stage, a.library.Jobs[0].Redo = core.StageComplete, ""

	// A name taken: the film's folder takes a number.
	a.MoveFilm(item, f.to)
	refresh(tt, a)
	next := filepath.Join(f.to, "Talk 2", "video.mp4")
	if _, err := os.Stat(next); err != nil {
		t.Fatalf("the film is not in Talk 2: %v", err)
	}
	f.wantKept(t, next)

	// A library whose folder is gone, as on a drive not plugged in: listed, disabled, and refused.
	gone := filepath.Join(filepath.Dir(f.from), "外接盘.kando")
	a.edit(func(s *session) { s.Libraries = append(s.Libraries, gone) })
	a.Choose(f.to)
	a.libraryMoveLook()
	waitUntil(t, tt, a, f.updates, func() bool { _, ok := a.lib.move.reach[gone]; return ok })
	item = must(films.Find(a.films(), "film"))
	items := a.MoveFilmMenu(a.cx, item)
	var labels []string
	for _, it := range items {
		labels = append(labels, it.Label)
		if it.Label == "外接盘" && !it.Disabled {
			t.Fatal("the library whose folder is gone can be chosen")
		}
	}
	if !slices.Equal(labels, []string{"我的影片", "外接盘"}) {
		t.Fatalf("移到资料库 lists %q", labels)
	}
	a.MoveFilm(item, gone)
	refresh(tt, a)
	wantText(t, tt, m.LibraryMoveUnreachable("外接盘"))

	// A local video kept outside the library: only its record moves.
	outside := filepath.Join(t.TempDir(), "home.mp4")
	if err := os.WriteFile(outside, []byte("v"), 0o644); err != nil {
		t.Fatal(err)
	}
	a.library.Jobs = append(a.library.Jobs, core.Job{ID: "home", Source: core.FileURL(outside), Language: "简体中文", Stage: core.StageComplete, VideoPath: outside, LibraryDir: f.to})
	refresh(tt, a)
	a.MoveFilm(must(films.Find(a.films(), "home")), f.from)
	refresh(tt, a)
	if _, err := os.Stat(outside); err != nil {
		t.Fatalf("the video outside the library moved: %v", err)
	}
	if job, _ := a.library.Find("home"); job.LibraryDir != f.from || job.VideoPath != outside {
		t.Fatalf("the outside film's job is %+v", job)
	}
	if _, ok := films.Find(a.films(), "home"); ok {
		t.Fatal("the outside film is still on this library's shelf")
	}
}

// TestMoveFilmFromThePlayerAndMenuBar checks that the film page's ⋯, on every system, and File › 移到资料库 of the menu bar, which the app has on macOS, move the open film, closing it.
func TestMoveFilmFromThePlayerAndMenuBar(t *testing.T) {
	f := newMoveFixture(t)
	a, tt := f.a, f.tt
	m := testlocale.Messages()
	a.Open(must(films.Find(a.films(), "film")))
	refresh(tt, a)
	tt.Press("id:film-more")
	tt.ChoosePopup(m.FilmsMoveToLibrary(), "英语听力")
	// A rename the system refuses (Windows, while a handle on the film's folder is still open) falls back to the copy, which ends on its own goroutine.
	waitUntil(t, tt, a, f.updates, func() bool { return a.lib.move.copying == nil })
	refresh(tt, a)
	if a.openKey != "" {
		t.Fatalf("the moved film is still open: %q", a.openKey)
	}
	if _, err := os.Stat(filepath.Join(f.to, "Talk", "video.mp4")); err != nil {
		t.Fatalf("the film page did not move the film: %v", err)
	}

	a.Choose(f.to)
	refresh(tt, a)
	a.Open(must(films.Find(a.films(), "film")))
	// The app has a menu bar of its own on macOS alone (appMenuBar); the test gives it one on every system, as the menu bar's own tests do.
	useMenuBar(t, a, tt)
	tt.ChooseMenu(m.MenuGroupFile(), m.FilmsMoveToLibrary(), "我的影片")
	waitUntil(t, tt, a, f.updates, func() bool { return a.lib.move.copying == nil })
	refresh(tt, a)
	if _, err := os.Stat(f.video); err != nil {
		t.Fatalf("the menu bar did not move the film back: %v", err)
	}
}

// TestRemoteMovesAFilm checks films.move: it moves a film of the open library to a library named by its name or its path, answers its id there, and refuses a library Kando does not know or a film being processed.
func TestRemoteMovesAFilm(t *testing.T) {
	f := newMoveFixture(t)
	a := f.a
	remoteServe(t, a)
	got, err := remoteCall[remote.Moved](t, f.updates, remote.MethodMove, remote.MoveParams{ID: "film", To: "英语听力"})
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != "film" || got.Library != f.to || got.Name != "英语听力" {
		t.Fatalf("films.move answered %+v", got)
	}
	if _, err := os.Stat(filepath.Join(f.to, "Talk", "video.mp4")); err != nil {
		t.Fatalf("films.move did not move the film: %v", err)
	}
	_, err = remoteCall[remote.Moved](t, f.updates, remote.MethodMove, remote.MoveParams{ID: "film", To: f.from})
	remoteWantCode(t, err, remote.CodeNotFound)

	a.Choose(f.to)
	got, err = remoteCall[remote.Moved](t, f.updates, remote.MethodMove, remote.MoveParams{ID: "film", To: f.from})
	if err != nil || got.Library != f.from {
		t.Fatalf("films.move by path = %+v, %v", got, err)
	}
	a.Choose(f.from)
	_, err = remoteCall[remote.Moved](t, f.updates, remote.MethodMove, remote.MoveParams{ID: "film", To: "Nowhere"})
	remoteWantCode(t, err, remote.CodeNotFound)
	a.library.Jobs[0].Stage = core.StageTranscribing
	_, err = remoteCall[remote.Moved](t, f.updates, remote.MethodMove, remote.MoveParams{ID: "film", To: "英语听力"})
	remoteWantCode(t, err, remote.CodeNotReady)
}
