package plugins

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/tsx/pkg"
)

// scriptPackage stands in for a script plugin's signed package: Validate checks only that it is the one the manifest names, by its hash.
const scriptPackage = "not really signed"

// scriptManifest is a script plugin's manifest naming scriptPackage.
func scriptManifest() string {
	sum := sha256.Sum256([]byte(scriptPackage))
	return `{
  "schema": 2,
  "id": "example.anki",
  "kind": "script",
  "version": "1.2.0",
  "kando": ">=0.30.0",
  "license": "MIT",
  "name": {"en": "Anki"},
  "script": {"package": "anki.nsgopkg", "sha256": "` + hex.EncodeToString(sum[:]) + `"},
  "capabilities": {
    "host": ["kando.saved.read", "kando.words.read", "kando.settings.get"],
    "network": {"http": [{"host": "127.0.0.1", "port": 8765, "schemes": ["http"], "methods": ["POST"], "paths": ["/"]}]}
  },
  "optional": ["kando.toast"],
  "contributes": {"commands": [{"id": "sync", "export": "run", "key": "mod+shift+k", "title": {"en": "Send to Anki"}}]},
  "settings": [
    {"key": "deck", "type": "text", "default": "Kando", "max": 100, "title": {"en": "Deck"}},
    {"key": "audio", "type": "bool", "default": true, "title": {"en": "Attach the sound"}},
    {"key": "model", "type": "choice", "choices": [{"id": "basic", "label": {"en": "Basic"}}, {"id": "cloze", "label": {"en": "Cloze"}}], "title": {"en": "Note type"}},
    {"key": "limit", "type": "number", "min": 1, "max": 50, "default": 20, "title": {"en": "Lines at once"}},
    {"key": "token", "type": "secret", "title": {"en": "Token"}}
  ],
  "events": ["film.opened", "job.completed"]
}`
}

// writeScript writes a script plugin folder of manifest and scriptPackage.
func writeScript(t *testing.T, manifest string) string {
	return writePack(t, map[string]string{ManifestName: manifest, "anki.nsgopkg": scriptPackage})
}

// TestValidateScript checks a script plugin's manifest as it is meant to be, and what each of its rules refuses.
func TestValidateScript(t *testing.T) {
	p, err := Validate(writeScript(t, scriptManifest()))
	if err != nil {
		t.Fatal(err)
	}
	d := p.Declared()
	if len(d.Perms) != 4 || !d.Has(PermToast) || len(d.Network) != 1 {
		t.Errorf("declared %+v", d)
	}
	for _, tt := range []struct {
		name, from, to string
		want           error
	}{
		{"schema 1", `"schema": 2`, `"schema": 1`, ErrSchema},
		{"kando", `">=0.30.0"`, `"0.30"`, ErrBadField},
		{"sha256", `"sha256": "`, `"sha256": "0`, ErrBadField},
		{"unknown permission", `"kando.saved.read"`, `"kando.glossary.suggest"`, ErrUnknownPerm},
		{"another capability", `"capabilities": {`, `"capabilities": {"clipboard": true,`, ErrBadCapabilities},
		{"open url", `"capabilities": {`, `"capabilities": {"openUrl": {"schemes": ["https"]},`, ErrBadCapabilities},
		{"any request", `"network": {"http"`, `"network": {"hosts": ["*"], "http"`, ErrBadCapabilities},
		{"no methods", `"methods": ["POST"], `, ``, ErrBadCapabilities},
		{"no paths", `, "paths": ["/"]`, ``, ErrBadCapabilities},
		{"a host pattern", `"host": "127.0.0.1"`, `"host": "*.example.com"`, ErrBadCapabilities},
		{"optional twice", `"optional": ["kando.toast"]`, `"optional": ["kando.saved.read"]`, ErrBadField},
		{"optional unknown", `"optional": ["kando.toast"]`, `"optional": ["kando.candidates.offer"]`, ErrUnknownPerm},
		{"another contribution", `"contributes": {`, `"contributes": {"widgets": [],`, ErrBadManifest},
		{"credentials without credentialFetch", `"events"`, `"credentials": [{"id": "k", "title": {"en": "Key"}, "header": "Authorization", "format": "Bearer {secret}", "hosts": ["api.example.com"], "methods": ["POST"], "pathPrefixes": ["/v1"]}], "events"`, ErrBadField},
		{"a bare key", `"mod+shift+k"`, `"k"`, ErrBadField},
		{"shift alone", `"mod+shift+k"`, `"shift+k"`, ErrBadField},
		{"a command's export", `"export": "run"`, `"export": "run me"`, ErrBadField},
		{"when", `"key": "mod+shift+k"`, `"key": "mod+shift+k", "when": "sometimes"`, ErrBadField},
		{"a setting's type", `"type": "bool"`, `"type": "color"`, ErrBadField},
		{"a default of another type", `"default": true`, `"default": "yes"`, ErrBadField},
		{"a choice not there", `"title": {"en": "Note type"}`, `"default": "other", "title": {"en": "Note type"}`, ErrBadField},
		{"a number out of bounds", `"default": 20`, `"default": 51`, ErrBadField},
		{"a secret's default", `"type": "secret"`, `"type": "secret", "default": "x"`, ErrBadField},
		{"an event", `"job.completed"`, `"line.played"`, ErrBadField},
		{"a data pack's field", `"events"`, `"glossary": {"from": "ja", "to": "en", "terms": [{"source": "a", "target": "b"}]}, "events"`, ErrBadField},
	} {
		manifest := strings.Replace(scriptManifest(), tt.from, tt.to, 1)
		if manifest == scriptManifest() {
			t.Fatalf("%s: %q is not in the manifest", tt.name, tt.from)
		}
		_, err := Validate(writeScript(t, manifest))
		wantErr(t, tt.name, err, tt.want)
	}
	// The package must be the one the manifest names.
	dir := writePack(t, map[string]string{ManifestName: scriptManifest(), "anki.nsgopkg": "another package"})
	_, err = Validate(dir)
	wantErr(t, "another package", err, ErrTampered)
	// A data pack has none of a script plugin's fields.
	_, err = Validate(writePack(t, map[string]string{ManifestName: strings.Replace(glossaryManifest, `"schema": 1,`, `"schema": 1, "events": ["film.opened"],`, 1)}))
	wantErr(t, "a glossary with events", err, ErrBadField)
	// A script plugin may be under kando.: the signature decides, not Validate.
	if _, err := Validate(writeScript(t, strings.Replace(scriptManifest(), "example.anki", "kando.anki", 1))); err != nil {
		t.Errorf("kando.anki: %v", err)
	}
}

// TestScriptSettingsResolve checks what kando.settings.get answers: the values set, the defaults of those not set or not valid, and never a secret.
func TestScriptSettingsResolve(t *testing.T) {
	p, err := Validate(writeScript(t, scriptManifest()))
	if err != nil {
		t.Fatal(err)
	}
	got := p.Resolve(Values{"deck": "Mining​", "limit": float64(99), "model": "cloze", "token": "s3cret"})
	want := map[string]any{"deck": "Mining", "audio": true, "model": "cloze", "limit": float64(20)}
	if len(got) != len(want) {
		t.Fatalf("resolved %v, want %v", got, want)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %v, want %v", k, got[k], v)
		}
	}
	root := t.TempDir()
	if err := SaveValues(root, "example.anki", Values{"deck": "x"}); err != nil {
		t.Fatal(err)
	}
	if v, err := LoadValues(root, "example.anki"); err != nil || v["deck"] != "x" {
		t.Errorf("loaded %v, %v", v, err)
	}
	if _, err := LoadValues(root, "../evil"); err == nil {
		t.Error("a path as an id")
	}
	if err := RemoveValues(root, "example.anki"); err != nil {
		t.Fatal(err)
	}
	if v, _ := LoadValues(root, "example.anki"); len(v) != 0 {
		t.Errorf("after removal %v", v)
	}
}

// TestScriptGrantSubsets checks the update rules of docs/plugin-system.md 6.2: an update that asks for no more keeps the grant; one that adds a permission, adds an HTTP rule or changes one in any field asks for more, and runs with what both allow.
func TestScriptGrantSubsets(t *testing.T) {
	rule := pkg.HTTPRule{Host: "127.0.0.1", Port: 8765, Methods: []string{"POST"}, Paths: []string{"/"}}
	before := Grant{Perms: []Perm{PermSavedRead, PermWordsRead}, Network: []pkg.HTTPRule{rule}}
	granted := Grant{Perms: []Perm{PermSavedRead}, Network: []pkg.HTTPRule{rule}}
	if g, widened := Regrant(granted, before, Grant{Perms: []Perm{PermSavedRead}, Network: []pkg.HTTPRule{rule}}); widened || len(g.Perms) != 1 || len(g.Network) != 1 {
		t.Errorf("a narrower update: %+v, widened %t", g, widened)
	}
	other := rule
	other.Paths = []string{"/other"}
	method := rule
	method.Methods = []string{"POST", "DELETE"}
	for name, now := range map[string]Grant{
		"a permission":     {Perms: []Perm{PermSavedRead, PermWordsRead, PermLinesSeen}, Network: []pkg.HTTPRule{rule}},
		"a rule":           {Perms: []Perm{PermSavedRead}, Network: []pkg.HTTPRule{rule, other}},
		"a rule's path":    {Perms: []Perm{PermSavedRead}, Network: []pkg.HTTPRule{other}},
		"a rule's methods": {Perms: []Perm{PermSavedRead}, Network: []pkg.HTTPRule{method}},
	} {
		g, widened := Regrant(granted, before, now)
		if !widened {
			t.Errorf("%s: not widened", name)
		}
		if !g.Within(granted) || !g.Within(now) {
			t.Errorf("%s: kept %+v", name, g)
		}
	}
}

// TestScriptEffective checks what a script plugin may do: nothing until it is turned on for its content and signer, what it was granted and declares, the permissions that need no grant, no network offline and no library while it is locked.
func TestScriptEffective(t *testing.T) {
	p, err := Validate(writeScript(t, scriptManifest()))
	if err != nil {
		t.Fatal(err)
	}
	p.Signer = "abc"
	var s State
	if g := s.Effective(p, false, false); len(g.Perms)+len(g.Network) != 0 || s.EnabledPack(p) {
		t.Errorf("never turned on: %+v", g)
	}
	grant := Grant{Perms: []Perm{PermSavedRead, PermLinesSeen}, Network: p.Declared().Network}
	s.Set(PluginState{ID: p.ID, Hash: p.Hash, Signer: "abc", Enabled: true, Grant: grant})
	g := s.Effective(p, false, false)
	if !g.Has(PermSavedRead) || g.Has(PermLinesSeen) || !g.Has(PermSettingsGet) || len(g.Network) != 1 || !s.EnabledPack(p) {
		t.Errorf("granted: %+v", g)
	}
	if g := s.Effective(p, true, true); g.Has(PermSavedRead) || !g.Has(PermSettingsGet) || len(g.Network) != 0 {
		t.Errorf("offline and locked: %+v", g)
	}
	other := p
	other.Signer = "def"
	if g := s.Effective(other, false, false); len(g.Perms) != 0 || s.EnabledPack(other) {
		t.Errorf("another signer: %+v", g)
	}
	// A state with grants is written in format 2, and read back as it was.
	root := t.TempDir()
	if err := s.Save(root); err != nil {
		t.Fatal(err)
	}
	back, err := LoadState(root)
	if err != nil || back.Format != 2 || !back.EnabledPack(p) || len(back.Effective(p, false, false).Network) != 1 {
		t.Errorf("read back %+v, %v", back, err)
	}
	// One without stays in format 1, which older versions read.
	data := State{Plugins: []PluginState{{ID: "example.jlpt", Hash: "1", Enabled: true}}}
	if err := data.Save(root); err != nil || data.Format != 1 {
		t.Errorf("a data pack's state in format %d, %v", data.Format, err)
	}
	if Risky(p.Manifest, Grant{Perms: []Perm{PermSavedRead}, Network: grant.Network}) || !Risky(p.Manifest, grant) {
		t.Error("risky")
	}
}

// TestScriptHealthAndInFlight checks health.json and the in-flight marks: a mark left is read once, and cleared marks are not.
func TestScriptHealthAndInFlight(t *testing.T) {
	root := t.TempDir()
	if err := MarkInFlight(root, "example.a", "h1"); err != nil {
		t.Fatal(err)
	}
	if err := MarkInFlight(root, "example.b", "h2"); err != nil {
		t.Fatal(err)
	}
	if err := ClearInFlight(root, "example.b"); err != nil {
		t.Fatal(err)
	}
	if err := MarkInFlight(root, "../x", "h"); err == nil {
		t.Error("a path as an id")
	}
	marks := InFlight(root)
	if len(marks) != 1 || marks["example.a"] != "h1" {
		t.Errorf("marks %v", marks)
	}
	if again := InFlight(root); len(again) != 0 {
		t.Errorf("read twice: %v", again)
	}
	hs := SetHealth(nil, Health{ID: "example.a", Hash: "h1", Reason: ReasonHung, At: time.Unix(1, 0)})
	if err := SaveHealth(root, hs); err != nil {
		t.Fatal(err)
	}
	back, err := LoadHealth(root)
	if err != nil {
		t.Fatal(err)
	}
	if h, ok := HealthOf(back, "example.a", "h1"); !ok || h.Reason != ReasonHung {
		t.Errorf("health %+v", back)
	}
	if _, ok := HealthOf(back, "example.a", "h2"); ok {
		t.Error("another content's health")
	}
	if hs := SetHealth(back, Health{ID: "example.a"}); len(hs) != 0 {
		t.Errorf("cleared %+v", hs)
	}
}

// TestScriptAudit checks that the audit log's lines go through the scrub function, one line each, and that the log moves aside past its size.
func TestScriptAudit(t *testing.T) {
	root := t.TempDir()
	scrub := func(s string) string { return strings.ReplaceAll(s, "/Users/someone", "~") }
	if err := Audit(root, scrub, time.Unix(0, 0), AuditGrant, "example.a", "perms /Users/someone/x\nnext"); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(AuditPath(root))
	if string(data) != "1970-01-01T00:00:00Z grant example.a perms ~/x next\n" {
		t.Errorf("audit.log %q", data)
	}
	big := strings.Repeat("x", auditMax)
	Audit(root, scrub, time.Unix(0, 0), AuditGrant, "example.a", big)
	Audit(root, scrub, time.Unix(0, 0), AuditGrant, "example.a", "after")
	if _, err := os.Stat(filepath.Join(root, auditName+".1")); err != nil {
		t.Errorf("not moved aside: %v", err)
	}
}

// TestScriptInstallAndGrantEntries checks the state.json entries a script plugin's install and a change of its grant start from: a first install, or one another key signed, is off and granted nothing; an update of the same signer keeps its switch and its grant as far as it still asks for it; a grant changed for a build another key signed starts from nothing, and one for a content the user never turned on keeps the grant but not the switch.
func TestScriptInstallAndGrantEntries(t *testing.T) {
	old := Pack{Manifest: Manifest{ID: "acme.a", Kind: KindScript, Capabilities: &pkg.Capabilities{Host: []string{"kando.saved.read", "kando.toast"}}}, Hash: "h1", Signer: "k1"}
	entry := PluginState{ID: "acme.a", Hash: "h1", Signer: "k1", Enabled: true, Grant: Grant{Perms: []Perm{PermSavedRead, PermToast}}}
	next := old
	next.Hash, next.Capabilities = "h2", &pkg.Capabilities{Host: []string{"kando.saved.read"}}
	if e := InstallEntry(next, &old, entry, true); !e.Enabled || len(e.Grant.Perms) != 1 || e.OldHash != "" || e.Hash != "h2" {
		t.Errorf("a narrower update: %+v", e)
	}
	other := next
	other.Signer = "k2"
	if e := InstallEntry(other, &old, entry, true); e.Enabled || len(e.Grant.Perms) != 0 || e.Signer != "k2" {
		t.Errorf("another signer: %+v", e)
	}
	if e := InstallEntry(next, nil, PluginState{}, false); e.Enabled || len(e.Grant.Perms) != 0 {
		t.Errorf("a first install: %+v", e)
	}
	s := State{Plugins: []PluginState{entry}}
	if e := s.GrantEntry(other); e.Enabled || len(e.Grant.Perms) != 0 || e.Signer != "k2" || e.Hash != "h2" {
		t.Errorf("a grant for a re-signed build: %+v", e)
	}
	if e := s.GrantEntry(next); e.Enabled || len(e.Grant.Perms) != 2 || e.Hash != "h2" {
		t.Errorf("a grant for a content never turned on: %+v", e)
	}
	if e := s.GrantEntry(old); !e.Enabled || len(e.Grant.Perms) != 2 {
		t.Errorf("a grant for the content turned on: %+v", e)
	}
}

// TestScriptScanCachesAndRechecks checks that Scan's hash cache takes a script plugin as it takes a data pack, and that a package changed on disk is read again and refused as not the one its manifest names.
func TestScriptScanCachesAndRechecks(t *testing.T) {
	root := t.TempDir()
	staged, p, err := Stage(root, writeScript(t, scriptManifest()))
	if err != nil {
		t.Fatal(err)
	}
	if err := Swap(root, staged, &p); err != nil {
		t.Fatal(err)
	}
	for i := range 2 {
		packs, issues := Scan(root)
		if len(packs) != 1 || len(issues) != 0 || packs[0].Hash != p.Hash || packs[0].Script == nil {
			t.Fatalf("scan %d: %+v, %+v", i, packs, issues)
		}
	}
	later := time.Now().Add(time.Minute)
	pkgPath := filepath.Join(p.Dir, "anki.nsgopkg")
	if err := os.WriteFile(pkgPath, []byte("not the package"), 0o644); err != nil {
		t.Fatal(err)
	}
	os.Chtimes(pkgPath, later, later)
	packs, issues := Scan(root)
	if len(packs) != 0 || len(issues) != 1 || !errors.Is(issues[0].Err, ErrTampered) {
		t.Errorf("a changed package: %+v, %+v", packs, issues)
	}
}
