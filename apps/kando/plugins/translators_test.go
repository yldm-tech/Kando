package plugins

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"
)

// translatorManifest is scriptManifest with two translators, one of any language and one from English and Japanese into Chinese, and a DeepL credential.
func translatorManifest() string {
	m := strings.Replace(scriptManifest(), `"contributes": {"commands": [`, `"contributes": {
    "translators": [
      {"id": "any", "title": {"en": "Any MT"}, "languages": "any"},
      {"id": "zh", "title": {"en": "Into Chinese", "zh-Hans": "译成中文"}, "languages": {"from": ["en", "ja"], "to": ["zh-Hans", "zh-Hant"]}}
    ],
    "commands": [`, 1)
	m = strings.Replace(m, `"kando.settings.get"]`, `"kando.settings.get", "kando.net.credentialFetch"]`, 1)
	return strings.Replace(m, `"events"`, `"credentials": [{"id": "deepl", "title": {"en": "DeepL API key"}, "header": "Authorization", "format": "DeepL-Auth-Key {secret}", "hosts": ["api-free.deepl.com", "api.deepl.com"], "methods": ["POST"], "pathPrefixes": ["/v2/translate", "/v2/usage"]}], "events"`, 1)
}

// TestValidateTranslators checks a script plugin's translators: their languages, written "any" or {from, to}, read back as they were written; the plugin must export translate; and what each rule refuses.
func TestValidateTranslators(t *testing.T) {
	p, err := Validate(writeScript(t, translatorManifest()))
	if err != nil {
		t.Fatal(err)
	}
	ts := p.Contributes.Translators
	if len(ts) != 2 || !ts[0].Languages.Any || ts[1].Languages.Any || strings.Join(ts[1].Languages.To, ",") != "zh-Hans,zh-Hant" {
		t.Fatalf("translators %+v", ts)
	}
	if got := p.ExportsNeeded(); !slices.Contains(got, TranslateExport) || slices.Contains(got, CandidatesExport) {
		t.Errorf("exports %v: translate is needed, candidates is not", got)
	}
	if !p.ReadsLines() {
		t.Error("a translator is given the lines")
	}
	data, err := json.Marshal(p.Contributes.Translators)
	if err != nil {
		t.Fatal(err)
	}
	if want := `"languages":"any"`; !strings.Contains(string(data), want) || !strings.Contains(string(data), `"languages":{"from":["en","ja"],"to":["zh-Hans","zh-Hant"]}`) {
		t.Errorf("written back as %s", data)
	}
	if !p.Candidater([]string{"translate", "candidates"}) || p.Candidater([]string{"translate"}) {
		t.Error("a plugin offers candidates when it exports them, and only then")
	}
	for _, tt := range []struct {
		name, from, to string
	}{
		{"a translator's id", `"id": "any"`, `"id": "Any MT"`},
		{"an id twice", `"id": "zh"`, `"id": "any"`},
		{"no title", `"title": {"en": "Any MT"}, `, ``},
		{"a title without English", `"title": {"en": "Any MT"}`, `"title": {"ja": "Any MT"}`},
		{"languages of another word", `"languages": "any"`, `"languages": "all"`},
		{"no languages", `, "languages": "any"`, ``},
		{"no languages to translate into", `"to": ["zh-Hans", "zh-Hant"]`, `"to": []`},
		{"a language that is not a tag", `"from": ["en", "ja"]`, `"from": ["English"]`},
		{"a language not in its canonical form", `"from": ["en", "ja"]`, `"from": ["EN"]`},
		{"another key", `"to": ["zh-Hans", "zh-Hant"]`, `"to": ["zh-Hans"], "via": ["en"]`},
		{"local, with a credential", `"languages": "any"}`, `"languages": "any", "local": true}`},
	} {
		manifest := strings.Replace(translatorManifest(), tt.from, tt.to, 1)
		if manifest == translatorManifest() {
			t.Fatalf("%s: %q is not in the manifest", tt.name, tt.from)
		}
		if _, err := Validate(writeScript(t, manifest)); err == nil {
			t.Errorf("%s: taken", tt.name)
		}
	}
	// A translator on this computer, in a plugin that reaches no network nor credential, is taken.
	local := strings.Replace(scriptManifest(), `"contributes": {"commands": [`, `"contributes": {"translators": [{"id": "here", "title": {"en": "Here"}, "languages": "any", "local": true}], "commands": [`, 1)
	local = strings.Replace(local, `,
    "network": {"http": [{"host": "127.0.0.1", "port": 8765, "schemes": ["http"], "methods": ["POST"], "paths": ["/"]}]}`, "", 1)
	if p, err := Validate(writeScript(t, local)); err != nil || !p.Contributes.Translators[0].Local {
		t.Errorf("a local translator: %v", err)
	}
}

// TestTranslatorTakes checks which languages a translator takes: any for one of any language; else a spoken language among its from, or not known, and a target among its to, a language being among them when one is the other without its script or region.
func TestTranslatorTakes(t *testing.T) {
	anyLang := Translator{Languages: TranslatorLanguages{Any: true}}
	some := Translator{Languages: TranslatorLanguages{From: []string{"en", "ja", "zh"}, To: []string{"zh-Hans", "pt-BR", "en"}}}
	for _, tc := range []struct {
		t        Translator
		from, to string
		want     bool
	}{
		{anyLang, "", "", true},
		{anyLang, "xx", "yy", true},
		{some, "ja", "zh-Hans", true},
		{some, "", "zh-Hans", true},
		{some, "zh", "en", true},
		{some, "ja", "zh", true},
		{some, "ja", "pt", true},
		{some, "ja", "zh-Hant", false},
		{some, "ko", "zh-Hans", false},
		{some, "ja", "", false},
	} {
		if got := tc.t.Takes(tc.from, tc.to); got != tc.want {
			t.Errorf("%+v takes %q → %q: %v", tc.t.Languages, tc.from, tc.to, got)
		}
	}
}

// TestTranslatedLines checks what Kando takes of a translator's answer: as many strings as lines, in order, cleaned; candidates as strings, at most eight, null as none.
func TestTranslatedLines(t *testing.T) {
	if got, ok := TranslatedLines([]any{"一", "二\n行", ""}, 3); !ok || strings.Join(got, "|") != "一|二 行|" {
		t.Errorf("three lines: %q %v", got, ok)
	}
	for _, bad := range []any{[]any{"一"}, []any{"一", 2, "三"}, "一二三", nil, []any{"一", "二", strings.Repeat("长", translateLineMax+1)}} {
		if _, ok := TranslatedLines(bad, 3); ok {
			t.Errorf("%v taken for three lines", bad)
		}
	}
	if got, ok := CandidateLines([]any{"a", " ", "b", "c", "d", "e", "f", "g", "h", "i"}); !ok || strings.Join(got, "") != "abcdefg" {
		t.Errorf("candidates %q %v", got, ok)
	}
	if got, ok := CandidateLines(nil); !ok || got != nil {
		t.Errorf("null: %q %v", got, ok)
	}
	if _, ok := CandidateLines([]any{"a", 1}); ok {
		t.Error("a number taken as a candidate")
	}
}

// TestTranslateError checks a translator's failure: a known code is kept and read back from its text, as library.json keeps it; another code is none; what the plugin said is cleaned and cut.
func TestTranslateError(t *testing.T) {
	e := NewTranslateError(TranslateQuota, "Quota\nExceeded")
	if e.Error() != "quota: Quota Exceeded" {
		t.Fatalf("text %q", e.Error())
	}
	if code, detail := TranslateCode(e.Error()); code != TranslateQuota || detail != "Quota Exceeded" {
		t.Errorf("read back %q %q", code, detail)
	}
	if code, _ := TranslateCode(NewTranslateError(TranslateNotRunning, "").Error()); code != TranslateNotRunning {
		t.Errorf("a code alone read back as %q", code)
	}
	if e := NewTranslateError("teapot", "I'm a teapot"); e.Code != "" || e.Error() != "I'm a teapot" {
		t.Errorf("an unknown code: %+v %q", e, e.Error())
	}
	if code, detail := TranslateCode("quotas are fine"); code != "" || detail != "quotas are fine" {
		t.Errorf("a text of no code read as %q %q", code, detail)
	}
	if long := NewTranslateError("", strings.Repeat("x", 500)); len([]rune(long.Detail)) != translateDetailMax {
		t.Errorf("not cut: %d", len([]rune(long.Detail)))
	}
}

// TestSpokenTag checks that whisper's codes become the tags translators name their languages by, so a translator from Norwegian Bokmål (nb) takes a film whisper heard as no.
func TestSpokenTag(t *testing.T) {
	for code, want := range map[string]string{"no": "nb", "jw": "jv", "JA": "ja", " en ": "en", "": ""} {
		if got := SpokenTag(code); got != want {
			t.Errorf("%q: %q, want %q", code, got, want)
		}
	}
	deepl := Translator{Languages: TranslatorLanguages{From: []string{"de", "nb"}, To: []string{"de", "nb"}}}
	if deepl.Takes("no", "de") || !deepl.Takes(SpokenTag("no"), "de") {
		t.Error("Norwegian as whisper writes it is taken only as nb")
	}
}
