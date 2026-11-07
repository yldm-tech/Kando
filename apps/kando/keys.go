package main

import (
	"slices"

	"kando/apps/kando/settings"

	"github.com/egoist/mygo/ui"
)

// shortcut is one row of the app's shortcut table. The cheat sheet and Settings › 快捷键 list the same rows wireKeys runs.
type shortcut struct {
	// Group is the heading the row is listed under, such as "播放".
	Group string
	Label string
	Mods  ui.Modifiers
	// Key is the row's key. A row without one (0) is a command of the menu bar only: wireKeys does not register it, and neither the cheat sheet nor Settings › 快捷键 lists it, as there is no key to show.
	Key ui.Key
	// When reports whether the shortcut applies now; nil means always.
	When func() bool
	// Checked reports whether the menu bar shows a ✓ by the row, as by the screen the window shows now; nil means the row is a plain command.
	Checked func() bool
	Run     func()
}

// shortcuts is the whole table, in the order the cheat sheet lists it. Each area keeps its rows next to its own code: the app's here, the sidebar's in state_sidebar.go, the library's in bind_films.go, the queue's in queue.go, the player's in bind_film.go, the settings' in state_settings.go, the overlays' in state_overlay.go, the menu bar's in state_menu.go and the feedback dialog's in state_feedback.go.
func (a *app) shortcuts() []shortcut {
	return slices.Concat(a.appKeys(), a.sidebarKeys(), a.libraryKeys(), a.queueKeys(), a.playerKeys(), a.settingsKeys(), a.overlayKeys(), a.menuKeys(), a.feedbackKeys(), a.paletteKeys())
}

// appKeys are the shortcuts that work anywhere in the window.
func (a *app) appKeys() []shortcut {
	return []shortcut{
		{Group: "看懂", Label: "设置…", Mods: ui.Cmd, Key: ui.KeyComma, Run: func() { a.OpenSettings(settings.SectionGeneral) }},
	}
}

// wireKeys registers the table with the window once per frame. A shortcut whose When is false is left out of this frame. Single keys go through OnShortcut, so a focused text field or button that takes the key keeps it. Once the app has its own menu bar, ⌘ shortcuts are the menu's accelerators and are not registered here.
//
// OnShortcut runs a handler only after the whole view has been built, and the pages can change the state When reads during that build (the library page closes a film that left the library and stops its playback), so When is checked again when the key runs.
func (a *app) wireKeys(c *ui.Context) {
	for _, s := range a.shortcuts() {
		if s.Run == nil || s.Key == 0 || (s.When != nil && !s.When()) {
			continue
		}
		if a.menu.menuBar && s.Mods&ui.Cmd != 0 {
			continue
		}
		c.OnShortcut(s.Mods, s.Key, s.guarded())
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
