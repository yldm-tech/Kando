package cli

import (
	"archive/zip"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"kando/apps/kando/plugins"
	"kando/apps/kando/plugins/index"
	"kando/apps/kando/plugins/script"

	"github.com/nsgo-dev/nsgo/tsx/pkg"
)

// zipFolder zips the files of the folder dir, at its top, into path.
func zipFolder(t *testing.T, dir, path string) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	w := zip.NewWriter(f)
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		out, err := w.Create(e.Name())
		if err == nil {
			_, err = out.Write(data)
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
}

// indexKey puts a new index key's secret in IndexKeyEnv for the test and returns its public key.
func indexKey(t *testing.T) ed25519.PublicKey {
	t.Helper()
	key, err := pkg.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv(IndexKeyEnv, pkg.EncodeSecretKey(key))
	public := key.Public().(ed25519.PublicKey)
	indexPublicKey = func() string { return pkg.EncodePublicKey(public) }
	t.Cleanup(func() { indexPublicKey = func() string { return script.IndexKey } })
	return public
}

// TestPluginIndex checks that kando plugin index lists every pack file of a folder, a data pack and a script plugin certified by the root it trusts, with the size, SHA-256 and address of each file, signed with the key in KANDO_PLUGINS_INDEX_KEY, as Kando reads it (index.Parse).
func TestPluginIndex(t *testing.T) {
	shortSocket(t)
	public := indexKey(t)
	indexNow = func() time.Time { return time.Date(2026, 11, 19, 10, 0, 0, 0, time.UTC) }
	t.Cleanup(func() { indexNow = time.Now })
	// A key standing in for Kando's official root certifies the script plugin's publisher, as Kando's would.
	secret, root, err := script.NewDevKey()
	if err != nil {
		t.Fatal(err)
	}
	key, _, _ := script.DevKey(secret)
	indexRoots = func() script.Roots { return script.Roots{Official: root} }
	t.Cleanup(func() { indexRoots = func() script.Roots { return script.DefaultRoots(nil) } })

	packs := t.TempDir()
	zipFolder(t, pluginFolder(t, pluginManifest), filepath.Join(packs, "terms.zip"))
	src, built := t.TempDir(), t.TempDir()
	fields := `"capabilities": {"host": ["kando.toast"]}`
	os.WriteFile(filepath.Join(src, plugins.ManifestName), []byte(`{"schema": 2, "id": "acme.hello", "kind": "script", "version": "0.1.0", "kando": ">=0.30.0", "license": "MIT", "name": {"en": "Hello"}, `+fields+`}`), 0o644)
	os.WriteFile(filepath.Join(src, script.Entry), []byte("export function kandoManifest() { return {"+fields+"} }\n"), 0o644)
	if err := script.PackFolder(src, built, key); err != nil {
		t.Fatal(err)
	}
	zipFolder(t, built, filepath.Join(packs, "hello 1.kandoplugin"))
	os.WriteFile(filepath.Join(packs, ".DS_Store"), []byte("x"), 0o644)

	out := filepath.Join(t.TempDir(), "index.json")
	code, stdout, stderr := run("plugin", "index", packs, "--base", "https://plugins.example.org/packs/", "--out", out)
	if code != 0 || !strings.Contains(stdout, "acme.hello") || !strings.Contains(stdout, "example.terms") || !strings.Contains(stderr, "2 packs") {
		t.Fatalf("index = %d, %q, %q", code, stdout, stderr)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	idx, err := index.Parse(data, public)
	if err != nil || len(idx.Entries) != 2 {
		t.Fatalf("Parse = %+v, %v", idx, err)
	}
	hello, terms := idx.Entries[0], idx.Entries[1]
	zipped, _ := os.ReadFile(filepath.Join(packs, "terms.zip"))
	sum := sha256.Sum256(zipped)
	if terms.ID != "example.terms" || terms.Source != "" || terms.URL != "https://plugins.example.org/packs/terms.zip" || terms.Size != int64(len(zipped)) || terms.SHA256 != hex.EncodeToString(sum[:]) || strings.Join(terms.Languages, ",") != "ja,en" {
		t.Errorf("the glossary's entry %+v", terms)
	}
	if hello.ID != "acme.hello" || hello.Kind != plugins.KindScript || hello.Source != plugins.SourceVerified || hello.MinKando != "0.30.0" || hello.URL != "https://plugins.example.org/packs/hello%201.kandoplugin" || hello.Publisher != "acme" {
		t.Errorf("the script plugin's entry %+v", hello)
	}
	if !idx.Generated.Equal(indexNow()) {
		t.Errorf("made %v", idx.Generated)
	}

	// Under Kando's own root alone, the plugin certified by another key is refused, and nothing is written.
	indexRoots = func() script.Roots { return script.DefaultRoots(nil) }
	os.Remove(out)
	if code, _, stderr := run("plugin", "index", packs, "--base", "https://plugins.example.org/packs", "--out", out); code != 1 || !strings.Contains(stderr, "hello 1.kandoplugin") || !strings.Contains(stderr, "not signed by a key Kando trusts") {
		t.Errorf("an untrusted script plugin = %d, %q", code, stderr)
	}
	if _, err := os.Stat(out); err == nil {
		t.Error("an index was written")
	}
}

// TestPluginIndexRefuses checks what kando plugin index refuses before it writes anything: no key in the environment, no --base or an http one, a folder holding something other than pack files, a pack that is not valid, and an id in two files.
func TestPluginIndexRefuses(t *testing.T) {
	shortSocket(t)
	packs := t.TempDir()
	zipFolder(t, pluginFolder(t, pluginManifest), filepath.Join(packs, "terms.zip"))
	out := filepath.Join(t.TempDir(), "index.json")
	t.Setenv(IndexKeyEnv, "")
	if code, _, stderr := run("plugin", "index", packs, "--base", "https://plugins.example.org", "--out", out); code != 1 || !strings.Contains(stderr, IndexKeyEnv) {
		t.Errorf("no key = %d, %q", code, stderr)
	}
	t.Setenv(IndexKeyEnv, "not a key")
	if code, _, stderr := run("plugin", "index", packs, "--base", "https://plugins.example.org", "--out", out); code != 1 || !strings.Contains(stderr, IndexKeyEnv) {
		t.Errorf("a bad key = %d, %q", code, stderr)
	}
	// A key that is not the one Kando checks with signs nothing.
	indexKey(t)
	other, _ := pkg.GenerateKey()
	t.Setenv(IndexKeyEnv, pkg.EncodeSecretKey(other))
	if code, _, stderr := run("plugin", "index", packs, "--base", "https://plugins.example.org", "--out", out); code != 1 || !strings.Contains(stderr, "script.IndexKey") {
		t.Errorf("another key = %d, %q", code, stderr)
	}
	indexKey(t)
	if code, _, _ := run("plugin", "index", packs, "--out", out); code != 2 {
		t.Errorf("no --base = %d", code)
	}
	if code, _, stderr := run("plugin", "index", packs, "--base", "http://plugins.example.org", "--out", out); code != 1 || !strings.Contains(stderr, "not https") {
		t.Errorf("an http --base = %d, %q", code, stderr)
	}
	if code, _, _ := run("plugin", "index", "--base", "https://plugins.example.org"); code != 2 {
		t.Errorf("no folder = %d", code)
	}
	// After "--", index is still the subcommand, and what follows it positional.
	if code, _, stderr := run("plugin", "--", "index", packs); code != 2 || !strings.Contains(stderr, "--base") {
		t.Errorf("plugin -- index = %d, %q", code, stderr)
	}

	twice := t.TempDir()
	zipFolder(t, pluginFolder(t, pluginManifest), filepath.Join(twice, "a.zip"))
	zipFolder(t, pluginFolder(t, pluginManifest), filepath.Join(twice, "b.kandopack"))
	if code, _, stderr := run("plugin", "index", twice, "--base", "https://plugins.example.org", "--out", out); code != 1 || !strings.Contains(stderr, "b.kandopack") {
		t.Errorf("an id twice = %d, %q", code, stderr)
	}
	stray := t.TempDir()
	zipFolder(t, pluginFolder(t, pluginManifest), filepath.Join(stray, "a.zip"))
	os.WriteFile(filepath.Join(stray, "notes.txt"), []byte("x"), 0o644)
	if code, _, stderr := run("plugin", "index", stray, "--base", "https://plugins.example.org", "--out", out); code != 1 || !strings.Contains(stderr, "notes.txt") {
		t.Errorf("another file = %d, %q", code, stderr)
	}
	bad := t.TempDir()
	zipFolder(t, pluginFolder(t, strings.Replace(pluginManifest, `"license": "MIT"`, `"license": ""`, 1)), filepath.Join(bad, "bad.zip"))
	if code, _, stderr := run("plugin", "index", bad, "--base", "https://plugins.example.org", "--out", out); code != 1 || !strings.Contains(stderr, "bad.zip") {
		t.Errorf("an invalid pack = %d, %q", code, stderr)
	}
	if _, err := os.Stat(out); err == nil {
		t.Error("an index was written")
	}
}
