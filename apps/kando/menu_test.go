package main

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"testing"

	"kando/apps/kando/films"
	"kando/apps/kando/locales"
	"kando/apps/kando/locales/testlocale"
	"kando/apps/kando/settings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	breeze "github.com/yldm-tech/mygo-breeze-ui/ui/breeze"
)

// findMenuItem finds the item labeled label in the menu titled menu of the menu bar bar.
func findMenuItem(t *testing.T, bar []*mygo.MenuItem, menu, label string) *mygo.MenuItem {
	t.Helper()
	for _, m := range bar {
		if m.Label != menu {
			continue
		}
		for _, it := range m.Submenu {
			if it.Label == label {
				return it
			}
		}
		t.Fatalf("menu %q has no %q", menu, label)
	}
	t.Fatalf("no menu %q", menu)
	return nil
}

// choose clicks it as the menu bar would and runs what the click hands to the UI thread.
func choose(t *testing.T, it *mygo.MenuItem, updates chan func()) {
	t.Helper()
	it.Click(it, nil)
	waitUpdate(t, updates)
}

// TestMenuBarListsTheShortcutTable checks that every row of the shortcut table is an item of the menu its group names, and that every ⌘ row carries its shortcut as an accelerator, since wireKeys leaves those to the menu bar once it is installed, but for a row whose key another row carries now (menuSharedKeyCarriers). Single keys carry none, so a focused text field keeps them.
func TestMenuBarListsTheShortcutTable(t *testing.T) {
	a, tt, _ := newTestApp(&fakePipeline{})
	tt.Frame()
	bar := a.menuBar()
	carrier := menuSharedKeyCarriers(a.shortcuts())
	for _, s := range a.shortcuts() {
		it := findMenuItem(t, bar, groupTitle(s.Group), s.Label)
		if id, shared := carrier[[2]int{int(s.Mods), int(s.Key)}]; shared && id != menuRowID(s) {
			if it.Accelerator != "" {
				t.Fatalf("%q has accelerator %q, which %q carries", s.Label, it.Accelerator, id)
			}
			continue
		}
		if s.Mods&ui.Cmd != 0 {
			if it.Accelerator == "" || it.Accelerator != accelerator(s.Mods, s.Key) {
				t.Fatalf("%q has accelerator %q", s.Label, it.Accelerator)
			}
		} else if it.Accelerator != "" {
			t.Fatalf("single key %q has accelerator %q", s.Label, it.Accelerator)
		}
	}
	var titles []string
	for _, m := range bar {
		titles = append(titles, m.Label)
	}
	order := make([]string, len(menuOrder))
	for i, group := range menuOrder {
		order[i] = groupTitle(group)
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

func TestAccelerator(t *testing.T) {
	for _, c := range []struct {
		mods ui.Modifiers
		key  ui.Key
		want string
	}{
		{ui.Super | ui.Shift, ui.KeyN, "Super+Shift+n"},
		{ui.Ctrl, ui.KeySlash, "Ctrl+/"},
		{ui.Super, ui.Key1, "Super+1"},
		{ui.Super | ui.Alt, ui.KeyLeft, "Super+Alt+Left"},
		{ui.Super, ui.KeyComma, "Super+,"},
		{ui.Super, ui.KeyBack, ""},
	} {
		if got := accelerator(c.mods, c.key); got != c.want {
			t.Errorf("accelerator(%v, %v) = %q, want %q", c.mods, c.key, got, c.want)
		}
	}
}

// TestMenuItemRunsItsShortcut checks that choosing an item runs its row on the UI thread, where the window draws what it changed.
func TestMenuItemRunsItsShortcut(t *testing.T) {
	a, tt, updates := newTestApp(&fakePipeline{})
	tt.Frame()
	it := findMenuItem(t, a.menuBar(), "看懂", "设置…")
	it.Click(it, nil)
	if a.prefs.open {
		t.Fatalf("the click opened the settings at %q before the UI thread ran it", a.prefs.section)
	}
	waitUpdate(t, updates)
	wantSettings(t, a, settings.SectionGeneral)
}

// TestMenuFollowsTheFilm checks that the player's items are enabled only while a film is open, and that 窗口 marks the screen the window shows.
func TestMenuFollowsTheFilm(t *testing.T) {
	a, tt, _ := newTestApp(&fakePipeline{})
	tt.Frame()
	bar := a.menuBar()
	if !findMenuItem(t, bar, "显示", "全屏").Disabled || !findMenuItem(t, bar, "窗口", "播放器").Disabled {
		t.Fatal("player items enabled without a film")
	}
	if library := findMenuItem(t, bar, "窗口", "片库"); library.Type != mygo.MenuItemCheckbox || !library.Checked {
		t.Fatalf("片库 is not checked in the library: %+v", library)
	}

	a, tt, _ = openClip(t, 2)
	tt.Frame()
	bar = a.menuBar()
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
	a, tt, updates := newTestApp(&fakePipeline{})
	tt.Frame()
	settings := findMenuItem(t, a.menuBar(), "看懂", "设置…")
	a.toggleCheatsheet()
	tt.Frame()
	bar := a.menuBar()
	if !findMenuItem(t, bar, "看懂", "设置…").Disabled || !findMenuItem(t, bar, "文件", locales.MenuRevealLibrary(platformName())).Disabled {
		t.Fatal("commands enabled behind the cheat sheet")
	}
	// An item of a menu built before the sheet opened.
	choose(t, settings, updates)
	if a.prefs.open {
		t.Fatalf("设置… ran behind the cheat sheet: settings open at %q", a.prefs.section)
	}
	sheet := findMenuItem(t, bar, "帮助", "快捷键速查")
	if sheet.Disabled {
		t.Fatal("快捷键速查 disabled while the cheat sheet is open")
	}
	choose(t, sheet, updates)
	if a.overlay.cheatsheet {
		t.Fatal("快捷键速查 did not close the cheat sheet")
	}
}

// TestMenuKeepsMygosDefaultCommands checks that the menu bar keeps what the one mygo installs for an app without its own has beyond the table: full screen on every page, not only with a film open, with mygo's key for it (⌃⌘F on macOS, F11 elsewhere), and the Edit menu's 粘贴并匹配样式 and 语音.
func TestMenuKeepsMygosDefaultCommands(t *testing.T) {
	a, tt, _ := newTestApp(&fakePipeline{})
	tt.Frame()
	menu := mygo.NewMenu(a.menuBar())
	var full *mygo.MenuItem
	roles := map[mygo.MenuRole]bool{}
	var walk func([]*mygo.MenuItem)
	walk = func(items []*mygo.MenuItem) {
		for _, it := range items {
			roles[it.Role] = true
			if it.Role == mygo.RoleToggleFullScreen {
				full = it
			}
			walk(it.Submenu)
		}
	}
	for _, m := range menu.Items() {
		if m.Label == "显示" {
			walk(m.Submenu)
		}
	}
	if full == nil || full.Disabled {
		t.Fatalf("显示 has no enabled full screen item without a film: %+v", full)
	}
	want := "F11"
	if runtime.GOOS == "darwin" {
		want = "Ctrl+Cmd+F"
	}
	if full.Accelerator != want {
		t.Fatalf("full screen accelerator %q", full.Accelerator)
	}
	for _, m := range menu.Items() {
		if m.Label == "编辑" {
			walk(m.Submenu)
		}
	}
	for _, role := range []mygo.MenuRole{mygo.RolePasteAndMatchStyle, mygo.RoleStartSpeaking, mygo.RoleStopSpeaking} {
		if !roles[role] {
			t.Fatalf("编辑 has no %q item", role)
		}
	}
}

// TestMenuWaitsBehindAFilePanel checks that while a file panel is up as a sheet over the window, every menu bar command waits, the cheat sheet's too, and that a click from a menu built before the panel came up does nothing.
func TestMenuWaitsBehindAFilePanel(t *testing.T) {
	a, tt, updates := newTestApp(&fakePipeline{})
	tt.Frame()
	library := findMenuItem(t, a.menuBar(), "文件", "新建资料库…")
	settings := findMenuItem(t, a.menuBar(), "看懂", "设置…")
	sheet := findMenuItem(t, a.menuBar(), "帮助", "快捷键速查")
	release := a.holdModal()
	tt.Frame()
	bar := a.menuBar()
	for _, it := range []struct{ menu, label string }{{"文件", "打开资料库…"}, {"文件", "新建资料库…"}, {"文件", "打开本地视频…"}, {"文件", locales.MenuRevealLibrary(platformName())}, {"看懂", "设置…"}, {"窗口", "片库"}, {"帮助", "快捷键速查"}} {
		if !findMenuItem(t, bar, it.menu, it.label).Disabled {
			t.Fatalf("%s › %s enabled behind a file panel", it.menu, it.label)
		}
	}
	choose(t, library, updates)
	if a.snap().MakingLibrary {
		t.Fatal("新建资料库… ran behind a file panel")
	}
	choose(t, settings, updates)
	if a.prefs.open {
		t.Fatalf("设置… ran behind a file panel: settings open at %q", a.prefs.section)
	}
	choose(t, sheet, updates)
	if a.overlay.cheatsheet {
		t.Fatal("快捷键速查 opened the cheat sheet behind a file panel")
	}
	// Once the panel has closed, the commands run again.
	release()
	tt.Frame()
	if findMenuItem(t, a.menuBar(), "看懂", "设置…").Disabled {
		t.Fatal("设置… still disabled after the file panel closed")
	}
}

// TestSyncMenuInstallsOnlyChanges checks that the installed menu bar is replaced when what it shows changes, and left alone by frames that change none of it.
func TestSyncMenuInstallsOnlyChanges(t *testing.T) {
	a, tt, updates := newTestApp(&fakePipeline{})
	a.menu.menuBar = true
	tt.Frame()
	first := mygo.App.Menu()
	if first == nil || a.menu.menuSig == "" {
		t.Fatal("no menu bar installed")
	}
	tt.Frame()
	if mygo.App.Menu() != first {
		t.Fatal("a frame that changed nothing installed a new menu bar")
	}
	a.toggleCheatsheet()
	tt.Frame()
	second := mygo.App.Menu()
	if second == first {
		t.Fatal("opening the cheat sheet left the menu bar as it was")
	}
	// mygo flips a checkbox item's ✓ before its Click, so choosing one has the next frame install the menu again with the app's ✓.
	a.toggleCheatsheet()
	tt.Frame()
	third := mygo.App.Menu()
	for _, m := range third.Items() {
		if m.Label != "窗口" {
			continue
		}
		for _, it := range m.Submenu {
			if it.Label == "片库" {
				it.SetChecked(!it.IsChecked())
				it.Click(it, nil)
			}
		}
	}
	waitUpdate(t, updates)
	tt.Frame()
	if mygo.App.Menu() == third {
		t.Fatal("choosing a checkbox item left the menu bar with mygo's ✓")
	}
}

// TestWindowKeysGoBackToTheLastFilm checks ⌘1 and ⌘2: the library, and back to the film the window showed last, which going to the library does not forget.
func TestWindowKeysGoBackToTheLastFilm(t *testing.T) {
	a, tt, updates := openClip(t, 2)
	tt.Key(ui.Cmd, ui.Key1)
	tt.Frame()
	if a.openKey != "" || a.player.play != nil || a.page != pageFilms {
		t.Fatalf("⌘1: open %q play %v page %q", a.openKey, a.player.play != nil, a.page)
	}
	tt.Key(ui.Cmd, ui.Key2)
	tt.Frame()
	if a.openKey != "clip" {
		t.Fatalf("⌘2 opened %q", a.openKey)
	}
	waitUntil(t, updates, func() bool { return a.player.play != nil })

	// From a settings page too.
	tt.Key(ui.Cmd, ui.KeyComma)
	tt.Frame()
	wantSettings(t, a, settings.SectionGeneral)
	tt.Key(ui.Cmd, ui.Key2)
	tt.Frame()
	if a.page != pageFilms || a.openKey != "clip" {
		t.Fatalf("⌘2 from settings: page %q open %q", a.page, a.openKey)
	}

	// Once the film has left the library there is nothing to go back to.
	tt.Key(ui.Cmd, ui.Key1)
	tt.Frame()
	a.library.Jobs = nil
	tt.Frame()
	tt.Key(ui.Cmd, ui.Key2)
	tt.Frame()
	if a.openKey != "" {
		t.Fatalf("⌘2 opened %q after the film left", a.openKey)
	}
	if !findMenuItem(t, a.menuBar(), "窗口", "播放器").Disabled {
		t.Fatal("播放器 enabled after the film left")
	}
}

// TestCheatsheetCloseButton checks the cheat sheet's × button.
func TestCheatsheetCloseButton(t *testing.T) {
	a, tt, _ := newTestApp(&fakePipeline{})
	a.toggleCheatsheet()
	tt.Frame()
	if err := tt.Click("关闭"); err != nil {
		t.Fatalf("%v; texts %q", err, tt.Texts())
	}
	tt.Frame()
	if a.overlay.cheatsheet {
		t.Fatal("× left the cheat sheet open")
	}
}

// TestMenuBarInEnglish checks the menu bar in English, in Apple's wording for the standard items.
func TestMenuBarInEnglish(t *testing.T) {
	testlocale.Use(t, "en")
	a, tt, _ := newTestApp(&fakePipeline{})
	tt.Frame()
	bar := a.menuBar()
	for _, want := range [][2]string{
		{"Kando", "About Kando"}, {"Kando", "Settings…"}, {"Kando", "Hide Kando"}, {"Kando", "Quit Kando"},
		{"File", "New Library…"}, {"File", "Open Library…"}, {"File", "Switch Library"}, {"File", locales.MenuRevealLibrary(platformName())}, {"File", "Close Window"},
		{"Edit", "Undo"}, {"Edit", "Copy"}, {"Edit", "Paste and Match Style"},
		{"View", "Toggle Full Screen"},
		{"Study", "Ask This Film / Commands…"},
		{"Window", "Minimize"}, {"Window", "Library"}, {"Window", "Player"}, {"Window", "Bring All to Front"},
		{"Help", "Shortcut Cheat Sheet"}, {"Help", "Kando Website"}, {"Help", "Release Notes"},
	} {
		findMenuItem(t, bar, want[0], want[1])
	}
}

// TestMenuNamesTheSystemsFileManager checks that the File menu's reveal names the file manager of the system it runs on.
func TestMenuNamesTheSystemsFileManager(t *testing.T) {
	for p, want := range map[breeze.Platform]string{breeze.Mac: "在访达中显示资料库", breeze.Windows: "在文件资源管理器中显示资料库", breeze.Linux: "在文件管理器中显示资料库"} {
		t.Run(p.String(), func(t *testing.T) {
			usePlatform(t, p)
			a, tt, _ := newTestApp(&fakePipeline{})
			tt.Frame()
			findMenuItem(t, a.menuBar(), "文件", want)
		})
	}
}

// fakeDock stands in for mygo's Dock menu for the test, which mygo would install on the main thread a test does not run on, and returns the menus set on it, the last one last.
func fakeDock(t *testing.T) *[]*mygo.Menu {
	t.Helper()
	var set []*mygo.Menu
	prev := setDockMenu
	setDockMenu = func(m *mygo.Menu) { set = append(set, m) }
	t.Cleanup(func() { setDockMenu = prev })
	return &set
}

// twoLibraries opens the first of two libraries, 甲 and 乙, in a test app that saves its settings to a file of its own.
func twoLibraries(t *testing.T) (*app, *ui.Tester, chan func(), string, string) {
	t.Helper()
	parent := t.TempDir()
	first, second := filepath.Join(parent, "甲.kando"), filepath.Join(parent, "乙.kando")
	for _, dir := range []string{first, second} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	a, tt, updates := newTestApp(&fakePipeline{})
	a.settingsPath = filepath.Join(t.TempDir(), "settings.json")
	a.edit(func(s *session) {
		s.LibraryDir = first
		s.Libraries = []string{first, second}
	})
	a.refreshLibrary()
	tt.Frame()
	return a, tt, updates, first, second
}

// libraryChoices writes down the items of a library list as label, then * for the chosen one and - for one that is disabled.
func libraryChoices(t *testing.T, items []*mygo.MenuItem) []string {
	t.Helper()
	var out []string
	for _, it := range items {
		if it.Type != mygo.MenuItemRadio {
			t.Fatalf("%q is a %q item, not a radio one", it.Label, it.Type)
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

// TestMenuSwitchesTheLibrary checks 文件 › 切换资料库: it lists the known libraries with the open one chosen, choosing another opens it on the library page, and the next frame installs the menu again with the new one chosen, as mygo moved the radio's choice itself.
func TestMenuSwitchesTheLibrary(t *testing.T) {
	a, tt, updates, _, second := twoLibraries(t)
	a.menu.menuBar = true
	a.Select(pageYouTube)
	tt.Frame()
	sw := findMenuItem(t, a.menuBar(), "文件", "切换资料库")
	if got, want := libraryChoices(t, sw.Submenu), []string{"甲*", "乙"}; !slices.Equal(got, want) {
		t.Fatalf("切换资料库 lists %q, want %q", got, want)
	}
	installed := a.menu.menuSig
	choose(t, sw.Submenu[1], updates)
	if a.snap().LibraryDir != second || a.page != pageFilms {
		t.Fatalf("after choosing 乙: library %q page %q", a.snap().LibraryDir, a.page)
	}
	tt.Frame()
	if a.menu.menuSig == installed {
		t.Fatal("the menu bar was not installed again after switching")
	}
	sw = findMenuItem(t, a.menuBar(), "文件", "切换资料库")
	if got, want := libraryChoices(t, sw.Submenu), []string{"甲", "乙*"}; !slices.Equal(got, want) {
		t.Fatalf("after switching 切换资料库 lists %q, want %q", got, want)
	}
}

// TestMenuSwitchLibraryWaitsBehindAFilePanel checks that the library items wait behind a file panel, and that a click from a menu built before it came up leaves the open library as it was but still has the menu installed again, as mygo moved the radio's choice.
func TestMenuSwitchLibraryWaitsBehindAFilePanel(t *testing.T) {
	a, tt, updates, first, _ := twoLibraries(t)
	a.menu.menuBar = true
	tt.Frame()
	early := findMenuItem(t, a.menuBar(), "文件", "切换资料库").Submenu[1]
	release := a.holdModal()
	t.Cleanup(release)
	tt.Frame()
	if got, want := libraryChoices(t, findMenuItem(t, a.menuBar(), "文件", "切换资料库").Submenu), []string{"甲*-", "乙-"}; !slices.Equal(got, want) {
		t.Fatalf("behind a file panel 切换资料库 lists %q, want %q", got, want)
	}
	choose(t, early, updates)
	if a.snap().LibraryDir != first {
		t.Fatalf("乙 opened behind a file panel: library %q", a.snap().LibraryDir)
	}
	if a.menu.menuSig != "" {
		t.Fatal("a held-back click left the menu bar with mygo's choice")
	}
}

// TestMenuSwitchLibraryNamesTwinsByPath checks that two libraries with the same name are told apart by their paths, as Settings › 通用 does.
func TestMenuSwitchLibraryNamesTwinsByPath(t *testing.T) {
	first, second := filepath.Join(t.TempDir(), "影片.kando"), filepath.Join(t.TempDir(), "影片.kando")
	a, tt, _ := newTestApp(&fakePipeline{})
	a.edit(func(s *session) {
		s.LibraryDir = first
		s.Libraries = []string{first, second}
	})
	tt.Frame()
	got := libraryChoices(t, findMenuItem(t, a.menuBar(), "文件", "切换资料库").Submenu)
	if want := []string{films.DisplayPath(first) + "*", films.DisplayPath(second)}; !slices.Equal(got, want) {
		t.Fatalf("切换资料库 lists %q, want %q", got, want)
	}
}

// TestDockMenuListsTheLibraries checks that installing the menu bar sets the Dock menu to the same library items, which switch the library too, and that it follows a switch made elsewhere.
func TestDockMenuListsTheLibraries(t *testing.T) {
	a, tt, updates, first, second := twoLibraries(t)
	dock := fakeDock(t)
	a.menu.menuBar = true
	tt.Frame()
	last := func() []*mygo.MenuItem {
		t.Helper()
		if len(*dock) == 0 || (*dock)[len(*dock)-1] == nil {
			t.Fatalf("no Dock menu set: %v", *dock)
		}
		return (*dock)[len(*dock)-1].Items()
	}
	if got, want := libraryChoices(t, last()), []string{"甲*", "乙"}; !slices.Equal(got, want) {
		t.Fatalf("Dock menu lists %q, want %q", got, want)
	}
	sets := len(*dock)
	tt.Frame()
	if len(*dock) != sets {
		t.Fatal("a frame that changed nothing set the Dock menu again")
	}
	choose(t, last()[1], updates)
	if a.snap().LibraryDir != second {
		t.Fatalf("the Dock menu's 乙 opened %q", a.snap().LibraryDir)
	}
	tt.Frame()
	if got, want := libraryChoices(t, last()), []string{"甲", "乙*"}; !slices.Equal(got, want) {
		t.Fatalf("after switching the Dock menu lists %q, want %q", got, want)
	}
	a.Choose(first)
	tt.Frame()
	if got, want := libraryChoices(t, last()), []string{"甲*", "乙"}; !slices.Equal(got, want) {
		t.Fatalf("after the sidebar switched back the Dock menu lists %q, want %q", got, want)
	}
}

// TestMenuWithoutLibraries checks that with no library known, 文件 has no 切换资料库 and the Dock menu is removed.
func TestMenuWithoutLibraries(t *testing.T) {
	a, tt, _ := newTestApp(&fakePipeline{})
	a.edit(func(s *session) { s.LibraryDir, s.Libraries = "", nil })
	dock := fakeDock(t)
	a.menu.menuBar = true
	tt.Frame()
	for _, m := range a.menuBar() {
		for _, it := range m.Submenu {
			if it.Label == "切换资料库" {
				t.Fatalf("%s has 切换资料库 without a library", m.Label)
			}
		}
	}
	if len(*dock) == 0 || (*dock)[len(*dock)-1] != nil {
		t.Fatalf("Dock menu without a library: %v", *dock)
	}
}

// TestHelpMenuOpensTheLinks checks 帮助 › 看懂官网 and 更新说明: they open the website and the running version's release in the browser, are items of the menu bar and the palette but not of the cheat sheet, and say in the message bar when the browser could not be opened.
func TestHelpMenuOpensTheLinks(t *testing.T) {
	var opened []string
	var fail error
	prev := openExternal
	openExternal = func(url string) error { opened = append(opened, url); return fail }
	t.Cleanup(func() { openExternal = prev })
	keep := appVersion
	appVersion = "1.2.3"
	t.Cleanup(func() { appVersion = keep })

	a, tt, updates := newTestApp(&fakePipeline{})
	tt.Frame()
	choose(t, findMenuItem(t, a.menuBar(), "帮助", "看懂官网"), updates)
	choose(t, findMenuItem(t, a.menuBar(), "帮助", "更新说明"), updates)
	if want := []string{"https://kando.yldm.ai", "https://github.com/yldm-tech/Kando/releases/tag/v1.2.3"}; !slices.Equal(opened, want) {
		t.Fatalf("opened %q, want %q", opened, want)
	}
	var palette []string
	for _, cmd := range a.paletteCommands() {
		palette = append(palette, cmd.label)
	}
	for _, label := range []string{"看懂官网", "更新说明"} {
		if !slices.Contains(palette, label) {
			t.Fatalf("the palette has no %q: %q", label, palette)
		}
	}
	for _, s := range a.shortcuts() {
		if (s.Label == "看懂官网" || s.Label == "更新说明") && s.Key != 0 {
			t.Fatalf("%q has a key, so the cheat sheet lists it", s.Label)
		}
	}

	fail = errors.New("exit status 1")
	choose(t, findMenuItem(t, a.menuBar(), "帮助", "看懂官网"), updates)
	if want := "没能打开 https://kando.yldm.ai：exit status 1"; a.lib.notice.Text != want || a.lib.notice.Kind != "warning" {
		t.Fatalf("notice %+v, want warning %q", a.lib.notice, want)
	}
	if a.player.film.osd != "" {
		t.Fatalf("on the library page the failure was also put on the picture: %q", a.player.film.osd)
	}
	// The message bar shows only on the library page, so while a film is open the failure is said on the picture too.
	a.openKey = "j"
	a.menuOpenLink(menuReleaseNotesURL())
	if want := "没能打开 https://github.com/yldm-tech/Kando/releases/tag/v1.2.3：exit status 1"; a.player.film.osd != want {
		t.Fatalf("with a film open the picture says %q, want %q", a.player.film.osd, want)
	}
}
