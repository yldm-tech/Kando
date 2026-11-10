package settings

import (
	"testing"

	"kando/packages/media"
)

// TestSpokenLanguagesHaveNames checks that the spoken-language drop-down names every language whisper can be told to hear, rather than showing its code.
func TestSpokenLanguagesHaveNames(t *testing.T) {
	seen := map[string]string{spokenLanguageName(""): ""}
	for _, code := range media.SpeechLanguages() {
		name := spokenLanguageName(code)
		if name == code || name == "" {
			t.Fatalf("%q has no name", code)
		}
		if other, ok := seen[name]; ok {
			t.Fatalf("%q and %q are both named %q", code, other, name)
		}
		seen[name] = code
	}
}
