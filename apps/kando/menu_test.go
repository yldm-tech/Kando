package main

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"kando/apps/kando/films"
	"kando/apps/kando/locales/testlocale"
	"kando/apps/kando/settings"

	ns "github.com/nsgo-dev/nsgo/core"
	nsdesktop "github.com/nsgo-dev/nsgo/desktop"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

// findMenuItem finds the item labeled label in the menu titled menu of the menu bar bar.
func findMenuItem(t *testing.T, bar []nsdesktop.MenuItem, menu, label string) nsdesktop.MenuItem {
	t.Helper()
	for _, m := range bar {
		if m.Label != menu {
			continue
		}
		for _, it := range m.Items {
			if it.Label == label {
				return it
			}
		}
		t.Fatalf("menu %q has no %q", menu, label)
	}
	t.Fatalf("no menu %q", menu)
	return nsdesktop.MenuItem{}
}

// choose chooses it as the menu bar does, on the engine's goroutine, which is the test's, and refreshes.
func choose(tt *nsgotest.App, a *app, it nsdesktop.MenuItem) {
	it.OnSelect()
	refresh(tt, a)
}

// useMenuBar gives the test's app its own menu bar, as run does on macOS, whatever system the tests run on, until t ends.
func useMenuBar(t *testing.T, a *app, tt *nsgotest.App) {
	t.Helper()
	prev := appMenuBar
	appMenuBar = true
	t.Cleanup(func() { appMenuBar = prev })
	a.installMenu(a.cx)
	refresh(tt, a)
}

// TestMenuBarListsTheShortcutTable checks that every row of the shortcut table is an item of the menu its group names, and that every ⌘ row carries its key as the item's shortcut, since wireKeys leaves those to the menu bar once it is installed, but for a row whose key another row carries now (menuSharedKeyCarriers). Single keys carry none, so a focused text field keeps them.
func TestMenuBarListsTheShortcutTable(t *testing.T) {
	a, _, _ := newTestApp(t, &fakePipeline{})
	bar := a.menuBar(a.cx)
	table := a.shortcuts(a.cx)
	carrier := menuSharedKeyCarriers(table)
	for _, s := range table {
		it := findMenuItem(t, bar, groupTitle(a.cx, s.Group), s.Label)
		if id, shared := carrier[s.Key]; shared && id != menuRowID(s) {
			if it.Shortcut != "" {
				t.Fatalf("%q has shortcut %q, which %q carries", s.Label, it.Shortcut, id)
			}
			continue
		}
		if hasCommand(s.Key) {
			if it.Shortcut != s.Key {
				t.Fatalf("%q has shortcut %q, want %q", s.Label, it.Shortcut, s.Key)
			}
		} else if it.Shortcut != "" {
			t.Fatalf("single key %q has shortcut %q", s.Label, it.Shortcut)
		}
	}
	var titles []string
	for _, m := range bar {
		titles = append(titles, m.Label)
	}
	order := make([]string, len(menuOrder))
	for i, group := range menuOrder {
		order[i] = groupTitle(a.cx, group)
	}
	for i := 1; i < len(titles); i++ {
		if slices.Index(order, titles[i-1]) >= slices.Index(order, titles[i]) {
			t.Fatalf("menus out of order: %q", titles)
		}
	}
	for _, want := range []string{"看懂", "文件", "编辑", "窗口", "帮助"} {
		if !slices.Contains(titles, want) {
			t.Fatalf("no %q menu: %q", want, titles)
		}
	}
}

// TestMenuGroupsTheTableInMenuOrder checks that a group the menu bar does not name gets a menu before 窗口.
func TestMenuGroupsTheTableInMenuOrder(t *testing.T) {
	order, rows := grouped([]shortcut{{Group: groupHelp}, {Group: "工具"}, {Group: groupPlayback}, {Group: "工具"}})
	if at, win := slices.Index(order, "工具"), slices.Index(order, groupWindow); at < 0 || at != win-1 || slices.Index(order, groupView) > at {
		t.Fatalf("order %q", order)
	}
	if len(rows["工具"]) != 2 {
		t.Fatalf("rows %v", rows)
	}
}

// TestHasCommand checks which keys the menu bar carries as its items' shortcuts: those held with ⌘ (mod), whatever else is held, and no single key or ⇧ key.
func TestHasCommand(t *testing.T) {
	for key, want := range map[string]bool{
		"mod+shift+n":  true,
		"mod+/":        true,
		"mod+1":        true,
		"mod+alt+left": true,
		"mod+,":        true,
		"shift+.":      false,
		"space":        false,
		"f5":           false,
		"m":            false,
	} {
		if got := hasCommand(key); got != want {
			t.Errorf("hasCommand(%q) = %v, want %v", key, got, want)
		}
	}
}

// TestMenuItemRunsItsShortcut checks that choosing an item runs its row, on the engine's goroutine, where the window follows what it changed.
func TestMenuItemRunsItsShortcut(t *testing.T) {
	a, tt, _ := newTestApp(t, &fakePipeline{})
	choose(tt, a, findMenuItem(t, a.menuBar(a.cx), "看懂", "设置…"))
	wantSettings(t, a, settings.SectionGeneral)
}

// TestMenuFollowsTheFilm checks that the player's items are enabled only while a film is open, and that 窗口 marks the screen the window shows.
func TestMenuFollowsTheFilm(t *testing.T) {
	a, _, _ := newTestApp(t, &fakePipeline{})
	bar := a.menuBar(a.cx)
	if !findMenuItem(t, bar, "显示", "全屏").Disabled || !findMenuItem(t, bar, "窗口", "播放器").Disabled {
		t.Fatal("player items enabled without a film")
	}
	if library := findMenuItem(t, bar, "窗口", "片库"); !library.Checkable || !library.Checked {
		t.Fatalf("片库 is not checked in the library: %+v", library)
	}

	a, _, _ = openClip(t, 2)
	bar = a.menuBar(a.cx)
	for _, label := range []string{"播放/暂停", "后退 5 秒", "前进 5 秒"} {
		if findMenuItem(t, bar, "播放", label).Disabled {
			t.Fatalf("%q disabled with the film open", label)
		}
	}
	if player := findMenuItem(t, bar, "窗口", "播放器"); player.Disabled || !player.Checked {
		t.Fatalf("播放器 with the film open: %+v", player)
	}
	if findMenuItem(t, bar, "窗口", "片库").Checked {
		t.Fatal("片库 checked with the film open")
	}
}

// TestMenuWaitsBehindTheCheatsheet checks that while the cheat sheet is open the menu bar's commands wait, as the window's shortcuts do, except the cheat sheet's own, which closes it.
func TestMenuWaitsBehindTheCheatsheet(t *testing.T) {
	a, tt, _ := newTestApp(t, &fakePipeline{})
	settings := findMenuItem(t, a.menuBar(a.cx), "看懂", "设置…")
	a.toggleCheatsheet()
	refresh(tt, a)
	bar := a.menuBar(a.cx)
	if !findMenuItem(t, bar, "看懂", "设置…").Disabled || !findMenuItem(t, bar, "文件", testlocale.Messages().MenuRevealLibrary(platformName())).Disabled {
		t.Fatal("commands enabled behind the cheat sheet")
	}
	// An item of a menu built before the sheet opened.
	choose(tt, a, settings)
	if a.prefs.open {
		t.Fatalf("设置… ran behind the cheat sheet: settings open at %q", a.prefs.section)
	}
	sheet := findMenuItem(t, bar, "帮助", "快捷键速查")
	if sheet.Disabled {
		t.Fatal("快捷键速查 disabled while the cheat sheet is open")
	}
	choose(tt, a, sheet)
	if a.overlay.cheatsheet {
		t.Fatal("快捷键速查 did not close the cheat sheet")
	}
}

// TestMenuKeepsTheDefaultCommands checks that the menu bar keeps what the platform's default one has beyond the table: full screen on every page, not only with a film open, which the system gives its key (⌃⌘F on macOS), and the Edit menu's commands for text.
func TestMenuKeepsTheDefaultCommands(t *testing.T) {
	a, _, _ := newTestApp(t, &fakePipeline{})
	roles := map[string]map[nsdesktop.MenuRole]nsdesktop.MenuItem{}
	for _, m := range a.menuBar(a.cx) {
		roles[m.Label] = map[nsdesktop.MenuRole]nsdesktop.MenuItem{}
		for _, it := range m.Items {
			if it.Role != 0 {
				roles[m.Label][it.Role] = it
			}
		}
	}
	if full, ok := roles["显示"][nsdesktop.MenuToggleFullScreen]; !ok || full.Disabled {
		t.Fatalf("显示 has no enabled full screen item without a film: %+v", roles["显示"])
	}
	// TODO(nsgo#118): check 粘贴并匹配样式 and the 语音 items here again once NSGo has their roles (menu.go) https://github.com/nsgo-dev/nsgo/issues/118
	for _, role := range []nsdesktop.MenuRole{nsdesktop.MenuUndo, nsdesktop.MenuRedo, nsdesktop.MenuCut, nsdesktop.MenuCopy, nsdesktop.MenuPaste, nsdesktop.MenuSelectAll} {
		if _, ok := roles["编辑"][role]; !ok {
			t.Fatalf("编辑 has no item of role %v", role)
		}
	}
}

// TestMenuWaitsBehindAFilePanel checks that while a file panel is up over the window, every menu bar command waits, the cheat sheet's too, and that a click from a menu built before the panel came up does nothing.
func TestMenuWaitsBehindAFilePanel(t *testing.T) {
	a, tt, _ := newTestApp(t, &fakePipeline{})
	library := findMenuItem(t, a.menuBar(a.cx), "文件", "新建资料库…")
	settings := findMenuItem(t, a.menuBar(a.cx), "看懂", "设置…")
	sheet := findMenuItem(t, a.menuBar(a.cx), "帮助", "快捷键速查")
	release := a.holdModal()
	refresh(tt, a)
	bar := a.menuBar(a.cx)
	for _, it := range []struct{ menu, label string }{{"文件", "打开资料库…"}, {"文件", "新建资料库…"}, {"文件", "打开本地视频…"}, {"文件", testlocale.Messages().MenuRevealLibrary(platformName())}, {"看懂", "设置…"}, {"窗口", "片库"}, {"帮助", "快捷键速查"}} {
		if !findMenuItem(t, bar, it.menu, it.label).Disabled {
			t.Fatalf("%s › %s enabled behind a file panel", it.menu, it.label)
		}
	}
	choose(tt, a, library)
	if a.snap().MakingLibrary {
		t.Fatal("新建资料库… ran behind a file panel")
	}
	choose(tt, a, settings)
	if a.prefs.open {
		t.Fatalf("设置… ran behind a file panel: settings open at %q", a.prefs.section)
	}
	choose(tt, a, sheet)
	if a.overlay.cheatsheet {
		t.Fatal("快捷键速查 opened the cheat sheet behind a file panel")
	}
	// Once the panel has closed, the commands run again.
	release()
	refresh(tt, a)
	if findMenuItem(t, a.menuBar(a.cx), "看懂", "设置…").Disabled {
		t.Fatal("设置… still disabled after the file panel closed")
	}
}

// TestSyncMenuInstallsOnlyChanges checks that the installed menu bar is replaced when what it shows changes, and left alone by changes that alter none of it, and that choosing a checkable item installs it again with the app's ✓.
func TestSyncMenuInstallsOnlyChanges(t *testing.T) {
	a, tt, _ := newTestApp(t, &fakePipeline{})
	useMenuBar(t, a, tt)
	first := a.menu.installed
	if first == nil || a.menu.menuSig == "" || len(tt.Menu) == 0 {
		t.Fatal("no menu bar installed")
	}
	refresh(tt, a)
	if a.menu.installed != first {
		t.Fatal("a change that altered nothing installed a new menu bar")
	}
	a.toggleCheatsheet()
	refresh(tt, a)
	second := a.menu.installed
	if second == first {
		t.Fatal("opening the cheat sheet left the menu bar as it was")
	}
	a.toggleCheatsheet()
	refresh(tt, a)
	third := a.menu.installed
	tt.ChooseMenu("窗口", "片库")
	refresh(tt, a)
	if a.menu.installed == third {
		t.Fatal("choosing a checkable item left the menu bar as it was installed")
	}
	if a.page != pageFilms || a.openKey != "" {
		t.Fatalf("片库 showed page %q open %q", a.page, a.openKey)
	}
}

// TestWindowKeysGoBackToTheLastFilm checks ⌘1 and ⌘2: the library, and back to the film the window showed last, which going to the library does not forget.
func TestWindowKeysGoBackToTheLastFilm(t *testing.T) {
	a, tt, updates := openClip(t, 2)
	tt.Key("mod+1")
	if a.openKey != "" || a.Play() != nil || a.page != pageFilms {
		t.Fatalf("⌘1: open %q play %v page %q", a.openKey, a.Play() != nil, a.page)
	}
	tt.Key("mod+2")
	if a.openKey != "talk" {
		t.Fatalf("⌘2 opened %q", a.openKey)
	}
	waitUntil(t, tt, a, updates, func() bool { return a.Play() != nil })

	// With the settings open too.
	tt.Key("mod+,")
	wantSettings(t, a, settings.SectionGeneral)
	tt.Key("mod+2")
	if a.page != pageFilms || a.openKey != "talk" {
		t.Fatalf("⌘2 from settings: page %q open %q", a.page, a.openKey)
	}

	// Once the film has left the library there is nothing to go back to.
	tt.Key("mod+1")
	a.library.Jobs = nil
	refresh(tt, a)
	tt.Key("mod+2")
	if a.openKey != "" {
		t.Fatalf("⌘2 opened %q after the film left", a.openKey)
	}
	if !findMenuItem(t, a.menuBar(a.cx), "窗口", "播放器").Disabled {
		t.Fatal("播放器 enabled after the film left")
	}
}

// TestCheatsheetCloseButton checks the cheat sheet's × button.
func TestCheatsheetCloseButton(t *testing.T) {
	a, tt, _ := newTestApp(t, &fakePipeline{})
	a.toggleCheatsheet()
	refresh(tt, a)
	tt.Press(testlocale.Messages().CheatsheetClose())
	if a.overlay.cheatsheet {
		t.Fatal("× left the cheat sheet open")
	}
}

// TestMenuBarInEnglish checks the menu bar in English, in Apple's wording for the standard items.
func TestMenuBarInEnglish(t *testing.T) {
	testlocale.Use(t, "en")
	a, _, _ := newTestApp(t, &fakePipeline{})
	bar := a.menuBar(a.cx)
	// TODO(nsgo#118): check Edit › Paste and Match Style and Window › Bring All to Front here again once NSGo has their roles (menu.go) https://github.com/nsgo-dev/nsgo/issues/118
	for _, want := range [][2]string{
		{"Kando", "About Kando"}, {"Kando", "Settings…"}, {"Kando", "Hide Kando"}, {"Kando", "Quit Kando"},
		{"File", "New Library…"}, {"File", "Open Library…"}, {"File", "Switch Library"}, {"File", testlocale.Messages().MenuRevealLibrary(platformName())}, {"File", "Close Window"},
		{"Edit", "Undo"}, {"Edit", "Copy"},
		{"View", "Toggle Full Screen"},
		{"Study", "Ask This Film / Commands…"},
		{"Window", "Minimize"}, {"Window", "Library"}, {"Window", "Player"},
		{"Help", "Shortcut Cheat Sheet"}, {"Help", "Kando Website"}, {"Help", "Release Notes"},
	} {
		findMenuItem(t, bar, want[0], want[1])
	}
}

// TestMenuNamesTheSystemsFileManager checks that the File menu's reveal names the file manager of the system it runs on.
func TestMenuNamesTheSystemsFileManager(t *testing.T) {
	for _, c := range []struct {
		name string
		os   ns.OS
		want string
	}{{"macOS", ns.OSMacOS, "在访达中显示资料库"}, {"Windows", ns.OSWindows, "在文件资源管理器中显示资料库"}, {"Linux", ns.OSLinux, "在文件管理器中显示资料库"}} {
		t.Run(c.name, func(t *testing.T) {
			usePlatform(t, c.os)
			a, _, _ := newTestApp(t, &fakePipeline{})
			findMenuItem(t, a.menuBar(a.cx), "文件", c.want)
		})
	}
}

// twoLibraries opens the first of two libraries, 甲 and 乙, in a test app that saves its settings to a file of its own.
func twoLibraries(t *testing.T) (*app, *nsgotest.App, chan func(), string, string) {
	t.Helper()
	parent := t.TempDir()
	first, second := filepath.Join(parent, "甲.kando"), filepath.Join(parent, "乙.kando")
	for _, dir := range []string{first, second} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	a, tt, updates := newTestApp(t, &fakePipeline{})
	a.settingsPath = filepath.Join(t.TempDir(), "settings.json")
	a.edit(func(s *session) {
		s.LibraryDir = first
		s.Libraries = []string{first, second}
	})
	a.refreshLibrary()
	refresh(tt, a)
	return a, tt, updates, first, second
}

// libraryChoices writes down the items of a library list as label, then * for the chosen one and - for one that is disabled.
func libraryChoices(t *testing.T, items []nsdesktop.MenuItem) []string {
	t.Helper()
	var out []string
	for _, it := range items {
		if !it.Checkable {
			t.Fatalf("%q is not an item that can be chosen", it.Label)
		}
		label := it.Label
		if it.Checked {
			label += "*"
		}
		if it.Disabled {
			label += "-"
		}
		out = append(out, label)
	}
	return out
}

// TestMenuSwitchesTheLibrary checks 文件 › 切换资料库: it lists the known libraries with the open one chosen, choosing another opens it on the library page, and the menu bar is installed again with the new one chosen.
func TestMenuSwitchesTheLibrary(t *testing.T) {
	a, tt, _, _, second := twoLibraries(t)
	useMenuBar(t, a, tt)
	a.Select(pageYouTube)
	refresh(tt, a)
	sw := findMenuItem(t, a.menuBar(a.cx), "文件", "切换资料库")
	if got, want := libraryChoices(t, sw.Items), []string{"甲*", "乙"}; !slices.Equal(got, want) {
		t.Fatalf("切换资料库 lists %q, want %q", got, want)
	}
	installed := a.menu.installed
	choose(tt, a, sw.Items[1])
	if a.snap().LibraryDir != second || a.page != pageFilms {
		t.Fatalf("after choosing 乙: library %q page %q", a.snap().LibraryDir, a.page)
	}
	if a.menu.installed == installed {
		t.Fatal("the menu bar was not installed again after switching")
	}
	sw = findMenuItem(t, a.menuBar(a.cx), "文件", "切换资料库")
	if got, want := libraryChoices(t, sw.Items), []string{"甲", "乙*"}; !slices.Equal(got, want) {
		t.Fatalf("after switching 切换资料库 lists %q, want %q", got, want)
	}
}

// TestMenuSwitchLibraryWaitsBehindAFilePanel checks that the library items wait behind a file panel, and that a click from a menu built before it came up leaves the open library, and the one the menu bar shows chosen, as they were.
func TestMenuSwitchLibraryWaitsBehindAFilePanel(t *testing.T) {
	a, tt, _, first, _ := twoLibraries(t)
	useMenuBar(t, a, tt)
	early := findMenuItem(t, a.menuBar(a.cx), "文件", "切换资料库").Items[1]
	release := a.holdModal()
	t.Cleanup(release)
	refresh(tt, a)
	if got, want := libraryChoices(t, findMenuItem(t, a.menuBar(a.cx), "文件", "切换资料库").Items), []string{"甲*-", "乙-"}; !slices.Equal(got, want) {
		t.Fatalf("behind a file panel 切换资料库 lists %q, want %q", got, want)
	}
	choose(tt, a, early)
	if a.snap().LibraryDir != first {
		t.Fatalf("乙 opened behind a file panel: library %q", a.snap().LibraryDir)
	}
	// What the system shows is the menu installed, not the one menuBar would build now: it keeps the open library's ✓ and both held back.
	var installed []string
	for _, m := range tt.Menu {
		for _, it := range m.Items {
			if m.Label != "文件" || it.Label != "切换资料库" {
				continue
			}
			for _, lib := range it.Items {
				label := lib.Label
				if lib.Checked {
					label += "*"
				}
				if lib.Disabled {
					label += "-"
				}
				installed = append(installed, label)
			}
		}
	}
	if want := []string{"甲*-", "乙-"}; !slices.Equal(installed, want) {
		t.Fatalf("after the held-back click the installed 切换资料库 lists %q, want %q", installed, want)
	}
}

// TestMenuSwitchLibraryNamesTwinsByPath checks that two libraries with the same name are told apart by their paths, as Settings › 通用 does.
func TestMenuSwitchLibraryNamesTwinsByPath(t *testing.T) {
	first, second := filepath.Join(t.TempDir(), "影片.kando"), filepath.Join(t.TempDir(), "影片.kando")
	a, tt, _ := newTestApp(t, &fakePipeline{})
	a.edit(func(s *session) {
		s.LibraryDir = first
		s.Libraries = []string{first, second}
	})
	refresh(tt, a)
	got := libraryChoices(t, findMenuItem(t, a.menuBar(a.cx), "文件", "切换资料库").Items)
	if want := []string{films.DisplayPath(testlocale.Messages(), first) + "*", films.DisplayPath(testlocale.Messages(), second)}; !slices.Equal(got, want) {
		t.Fatalf("切换资料库 lists %q, want %q", got, want)
	}
}

// TestMenuWithoutLibraries checks that with no library known, 文件 has no 切换资料库.
func TestMenuWithoutLibraries(t *testing.T) {
	a, tt, _ := newTestApp(t, &fakePipeline{})
	a.edit(func(s *session) { s.LibraryDir, s.Libraries = "", nil })
	refresh(tt, a)
	// TODO(nsgo): check again that the Dock menu lists the libraries, switches them and follows a switch made elsewhere, and that it goes without a library, as TestDockMenuListsTheLibraries did, once NSGo has a Dock menu (menu.go's syncMenu)
	for _, m := range a.menuBar(a.cx) {
		for _, it := range m.Items {
			if it.Label == "切换资料库" {
				t.Fatalf("%s has 切换资料库 without a library", m.Label)
			}
		}
	}
}

// TestHelpMenuOpensTheLinks checks 帮助 › 看懂官网 and 更新说明: they open the website and the running version's release in the browser, are items of the menu bar and the palette but not of the cheat sheet, and say in the message bar when the browser could not be opened.
func TestHelpMenuOpensTheLinks(t *testing.T) {
	keep := appVersion
	appVersion = "1.2.3"
	t.Cleanup(func() { appVersion = keep })

	a, tt, _ := newTestApp(t, &fakePipeline{})
	choose(tt, a, findMenuItem(t, a.menuBar(a.cx), "帮助", "看懂官网"))
	choose(tt, a, findMenuItem(t, a.menuBar(a.cx), "帮助", "更新说明"))
	if want := []string{"https://kando.yldm.ai", "https://github.com/yldm-tech/Kando/releases/tag/v1.2.3"}; !slices.Equal(tt.OpenedURLs, want) {
		t.Fatalf("opened %q, want %q", tt.OpenedURLs, want)
	}
	var palette []string
	for _, cmd := range a.paletteCommands(a.cx) {
		palette = append(palette, cmd.label)
	}
	for _, label := range []string{"看懂官网", "更新说明"} {
		if !slices.Contains(palette, label) {
			t.Fatalf("the palette has no %q: %q", label, palette)
		}
	}
	for _, s := range a.shortcuts(a.cx) {
		if (s.Label == "看懂官网" || s.Label == "更新说明") && s.Key != "" {
			t.Fatalf("%q has a key, so the cheat sheet lists it", s.Label)
		}
	}

	tt.OpenURL = func(string) bool { return false }
	choose(tt, a, findMenuItem(t, a.menuBar(a.cx), "帮助", "看懂官网"))
	if want := testlocale.Messages().MenuOpenLinkFailed("https://kando.yldm.ai"); a.lib.notice.Text != want || a.lib.notice.Kind != "warning" {
		t.Fatalf("notice %+v, want warning %q", a.lib.notice, want)
	}
	if a.player.film.osd != "" {
		t.Fatalf("on the library page the failure was also put on the picture: %q", a.player.film.osd)
	}
	// The message bar shows only on the library page, so while a film is open the failure is said on the picture too.
	a.openKey = "j"
	a.menuOpenLink(menuReleaseNotesURL())
	tt.Settle()
	if want := testlocale.Messages().MenuOpenLinkFailed("https://github.com/yldm-tech/Kando/releases/tag/v1.2.3"); a.player.film.osd != want {
		t.Fatalf("with a film open the picture says %q, want %q", a.player.film.osd, want)
	}
}
