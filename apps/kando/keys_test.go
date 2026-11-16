package main

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"kando/apps/kando/cheatsheet"
	"kando/apps/kando/locales/testlocale"
	"kando/apps/kando/settings"
	"kando/packages/core"

	"github.com/nsgo-dev/nsgo/nsgotest"
)

// skipWithoutFFmpeg skips a test that needs ffmpeg when ffmpeg is missing or cannot make its clip. CI sets KANDO_REQUIRE_FFMPEG where it installs ffmpeg, and there the test fails instead, so that it cannot pass by skipping.
func skipWithoutFFmpeg(t *testing.T, format string, args ...any) {
	t.Helper()
	if os.Getenv("KANDO_REQUIRE_FFMPEG") != "" {
		t.Fatalf(format, args...)
	}
	t.Skipf(format, args...)
}

// openClip opens a film of the given seconds from the library and has the system's player load it (playerLoadFilm), so it plays, holds and seeks.
func openClip(t *testing.T, seconds int) (*app, *nsgotest.App, chan func()) {
	t.Helper()
	a, tt, updates, item := playerFilm(t, seconds, nil)
	playerOpenFilm(t, a, tt, updates, item)
	playerLoadFilm(tt, a, time.Duration(seconds)*time.Second)
	return a, tt, updates
}

func TestSpaceTogglesTheOpenFilm(t *testing.T) {
	a, tt, _ := openClip(t, 1)
	if a.Play().Playing() {
		t.Fatal("the film started before Space")
	}
	tt.Key("space")
	if !a.Play().Playing() {
		t.Fatal("Space did not start the film")
	}
	tt.Key("space")
	if a.Play().Playing() {
		t.Fatal("Space did not hold the film")
	}
}

func TestArrowKeysSkipWithinTheFilm(t *testing.T) {
	a, tt, updates := openClip(t, 12)
	tt.Key("right")
	waitUntil(t, tt, a, updates, func() bool { return a.Play() != nil })
	if head := a.PlayHead(); head < 5*time.Second || head > 7*time.Second {
		t.Fatalf("head %v after →, want about 5s", head)
	}
	tt.Key("left")
	waitUntil(t, tt, a, updates, func() bool { return a.Play() != nil })
	if head := a.PlayHead(); head > 2*time.Second {
		t.Fatalf("head %v after ←, want about the start", head)
	}
}

func TestEscapeClosesTheFilm(t *testing.T) {
	a, tt, _ := openClip(t, 1)
	tt.Key("escape")
	if a.openKey != "" || a.Play() != nil {
		t.Fatalf("open %q play %v after Escape", a.openKey, a.Play() != nil)
	}
	if !hasText(tt, testlocale.Messages().FilmsStart()) {
		t.Fatalf("library not shown: %q", tt.Texts())
	}
}

func TestPlayerKeysWaitForAFilm(t *testing.T) {
	a, tt, _ := newTestApp(t, &fakePipeline{})
	tt.Key("escape")
	tt.Key("space")
	if a.page != pageFilms || a.openKey != "" || a.Play() != nil {
		t.Fatalf("page %q open %q", a.page, a.openKey)
	}
}

func TestCheatsheetListsTheShortcutTable(t *testing.T) {
	a, tt, _ := newTestApp(t, &fakePipeline{})
	tt.Key("mod+/")
	if !a.overlay.cheatsheet {
		t.Fatal("⌘/ did not open the cheat sheet")
	}
	listed := map[string]bool{}
	for _, g := range a.cheatsheetGroups(a.cx) {
		if !hasText(tt, g.Title) {
			t.Fatalf("cheat sheet misses the group %q: %q", g.Title, tt.Texts())
		}
		for _, k := range g.Keys {
			if !hasText(tt, k.Label) {
				t.Fatalf("cheat sheet misses %q under %q: %q", k.Label, g.Title, tt.Texts())
			}
			for _, c := range k.Combos {
				listed[c] = true
			}
		}
	}
	for _, s := range a.shortcuts(a.cx) {
		if s.Key != "" && !listed[s.Key] {
			t.Fatalf("cheat sheet misses the keys of %q", s.Label)
		}
	}
	title := whole(testlocale.Messages().CheatsheetTitle())
	tt.Key("mod+/")
	if shown := tt.Exists(title); a.overlay.cheatsheet || shown {
		t.Fatalf("a second ⌘/ left the cheat sheet open: %q", tt.Texts())
	}
	tt.Key("mod+/")
	if !a.overlay.cheatsheet {
		t.Fatal("⌘/ did not open the cheat sheet again")
	}
	tt.Key("escape")
	if shown := tt.Exists(title); a.overlay.cheatsheet || shown {
		t.Fatalf("Escape left the cheat sheet open: %q", tt.Texts())
	}
}

// TestCheatsheetFollowsThePrototypeLayout checks that the cheat sheet groups the table as the prototype's sheet does, not as the menu bar: 播放 first with ← and → joined on one line and 全屏 under it, then 窗口 with ⌘/, ⌘, and Esc, and the rows the prototype's sheet does not place after them, under their menus' groups, ⌘1 and ⌘2 under 窗口 and 文件's in a group of its own.
func TestCheatsheetFollowsThePrototypeLayout(t *testing.T) {
	a, _, _ := newTestApp(t, &fakePipeline{})
	// Other areas add rows to these groups and groups of their own, so this checks that these lines keep their groups and order, not that the sheet has nothing else.
	want := []struct {
		title  string
		labels []string
	}{
		{"播放", []string{"播放 / 暂停", "后退 / 前进 5 秒", "全屏"}},
		{"窗口", []string{"快捷键速查", "设置", "返回资料库", testlocale.Messages().MenuWindowLibrary(), "播放器"}},
		{"文件", []string{"打开本地视频…", "新建资料库…", "打开资料库…"}},
	}
	groups := a.cheatsheetGroups(a.cx)
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
	if k := a.cheatsheetGroups(a.cx)[0].Keys[1]; !slices.Equal(k.Combos, []string{"left", "right"}) {
		t.Fatalf("后退 / 前进 5 秒 runs %v, want ← and →", k.Combos)
	}
}

// TestCheatsheetPlacesOtherAreasRows checks the layout against a fixed table, so rows another area adds land where the prototype's sheet puts them and cannot change this test: the 30-second skips join under 播放, the subtitle keys and ⇧⌘T form 字幕 right after 播放, a row the layout does not name (⌘=) joins its menu's group, and a group the layout does not know follows the layout's groups.
func TestCheatsheetPlacesOtherAreasRows(t *testing.T) {
	a, _, _ := newTestApp(t, &fakePipeline{})
	rows := []shortcut{
		{Group: groupPlayback, Label: "播放/暂停", Key: "space"},
		{Group: groupPlayback, Label: "后退 5 秒", Key: "left"},
		{Group: groupPlayback, Label: "前进 5 秒", Key: "right"},
		{Group: groupPlayback, Label: "后退 30 秒", Key: "mod+left"},
		{Group: groupPlayback, Label: "前进 30 秒", Key: "mod+right"},
		{Group: groupSubtitles, Label: "显示/隐藏字幕", Key: "v"},
		{Group: groupSubtitles, Label: "切换原文/译文/双语", Key: "s"},
		{Group: groupSubtitles, Label: "字幕提前 0.1 秒", Key: "z"},
		{Group: groupSubtitles, Label: "字幕推后 0.1 秒", Key: "x"},
		{Group: groupSubtitles, Label: "放大字幕", Key: "mod+="},
		{Group: groupSubtitles, Label: "搜索字幕", Key: "mod+f"},
		{Group: groupView, Label: "显示/隐藏侧栏", Key: "mod+shift+t"},
		{Group: groupView, Label: "全屏", Key: "f"},
		{Group: groupWindow, Label: "快捷键速查", Key: "mod+/"},
		{Group: groupStudy, Label: "复习", Key: "n"},
	}
	var got []string
	for _, g := range cheatsheetGroupsOf(a.cx, rows) {
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

// TestCheatsheetIsDark checks that the cheat sheet is drawn in the player's dark theme over the light window, as dark glass: light words over its tint, rgb(28, 28, 32) at 86%.
func TestCheatsheetIsDark(t *testing.T) {
	a, tt, _ := newTestApp(t, &fakePipeline{})
	a.overlay.cheatsheet = true
	refresh(tt, a)
	title := tt.Find(whole(testlocale.Messages().CheatsheetTitle()))
	if r, g, b, _ := title.TextStyle.Color.Components(); r < 0.9 || g < 0.9 || b < 0.9 {
		t.Fatalf("the cheat sheet's title is %#x, want the dark theme's light words", uint32(title.TextStyle.Color))
	}
	if r, g, b, alpha := backgroundUnder(title).Components(); r > 0.2 || g > 0.2 || b > 0.2 || alpha < 0.8 {
		t.Fatalf("cheat sheet behind its title = %#x, want the dark glass", uint32(backgroundUnder(title)))
	}
	// Headless runs have no pixels (nsgotest's Capture skips them), so the glass itself is checked on a platform only, under nsgo test -target: over the light window blurred under the sheet's scrim it is about rgb(51, 51, 54), as MyGo's Tester.Image read it. The point is in the sheet's left padding, halfway down, where no words are.
	if nsgotest.Target() == "" {
		return
	}
	sheet := title
	for sheet.Style.Background == 0 {
		sheet = sheet.Parent
	}
	box := pageFrame(sheet)
	got := capturedColor(tt.Capture(), 1100, box.Origin.X+8, box.Origin.Y+box.Size.Height/2)
	for i, want := range [3]int{51, 51, 54} {
		if d := int(got[i]) - want; d < -16 || d > 16 {
			t.Fatalf("the cheat sheet's glass is rgb%v in its left padding, want about rgb(51, 51, 54)", got)
		}
	}
}

// TestCheatsheetWritesEscapeAsEsc checks that 返回资料库's keycap reads Esc, as the prototype writes it, and not macOS's menu glyph ⎋, which on a keycap reads like an undo arrow.
func TestCheatsheetWritesEscapeAsEsc(t *testing.T) {
	a, tt, _ := newTestApp(t, &fakePipeline{})
	a.overlay.cheatsheet = true
	refresh(tt, a)
	if !hasText(tt, "返回资料库") || !hasText(tt, "Esc") || hasText(tt, "⎋") {
		t.Fatalf("cheat sheet does not write Escape as Esc: %q", tt.Texts())
	}
}

func TestSettingsShortcutOpensSettings(t *testing.T) {
	a, tt, _ := newTestApp(t, &fakePipeline{})
	a.prefs.section = settings.SectionAccount
	refresh(tt, a)
	tt.Key("mod+,")
	wantSettings(t, a, settings.SectionGeneral)
	if a.page != pageFilms {
		t.Fatalf("⌘, left the library for page %q", a.page)
	}
}

// TestMenuBarTakesCommandShortcuts checks that once the app has its own menu bar, ⌘ shortcuts are left to the menu's accelerators and single keys still work.
func TestMenuBarTakesCommandShortcuts(t *testing.T) {
	a, tt, _ := newTestApp(t, &fakePipeline{})
	a.menu.menuBar = true
	refresh(tt, a)
	tt.Key("mod+,")
	if a.prefs.open {
		t.Fatalf("⌘, ran beside the menu bar: settings open at %q", a.prefs.section)
	}
}

// TestShortcutsAreUnique checks that no two rows of the table have one label in one menu, and that rows share a key only when they take turns: each has a When, and in none of the window's states (a library page with films, a film open over it) do two of them hold at once, so the key runs one row at most, as the library's ⌘F and the player's do.
func TestShortcutsAreUnique(t *testing.T) {
	a, _, _ := libraryWith(t, "Talk")
	ids := map[string]bool{}
	keyed := map[string][]shortcut{}
	for _, s := range a.shortcuts(a.cx) {
		if s.Group == "" || s.Label == "" || s.Run == nil {
			t.Fatalf("incomplete shortcut %+v", s)
		}
		if ids[menuRowID(s)] {
			t.Fatalf("two rows %q in %q", s.Label, s.Group)
		}
		ids[menuRowID(s)] = true
		if s.Key != "" {
			keyed[s.Key] = append(keyed[s.Key], s)
		}
	}
	states := map[string]func(){
		"library": func() { a.openKey = "" },
		"film":    func() { a.openKey = "film" },
	}
	for _, same := range keyed {
		if len(same) < 2 {
			continue
		}
		for _, s := range same {
			if s.When == nil {
				t.Fatalf("%q shares its key with %q but always holds", s.Label, same[0].Label)
			}
		}
		for state, enter := range states {
			enter()
			var holding []string
			for _, s := range same {
				if s.When() {
					holding = append(holding, s.Label)
				}
			}
			if len(holding) > 1 {
				t.Fatalf("%q share a key and all hold on the %s page", holding, state)
			}
		}
	}
}

func TestDroppedVideoIsQueued(t *testing.T) {
	a, _, _ := newTestApp(t, &fakePipeline{})
	dir := t.TempDir()
	note := filepath.Join(dir, "a.txt")
	video := filepath.Join(dir, "b.mp4")
	for _, p := range []string{note, video} {
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	a.dropFiles([]string{note, video})
	if len(a.library.Jobs) != 1 || a.library.Jobs[0].Source != core.FileURL(video) {
		t.Fatalf("jobs %#v", a.library.Jobs)
	}
}

// TestPlayerKeysAfterTheFilmLeaves checks that a player key pressed in the frame the open film drops out of the library does nothing. wireKeys registers the key before the page closes the film in the same build, so the key must check again when it runs.
func TestPlayerKeysAfterTheFilmLeaves(t *testing.T) {
	for _, key := range []string{"space", "left", "right"} {
		a, tt, _ := openClip(t, 2)
		a.library.Jobs = nil
		tt.Key(key)
		if a.openKey != "" || a.Play() != nil {
			t.Fatalf("key %s: open %q play %v after the film left", key, a.openKey, a.Play() != nil)
		}
	}
}

// TestModalDialogsNest checks the window's modal flag: it stays up while any dialog holds it, as a file panel opened from a sheet does, and a release that runs twice counts once.
func TestModalDialogsNest(t *testing.T) {
	a, _, _ := newTestApp(t, &fakePipeline{})
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

// TestCheatsheetInEnglish checks the cheat sheet in English: its title and hint, and the prototype's lines and group titles.
func TestCheatsheetInEnglish(t *testing.T) {
	testlocale.Use(t, "en")
	a, tt, _ := newTestApp(t, &fakePipeline{})
	tt.Key("mod+/")
	if !a.overlay.cheatsheet {
		t.Fatal("⌘/ did not open the cheat sheet")
	}
	for _, want := range []string{"Keyboard Shortcuts", "Single keys work only while no text field has focus", "Playback", "Play / Pause", "Back / Forward 5 Seconds", "Window", "Ask This Film / Commands", "Settings", "Shortcut Cheat Sheet"} {
		wantText(t, tt, want)
	}
	if hasText(tt, "快捷键") || hasText(tt, "播放 / 暂停") {
		t.Fatalf("the cheat sheet still reads Chinese: %q", tt.Texts())
	}
}
