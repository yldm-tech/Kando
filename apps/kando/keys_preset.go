package main

import (
	"kando/apps/kando/hotkey"

	"github.com/nsgo-dev/nsgo/core"
)

// Settings › 快捷键's preset: the player's keys as Kando has them, or moved to where IINA or mpv put the same commands (hotkey.Remap).

// savedShortcuts is the shortcut preset's part of settings.json, "" for Kando's own keys.
type savedShortcuts struct {
	ShortcutPreset string `json:"shortcutPreset,omitempty"`
}

func (s *savedShortcuts) clean() {
	if !hotkey.Known(s.ShortcutPreset) {
		s.ShortcutPreset = hotkey.PresetSystem
	}
}

// keysPreset moves the player's rows of the table to the keys the preset chosen gives them on the system cx runs on (presetKey); the other areas' keep theirs.
func (a *app) keysPreset(cx *core.Scope, rows []shortcut) []shortcut {
	if a.prefs.shortcutPreset == hotkey.PresetSystem {
		return rows
	}
	for i, row := range rows {
		rows[i].Key = presetKey(a.prefs.shortcutPreset, cx.Env().OS(), row.Group, row.Key)
	}
	return rows
}

// presetKey is the key a row of group whose own key is key has under preset on os: for the playback and subtitle groups, which are the player's, the one hotkey.Remap moves it to; for the others, and for a row without a key, key itself. The table (keysPreset), the cheat sheet's layout (cheatsheetGroupsOf) and the player's tooltips (SpeedKeys) all move keys through it, so they name the same keys.
func presetKey(preset string, os core.OS, group, key string) string {
	if key == "" || (group != groupPlayback && group != groupSubtitles) {
		return key
	}
	return hotkey.Remap(preset, os, key)
}

// useShortcutPreset moves the player's keys to preset's at once, in the window, the menu bar and the cheat sheet, and saves it; a preset Kando does not know is ignored.
func (a *app) useShortcutPreset(preset string) {
	if !hotkey.Known(preset) || a.prefs.shortcutPreset == preset {
		return
	}
	a.prefs.shortcutPreset = preset
	if err := a.saveSettings(); err != nil {
		a.saveErr = a.messages().SettingsSaveFailed(err.Error())
	}
}
