package hotkey

import "testing"

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
	if Remap(PresetSystem, "a") != "a" || Remap(PresetMPV, "a") != "ctrl+left" || Remap(PresetMPV, "space") != "space" {
		t.Fatal("Remap does not follow the tables")
	}
}
