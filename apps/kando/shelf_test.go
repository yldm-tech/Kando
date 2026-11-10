package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"kando/apps/kando/locales"
	"kando/apps/kando/settings"
	"kando/apps/kando/shelf"
	"kando/packages/core"
	"kando/packages/media"
)

func TestMissingLibraryFollowsTheRenamedBundle(t *testing.T) {
	parent := t.TempDir()
	current := filepath.Join(parent, "影视库.kando")
	old := filepath.Join(parent, "影视库 2.kando")
	renamed := filepath.Join(parent, "missav.kando")
	if err := os.Mkdir(current, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(renamed, 0o755); err != nil {
		t.Fatal(err)
	}
	a := newApp(&fakePipeline{}, func(func()) {})
	a.edit(func(s *session) {
		s.LibraryDir = current
		s.Libraries = []string{current, old}
	})
	if !a.reconcileMissingLibraries() {
		t.Fatal("the renamed bundle was not picked up")
	}
	got := a.knownLibraries()
	if len(got) != 2 || !containsPath(got, renamed) || containsPath(got, old) {
		t.Fatalf("libraries = %v", got)
	}
	if a.snap().LibraryDir != current {
		t.Fatalf("open library = %q", a.snap().LibraryDir)
	}
}

func TestRenameMovesTheInodeWithTheLibrary(t *testing.T) {
	parent := t.TempDir()
	old := filepath.Join(parent, "影视库.kando")
	next := filepath.Join(parent, "missav.kando")
	a := newApp(&fakePipeline{}, func(func()) {})
	a.settingsPath = filepath.Join(t.TempDir(), "settings.json")
	a.edit(func(s *session) {
		s.LibraryDir = old
		s.Libraries = []string{old}
	})
	a.rememberID(old, media.MakeDirID(7, 99))
	a.renameLibraryPath(old, next)
	if _, ok := a.snap().Inodes[old]; ok {
		t.Fatal("old path kept its inode")
	}
	dev, ino, ok := a.inode(next).Parts()
	if !ok || dev != 7 || ino != 99 {
		t.Fatalf("inode = %d %d ok=%v", dev, ino, ok)
	}
	if a.snap().LibraryDir != next {
		t.Fatalf("open library = %q", a.snap().LibraryDir)
	}
	if err := a.saveSettings(); err != nil {
		t.Fatal(err)
	}
	saved := loadSavedSettings(a.settingsPath)
	if saved.LibraryDir != next {
		t.Fatalf("saved library = %q", saved.LibraryDir)
	}
	got := saved.LibraryInodes[next]
	if got.Dev != 7 || got.Ino != 99 {
		t.Fatalf("saved inode = %+v", got)
	}
	if _, ok := saved.LibraryInodes[old]; ok {
		t.Fatal("saved the old path")
	}
}

func TestWelcomeCreatesAndSwitchesLibraries(t *testing.T) {
	parent := t.TempDir()
	a, tt, _ := newTestApp(&fakePipeline{})
	a.edit(func(s *session) {
		s.LibraryDir = ""
		s.Libraries = nil
		s.LibraryParent = parent
		s.LibraryName = "第一库"
	})
	a.settingsPath = filepath.Join(t.TempDir(), "settings.json")
	tt.Frame()
	for _, text := range []string{"还没有资料库", "新建资料库…", "打开已有资料库…"} {
		if !tt.HasText(text) {
			t.Fatalf("missing %q in %q", text, tt.Texts())
		}
	}
	if err := tt.Click("新建资料库…"); err != nil {
		t.Fatal(err)
	}
	a.edit(func(s *session) { s.LibraryName = "第一库" })
	tt.Frame()
	for _, text := range []string{"新建资料库", "名称", "位置", "更改…", "取消", "创建"} {
		if !tt.HasText(text) {
			t.Fatalf("missing %q in the sheet %q", text, tt.Texts())
		}
	}
	if err := tt.Click("创建"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	first := filepath.Join(parent, "第一库.kando")
	if a.snap().LibraryDir != first {
		t.Fatalf("library = %q", a.snap().LibraryDir)
	}
	if _, err := os.Stat(first); err != nil {
		t.Fatal(err)
	}
	if tt.HasText("还没有资料库") || tt.HasText("新建资料库") {
		t.Fatalf("the empty page or the sheet stayed after the library was created: %q", tt.Texts())
	}
	chooseFromSwitcher(t, tt, "新建资料库…")
	a.edit(func(s *session) { s.LibraryName = "第二库" })
	tt.Frame()
	if !tt.HasText("新建资料库") || !tt.HasText("创建") {
		t.Fatalf("texts %q", tt.Texts())
	}
	if err := tt.Click("创建"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	second := filepath.Join(parent, "第二库.kando")
	if a.snap().LibraryDir != second || len(a.snap().Libraries) != 2 {
		t.Fatalf("library %q list %v", a.snap().LibraryDir, a.snap().Libraries)
	}
	chooseFromSwitcher(t, tt, "第一库")
	tt.Frame()
	if a.snap().LibraryDir != first {
		t.Fatalf("switched library = %q", a.snap().LibraryDir)
	}
	general := settingsOpenAt(a, tt, settings.SectionGeneral)
	if !general.HasText("第一库") || !general.HasText("只从看懂的列表里拿掉") {
		t.Fatalf("texts %q", general.Texts())
	}
	if err := general.Click("移出"); err != nil {
		t.Fatalf("%v texts %q", err, general.Texts())
	}
	general.Frame()
	if a.snap().LibraryDir != second {
		t.Fatalf("after forget, library = %q", a.snap().LibraryDir)
	}
	if _, err := os.Stat(first); err != nil {
		t.Fatal(err)
	}
	saved := loadSavedSettings(a.settingsPath)
	if saved.LibraryDir != second || len(saved.Libraries) != 1 || saved.Libraries[0] != second {
		t.Fatalf("saved %#v", saved)
	}
}

func TestSwitchLibraryUpdatesOutputDir(t *testing.T) {
	a, _, _ := newTestApp(media.Tools{OutputDir: t.TempDir()})
	dir := filepath.Join(t.TempDir(), "movies")
	a.switchLibrary(dir)
	tools, ok := a.pipeline.(media.Tools)
	if !ok || tools.OutputDir != dir {
		t.Fatalf("output = %#v", a.pipeline)
	}
}

// TestLibraryRenamedWhileClosedKeepsItsFilms checks that a library folder renamed while the app was closed keeps its films when the app starts again, as main starts it: the settings are restored before library.json is read, so the jobs, the resume positions and what was watched follow the new path once it is read. A video imported from outside the library stays where it was, so only its job's library tells which shelf it is on.
func TestLibraryRenamedWhileClosedKeepsItsFilms(t *testing.T) {
	t.Setenv("KANDO_OUTPUT_DIR", "")
	parent := t.TempDir()
	old := filepath.Join(parent, "我的影片.kando")
	inside := filepath.Join(old, "talk", "video.mp4")
	away := t.TempDir()
	outside := filepath.Join(away, "clip.mp4")
	outsideSub := filepath.Join(away, "clip.srt")
	if err := os.MkdirAll(filepath.Dir(inside), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{inside, outside, outsideSub} {
		if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	settingsPath := filepath.Join(t.TempDir(), "settings.json")
	libraryPath := filepath.Join(t.TempDir(), "library.json")

	before := newApp(&fakePipeline{}, func(func()) {})
	before.settingsPath = settingsPath
	before.openLibrary(libraryPath)
	before.edit(func(s *session) {
		s.LibraryDir = old
		s.Libraries = []string{old}
	})
	before.rememberLibrary(old)
	before.library.Jobs = []core.Job{
		{ID: "clip", Source: core.FileURL(outside), Stage: core.StageComplete, VideoPath: outside, SubtitlePath: outsideSub, LibraryDir: old},
		{ID: "talk", Source: "https://www.youtube.com/watch?v=jNQXAC9IVRw", Stage: core.StageComplete, VideoPath: inside, LibraryDir: old},
	}
	before.library.Positions = map[string]time.Duration{outside: 5 * time.Second, inside: 7 * time.Second}
	before.library.Furthest = map[string]time.Duration{outside: 9 * time.Second, inside: 11 * time.Second}
	if err := before.library.Save(); err != nil {
		t.Fatal(err)
	}
	if err := before.saveSettings(); err != nil {
		t.Fatal(err)
	}

	renamed := filepath.Join(parent, "改名库.kando")
	if err := os.Rename(old, renamed); err != nil {
		t.Fatal(err)
	}
	moved := filepath.Join(renamed, "talk", "video.mp4")

	a := newApp(&fakePipeline{}, func(func()) {})
	a.settingsPath = settingsPath
	a.restoreSettings(a.loadSettings())
	a.openLibrary(libraryPath)
	a.refreshLibrary()

	if a.snap().LibraryDir != renamed {
		t.Fatalf("open library = %q, want %q", a.snap().LibraryDir, renamed)
	}
	if got := a.films(); len(got) != 2 {
		t.Fatalf("films on the renamed library = %+v, jobs %+v", got, a.library.Jobs)
	}
	if at := a.ResumeAt(outside); at != 5*time.Second {
		t.Fatalf("ResumeAt(outside) = %v", at)
	}
	if at := a.ResumeAt(moved); at != 7*time.Second {
		t.Fatalf("ResumeAt(%s) = %v, positions %v", moved, at, a.resumePositions())
	}
	if a.palette.furthest[outside] != 9*time.Second || a.palette.furthest[moved] != 11*time.Second || len(a.palette.furthest) != 2 {
		t.Fatalf("furthest %v", a.palette.furthest)
	}
	lib, err := shelf.Load(libraryPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, job := range lib.Jobs {
		if filepath.Clean(job.LibraryDir) != renamed {
			t.Fatalf("library.json keeps job %s in %q", job.ID, job.LibraryDir)
		}
	}
	if lib.Positions[moved] != 7*time.Second || lib.Furthest[moved] != 11*time.Second || lib.Positions[outside] != 5*time.Second || lib.Furthest[outside] != 9*time.Second {
		t.Fatalf("library.json positions %v furthest %v", lib.Positions, lib.Furthest)
	}

	again := newApp(&fakePipeline{}, func(func()) {})
	again.settingsPath = settingsPath
	again.restoreSettings(again.loadSettings())
	again.openLibrary(libraryPath)
	again.refreshLibrary()
	if got := again.films(); again.snap().LibraryDir != renamed || len(got) != 2 {
		t.Fatalf("after another start: library %q films %+v", again.snap().LibraryDir, got)
	}
}

// shelfWriteVideos writes a placeholder file at each path, making its folders.
func shelfWriteVideos(t *testing.T, paths ...string) {
	t.Helper()
	for _, path := range paths {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("v"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// shelfJobIDs is the IDs of jobs, in order.
func shelfJobIDs(jobs []core.Job) []string {
	ids := make([]string, len(jobs))
	for i, job := range jobs {
		ids[i] = job.ID
	}
	return ids
}

// TestRefreshKeepsTheJobsOfAnotherLibraryWhoseDiskIsGone checks that a refresh of the open library drops only its own finished jobs whose videos are gone. library.json holds every library's jobs, and another library on a disk that is not mounted must keep its records, so its films come back with their subtitles when the disk does; the refresh saves library.json with them.
func TestRefreshKeepsTheJobsOfAnotherLibraryWhoseDiskIsGone(t *testing.T) {
	parent := t.TempDir()
	here := filepath.Join(parent, "我的影片.kando")
	away := filepath.Join(parent, "外接盘", "影片.kando")
	kept := filepath.Join(here, "kept", "video.mp4")
	gone := filepath.Join(here, "gone", "video.mp4")
	elsewhere := filepath.Join(away, "talk", "video.mp4")
	shelfWriteVideos(t, kept, gone, elsewhere)
	path := filepath.Join(t.TempDir(), "library.json")
	a := newApp(&fakePipeline{}, func(func()) {})
	a.openLibrary(path)
	a.edit(func(s *session) {
		s.LibraryDir = here
		s.Libraries = []string{here, away}
	})
	a.library.Jobs = []core.Job{
		{ID: "kept", Source: "https://youtu.be/kept", Stage: core.StageComplete, VideoPath: kept, LibraryDir: here},
		{ID: "gone", Source: "https://youtu.be/gone", Stage: core.StageComplete, VideoPath: gone, LibraryDir: here},
		{ID: "elsewhere", Source: "https://youtu.be/talk", Language: "简体中文", Stage: core.StageComplete, VideoPath: elsewhere, SubtitlePath: filepath.Join(away, "talk", "video.bilingual.srt"), LibraryDir: away, Untranslated: 2},
	}
	if err := os.RemoveAll(filepath.Join(parent, "外接盘")); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(filepath.Dir(gone)); err != nil {
		t.Fatal(err)
	}
	a.refreshLibrary()
	if got := shelfJobIDs(a.library.Jobs); len(got) != 2 || got[0] != "kept" || got[1] != "elsewhere" {
		t.Fatalf("jobs after the refresh = %v", got)
	}
	lib, err := shelf.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := shelfJobIDs(lib.Jobs); len(got) != 2 || got[0] != "kept" || got[1] != "elsewhere" {
		t.Fatalf("saved jobs = %v", got)
	}
	if job := lib.Jobs[1]; job.VideoPath != elsewhere || job.Language != "简体中文" || job.Untranslated != 2 || job.Source != "https://youtu.be/talk" {
		t.Fatalf("saved %+v", job)
	}
}

// TestRefreshDoesNotTakeAnotherLibrarysJobForAFilmOfTheSameName checks that a job of another library whose disk is not mounted is not pointed at a film of the same folder and file name in the open library, which would give that film the other one's subtitles.
func TestRefreshDoesNotTakeAnotherLibrarysJobForAFilmOfTheSameName(t *testing.T) {
	parent := t.TempDir()
	here := filepath.Join(parent, "我的影片.kando")
	away := filepath.Join(parent, "外接盘", "影片.kando")
	mine := filepath.Join(here, "talk", "video.mp4")
	theirs := filepath.Join(away, "talk", "video.mp4")
	shelfWriteVideos(t, mine, theirs)
	a := newApp(&fakePipeline{}, func(func()) {})
	a.edit(func(s *session) {
		s.LibraryDir = here
		s.Libraries = []string{here, away}
	})
	a.library.Jobs = []core.Job{{ID: "theirs", Source: "https://youtu.be/talk", Stage: core.StageComplete, VideoPath: theirs, SubtitlePath: filepath.Join(away, "talk", "video.bilingual.srt"), LibraryDir: away}}
	if err := os.RemoveAll(filepath.Join(parent, "外接盘")); err != nil {
		t.Fatal(err)
	}
	a.refreshLibrary()
	if len(a.library.Jobs) != 1 || a.library.Jobs[0].VideoPath != theirs || a.library.Jobs[0].SubtitlePath != filepath.Join(away, "talk", "video.bilingual.srt") {
		t.Fatalf("jobs = %+v", a.library.Jobs)
	}
}

// TestRelocateMissingFollowsAFilmOfTheOpenLibrary checks that a finished job of the open library whose folder is gone is pointed at the film of the same folder and file name at the top of the library, with its subtitle and resume position, and is then not dropped as missing.
func TestRelocateMissingFollowsAFilmOfTheOpenLibrary(t *testing.T) {
	here := filepath.Join(t.TempDir(), "我的影片.kando")
	old := filepath.Join(here, "gone", "talk", "video.mp4")
	oldSub := filepath.Join(here, "gone", "talk", "video.bilingual.srt")
	video := filepath.Join(here, "talk", "video.mp4")
	sub := filepath.Join(here, "talk", "video.bilingual.srt")
	shelfWriteVideos(t, video, sub)
	a := newApp(&fakePipeline{}, func(func()) {})
	a.edit(func(s *session) {
		s.LibraryDir = here
		s.Libraries = []string{here}
	})
	a.library.Jobs = []core.Job{{ID: "talk", Source: "https://youtu.be/talk", Stage: core.StageComplete, VideoPath: old, SubtitlePath: oldSub, LibraryDir: here}}
	a.library.Positions = map[string]time.Duration{old: 42 * time.Second}
	if !a.relocateMissing() {
		t.Fatal("relocateMissing reported no change")
	}
	if job := a.library.Jobs[0]; job.VideoPath != video || job.SubtitlePath != sub {
		t.Fatalf("job = %+v", job)
	}
	if at, ok := a.library.Positions[video]; !ok || at != 42*time.Second {
		t.Fatalf("positions = %v", a.library.Positions)
	}
	if a.dropMissing() || len(a.library.Jobs) != 1 {
		t.Fatalf("jobs after dropMissing = %+v", a.library.Jobs)
	}
}

// TestDropMissingKeepsEverythingWhileTheLibraryIsGone checks that while the open library's own folder cannot be found, as when its disk was unplugged after the refresh started, no job is dropped.
func TestDropMissingKeepsEverythingWhileTheLibraryIsGone(t *testing.T) {
	here := filepath.Join(t.TempDir(), "我的影片.kando")
	a := newApp(&fakePipeline{}, func(func()) {})
	a.edit(func(s *session) { s.LibraryDir = here })
	a.library.Jobs = []core.Job{{ID: "talk", Source: "https://youtu.be/talk", Stage: core.StageComplete, VideoPath: filepath.Join(here, "talk", "video.mp4"), LibraryDir: here}}
	if a.dropMissing() || len(a.library.Jobs) != 1 {
		t.Fatalf("jobs = %+v", a.library.Jobs)
	}
}

// TestDropMissingKeepsALocalVideoWhoseFolderIsGone checks a video picked where it was and kept outside the library: it is dropped once the file is deleted from its folder, but kept while the folder itself is not there, as on a disk that is not mounted.
func TestDropMissingKeepsALocalVideoWhoseFolderIsGone(t *testing.T) {
	here := filepath.Join(t.TempDir(), "我的影片.kando")
	disk := filepath.Join(t.TempDir(), "外接盘")
	deleted := filepath.Join(t.TempDir(), "deleted.mp4")
	unmounted := filepath.Join(disk, "clip.mp4")
	if err := os.MkdirAll(here, 0o755); err != nil {
		t.Fatal(err)
	}
	a := newApp(&fakePipeline{}, func(func()) {})
	a.edit(func(s *session) { s.LibraryDir = here })
	a.library.Jobs = []core.Job{
		{ID: "deleted", Source: core.FileURL(deleted), Stage: core.StageComplete, VideoPath: deleted, LibraryDir: here},
		{ID: "unmounted", Source: core.FileURL(unmounted), Stage: core.StageComplete, VideoPath: unmounted, LibraryDir: here},
	}
	if !a.dropMissing() {
		t.Fatal("the deleted video's job was not dropped")
	}
	if got := shelfJobIDs(a.library.Jobs); len(got) != 1 || got[0] != "unmounted" {
		t.Fatalf("jobs = %v", got)
	}
}

// TestDamagedLibraryIsKeptAsBad checks that a library.json cut short is copied byte for byte to library.json.bad when the app opens it, and the sidebar says so, so the save that follows, which writes the shelf as Kando could read it, does not lose what the file held.
func TestDamagedLibraryIsKeptAsBad(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.json")
	damaged := []byte(`{"jobs": [{"id": "talk", "source": "https://youtu.be/talk", "stage": "complete", "videoPath": "/v/talk/video.mp4"}, {"id": "cut`)
	if err := os.WriteFile(path, damaged, 0o600); err != nil {
		t.Fatal(err)
	}
	a := newApp(&fakePipeline{}, func(func()) {})
	a.openLibrary(path)
	if want := locales.LibraryShelfDamaged("unexpected end of JSON input"); a.saveErr != want {
		t.Fatalf("saveErr = %q, want %q", a.saveErr, want)
	}
	bad, err := os.ReadFile(path + ".bad")
	if err != nil || !bytes.Equal(bad, damaged) {
		t.Fatalf("library.json.bad = %q (%v), want %q", bad, err, damaged)
	}
	if info, err := os.Stat(path + ".bad"); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("library.json.bad: %v %v", info, err)
	}
	a.library.Upsert(core.Job{ID: "new", Source: "https://youtu.be/new", Stage: core.StageQueued})
	if err := a.library.Save(); err != nil {
		t.Fatal(err)
	}
	if bad, err := os.ReadFile(path + ".bad"); err != nil || !bytes.Equal(bad, damaged) {
		t.Fatalf("library.json.bad after a save = %q (%v)", bad, err)
	}
	lib, err := shelf.Load(path)
	if err != nil || len(lib.Jobs) != 1 || lib.Jobs[0].ID != "new" {
		t.Fatalf("library.json after a save: %+v %v", lib.Jobs, err)
	}
}

// TestDamagedLibraryThatCannotBeKeptIsNotSaved checks that when library.json cannot be read in full and the copy beside it cannot be written, the sidebar says so and nothing writes over library.json.
func TestDamagedLibraryThatCannotBeKeptIsNotSaved(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.json")
	damaged := []byte(`{"jobs": [{"id": "cut`)
	if err := os.WriteFile(path, damaged, 0o600); err != nil {
		t.Fatal(err)
	}
	// A folder where library.json.bad goes cannot be written over as a file.
	if err := os.Mkdir(path+".bad", 0o700); err != nil {
		t.Fatal(err)
	}
	writeErr := os.WriteFile(path+".bad", damaged, 0o600)
	if writeErr == nil {
		t.Fatal("library.json.bad could be written over the folder")
	}
	a := newApp(&fakePipeline{}, func(func()) {})
	a.openLibrary(path)
	if want := locales.LibraryShelfDamagedUnsaved(writeErr.Error()); a.saveErr != want {
		t.Fatalf("saveErr = %q, want %q", a.saveErr, want)
	}
	a.show(core.Job{ID: "new", Source: "https://youtu.be/new", Stage: core.StageQueued})
	if want := locales.LibraryShelfNotSaved(); a.saveErr != want {
		t.Fatalf("saveErr after a save = %q, want %q", a.saveErr, want)
	}
	if got, err := os.ReadFile(path); err != nil || !bytes.Equal(got, damaged) {
		t.Fatalf("library.json after a save = %q (%v), want %q", got, err, damaged)
	}
}

// TestUnreadableLibraryIsNotSaved checks that a library.json that is there but cannot be read is not written over by the empty shelf Kando goes on with.
func TestUnreadableLibraryIsNotSaved(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows has no permission bits that stop the owner reading a file")
	}
	path := filepath.Join(t.TempDir(), "library.json")
	saved := []byte(`{"jobs": [{"id": "talk", "source": "https://youtu.be/talk", "stage": "complete"}]}` + "\n")
	if err := os.WriteFile(path, saved, 0o200); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(path, 0o600) })
	if _, err := os.ReadFile(path); err == nil {
		t.Skip("this user reads files whatever their permissions, as root does")
	}
	a := newApp(&fakePipeline{}, func(func()) {})
	a.openLibrary(path)
	if !strings.HasPrefix(a.saveErr, locales.LibraryShelfReadFailed("")) {
		t.Fatalf("saveErr = %q", a.saveErr)
	}
	if err := a.library.Save(); !errors.Is(err, shelf.ErrUnread) {
		t.Fatalf("Save = %v, want shelf.ErrUnread", err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(path); err != nil || !bytes.Equal(got, saved) {
		t.Fatalf("library.json = %q (%v), want %q", got, err, saved)
	}
}
