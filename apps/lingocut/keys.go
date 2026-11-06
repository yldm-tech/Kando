package main

import (
	"slices"

	"lingocut/apps/lingocut/settings"

	"github.com/egoist/mygo/ui"
)

// shortcut is one row of the app's shortcut table. The cheat sheet and Settings › 快捷键 list the same rows wireKeys runs.
type shortcut struct {
	// Group is the heading the row is listed under, such as "播放".
	Group string
	Label string
	Mods  ui.Modifiers
	Key   ui.Key
	// When reports whether the shortcut applies now; nil means always.
	When func() bool
	Run  func()
}

// shortcuts is the whole table, in the order the cheat sheet lists it. Each area keeps its rows next to its own code: the app's here, the library's in bind_films.go, the player's in bind_film.go and the overlays' in state_overlay.go.
func (a *app) shortcuts() []shortcut {
	return slices.Concat(a.appKeys(), a.libraryKeys(), a.playerKeys(), a.overlayKeys())
}

// appKeys are the shortcuts that work anywhere in the window.
func (a *app) appKeys() []shortcut {
	return []shortcut{
		{Group: "LingoCut", Label: "设置…", Mods: ui.Cmd, Key: ui.KeyComma, Run: func() { a.OpenSettings(settings.SectionGeneral) }},
	}
}

// wireKeys registers the table with the window once per frame. A shortcut whose When is false is left out of this frame. Single keys go through OnShortcut, so a focused text field or button that takes the key keeps it. Once the app has its own menu bar, ⌘ shortcuts are the menu's accelerators and are not registered here.
func (a *app) wireKeys(c *ui.Context) {
	for _, s := range a.shortcuts() {
		if s.Run == nil || (s.When != nil && !s.When()) {
			continue
		}
		if a.overlay.menuBar && s.Mods&ui.Cmd != 0 {
			continue
		}
		c.OnShortcut(s.Mods, s.Key, s.Run)
	}
}
