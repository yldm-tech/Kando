package hotkey

import (
	"testing"

	"github.com/nsgo-dev/nsgo/core"
)

func TestPresetsMoveKeysWithoutTakingTwoToOne(t *testing.T) {
	for preset, keys := range presetKeys {
		if !Known(preset) {
			t.Fatalf("%q is not listed", preset)
		}
		to := map[string]string{}
		for from, moved := range keys {
			if other, taken := to[moved]; taken {
				t.Fatalf("%s moves %q and %q both to %q", preset, from, other, moved)
			}
			to[moved] = from
		}
	}
	if Remap(PresetSystem, core.OSWindows, "a") != "a" || Remap(PresetMPV, core.OSWindows, "a") != "ctrl+left" || Remap(PresetMPV, core.OSLinux, "space") != "space" {
		t.Fatal("Remap does not follow the tables")
	}
	// macOS moves between Spaces on ⌃← and ⌃→ before the window sees them, so the line keys stay A and D there; the rest of mpv's keys still move.
	if Remap(PresetMPV, core.OSMacOS, "a") != "a" || Remap(PresetMPV, core.OSMacOS, "d") != "d" || Remap(PresetMPV, core.OSMacOS, "up") != "0" {
		t.Fatal("Remap moves a key to one macOS takes")
	}
}
