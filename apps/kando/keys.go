package main

import (
	"slices"
	"strings"

	"kando/apps/kando/live"
	"kando/apps/kando/locales"
	"kando/apps/kando/settings"

	"github.com/nsgo-dev/nsgo/core"
)

// shortcut is one row of the app's shortcut table. The cheat sheet and Settings › 快捷键 list the same rows wireKeys binds.
type shortcut struct {
	// Group is the menu the row is listed under, one of the group IDs below; groupTitle is what it is called.
	Group string
	Label string
	// Key is the row's key in NSGo's form (core.Scope.OnKey): modifiers and a key joined by "+", "mod" being ⌘ on macOS and Ctrl elsewhere, as MyGo's ui.Cmd was, and the key a letter, a digit, a punctuation mark or a name: "mod+shift+n", "mod+,", "shift+.", "space", "left", "escape", "f5". cx.KeyLabel writes it as the platform shows it (⌘⇧N, Ctrl+Shift+N). A row without one ("") is a command of the menu bar only: wireKeys does not bind it, and neither the cheat sheet nor Settings › 快捷键 lists it, as there is no key to show.
	Key string
	// When reports whether the shortcut applies now; nil means always.
	When func() bool
	// Checked reports whether the menu bar shows a ✓ by the row, as by the screen the window shows now; nil means the row is a plain command.
	Checked func() bool
	// Repeat says the key runs the row again while it is held, as MyGo's shortcuts all did: the rows that step a value (seeking, the volume, the speed, the subtitle delay, the previous or next line) set it; a row that toggles something (Space, F, M) or opens something leaves it false, so holding the key does not flip it back and forth.
	Repeat bool
	// InText lets the key run while a text field has the focus, for a ⌘ key no text field uses although NSGo counts it among the ones it does (core.EditsText counts every ⌘C as copy), as ⌘⇧C, which copies the playing line from the subtitle search too.
	InText bool
	Run    func()
}

// hasCommand reports whether key is held with ⌘, Ctrl on Windows and Linux: a row the menu bar carries as its item's shortcut.
func hasCommand(key string) bool {
	parts := strings.Split(key, "+")
	return slices.Contains(parts[:len(parts)-1], "mod")
}

// The groups of the shortcut table, by ID: the menus of the menu bar (menuOrder), and the headings of the cheat sheet and of Settings › 快捷键. groupTitle names them in the interface's language.
const (
	groupApp       = "app"
	groupFile      = "file"
	groupEdit      = "edit"
	groupPlayback  = "playback"
	groupSubtitles = "subtitles"
	groupView      = "view"
	groupStudy     = "study"
	groupWindow    = "window"
	groupHelp      = "help"
)

// groupTitle is what the group of a shortcut is called in the language the app shows under cx; a group it does not know is called by its ID.
func groupTitle(cx *core.Scope, group string) string {
	switch group {
	case groupApp:
		return locales.MenuGroupApp(cx)
	case groupFile:
		return locales.MenuGroupFile(cx)
	case groupEdit:
		return locales.MenuGroupEdit(cx)
	case groupPlayback:
		return locales.MenuGroupPlayback(cx)
	case groupSubtitles:
		return locales.MenuGroupSubtitles(cx)
	case groupView:
		return locales.MenuGroupView(cx)
	case groupStudy:
		return locales.MenuGroupStudy(cx)
	case groupWindow:
		return locales.MenuGroupWindow(cx)
	case groupHelp:
		return locales.MenuGroupHelp(cx)
	}
	return group
}

// shortcuts is the whole table, in the order the cheat sheet lists it, its labels in the language the app shows under cx. Each area keeps its rows next to its own code: the app's here, the sidebar's in state_sidebar.go, the library's in bind_films.go, the queue's in queue.go, the player's in bind_film.go, the settings' in state_settings.go, the overlays' in state_overlay.go, the menu bar's in state_menu.go, the feedback dialog's in state_feedback.go, the first-run guide's in state_onboarding.go, 显示日志文件 in state_logs.go, 检查更新… in updates.go, the main window's in state_window.go and the browser's in state_browse.go.
func (a *app) shortcuts(cx *core.Scope) []shortcut {
	return slices.Concat(a.appKeys(cx), a.sidebarKeys(cx), a.libraryKeys(cx), a.queueKeys(cx), a.playerKeys(cx), a.settingsKeys(cx), a.overlayKeys(cx), a.menuKeys(cx), a.feedbackKeys(cx), a.paletteKeys(cx), a.onboardingKeys(cx), a.sitesKeys(cx), a.logsKeys(cx), a.updatesKeys(cx), a.windowKeys(cx), a.browseKeys(cx))
}

// appKeys are the shortcuts that work anywhere in the window.
func (a *app) appKeys(cx *core.Scope) []shortcut {
	return []shortcut{
		{Group: groupApp, Label: locales.AppSettings(cx), Key: "mod+,", Run: func() { a.OpenSettings(settings.SectionGeneral) }},
	}
}

// wireKeys binds the table in the main window, under cx, the view's scope. A key binds only while its row's When holds, so a key no row takes now, such as Space with no film open, reaches the focused control as before; NSGo consumes the keys it binds. The bindings are made again only when the keys whose When holds change (active), not after every change of the app's state, and When is checked again when a key runs, as a row's state can change between the two. Once the app has its own menu bar, ⌘ shortcuts are the menu's (installMenu) and are not bound here, so a key does not run twice. A key fires again while held when its row says so (shortcut.Repeat). Under NSGo's default (core.KeyAuto) a key named by more than one character, as escape, enter, space or an arrow, does not fire while a text field has the focus, and a key bound here is taken from the focused control: Space then runs the table's row, not a focused button.
func (a *app) wireKeys(cx *core.Scope) core.Node {
	active := live.Memo(cx, a, func() string {
		var keys []string
		for _, s := range a.shortcuts(cx) {
			if a.binds(cx, s) {
				keys = append(keys, s.Key)
			}
		}
		return strings.Join(keys, "\x00")
	})
	return core.Dynamic(func(cx *core.Scope) core.Node {
		active()
		rows := core.Untracked(cx, func() []shortcut { return a.shortcuts(cx) })
		bound := map[string]bool{}
		for _, s := range rows {
			if bound[s.Key] || !core.Untracked(cx, func() bool { return a.binds(cx, s) }) {
				continue
			}
			bound[s.Key] = true
			// TODO(nsgo#205): let a binding give way to the focused control that uses its key, as core.KeyAuto does for text inputs: a focused button's or menu row's Space and Enter, a slider's, tab bar's or radio group's arrows. MyGo's OnShortcut left those keys to such a control; here the player's Space and arrows act on the film instead, except while its speed or mode menu is open (playerKeys) https://github.com/nsgo-dev/nsgo/issues/205
			when := core.KeyAuto
			if s.InText {
				when = core.KeyAlways
			}
			cx.OnKeyWith(s.Key, core.KeyOptions{Repeat: s.Repeat, When: when}, live.Do(a, a.keyRun(cx, s.Key)))
		}
		return nil
	})
}

// binds reports whether wireKeys binds s now. While a modal dialog is up (modalOpen) or the settings show over the main window (settingsOverlay) no row binds, and while the cheat sheet shows only its own keys do (overlayKeys), as MyGo held the window's shortcuts behind a dialog and kept them out of its settings window: Space, F, M and the arrows do not act on the film behind it, Space presses the dialog's focused control, Escape closes the dialog, and no row binds a key again over the bindings a dialog makes for itself (core.KeyAlways).
func (a *app) binds(cx *core.Scope, s shortcut) bool {
	if s.Run == nil || s.Key == "" || (s.When != nil && !s.When()) {
		return false
	}
	if a.modalOpen() || (a.prefs.open && a.prefs.inWindow) || (a.overlay.cheatsheet && !slices.ContainsFunc(a.overlayKeys(cx), func(o shortcut) bool { return o.Key == s.Key })) {
		return false
	}
	return !(a.menu.menuBar && hasCommand(s.Key))
}

// keyRun is what key does when it is pressed: the first row of the table with that key whose When holds then. Rows share a key only when they take turns (TestShortcutsAreUnique), as the library's ⌘F and the player's do.
func (a *app) keyRun(cx *core.Scope, key string) func() {
	return func() {
		for _, s := range a.shortcuts(cx) {
			if s.Key == key && s.Run != nil && (s.When == nil || s.When()) {
				s.Run()
				return
			}
		}
	}
}

// guarded is Run behind a fresh check of When.
func (s shortcut) guarded() func() {
	if s.When == nil {
		return s.Run
	}
	return func() {
		if s.When() {
			s.Run()
		}
	}
}
