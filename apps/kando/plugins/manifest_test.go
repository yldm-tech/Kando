package plugins

import (
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	m, err := Parse([]byte(glossaryManifest))
	if err != nil {
		t.Fatal(err)
	}
	if m.ID != "example.anime-ja-zh" || m.Kind != KindGlossary || m.Glossary == nil || len(m.Glossary.Terms) != 1 || m.Name["zh-Hans"] != "动画常用词" {
		t.Errorf("Parse = %+v", m)
	}
}

// TestParseRejectsUnknownKeys checks that a key the format does not have, at any level, refuses the whole manifest.
func TestParseRejectsUnknownKeys(t *testing.T) {
	for name, data := range map[string]string{
		"top level":  strings.Replace(glossaryManifest, `"schema": 1,`, `"schema": 1, "extra": true,`, 1),
		"nested":     strings.Replace(glossaryManifest, `"from": "ja",`, `"from": "ja", "regex": ".*",`, 1),
		"in a term":  strings.Replace(glossaryManifest, `"target": "前辈"`, `"target": "前辈", "locked": true`, 1),
		"not JSON":   `{"schema": 1, "kind": "glossary"`,
		"two values": glossaryManifest + `{}`,
		"wrong type": strings.Replace(glossaryManifest, `"version": "1.2.0"`, `"version": 1`, 1),
	} {
		_, err := Parse([]byte(data))
		wantErr(t, name, err, ErrBadManifest)
	}
}

func TestParseKinds(t *testing.T) {
	_, err := Parse([]byte(`{"schema": 1, "id": "a.b", "kind": "plugin"}`))
	wantErr(t, "unknown kind", err, ErrUnknownKind)
	// A kind known but not implemented yet is read as far as its header, its other keys not checked, as they are only defined in its phase.
	m, err := Parse([]byte(`{"schema": 2, "id": "kando.ankiconnect", "kind": "script", "capabilities": {"host": []}, "script": {"package": "a.nsgopkg"}}`))
	if err != nil || m.Kind != KindScript || m.ID != "kando.ankiconnect" || m.Schema != 2 {
		t.Errorf("script = %+v, %v", m, err)
	}
	_, err = Parse([]byte(strings.Repeat(" ", manifestMax+1)))
	wantErr(t, "too large", err, ErrTooLarge)
}

func TestTextIn(t *testing.T) {
	text := Text{"en": "Terms", "zh-Hant": "術語", "ja": "用語"}
	for lang, want := range map[string]string{
		"ja":      "用語",
		"zh-Hant": "術語",
		// zh-TW's parent is zh-Hant.
		"zh-TW":   "術語",
		"zh-Hans": "Terms",
		"fr":      "Terms",
		"en-AU":   "Terms",
		"":        "Terms",
	} {
		if got := text.In(lang); got != want {
			t.Errorf("In(%q) = %q, want %q", lang, got, want)
		}
	}
}
