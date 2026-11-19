package cli

import (
	"archive/zip"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"kando/apps/kando/plugins"
	"kando/apps/kando/plugins/script"
)

const pluginManifest = `{
  "schema": 1,
  "id": "example.terms",
  "kind": "glossary",
  "version": "1.0.0",
  "license": "MIT",
  "name": {"en": "Terms", "zh-Hans": "术语"},
  "glossary": {"from": "ja", "to": "en", "terms": [{"source": "先輩", "target": "senpai"}]}
}`

// pluginFolder writes a pack folder with manifest and returns it.
func pluginFolder(t *testing.T, manifest string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, plugins.ManifestName), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// pluginZip writes a zip of a pack with manifest and returns its path.
func pluginZip(t *testing.T, manifest string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "terms.zip")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	w := zip.NewWriter(f)
	entry, err := w.Create(plugins.ManifestName)
	if err == nil {
		_, err = entry.Write([]byte(manifest))
	}
	if err == nil {
		err = w.Close()
	}
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		t.Fatal(err)
	}
	return path
}

// TestPluginValidate checks that kando plugin validate checks a folder or a zip without the app, prints what the pack is, and fails with why when it is not valid.
func TestPluginValidate(t *testing.T) {
	shortSocket(t)
	noLaunch(t)
	for _, src := range []string{pluginFolder(t, pluginManifest), pluginZip(t, pluginManifest)} {
		code, stdout, stderr := run("plugin", "validate", src)
		if code != 0 || stderr != "" || !strings.Contains(stdout, "example.terms") || !strings.Contains(stdout, "glossary") || !strings.Contains(stdout, "Terms") {
			t.Errorf("validate %s = %d, %q, %q", src, code, stdout, stderr)
		}
	}
	code, stdout, _ := run("plugin", "validate", "--json", pluginZip(t, pluginManifest))
	var got pluginInfo
	if err := json.Unmarshal([]byte(stdout), &got); code != 0 || err != nil || got.ID != "example.terms" || got.Kind != "glossary" || len(got.Hash) != 64 || got.Enabled != nil {
		t.Errorf("validate --json = %d, %q, %v", code, stdout, err)
	}

	reserved := strings.Replace(pluginManifest, "example.terms", "kando.terms", 1)
	code, stdout, stderr := run("plugin", "validate", pluginFolder(t, reserved))
	if code != 1 || stdout != "" || !strings.Contains(stderr, "kando plugin:") || !strings.Contains(stderr, "reserved") {
		t.Errorf("validate a reserved id = %d, %q, %q", code, stdout, stderr)
	}
	// Every kind Kando knows is installable now, so a kind it does not know is the one refused for its kind.
	code, stdout, _ = run("plugin", "validate", "--json", pluginFolder(t, `{"schema": 1, "id": "example.x", "kind": "plugin"}`))
	if code != 1 || !strings.Contains(stdout, `"error"`) || !strings.Contains(stdout, "kind of pack is unknown") {
		t.Errorf("validate --json a pack of an unknown kind = %d, %q", code, stdout)
	}
	if code, _, stderr := run("plugin", "validate", filepath.Join(t.TempDir(), "missing.zip")); code != 1 || !strings.Contains(stderr, "missing.zip") {
		t.Errorf("validate a missing file = %d, %q", code, stderr)
	}
}

func TestPluginUsage(t *testing.T) {
	shortSocket(t)
	noLaunch(t)
	for _, args := range [][]string{{"plugin"}, {"plugin", "validate"}, {"plugin", "validate", "a", "b"}, {"plugin", "list", "extra"}, {"plugin", "install"}, {"plugin", "install", "a.zip", "b.zip"}, {"plugin", "install", t.TempDir()}, {"plugin", "enable", "a"}, {"plugin", "list", "--base", "https://example.org"}, {"plugin", "validate", "--out", "x.json", "a.zip"}} {
		if code, _, stderr := run(args...); code != 2 || !strings.Contains(stderr, "Usage: kando plugin (validate <file-or-folder> | list | install <file> | index <folder> --base <url> [--out <file>]) [--json]") {
			t.Errorf("%q = %d, %q", args, code, stderr)
		}
	}
}

// TestPluginList checks that kando plugin list lists the packs of the plugins folder with whether each is on, and says which folders are not valid packs.
func TestPluginList(t *testing.T) {
	shortSocket(t)
	noLaunch(t)
	root := t.TempDir()
	old := pluginsRoot
	pluginsRoot = func() string { return root }
	t.Cleanup(func() { pluginsRoot = old })

	code, stdout, stderr := run("plugin", "list")
	if code != 0 || stdout != "" || stderr != "No plugins.\n" {
		t.Errorf("list with no packs = %d, %q, %q", code, stdout, stderr)
	}

	for _, manifest := range []string{pluginManifest, strings.Replace(pluginManifest, "example.terms", "example.more", 1)} {
		staged, p, err := plugins.Stage(root, pluginFolder(t, manifest))
		if err != nil {
			t.Fatal(err)
		}
		if err := plugins.Swap(root, staged, &p); err != nil {
			t.Fatal(err)
		}
	}
	state := plugins.State{Format: plugins.StateFormat, Plugins: []plugins.PluginState{{ID: "example.more", Enabled: false}}}
	if err := state.Save(root); err != nil {
		t.Fatal(err)
	}
	broken := filepath.Join(root, "glossary", "example.broken")
	if err := os.MkdirAll(broken, 0o755); err != nil {
		t.Fatal(err)
	}

	code, stdout, stderr = run("plugin", "list")
	lines := strings.Split(strings.TrimSpace(stdout), "\n")
	if code != 0 || len(lines) != 3 || !strings.HasPrefix(lines[0], "ID") || !strings.Contains(lines[1], "example.more") || !strings.Contains(lines[1], "off") || !strings.Contains(lines[2], "example.terms") || !strings.Contains(lines[2], " on ") {
		t.Errorf("list = %d, %q", code, stdout)
	}
	if !strings.Contains(stderr, "example.broken") {
		t.Errorf("list did not report the broken pack: %q", stderr)
	}

	code, stdout, _ = run("plugin", "list", "--json")
	var got struct {
		Plugins []pluginInfo  `json:"plugins"`
		Issues  []pluginIssue `json:"issues"`
	}
	if err := json.Unmarshal([]byte(stdout), &got); code != 0 || err != nil || len(got.Plugins) != 2 || len(got.Issues) != 1 || got.Issues[0].Dir != broken {
		t.Fatalf("list --json = %d, %q, %v", code, stdout, err)
	}
	if p := got.Plugins[1]; p.ID != "example.terms" || p.Enabled == nil || !*p.Enabled || p.Dir != filepath.Join(root, "glossary", "example.terms") {
		t.Errorf("list --json plugin = %+v", p)
	}
}

// TestPluginInstall checks that kando plugin install checks a pack file and only drops it into the inbox of the plugins folder, installing nothing, and that a pack that is not valid never reaches the inbox.
func TestPluginInstall(t *testing.T) {
	shortSocket(t)
	noLaunch(t)
	root := t.TempDir()
	old := pluginsRoot
	pluginsRoot = func() string { return root }
	t.Cleanup(func() { pluginsRoot = old })

	code, stdout, stderr := run("plugin", "install", pluginZip(t, pluginManifest))
	if code != 0 || stderr != "" || !strings.Contains(stdout, "Terms 1.0.0") {
		t.Fatalf("install = %d, %q, %q", code, stdout, stderr)
	}
	inbox, err := plugins.Inbox(root)
	if err != nil || len(inbox) != 1 {
		t.Fatalf("inbox %q, %v", inbox, err)
	}
	if packs, _ := plugins.Scan(root); len(packs) != 0 {
		t.Errorf("install installed %d packs itself", len(packs))
	}

	code, stdout, _ = run("plugin", "install", "--json", pluginZip(t, pluginManifest))
	var got struct {
		ID    string `json:"id"`
		Inbox string `json:"inbox"`
	}
	if err := json.Unmarshal([]byte(stdout), &got); code != 0 || err != nil || got.ID != "example.terms" || filepath.Dir(got.Inbox) != filepath.Join(root, plugins.InboxName) {
		t.Errorf("install --json = %d, %q, %v", code, stdout, err)
	}

	reserved := strings.Replace(pluginManifest, "example.terms", "kando.terms", 1)
	if code, _, stderr := run("plugin", "install", pluginZip(t, reserved)); code != 1 || !strings.Contains(stderr, "reserved") {
		t.Errorf("install a reserved id = %d, %q", code, stderr)
	}
	if inbox, _ := plugins.Inbox(root); len(inbox) != 2 {
		t.Errorf("inbox after a refused install %q", inbox)
	}
}

// TestPluginValidateScript checks that validate verifies a script plugin's signature as the app does, against Kando's official root: one signed with a developer key, which the command line does not read from the system's secure store, is refused as signed by a key not trusted, after the checks every pack passes.
func TestPluginValidateScript(t *testing.T) {
	shortSocket(t)
	pluginsRoot = func() string { return filepath.Join(t.TempDir(), "plugins") }
	t.Cleanup(func() { pluginsRoot = plugins.DefaultRoot })
	secret, _, err := script.NewDevKey()
	if err != nil {
		t.Fatal(err)
	}
	key, _, err := script.DevKey(secret)
	if err != nil {
		t.Fatal(err)
	}
	src, dst := t.TempDir(), t.TempDir()
	fields := `"capabilities": {"host": ["kando.toast"]}`
	os.WriteFile(filepath.Join(src, plugins.ManifestName), []byte(`{"schema": 2, "id": "acme.hello", "kind": "script", "version": "0.1.0", "license": "MIT", "name": {"en": "Hello"}, `+fields+`}`), 0o644)
	os.WriteFile(filepath.Join(src, script.Entry), []byte("export function kandoManifest() { return {"+fields+"} }\n"), 0o644)
	if err := script.PackFolder(src, dst, key); err != nil {
		t.Fatal(err)
	}
	code, stdout, stderr := run("plugin", "validate", dst)
	if code != 1 || stdout != "" || !strings.Contains(stderr, "not signed by a key Kando trusts") {
		t.Errorf("validate a developer plugin = %d, %q, %q", code, stdout, stderr)
	}
}

// TestPluginValidateDataPackOpensNoTrust checks that kando plugin validate opens the plugins folder's trust only for a script plugin: a data pack passes even when that trust cannot be read.
func TestPluginValidateDataPackOpensNoTrust(t *testing.T) {
	shortSocket(t)
	noLaunch(t)
	root := filepath.Join(t.TempDir(), "plugins")
	if err := os.MkdirAll(script.TrustDir(root), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(script.TrustDir(root), "trust.json"), []byte("not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	old := pluginsRoot
	pluginsRoot = func() string { return root }
	t.Cleanup(func() { pluginsRoot = old })
	if code, stdout, stderr := run("plugin", "validate", pluginZip(t, pluginManifest)); code != 0 || !strings.Contains(stdout, "example.terms") {
		t.Errorf("validate a data pack with an unreadable trust = %d, %q, %q", code, stdout, stderr)
	}
}
