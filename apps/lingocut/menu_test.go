package main

import (
	"slices"
	"testing"

	"lingocut/apps/lingocut/settings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
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

// TestMenuBarListsTheShortcutTable checks that every row of the shortcut table is an item of the menu its group names, and that every ⌘ row carries its shortcut as an accelerator, since wireKeys leaves those to the menu bar once it is installed. Single keys carry none, so a focused text field keeps them.
func TestMenuBarListsTheShortcutTable(t *testing.T) {
	a, tt, _ := newTestApp(&fakePipeline{})
	tt.Frame()
	bar := a.menuBar()
	for _, s := range a.shortcuts() {
		it := findMenuItem(t, bar, s.Group, s.Label)
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
	for i := 1; i < len(titles); i++ {
		if slices.Index(menuOrder, titles[i-1]) >= slices.Index(menuOrder, titles[i]) {
			t.Fatalf("menus out of order: %q", titles)
		}
	}
	for _, want := range []string{"LingoCut", "文件", "编辑", "窗口", "帮助"} {
		if !slices.Contains(titles, want) {
			t.Fatalf("no %q menu: %q", want, titles)
		}
	}
}

// TestMenuGroupsTheTableInMenuOrder checks that a group the menu bar does not name gets a menu before 窗口, and that the cheat sheet lists its groups in the menu bar's order.
func TestMenuGroupsTheTableInMenuOrder(t *testing.T) {
	order, rows := grouped([]shortcut{{Group: "帮助"}, {Group: "学习"}, {Group: "播放"}, {Group: "学习"}})
	if at, win := slices.Index(order, "学习"), slices.Index(order, "窗口"); at < 0 || at != win-1 || slices.Index(order, "显示") > at {
		t.Fatalf("order %q", order)
	}
	if len(rows["学习"]) != 2 {
		t.Fatalf("rows %v", rows)
	}
	a, _, _ := newTestApp(&fakePipeline{})
	var titles []string
	for _, g := range a.cheatsheetGroups() {
		titles = append(titles, g.Title)
	}
	for i := 1; i < len(titles); i++ {
		if slices.Index(menuOrder, titles[i-1]) >= slices.Index(menuOrder, titles[i]) {
			t.Fatalf("cheat sheet groups out of order: %q", titles)
		}
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
	it := findMenuItem(t, a.menuBar(), "LingoCut", "设置…")
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
	settings := findMenuItem(t, a.menuBar(), "LingoCut", "设置…")
	a.toggleCheatsheet()
	tt.Frame()
	bar := a.menuBar()
	if !findMenuItem(t, bar, "LingoCut", "设置…").Disabled || !findMenuItem(t, bar, "文件", "在访达中显示资料库").Disabled {
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

// TestMenuKeepsMygosDefaultCommands checks that the menu bar keeps what the one mygo installs for an app without its own has beyond the table: ⌃⌘F full screen on every page, not only with a film open, and the Edit menu's 粘贴并匹配样式 and 语音.
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
	if full.Accelerator != "Ctrl+Cmd+F" {
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
	settings := findMenuItem(t, a.menuBar(), "LingoCut", "设置…")
	sheet := findMenuItem(t, a.menuBar(), "帮助", "快捷键速查")
	release := a.holdModal()
	tt.Frame()
	bar := a.menuBar()
	for _, it := range []struct{ menu, label string }{{"文件", "打开资料库…"}, {"文件", "新建资料库…"}, {"文件", "打开本地视频…"}, {"文件", "在访达中显示资料库"}, {"LingoCut", "设置…"}, {"窗口", "片库"}, {"帮助", "快捷键速查"}} {
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
	if findMenuItem(t, a.menuBar(), "LingoCut", "设置…").Disabled {
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
