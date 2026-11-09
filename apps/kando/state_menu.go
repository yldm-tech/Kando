package main

import (
	"kando/apps/kando/locales"

	"github.com/egoist/mygo/ui"
)

// menuUI is the state of the app's menu bar.
type menuUI struct {
	// menuBar is set once the app installs its own menu bar. From then on the menu's accelerators run the ⌘ shortcuts and wireKeys leaves them alone, so a shortcut does not run twice.
	menuBar bool
	// menuSig describes the menu bar installed last (menuSignature), so syncMenu installs a new one only when it would show something else.
	menuSig string
	// lastFilm is the key of the film the window showed last, which 窗口 › 播放器 goes back to. Going to another page does not clear it.
	lastFilm string
}

func newMenuUI() menuUI { return menuUI{} }

// savedMenu is the menu bar's part of settings.json.
type savedMenu struct{}

func (s *savedMenu) clean() {}

// menuHooks: the menu bar is installed once the window exists, and after each frame it remembers the film the window shows and brings itself up to date with what the pages settled in that frame (menu.go).
func (a *app) menuHooks() areaHooks {
	return areaHooks{
		ready: a.installMenu,
		afterFrame: func() {
			a.rememberFilm()
			a.syncMenu()
		},
	}
}

// menuKeys are the shortcuts of the commands the menu bar brings that no page owns: the library's files and the window's two screens.
func (a *app) menuKeys() []shortcut {
	return []shortcut{
		{Group: groupFile, Label: locales.MenuNewLibrary(), Mods: ui.Cmd | ui.Shift, Key: ui.KeyN, Run: a.beginNewLibrary},
		{Group: groupFile, Label: locales.MenuOpenLibrary(), Mods: ui.Cmd | ui.Shift, Key: ui.KeyO, Run: a.pickLibrary},
		{Group: groupWindow, Label: locales.MenuWindowLibrary(), Mods: ui.Cmd, Key: ui.Key1, Checked: func() bool { return a.page == pageFilms && a.openKey == "" }, Run: func() { a.Select(pageFilms) }},
		{Group: groupWindow, Label: locales.MenuWindowPlayer(), Mods: ui.Cmd, Key: ui.Key2, When: func() bool { _, ok := a.lastFilm(); return ok }, Checked: func() bool { return a.openKey != "" }, Run: a.reopenFilm},
	}
}
