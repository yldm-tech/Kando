package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

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
		{ID: "clip", Source: "file://" + outside, Stage: core.StageComplete, VideoPath: outside, SubtitlePath: outsideSub, LibraryDir: old},
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
