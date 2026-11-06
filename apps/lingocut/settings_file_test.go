package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"lingocut/apps/lingocut/film"
	"lingocut/packages/media"
)

// TestSavedSettingsKeysAreUnique checks that the areas' parts of settings.json do not share a key. encoding/json writes the fields of embedded structs side by side and, when two of them have the same key, drops both without an error.
func TestSavedSettingsKeysAreUnique(t *testing.T) {
	seen := map[string]string{}
	whole := reflect.TypeFor[savedSettings]()
	for i := range whole.NumField() {
		part := whole.Field(i)
		if !part.Anonymous || part.Type.Kind() != reflect.Struct || part.Tag.Get("json") != "" {
			t.Fatalf("savedSettings.%s is not an area's part embedded without a key", part.Name)
		}
		for j := range part.Type.NumField() {
			field := part.Type.Field(j)
			key, _, _ := strings.Cut(field.Tag.Get("json"), ",")
			if key == "-" {
				continue
			}
			if key == "" {
				key = field.Name
			}
			where := part.Name + "." + field.Name
			if other, ok := seen[key]; ok {
				t.Errorf("%s and %s are both saved as %q", other, where, key)
			}
			seen[key] = where
		}
	}
}

// TestStartupKeepsTheSettingsWhenALibraryWasRenamed checks that following a library renamed while the app was closed writes settings.json with every area's settings, not with the defaults the areas hold before they restore theirs.
func TestStartupKeepsTheSettingsWhenALibraryWasRenamed(t *testing.T) {
	for _, env := range []string{"LINGOCUT_OUTPUT_DIR", "LINGOCUT_LLM_BASE_URL", "LINGOCUT_LLM_MODEL", "LINGOCUT_LLM_API_KEY", "EVERYAPI_API_KEY", "LINGOCUT_R2_SECRET"} {
		t.Setenv(env, "")
	}
	parent := t.TempDir()
	current := filepath.Join(parent, "影视库.lingocut")
	old := filepath.Join(parent, "影视库 2.lingocut")
	renamed := filepath.Join(parent, "renamed.lingocut")
	for _, dir := range []string{current, renamed} {
		if err := os.Mkdir(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	path := filepath.Join(t.TempDir(), "settings.json")
	data, err := json.Marshal(map[string]any{
		"libraryDir":  current,
		"libraries":   []string{current, old},
		"language":    "日本語",
		"llmBaseURL":  "https://api.openai.com/v1",
		"llmAPIKey":   "sk-test",
		"googleEmail": "me@example.com",
		"r2Secret":    "secret",
		"subMode":     film.SubOriginal,
		"onboarded":   true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}

	a := newApp(media.Tools{}, func(func()) {})
	a.settingsPath = path
	a.restoreSettings(loadSavedSettings(path))

	saved := loadSavedSettings(path)
	if !containsPath(saved.Libraries, renamed) || containsPath(saved.Libraries, old) {
		t.Fatalf("libraries saved as %v, want the renamed one", saved.Libraries)
	}
	if saved.Language != "日本語" || saved.LLMAPIKey != "sk-test" || saved.GoogleEmail != "me@example.com" || saved.R2Secret != "secret" || saved.SubMode != film.SubOriginal || !saved.Onboarded {
		t.Fatalf("settings.json lost the areas' settings: language %q, key %q, account %q, secret %q, subMode %q, onboarded %v", saved.Language, saved.LLMAPIKey, saved.GoogleEmail, saved.R2Secret, saved.SubMode, saved.Onboarded)
	}
}
