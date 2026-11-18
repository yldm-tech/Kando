package main

import (
	"kando/apps/kando/hotkey"
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

// keysPreset moves the player's rows of the table to the keys the preset chosen gives them (hotkey.Remap): those of the playback and subtitle groups, which are the player's; the other areas' keep theirs.
func (a *app) keysPreset(rows []shortcut) []shortcut {
	if a.prefs.shortcutPreset == hotkey.PresetSystem {
		return rows
	}
	for i, row := range rows {
		if row.Key != "" && (row.Group == groupPlayback || row.Group == groupSubtitles) {
			rows[i].Key = hotkey.Remap(a.prefs.shortcutPreset, row.Key)
		}
	}
	return rows
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
