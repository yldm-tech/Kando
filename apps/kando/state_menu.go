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

// menuHooks: the menu bar is installed once the window exists, and after each frame it remembers the film the window shows and brings itself up to date with what the pages settled in that frame (menu.go). It does so too when the main window is put away or comes back, as a hidden window draws no frame and 窗口 › 主窗口 is enabled only while it is hidden.
func (a *app) menuHooks() areaHooks {
	return areaHooks{
		ready: a.installMenu,
		afterFrame: func() {
			a.rememberFilm()
			a.syncMenu()
		},
		windowShown: func(bool) { a.syncMenu() },
	}
}

// menuKeys are the shortcuts of the commands the menu bar brings that no page owns: the library's files, the window's two screens, and the links of 帮助 to the website and to the running version's release notes, which have no key and so are items of the menu bar and the palette only.
func (a *app) menuKeys() []shortcut {
	return []shortcut{
		{Group: groupFile, Label: locales.MenuNewLibrary(), Mods: ui.Cmd | ui.Shift, Key: ui.KeyN, Run: a.beginNewLibrary},
		{Group: groupFile, Label: locales.MenuOpenLibrary(), Mods: ui.Cmd | ui.Shift, Key: ui.KeyO, Run: a.pickLibrary},
		{Group: groupWindow, Label: locales.MenuWindowLibrary(), Mods: ui.Cmd, Key: ui.Key1, Checked: func() bool { return a.page == pageFilms && a.openKey == "" }, Run: func() { a.Select(pageFilms) }},
		{Group: groupWindow, Label: locales.MenuWindowPlayer(), Mods: ui.Cmd, Key: ui.Key2, When: func() bool { _, ok := a.lastFilm(); return ok }, Checked: func() bool { return a.openKey != "" }, Run: a.reopenFilm},
		{Group: groupHelp, Label: locales.MenuWebsite(), Run: func() { a.menuOpenLink(menuWebsiteURL) }},
		{Group: groupHelp, Label: locales.MenuReleaseNotes(), Run: func() { a.menuOpenLink(menuReleaseNotesURL()) }},
	}
}

// menuWebsiteURL is Kando's website, the landing page apps/site renders.
const menuWebsiteURL = "https://kando.yldm.ai"

// menuReleasesURL is where the Release workflow publishes each version, under its tag vX.Y.Z.
const menuReleasesURL = "https://github.com/yldm-tech/Kando/releases/tag/"

// menuReleaseNotesURL is the release of the version running now, whose notes say what it changed.
func menuReleaseNotesURL() string { return menuReleasesURL + "v" + appVersion }

// menuOpenLink opens url in the user's browser through openExternal, and says in the library's message bar when the system could not, and on the picture too while a film is open, as the message bar shows only on the library page.
func (a *app) menuOpenLink(url string) {
	if err := openExternal(url); err != nil {
		msg := locales.MenuOpenLinkFailed(url, err.Error())
		a.warn(msg, "")
		if a.openKey != "" {
			a.flash(msg)
		}
	}
}
