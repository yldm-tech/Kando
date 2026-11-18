package phonesync

import (
	"testing"

	"kando/apps/kando/film/subtitle"
)

// Every language the desktop translates into has a tag a phone names it by, and back.
func TestEveryTargetLanguageHasATag(t *testing.T) {
	for _, value := range subtitle.TargetLanguages() {
		tag := TargetTag(value)
		if tag == "" {
			t.Errorf("%s has no tag", value)
			continue
		}
		if back, ok := TargetValue(tag); !ok || back != value {
			t.Errorf("tag %s of %s reads back as %q, %v", tag, value, back, ok)
		}
	}
	if TargetTag("Klingon") != "" {
		t.Error("a language typed in got a tag")
	}
	if v, ok := TargetValue("ZH-hans"); !ok || v != "简体中文" {
		t.Errorf("a tag in another case reads as %q, %v", v, ok)
	}
	if _, ok := TargetValue("xx"); ok {
		t.Error("a tag the desktop does not translate into was taken")
	}
}
