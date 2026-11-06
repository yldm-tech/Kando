package main

import (
	"reflect"
	"strings"
	"testing"
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
