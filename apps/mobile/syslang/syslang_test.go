package syslang

import (
	"slices"
	"testing"
)

// memory is an app's defaults in memory.
type memory map[string]any

func (m memory) strings(key string) []string {
	v, _ := m[key].([]string)
	return v
}

func (m memory) string(key string) string {
	v, _ := m[key].(string)
	return v
}

func (m memory) setStrings(key string, v []string) { m[key] = slices.Clone(v) }
func (m memory) remove(key string)                 { delete(m, key) }

// pin is what NSGo's apple.PinLanguage does to the app's defaults as the app forces lang, "" to follow the user again (internal/platform/apple/system.go:331-362 of the version go.mod pins).
func pin(m memory, lang string) {
	pinned, chosen := m.string(nsgoLanguage), ""
	if langs := m.strings(appleLanguages); len(langs) > 0 {
		chosen = langs[0]
	}
	switch {
	case lang != "" && (lang != pinned || lang != chosen):
		m.setStrings(appleLanguages, []string{lang})
		m[nsgoLanguage] = lang
	case lang == "" && pinned != "":
		if chosen == pinned {
			m.remove(appleLanguages)
		}
		m.remove(nsgoLanguage)
	}
}

// choose is the app choosing lang in its Settings: Keep, then NSGo's pin as SetLocale commits it.
func choose(m memory, lang string) {
	keep(m, lang)
	pin(m, lang)
}

// A language the user chose for Kando in iOS Settings is the app's again once the app follows the system after the user picked others in the app's Settings, as it was before. NSGo's pin alone removes it, and the app would follow the device's language from then on.
func TestTheLanguageChosenInIOSSettingsComesBack(t *testing.T) {
	m := memory{appleLanguages: []string{"zh-Hans-JP"}}
	choose(m, "en")
	if got := m.strings(appleLanguages); !slices.Equal(got, []string{"en"}) {
		t.Fatalf("forcing English leaves AppleLanguages %q", got)
	}
	choose(m, "ja")
	choose(m, "")
	if got := m.strings(appleLanguages); !slices.Equal(got, []string{"zh-Hans-JP"}) || len(m) != 1 {
		t.Fatalf("following the system again leaves the defaults %v, want the user's AppleLanguages [zh-Hans-JP] alone", m)
	}
}

// With no language of the app's own in iOS Settings, following the system again leaves none, so the app follows the device's languages.
func TestNoLanguageOfTheAppsOwnStaysNone(t *testing.T) {
	m := memory{}
	choose(m, "en")
	choose(m, "")
	if len(m) != 0 {
		t.Fatalf("the defaults hold %v, want nothing", m)
	}
}

// A language the user chose in iOS Settings while the app's own stood stays theirs: it is not written over by the one kept from before.
func TestALanguageChosenInIOSSettingsMeanwhileStays(t *testing.T) {
	m := memory{appleLanguages: []string{"zh-Hans"}}
	choose(m, "en")
	// iOS Settings › Kando › Language, picked while the app forced English.
	m.setStrings(appleLanguages, []string{"ko"})
	choose(m, "")
	if got := m.strings(appleLanguages); !slices.Equal(got, []string{"ko"}) {
		t.Fatalf("AppleLanguages is %q, want the user's latest [ko]", got)
	}
	if len(m) != 1 {
		t.Fatalf("the defaults hold %v, want AppleLanguages alone", m)
	}
}

// Keep runs at every launch with the language kept in the app's settings, and twice in a row changes nothing more than once.
func TestKeepingTwiceIsKeepingOnce(t *testing.T) {
	m := memory{appleLanguages: []string{"zh-Hans"}}
	choose(m, "en")
	choose(m, "en")
	choose(m, "")
	choose(m, "")
	if got := m.strings(appleLanguages); !slices.Equal(got, []string{"zh-Hans"}) || len(m) != 1 {
		t.Fatalf("the defaults hold %v, want AppleLanguages [zh-Hans] alone", m)
	}
}
