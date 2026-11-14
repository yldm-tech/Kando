package settings

import (
	"testing"

	"kando/apps/kando/locales/testlocale"
	"kando/packages/media"
)

// TestSpokenLanguagesHaveNames checks that the spoken-language drop-down names every language whisper can be told to hear, rather than showing its code.
func TestSpokenLanguagesHaveNames(t *testing.T) {
	m := testlocale.Messages()
	seen := map[string]string{spokenLanguageName(m, ""): ""}
	for _, code := range media.SpeechLanguages() {
		name := spokenLanguageName(m, code)
		if name == code || name == "" {
			t.Fatalf("%q has no name", code)
		}
		if other, ok := seen[name]; ok {
			t.Fatalf("%q and %q are both named %q", code, other, name)
		}
		seen[name] = code
	}
}
