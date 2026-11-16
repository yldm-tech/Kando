package main

import (
	"slices"
	"strings"
	"testing"

	"kando/apps/kando/film/subtitle"
	"kando/apps/kando/films"
	"kando/apps/kando/locales"
	"kando/apps/kando/onboarding"
	"kando/apps/kando/settings"
)

// TestAppRegistersTheBundleLanguages checks that the catalogs the app is built with are the languages of its interface (locales.Interface), which nsgo build declares to the platform, so NSGo's own texts, the About panel and the open panel speak the languages the interface does instead of staying English.
func TestAppRegistersTheBundleLanguages(t *testing.T) {
	want := slices.Clone(locales.Interface)
	slices.Sort(want[1:])
	if got := locales.Catalog.Languages(); !slices.Equal(got, want) {
		t.Fatalf("languages = %q, want %q", got, want)
	}
}

// TestTargetLanguagesAreNamedEverywhere checks that every target language the settings offer is named by the settings, the player and the library alike in every interface language, has a short tag of its own for a poster's badge, and is known to the first-run guide as the language it is written in, so a language added to settings.LanguageChoices is not shown as its stored value in one place and named in another.
func TestTargetLanguagesAreNamedEverywhere(t *testing.T) {
	tags := map[string]string{}
	codes := map[string]string{}
	for _, value := range settings.LanguageChoices("") {
		renamed := false
		for _, tag := range locales.Interface {
			m := locales.In(tag)
			name := subtitle.TargetLanguageNameIn(m, value)
			if got := films.LanguageName(m, value); got != name {
				t.Errorf("%s: the library names %q %q, the settings %q", tag, value, got, name)
			}
			renamed = renamed || name != value
		}
		if !renamed {
			t.Errorf("%q has no name in any interface language", value)
		}
		short := films.LanguageShort(locales.In("en"), value)
		if short == value {
			t.Errorf("%q has no short tag", value)
		} else if other, ok := tags[short]; ok {
			t.Errorf("%q and %q are both tagged %q", value, other, short)
		}
		tags[short] = value
		code := onboarding.LanguageCode(value)
		if code == "" {
			t.Errorf("the guide does not know what language %q is", value)
		} else if other, ok := codes[code]; ok && !(code == "zh" && strings.Contains(other+value, "中文")) {
			t.Errorf("%q and %q are both written in %q", value, other, code)
		}
		codes[code] = value
	}
	if got := onboarding.LanguageCode("Klingon"); got != "" {
		t.Errorf("a language Kando does not know is written in %q", got)
	}
}
