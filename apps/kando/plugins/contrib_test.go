package plugins

import (
	"strings"
	"testing"
)

// contribManifest is scriptManifest with a contribution of every kind and a credential.
func contribManifest() string {
	m := strings.Replace(scriptManifest(), `"contributes": {"commands": [`, `"contributes": {
    "lookups": [{"id": "wiki", "export": "lookUp", "languages": ["ja"], "label": {"en": "Wiki"}}],
    "subtitleFormats": [{"id": "lrc", "export": "exportSubtitles", "extension": "lrc", "label": {"en": "LRC lyrics"}}],
    "exporters": [{"id": "notes", "export": "exportNotes", "takes": ["lines", "words"], "label": {"en": "Anki (AnkiConnect)"}}],
    "annotators": [{"id": "pitch", "export": "annotate", "label": {"en": "Pitch accent"}}],
    "commands": [`, 1)
	m = strings.Replace(m, `"kando.settings.get"]`, `"kando.settings.get", "kando.net.credentialFetch", "kando.llm.chat"]`, 1)
	return strings.Replace(m, `"events"`, `"credentials": [{"id": "deepl", "title": {"en": "DeepL API key"}, "header": "Authorization", "format": "DeepL-Auth-Key {secret}", "hosts": ["api-free.deepl.com", "api.deepl.com"], "methods": ["POST"], "pathPrefixes": ["/v2/translate"]}], "events"`, 1)
}

// TestValidateContributions checks the contributions besides commands and the credentials, and what each of their rules refuses.
func TestValidateContributions(t *testing.T) {
	p, err := Validate(writeScript(t, contribManifest()))
	if err != nil {
		t.Fatal(err)
	}
	if got := p.ExportsNeeded(); strings.Join(got, ",") != "run,lookUp,exportSubtitles,exportNotes,annotate" {
		t.Errorf("exports %v", got)
	}
	if _, err := Validate(writeScript(t, strings.Replace(scriptManifest(), `"kando.settings.get"]`, `"kando.settings.get", "kando.net.credentialFetch"]`, 1))); err == nil {
		t.Error("credentialFetch without credentials taken")
	}
	d := p.Declared()
	if len(d.Credentials) != 1 || d.Credentials[0].ID != "deepl" || !d.Has(PermCredFetch) {
		t.Errorf("declared %+v", d)
	}
	if !p.ReadsLines() || !Risky(p.Manifest, Grant{Perms: []Perm{PermCredFetch}}) || Risky(Manifest{}, Grant{Perms: []Perm{PermCredFetch}}) {
		t.Error("a lookup source with a credential is risky; a credential alone is not")
	}
	for _, tt := range []struct {
		name, from, to string
	}{
		{"a lookup's id", `"id": "wiki"`, `"id": "Wiki!"`},
		{"a lookup's export", `"export": "lookUp"`, `"export": "look up"`},
		{"a lookup's language", `"languages": ["ja"]`, `"languages": ["JA"]`},
		{"a lookup's label", `"label": {"en": "Wiki"}`, `"label": {"ja": "Wiki"}`},
		{"an extension with a dot", `"extension": "lrc"`, `"extension": ".lrc"`},
		{"an exporter that takes nothing", `"takes": ["lines", "words"]`, `"takes": []`},
		{"an exporter that takes films", `"takes": ["lines", "words"]`, `"takes": ["films"]`},
		{"lines twice", `"takes": ["lines", "words"]`, `"takes": ["lines", "lines"]`},
		{"an annotator's id", `"id": "pitch"`, `"id": ""`},
		{"a header Kando sets", `"header": "Authorization"`, `"header": "Cookie"`},
		{"a header that is not one", `"header": "Authorization"`, `"header": "Auth orization"`},
		{"a format without the secret", `"DeepL-Auth-Key {secret}"`, `"DeepL-Auth-Key"`},
		{"a format with it twice", `"DeepL-Auth-Key {secret}"`, `"{secret} {secret}"`},
		{"a line break", `"DeepL-Auth-Key {secret}"`, `"DeepL-Auth-Key {secret}\r\nX-Evil: 1"`},
		{"an IP", `"api.deepl.com"]`, `"10.0.0.1"]`},
		{"a wildcard", `"api.deepl.com"]`, `"*.deepl.com"]`},
		{"a public suffix", `"api.deepl.com"]`, `"co.uk"]`},
		{"a method", `"methods": ["POST"], "pathPrefixes"`, `"methods": ["CONNECT"], "pathPrefixes"`},
		{"no prefix", `"pathPrefixes": ["/v2/translate"]`, `"pathPrefixes": []`},
		{"a prefix that climbs", `"pathPrefixes": ["/v2/translate"]`, `"pathPrefixes": ["/v2/../admin"]`},
		{"credentials without credentialFetch", `"kando.net.credentialFetch", `, ``},
	} {
		manifest := strings.Replace(contribManifest(), tt.from, tt.to, 1)
		if manifest == contribManifest() {
			t.Fatalf("%s: %q is not in the manifest", tt.name, tt.from)
		}
		if _, err := Validate(writeScript(t, manifest)); err == nil {
			t.Errorf("%s: taken", tt.name)
		}
	}
}

// TestCredentialAllows checks where a granted credential's secret may go: its hosts, methods and paths under its prefixes by whole segments, and nowhere else.
func TestCredentialAllows(t *testing.T) {
	c := GrantedCredential{ID: "deepl", Header: "Authorization", Format: "DeepL-Auth-Key {secret}", Hosts: []string{"api.deepl.com"}, Methods: []string{"POST"}, PathPrefixes: []string{"/v2/translate", "/v3/"}}
	for _, tt := range []struct {
		method, host, path string
		want               bool
	}{
		{"POST", "api.deepl.com", "/v2/translate", true},
		{"POST", "API.deepl.com", "/v2/translate/x", true},
		{"POST", "api.deepl.com", "/v3/anything", true},
		{"POST", "api.deepl.com", "/v2/translated", false},
		{"POST", "api.deepl.com", "/v2", false},
		{"GET", "api.deepl.com", "/v2/translate", false},
		{"POST", "api.deepl.com.evil.com", "/v2/translate", false},
		{"POST", "deepl.com", "/v2/translate", false},
	} {
		if got := c.Allows(tt.method, tt.host, tt.path); got != tt.want {
			t.Errorf("%s %s%s: %t, want %t", tt.method, tt.host, tt.path, got, tt.want)
		}
	}
	if c.Value("k") != "DeepL-Auth-Key k" {
		t.Errorf("value %q", c.Value("k"))
	}
}

// TestCredentialGrantWidens checks docs/plugin-system.md 6.2 for credentials: an update that adds a host, changes the header or the format, or adds a method or a prefix asks for more, and the credential is not sent (it is not in old ∩ now); one that keeps it as it was keeps it.
func TestCredentialGrantWidens(t *testing.T) {
	base := GrantedCredential{ID: "deepl", Header: "Authorization", Format: "DeepL-Auth-Key {secret}", Hosts: []string{"api.deepl.com"}, Methods: []string{"POST"}, PathPrefixes: []string{"/v2/translate"}}
	before := Grant{Perms: []Perm{PermCredFetch}, Credentials: []GrantedCredential{base}}
	if g, widened := Regrant(before, before, before); widened || len(g.Credentials) != 1 {
		t.Errorf("the same: %+v, widened %t", g, widened)
	}
	change := func(f func(c *GrantedCredential)) Grant {
		c := base
		c.Hosts, c.Methods, c.PathPrefixes = append([]string{}, base.Hosts...), append([]string{}, base.Methods...), append([]string{}, base.PathPrefixes...)
		f(&c)
		return Grant{Perms: []Perm{PermCredFetch}, Credentials: []GrantedCredential{c}}
	}
	for name, now := range map[string]Grant{
		"a host":   change(func(c *GrantedCredential) { c.Hosts = append(c.Hosts, "evil.example.com") }),
		"a header": change(func(c *GrantedCredential) { c.Header = "X-Key" }),
		"a format": change(func(c *GrantedCredential) { c.Format = "Bearer {secret}" }),
		"a method": change(func(c *GrantedCredential) { c.Methods = append(c.Methods, "DELETE") }),
		"a prefix": change(func(c *GrantedCredential) { c.PathPrefixes = []string{"/"} }),
	} {
		g, widened := Regrant(before, before, now)
		if !widened || len(g.Credentials) != 0 {
			t.Errorf("%s: kept %+v, widened %t", name, g, widened)
		}
	}
	// Offline, no credential, no AI service.
	var s State
	p, err := Validate(writeScript(t, contribManifest()))
	if err != nil {
		t.Fatal(err)
	}
	p.Signer = "abc"
	s.Set(PluginState{ID: p.ID, Hash: p.Hash, Signer: "abc", Enabled: true, Grant: p.Declared()})
	if g := s.Effective(p, false, false); len(g.Credentials) != 1 || !g.Has(PermLLMChat) {
		t.Errorf("online %+v", g)
	}
	if g := s.Effective(p, true, false); len(g.Credentials) != 0 || g.Has(PermCredFetch) || g.Has(PermLLMChat) {
		t.Errorf("offline %+v", g)
	}
}

// TestCredentialMismatched checks the warning of a credential that names a well-known service but goes elsewhere.
func TestCredentialMismatched(t *testing.T) {
	c := Credential{ID: "deepl", Title: Text{"en": "DeepL key"}, Hosts: []string{"api.deepl.com"}}
	if c.Mismatched() != "" {
		t.Error("DeepL's own host flagged")
	}
	c.Hosts = append(c.Hosts, "collector.example.com")
	if c.Mismatched() != "deepl" {
		t.Error("another host not flagged")
	}
	if (Credential{ID: "key", Title: Text{"en": "My service"}, Hosts: []string{"x.example.com"}}).Mismatched() != "" {
		t.Error("a service of no name flagged")
	}
}
