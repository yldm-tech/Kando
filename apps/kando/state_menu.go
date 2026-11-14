package main

import (
	"errors"

	"kando/apps/kando/desktop"
	"kando/apps/kando/live"
	"kando/apps/kando/locales"

	"github.com/nsgo-dev/nsgo/core"
)

// menuUI is the state of the app's menu bar.
type menuUI struct {
	// menuBar is set once the app installs its own menu bar. From then on the menu's shortcuts run the ⌘ rows and wireKeys leaves them alone, so a shortcut does not run twice.
	menuBar bool
	// menuSig describes the menu bar installed last (menuSignature), so syncMenu installs a new one only when it would show something else; installed is the scope its handlers run under, which goes when the next one is in.
	menuSig   string
	installed *core.Scope
	// lastFilm is the key of the film the window showed last, which 窗口 › 播放器 goes back to. Going to another page does not clear it.
	lastFilm string
}

func newMenuUI() menuUI { return menuUI{} }

// savedMenu is the menu bar's part of settings.json.
type savedMenu struct{}

func (s *savedMenu) clean() {}

// menuHooks: after every change the menu bar remembers the film the window shows (menu.go); the menu bar itself follows the app's state on its own (installMenu), from run.
func (a *app) menuHooks() areaHooks {
	return areaHooks{afterChange: a.rememberFilm}
}

// menuKeys are the shortcuts of the commands the menu bar brings that no page owns: the library's files, the window's two screens, and the links of 帮助 to the website and to the running version's release notes, which have no key and so are items of the menu bar and the palette only.
func (a *app) menuKeys(cx *core.Scope) []shortcut {
	return []shortcut{
		{Group: groupFile, Label: locales.MenuNewLibrary(cx), Key: "mod+shift+n", Run: a.beginNewLibrary},
		{Group: groupFile, Label: locales.MenuOpenLibrary(cx), Key: "mod+shift+o", Run: a.pickLibrary},
		{Group: groupWindow, Label: locales.MenuWindowLibrary(cx), Key: "mod+1", Checked: func() bool { return a.page == pageFilms && a.openKey == "" }, Run: func() { a.Select(pageFilms) }},
		{Group: groupWindow, Label: locales.MenuWindowPlayer(cx), Key: "mod+2", When: func() bool { _, ok := a.lastFilm(); return ok }, Checked: func() bool { return a.openKey != "" }, Run: a.reopenFilm},
		{Group: groupHelp, Label: locales.MenuWebsite(cx), Run: func() { a.menuOpenLink(menuWebsiteURL) }},
		{Group: groupHelp, Label: locales.MenuReleaseNotes(cx), Run: func() { a.menuOpenLink(menuReleaseNotesURL()) }},
	}
}

// menuWebsiteURL is Kando's website, the landing page apps/site renders.
const menuWebsiteURL = "https://kando.yldm.ai"

// menuReleasesURL is where the Release workflow publishes each version, under its tag vX.Y.Z.
const menuReleasesURL = "https://github.com/yldm-tech/Kando/releases/tag/"

// menuReleaseNotesURL is the release of the version running now, whose notes say what it changed.
func menuReleaseNotesURL() string { return menuReleasesURL + "v" + appVersion }

// errLinkNotOpened is why a link of the Help menu did not open: the system had no app for it.
var errLinkNotOpened = errors.New("no app opened it")

// menuOpenLink opens url in the user's browser, and says in the library's message bar when the system could not, and on the picture too while a film is open, as the message bar shows only on the library page. The answer comes back on the engine's goroutine, so it tells the revision itself.
func (a *app) menuOpenLink(url string) {
	if a.cx == nil {
		return
	}
	desktop.OpenURL(a.cx, url, live.On(a, func(opened bool) {
		if opened {
			return
		}
		msg := a.messages().MenuOpenLinkFailed(url, errLinkNotOpened.Error())
		a.warn(msg, "")
		if a.openKey != "" {
			a.flash(msg)
		}
	}))
}
