package main

import (
	"encoding"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"kando/apps/kando/film"
	"kando/packages/media"
)

// TestSavedSettingsKeysAreUnique checks that the areas' parts of settings.json do not share a key. encoding/json writes the fields of embedded structs side by side and, when two of them have the same key, drops both without an error. A struct embedded in a part is written the same way, so its keys count too.
func TestSavedSettingsKeysAreUnique(t *testing.T) {
	seen := map[string]string{}
	whole := reflect.TypeFor[savedSettings]()
	for i := range whole.NumField() {
		part := whole.Field(i)
		if !part.Anonymous || part.Type.Kind() != reflect.Struct || part.Tag.Get("json") != "" {
			t.Fatalf("savedSettings.%s is not an area's part embedded without a key", part.Name)
		}
		savedKeys(t, part.Type, part.Name, seen)
	}
}

// savedKeys records the JSON keys of the struct typ, the fields of the structs it embeds without a key included.
func savedKeys(t *testing.T, typ reflect.Type, where string, seen map[string]string) {
	t.Helper()
	for i := range typ.NumField() {
		field := typ.Field(i)
		key, _, _ := strings.Cut(field.Tag.Get("json"), ",")
		if key == "-" || !field.IsExported() && !field.Anonymous {
			continue
		}
		if field.Anonymous && key == "" && field.Type.Kind() == reflect.Struct {
			savedKeys(t, field.Type, where+"."+field.Name, seen)
			continue
		}
		if key == "" {
			key = field.Name
		}
		at := where + "." + field.Name
		if other, ok := seen[key]; ok {
			t.Errorf("%s and %s are both saved as %q", other, at, key)
		}
		seen[key] = at
	}
}

// TestSavedSettingsPartsHaveNoMarshalers checks that no part of settings.json encodes itself. Go promotes an embedded struct's methods, so a MarshalJSON or UnmarshalJSON on one part, or on a struct embedded in it, would encode or decode the whole of settings.json and drop every other area's keys without an error.
func TestSavedSettingsPartsHaveNoMarshalers(t *testing.T) {
	marshalers := []reflect.Type{
		reflect.TypeFor[json.Marshaler](),
		reflect.TypeFor[json.Unmarshaler](),
		reflect.TypeFor[encoding.TextMarshaler](),
		reflect.TypeFor[encoding.TextUnmarshaler](),
	}
	types := []reflect.Type{reflect.TypeFor[savedSettings]()}
	for i := 0; i < len(types); i++ {
		for j := range types[i].NumField() {
			if field := types[i].Field(j); field.Anonymous && field.Type.Kind() == reflect.Struct {
				types = append(types, field.Type)
			}
		}
	}
	for _, typ := range types {
		for _, m := range marshalers {
			if typ.Implements(m) || reflect.PointerTo(typ).Implements(m) {
				t.Errorf("%s implements %s, so it encodes settings.json in place of the parts", typ, m)
			}
		}
	}
}

// TestStartupKeepsTheSettingsWhenALibraryWasRenamed checks that following a library renamed while the app was closed writes settings.json with every area's settings, not with the defaults the areas hold before they restore theirs.
func TestStartupKeepsTheSettingsWhenALibraryWasRenamed(t *testing.T) {
	for _, env := range []string{"KANDO_OUTPUT_DIR", "KANDO_LLM_BASE_URL", "KANDO_LLM_MODEL", "KANDO_LLM_API_KEY", "EVERYAPI_API_KEY", "KANDO_R2_SECRET"} {
		t.Setenv(env, "")
	}
	parent := t.TempDir()
	current := filepath.Join(parent, "影视库.kando")
	old := filepath.Join(parent, "影视库 2.kando")
	renamed := filepath.Join(parent, "renamed.kando")
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
