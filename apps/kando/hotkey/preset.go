package hotkey

// The shortcut presets Settings › 快捷键 offers: the app's own keys (PresetSystem, the platform's usual ones), or the player's keys moved to where IINA or mpv put the same commands, for someone whose hands know those players. A preset moves only the player's keys whose command the other player has (Remap); the keys Kando adds for studying, such as the line keys, stay where they are.
const (
	PresetSystem = ""
	PresetIINA   = "iina"
	PresetMPV    = "mpv"
)

// Presets are the presets in the order the page lists them.
func Presets() []string { return []string{PresetSystem, PresetIINA, PresetMPV} }

// Known reports whether preset is one of Presets.
func Known(preset string) bool {
	for _, p := range Presets() {
		if p == preset {
			return true
		}
	}
	return false
}

// presetKeys gives, for each preset, the key a player's row moves to for the key it has in the app's own table. They follow the players' own default key bindings: IINA's iina-default-input.conf and mpv's etc/input.conf.
var presetKeys = map[string]map[string]string{
	PresetIINA: {
		// IINA: ⌥⌘[ and ⌥⌘] step the speed, ⌥Z and ⌥X move the subtitles by a tenth of a second, and G and F scale the subtitles (its mpv-style keys). Its ⌘L loop stays L, as ⌘L opens a location here.
		"shift+,": "alt+mod+[",
		"shift+.": "alt+mod+]",
		"z":       "alt+z",
		"x":       "alt+x",
		"mod+=":   "shift+g",
		"mod+-":   "shift+f",
	},
	PresetMPV: {
		// mpv: Ctrl+← and Ctrl+→ go to the previous and next subtitle, ↑ and ↓ seek further (a minute in mpv, Kando's longer skip here), 9 and 0 set the volume, [ and ] the speed, and G and F scale the subtitles. Z, X, V, M, F, L and Space are mpv's as they are.
		"a":         "ctrl+left",
		"d":         "ctrl+right",
		"mod+left":  "down",
		"mod+right": "up",
		"up":        "0",
		"down":      "9",
		"shift+,":   "[",
		"shift+.":   "]",
		"mod+=":     "shift+g",
		"mod+-":     "shift+f",
	},
}

// Remap is the key a player's row with key has under preset: the one presetKeys moves it to, or key itself.
func Remap(preset, key string) string {
	if moved, ok := presetKeys[preset][key]; ok {
		return moved
	}
	return key
}
