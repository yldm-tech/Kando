package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"testing"
	"time"

	"kando/apps/kando/cheatsheet"
	"kando/apps/kando/settings"
	"kando/packages/core"
	"kando/packages/media"

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
	listed := map[cheatsheet.Combo]bool{}
	for _, g := range a.cheatsheetGroups() {
		if !tt.HasText(g.Title) {
			t.Fatalf("cheat sheet misses the group %q: %q", g.Title, tt.Texts())
		}
		for _, k := range g.Keys {
			if !tt.HasText(k.Label) {
				t.Fatalf("cheat sheet misses %q under %q: %q", k.Label, g.Title, tt.Texts())
			}
			for _, c := range k.Combos {
				listed[c] = true
			}
		}
	}
	for _, s := range a.shortcuts() {
		if !listed[cheatsheet.Combo{Mods: s.Mods, Key: s.Key}] {
			t.Fatalf("cheat sheet misses the keys of %q", s.Label)
		}
	}
	tt.Key(ui.Cmd, ui.KeySlash)
	tt.Frame()
	if _, shown := tt.Find("快捷键"); a.overlay.cheatsheet || shown {
		t.Fatalf("a second ⌘/ left the cheat sheet open: %q", tt.Texts())
	}
	tt.Key(ui.Cmd, ui.KeySlash)
	tt.Frame()
	if !a.overlay.cheatsheet {
		t.Fatal("⌘/ did not open the cheat sheet again")
	}
	tt.Key(0, ui.KeyEscape)
	tt.Frame()
	if _, shown := tt.Find("快捷键"); a.overlay.cheatsheet || shown {
		t.Fatalf("Escape left the cheat sheet open: %q", tt.Texts())
	}
}

// TestCheatsheetFollowsThePrototypeLayout checks that the cheat sheet groups the table as the prototype's sheet does, not as the menu bar: 播放 first with ← and → joined on one line and 全屏 under it, then 窗口 with ⌘/, ⌘, and Esc, and the rows the prototype's sheet does not place after them, under their menus' groups, ⌘1 and ⌘2 under 窗口 and 文件's in a group of its own.
func TestCheatsheetFollowsThePrototypeLayout(t *testing.T) {
	a, _, _ := newTestApp(&fakePipeline{})
	// Other areas add rows to these groups and groups of their own, so this checks that these lines keep their groups and order, not that the sheet has nothing else.
	want := []struct {
		title  string
		labels []string
	}{
		{"播放", []string{"播放 / 暂停", "后退 / 前进 5 秒", "全屏"}},
		{"窗口", []string{"快捷键速查", "设置", "返回资料库", "片库", "播放器"}},
		{"文件", []string{"打开本地视频…", "新建资料库…", "打开资料库…"}},
	}
	groups := a.cheatsheetGroups()
	at := -1
	for _, w := range want {
		i := slices.IndexFunc(groups, func(g cheatsheet.Group) bool { return g.Title == w.title })
		if i <= at {
			t.Fatalf("cheat sheet group %s at %d, want it after the previous one (%d) in %v", w.title, i, at, groups)
		}
		at = i
		var labels []string
		for _, k := range groups[i].Keys {
			labels = append(labels, k.Label)
		}
		rest := labels
		for _, l := range w.labels {
			j := slices.Index(rest, l)
			if j < 0 {
				t.Fatalf("cheat sheet group %s = %q, want %q in this order", w.title, labels, w.labels)
			}
			rest = rest[j+1:]
		}
	}
	if k := a.cheatsheetGroups()[0].Keys[1]; !slices.Equal(k.Combos, []cheatsheet.Combo{{Key: ui.KeyLeft}, {Key: ui.KeyRight}}) {
		t.Fatalf("后退 / 前进 5 秒 runs %v, want ← and →", k.Combos)
	}
}

// TestCheatsheetPlacesOtherAreasRows checks the layout against a fixed table, so rows another area adds land where the prototype's sheet puts them and cannot change this test: the 30-second skips join under 播放, the subtitle keys and ⇧⌘T form 字幕 right after 播放, a row the layout does not name (⌘=) joins its menu's group, and a group the layout does not know follows the layout's groups.
func TestCheatsheetPlacesOtherAreasRows(t *testing.T) {
	rows := []shortcut{
		{Group: "播放", Label: "播放/暂停", Key: ui.KeySpace},
		{Group: "播放", Label: "后退 5 秒", Key: ui.KeyLeft},
		{Group: "播放", Label: "前进 5 秒", Key: ui.KeyRight},
		{Group: "播放", Label: "后退 30 秒", Mods: ui.Cmd, Key: ui.KeyLeft},
		{Group: "播放", Label: "前进 30 秒", Mods: ui.Cmd, Key: ui.KeyRight},
		{Group: "字幕", Label: "显示/隐藏字幕", Key: ui.KeyV},
		{Group: "字幕", Label: "切换原文/译文/双语", Key: ui.KeyS},
		{Group: "字幕", Label: "字幕提前 0.1 秒", Key: ui.KeyZ},
		{Group: "字幕", Label: "字幕推后 0.1 秒", Key: ui.KeyX},
		{Group: "字幕", Label: "放大字幕", Mods: ui.Cmd, Key: ui.KeyEqual},
		{Group: "字幕", Label: "搜索字幕", Mods: ui.Cmd, Key: ui.KeyF},
		{Group: "显示", Label: "显示/隐藏侧栏", Mods: ui.Cmd | ui.Shift, Key: ui.KeyT},
		{Group: "显示", Label: "全屏", Key: ui.KeyF},
		{Group: "窗口", Label: "快捷键速查", Mods: ui.Cmd, Key: ui.KeySlash},
		{Group: "学习", Label: "复习", Key: ui.KeyR},
	}
	var got []string
	for _, g := range cheatsheetGroupsOf(rows) {
		line := g.Title + ":"
		for _, k := range g.Keys {
			line += " " + k.Label
		}
		got = append(got, line)
	}
	want := []string{
		"播放: 播放 / 暂停 后退 / 前进 5 秒 后退 / 前进 30 秒 全屏",
		"字幕: 显示 / 隐藏 切换显示方式 字幕延迟 −/+ 0.1 秒 侧栏 搜索台词 放大字幕",
		"窗口: 快捷键速查",
		"学习: 复习",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("cheat sheet groups\n%q\nwant\n%q", got, want)
	}
}

// TestCheatsheetIsDark checks that the cheat sheet is drawn in the player's dark theme over the light window, as dark glass: its tint, rgb(28, 28, 32) at 86%, over the blurred white window under the sheet's scrim, black at 25%, gives about rgb(51, 51, 54).
func TestCheatsheetIsDark(t *testing.T) {
	a, tt, _ := newTestApp(&fakePipeline{})
	a.overlay.cheatsheet = true
	tt.Frame()
	r, ok := tt.Find("快捷键")
	if !ok {
		t.Fatalf("no cheat sheet: %q", tt.Texts())
	}
	img := tt.Image()
	scale := float32(img.Bounds().Dx()) / 1100
	px := img.RGBAAt(int((r.X-6)*scale), int((r.Y+r.H/2)*scale))
	if px.R < 41 || px.R > 61 || px.G < 41 || px.G > 61 || px.B < 44 || px.B > 64 {
		t.Fatalf("cheat sheet behind its title = %v, want the dark glass, about rgb(51, 51, 54)", px)
	}
}

// TestCheatsheetWritesEscapeAsEsc checks that 返回资料库's keycap reads Esc, as the prototype writes it, and not macOS's menu glyph ⎋, which on a keycap reads like an undo arrow.
func TestCheatsheetWritesEscapeAsEsc(t *testing.T) {
	a, tt, _ := newTestApp(&fakePipeline{})
	a.overlay.cheatsheet = true
	tt.Frame()
	if !tt.HasText("返回资料库") || !tt.HasText("Esc") || tt.HasText("⎋") {
		t.Fatalf("cheat sheet does not write Escape as Esc: %q", tt.Texts())
	}
}

func TestSettingsShortcutOpensSettings(t *testing.T) {
	a, tt, _ := newTestApp(&fakePipeline{})
	a.prefs.section = settings.SectionAccount
	tt.Frame()
	tt.Key(ui.Cmd, ui.KeyComma)
	tt.Frame()
	wantSettings(t, a, settings.SectionGeneral)
	if a.page != pageFilms {
		t.Fatalf("⌘, left the library for page %q", a.page)
	}
}

// TestMenuBarTakesCommandShortcuts checks that once the app has its own menu bar, ⌘ shortcuts are left to the menu's accelerators and single keys still work.
func TestMenuBarTakesCommandShortcuts(t *testing.T) {
	a, tt, _ := newTestApp(&fakePipeline{})
	a.menu.menuBar = true
	tt.Frame()
	tt.Key(ui.Cmd, ui.KeyComma)
	tt.Frame()
	if a.prefs.open {
		t.Fatalf("⌘, ran beside the menu bar: settings open at %q", a.prefs.section)
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
