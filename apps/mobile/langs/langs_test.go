package langs

import "testing"

func TestDefaultTargetFollowsThePhonesLanguage(t *testing.T) {
	for locale, want := range map[string]string{
		"zh-Hans-CN": "zh-Hans", "zh-CN": "zh-Hans", "zh": "zh-Hans", "zh-Hant-TW": "zh-Hant", "zh-TW": "zh-Hant", "zh_HK": "zh-Hant",
		"ja-JP": "ja", "ko-KR": "ko", "en-US": "en", "fr-FR": "fr", "pt_BR": "pt", "sw-KE": "en", "": "en",
	} {
		if got := DefaultTarget(locale); got != want {
			t.Errorf("DefaultTarget(%q) = %q, want %q", locale, got, want)
		}
	}
}

func TestEveryDefaultIsATarget(t *testing.T) {
	for _, locale := range []string{"zh-Hans", "zh-Hant", "ja", "ko", "en", "de", "th-TH", "sw"} {
		if TargetName(DefaultTarget(locale)) == "" {
			t.Errorf("the default for %q is not a target", locale)
		}
	}
}
