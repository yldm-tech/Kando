package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"lingocut/apps/lingocut/settings"
	"lingocut/packages/core"
	"lingocut/packages/media"

	"github.com/egoist/mygo/ui"
)

// openClip opens a black clip of the given seconds from the library and waits until its picture is loaded.
func openClip(t *testing.T, seconds int) (*app, *ui.Tester, chan func()) {
	t.Helper()
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg is not installed")
	}
	dir := t.TempDir()
	video := filepath.Join(dir, "clip.mp4")
	if out, err := exec.Command(ffmpeg, "-nostdin", "-loglevel", "error", "-y", "-f", "lavfi", "-i", "color=c=black:s=160x90:d="+strconv.Itoa(seconds)+":r=10", "-c:v", "mpeg4", "-pix_fmt", "yuv420p", video).CombinedOutput(); err != nil {
		t.Skipf("ffmpeg could not make a clip: %v %s", err, out)
	}
	a, tt, updates := newTestApp(media.Tools{FFmpeg: ffmpeg})
	a.edit(func(s *session) { s.LibraryDir = dir })
	a.library.Jobs = []core.Job{{ID: "clip", Source: "file://" + video, Language: "简体中文", Stage: core.StageComplete, VideoPath: video}}
	t.Cleanup(a.stopPlayback)
	tt.Frame()
	if err := tt.Click("clip"); err != nil {
		t.Fatalf("%v texts %q", err, tt.Texts())
	}
	waitUntil(t, updates, func() bool { return a.player.play != nil })
	tt.Frame()
	return a, tt, updates
}

func TestSpaceTogglesTheOpenFilm(t *testing.T) {
	a, tt, _ := openClip(t, 1)
	if a.player.play.player.Playing() {
		t.Fatal("the film started before Space")
	}
	tt.Key(0, ui.KeySpace)
	tt.Frame()
	if !a.player.play.player.Playing() {
		t.Fatal("Space did not start the film")
	}
	tt.Key(0, ui.KeySpace)
	tt.Frame()
	if a.player.play.player.Playing() {
		t.Fatal("Space did not hold the film")
	}
}

func TestArrowKeysSkipWithinTheFilm(t *testing.T) {
	a, tt, updates := openClip(t, 12)
	tt.Key(0, ui.KeyRight)
	tt.Frame()
	waitUntil(t, updates, func() bool { return a.player.play != nil })
	tt.Frame()
	if head := a.player.play.playhead(); head < 5*time.Second || head > 7*time.Second {
		t.Fatalf("head %v after →, want about 5s", head)
	}
	tt.Key(0, ui.KeyLeft)
	tt.Frame()
	waitUntil(t, updates, func() bool { return a.player.play != nil })
	if head := a.player.play.playhead(); head > 2*time.Second {
		t.Fatalf("head %v after ←, want about the start", head)
	}
}

func TestEscapeClosesTheFilm(t *testing.T) {
	a, tt, _ := openClip(t, 1)
	tt.Key(0, ui.KeyEscape)
	tt.Frame()
	if a.openKey != "" || a.player.play != nil {
		t.Fatalf("open %q play %v after Escape", a.openKey, a.player.play != nil)
	}
	if !tt.HasText("开始处理") {
		t.Fatalf("library not shown: %q", tt.Texts())
	}
}

func TestPlayerKeysWaitForAFilm(t *testing.T) {
	a, tt, _ := newTestApp(&fakePipeline{})
	tt.Frame()
	tt.Key(0, ui.KeyEscape)
	tt.Key(0, ui.KeySpace)
	tt.Frame()
	if a.page != pageFilms || a.openKey != "" || a.player.play != nil {
		t.Fatalf("page %q open %q", a.page, a.openKey)
	}
}

func TestCheatsheetListsTheShortcutTable(t *testing.T) {
	a, tt, _ := newTestApp(&fakePipeline{})
	tt.Frame()
	tt.Key(ui.Cmd, ui.KeySlash)
	tt.Frame()
	if !a.overlay.cheatsheet {
		t.Fatal("⌘/ did not open the cheat sheet")
	}
	for _, s := range a.shortcuts() {
		if !tt.HasText(s.Label) || !tt.HasText(s.Group) {
			t.Fatalf("cheat sheet misses %q under %q: %q", s.Label, s.Group, tt.Texts())
		}
	}
	tt.Key(ui.Cmd, ui.KeySlash)
	tt.Frame()
	if a.overlay.cheatsheet || tt.HasText("键盘快捷键") {
		t.Fatalf("a second ⌘/ left the cheat sheet open: %q", tt.Texts())
	}
	tt.Key(ui.Cmd, ui.KeySlash)
	tt.Frame()
	if !a.overlay.cheatsheet {
		t.Fatal("⌘/ did not open the cheat sheet again")
	}
	tt.Key(0, ui.KeyEscape)
	tt.Frame()
	if a.overlay.cheatsheet || tt.HasText("键盘快捷键") {
		t.Fatalf("Escape left the cheat sheet open: %q", tt.Texts())
	}
}

// TestCheatsheetIsDark checks that the cheat sheet is drawn in the player's dark theme over the light window.
func TestCheatsheetIsDark(t *testing.T) {
	a, tt, _ := newTestApp(&fakePipeline{})
	a.overlay.cheatsheet = true
	tt.Frame()
	r, ok := tt.Find("键盘快捷键")
	if !ok {
		t.Fatalf("no cheat sheet: %q", tt.Texts())
	}
	img := tt.Image()
	scale := float32(img.Bounds().Dx()) / 1100
	px := img.RGBAAt(int((r.X-6)*scale), int((r.Y+r.H/2)*scale))
	if px.R > 60 || px.G > 60 || px.B > 60 {
		t.Fatalf("cheat sheet behind its title = %v, want dark", px)
	}
}

func TestSettingsShortcutOpensSettings(t *testing.T) {
	a, tt, _ := newTestApp(&fakePipeline{})
	tt.Frame()
	tt.Key(ui.Cmd, ui.KeyComma)
	tt.Frame()
	wantSettings(t, a, settings.SectionGeneral)
}

// TestMenuBarTakesCommandShortcuts checks that once the app has its own menu bar, ⌘ shortcuts are left to the menu's accelerators and single keys still work.
func TestMenuBarTakesCommandShortcuts(t *testing.T) {
	a, tt, _ := newTestApp(&fakePipeline{})
	a.menu.menuBar = true
	tt.Frame()
	tt.Key(ui.Cmd, ui.KeyComma)
	tt.Frame()
	if a.page != pageFilms {
		t.Fatalf("⌘, ran beside the menu bar: page %q", a.page)
	}
}

func TestShortcutsAreUnique(t *testing.T) {
	a, _, _ := newTestApp(&fakePipeline{})
	seen := map[[2]int]string{}
	for _, s := range a.shortcuts() {
		if s.Group == "" || s.Label == "" || s.Run == nil {
			t.Fatalf("incomplete shortcut %+v", s)
		}
		k := [2]int{int(s.Mods), int(s.Key)}
		if other, ok := seen[k]; ok {
			t.Fatalf("%q and %q share a key", other, s.Label)
		}
		seen[k] = s.Label
	}
}

func TestDroppedVideoIsQueued(t *testing.T) {
	a, _, _ := newTestApp(&fakePipeline{})
	dir := t.TempDir()
	note := filepath.Join(dir, "a.txt")
	video := filepath.Join(dir, "b.mp4")
	for _, p := range []string{note, video} {
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	a.dropFiles([]string{note, video})
	if len(a.library.Jobs) != 1 || a.library.Jobs[0].Source != "file://"+video {
		t.Fatalf("jobs %#v", a.library.Jobs)
	}
}

// TestPlayerKeysAfterTheFilmLeaves checks that a player key pressed in the frame the open film drops out of the library does nothing. wireKeys registers the key before the page closes the film in the same build, so the key must check again when it runs.
func TestPlayerKeysAfterTheFilmLeaves(t *testing.T) {
	for _, key := range []ui.Key{ui.KeySpace, ui.KeyLeft, ui.KeyRight} {
		a, tt, _ := openClip(t, 2)
		a.library.Jobs = nil
		tt.Key(0, key)
		tt.Frame()
		if a.openKey != "" || a.player.play != nil {
			t.Fatalf("key %v: open %q play %v after the film left", key, a.openKey, a.player.play != nil)
		}
	}
}

// TestModalDialogsNest checks the window's modal flag: it stays up while any dialog holds it, as a file panel opened from a sheet does, and a release that runs twice counts once.
func TestModalDialogsNest(t *testing.T) {
	a, _, _ := newTestApp(&fakePipeline{})
	if a.modalOpen() {
		t.Fatal("modal open on a new window")
	}
	sheet := a.holdModal()
	panel := a.holdModal()
	panel()
	panel()
	if !a.modalOpen() {
		t.Fatal("closing the file panel closed the sheet's hold too")
	}
	sheet()
	if a.modalOpen() {
		t.Fatalf("modal still open after every dialog closed: %d", a.overlay.modal)
	}
}
