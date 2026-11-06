package main

import (
	"os"
	"path/filepath"
	"testing"

	"lingocut/apps/lingocut/settings"
	"lingocut/packages/media"
)

func TestMissingLibraryFollowsTheRenamedBundle(t *testing.T) {
	parent := t.TempDir()
	current := filepath.Join(parent, "影视库.lingocut")
	old := filepath.Join(parent, "影视库 2.lingocut")
	renamed := filepath.Join(parent, "missav.lingocut")
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
	old := filepath.Join(parent, "影视库.lingocut")
	next := filepath.Join(parent, "missav.lingocut")
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
	first := filepath.Join(parent, "第一库.lingocut")
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
	second := filepath.Join(parent, "第二库.lingocut")
	if a.snap().LibraryDir != second || len(a.snap().Libraries) != 2 {
		t.Fatalf("library %q list %v", a.snap().LibraryDir, a.snap().Libraries)
	}
	chooseFromSwitcher(t, tt, "第一库")
	tt.Frame()
	if a.snap().LibraryDir != first {
		t.Fatalf("switched library = %q", a.snap().LibraryDir)
	}
	tt = settingsOpenAt(a, tt, settings.SectionGeneral)
	if err := tt.Click("切换"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("从列表移出"); err != nil {
		t.Fatalf("%v texts %q", err, tt.Texts())
	}
	tt.Frame()
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
