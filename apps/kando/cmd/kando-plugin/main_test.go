package main

import (
	"archive/zip"
	"bytes"
	"crypto/ed25519"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"kando/apps/kando/plugins"
	"kando/apps/kando/plugins/index"
	"kando/apps/kando/plugins/script"
	"kando/apps/kando/plugins/script/signature"

	"github.com/nsgo-dev/nsgo/tsx/pkg"
)

const glossary = `{
  "schema": 1,
  "id": "example.terms",
  "kind": "glossary",
  "version": "1.0.0",
  "license": "MIT",
  "name": {"en": "Terms", "zh-Hans": "术语"},
  "description": {"en": "Ping @octocat, see #1 and [a link](https://evil.example) <img src=x> | pipe ` + "`tick`" + `"},
  "glossary": {"from": "ja", "to": "en", "terms": [{"source": "先輩", "target": "senpai"}]}
}`

// fixture is a test's tool, trusting root as Kando's official root, the index checked with indexKey's public half, made at a fixed time, its outputs in buffers.
type fixture struct {
	tool
	out, errOut *bytes.Buffer
	env         map[string]string
}

func newFixture(t *testing.T, root ed25519.PublicKey, indexKey ed25519.PublicKey) *fixture {
	t.Helper()
	f := &fixture{out: &bytes.Buffer{}, errOut: &bytes.Buffer{}, env: map[string]string{}}
	f.tool = tool{getenv: func(k string) string { return f.env[k] }, roots: signature.Roots{Official: root}, now: func() time.Time { return time.Date(2026, 11, 19, 10, 0, 0, 0, time.UTC) }, stdout: f.out, stderr: f.errOut}
	if indexKey != nil {
		f.indexPublicKey = pkg.EncodePublicKey(indexKey)
	}
	return f
}

// run runs args and returns the exit code, standard output and standard error, emptying them for the next run.
func (f *fixture) run(args ...string) (int, string, string) {
	code := f.tool.run(args)
	out, errOut := f.out.String(), f.errOut.String()
	f.out.Reset()
	f.errOut.Reset()
	return code, out, errOut
}

// folder writes a pack folder holding manifest and returns it.
func folder(t *testing.T, manifest string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, plugins.ManifestName), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// zipFolder zips the files of the folder dir, at its top, into path, and returns path.
func zipFolder(t *testing.T, dir, path string) string {
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
	return path
}

// scriptPlugin builds the script plugin acme.hello, which reads the lines and reaches the network, signed by a release key the root key certifies for acme.*, into the pack file path, and returns the root's public key.
func scriptPlugin(t *testing.T, path string) ed25519.PublicKey {
	t.Helper()
	secret, root, err := script.NewDevKey()
	if err != nil {
		t.Fatal(err)
	}
	key, _, _ := script.DevKey(secret)
	src, built := t.TempDir(), t.TempDir()
	fields := `"capabilities": {"host": ["kando.line.current"], "network": {"http": [{"host": "api.example.com", "methods": ["POST"], "pathPrefixes": ["/v1/"]}]}}`
	os.WriteFile(filepath.Join(src, plugins.ManifestName), []byte(`{"schema": 2, "id": "acme.hello", "kind": "script", "version": "0.1.0", "kando": ">=0.30.0", "license": "MIT", "name": {"en": "Hello", "ja": "こんにちは"}, `+fields+`, "optional": ["kando.toast"]}`), 0o644)
	os.WriteFile(filepath.Join(src, script.Entry), []byte("export function kandoManifest() { return {"+fields+`, "optional": ["kando.toast"]} }`+"\n"), 0o644)
	if err := script.PackFolder(src, built, key); err != nil {
		t.Fatal(err)
	}
	zipFolder(t, built, path)
	return root
}

// TestValidate checks validate on a pack file, a folder, a script plugin certified by the root it trusts, one it does not, and a pack that is not valid: a line each, or JSON with a stable code for each failure, and exit code 1 when one failed, after checking them all.
func TestValidate(t *testing.T) {
	dir := t.TempDir()
	terms := zipFolder(t, folder(t, glossary), filepath.Join(dir, "terms.zip"))
	hello := filepath.Join(dir, "hello.kandoplugin")
	root := scriptPlugin(t, hello)
	f := newFixture(t, root, nil)

	code, out, errOut := f.run("validate", terms, folder(t, glossary), hello)
	if code != exitOK || errOut != "" || strings.Count(out, "ok ") != 3 || !strings.Contains(out, "script acme.hello 0.1.0") {
		t.Fatalf("validate = %d, %q, %q", code, out, errOut)
	}

	reserved := folder(t, strings.Replace(glossary, "example.terms", "kando.terms", 1))
	code, out, _ = f.run("validate", "--json", terms, reserved, hello, filepath.Join(dir, "missing.zip"))
	var got struct {
		OK    bool        `json:"ok"`
		Packs []validated `json:"packs"`
	}
	if err := json.Unmarshal([]byte(out), &got); code != exitFailed || err != nil || got.OK || len(got.Packs) != 4 {
		t.Fatalf("validate --json = %d, %q, %v", code, out, err)
	}
	if p := got.Packs[0]; !p.OK || p.ID != "example.terms" || p.Kind != "glossary" || len(p.SHA256) != 64 || p.Size == 0 || p.Name != "Terms" {
		t.Errorf("the glossary %+v", p)
	}
	if p := got.Packs[1]; p.OK || p.Code != "reserved-id" || p.Error == "" {
		t.Errorf("the reserved id %+v", p)
	}
	if p := got.Packs[2]; !p.OK || p.Source != string(plugins.SourceVerified) || p.Signer == "" {
		t.Errorf("the script plugin %+v", p)
	}
	if p := got.Packs[3]; p.OK || p.Code != "not-found" {
		t.Errorf("a missing file %+v", p)
	}

	// Under another root, the script plugin is not trusted.
	other := newFixture(t, signature.DefaultRoots(nil).Official, nil)
	if code, out, _ := other.run("validate", "--json", hello); code != exitFailed || !strings.Contains(out, `"code": "untrusted"`) {
		t.Errorf("an untrusted script plugin = %d, %q", code, out)
	}
}

// TestUsage checks the usage errors, which exit with 2.
func TestUsage(t *testing.T) {
	f := newFixture(t, nil, nil)
	for _, args := range [][]string{{}, {"lint"}, {"validate"}, {"preview"}, {"preview", "--markdown=false", "a.zip"}, {"index"}, {"index", "a", "b"}, {"validate", "--base", "x", "a.zip"}} {
		if code, _, errOut := f.run(args...); code != exitUsage || !strings.Contains(errOut, "usage:") {
			t.Errorf("%q = %d, %q", args, code, errOut)
		}
	}
}

// TestPreview checks the Markdown preview: each pack's id, kind, names in each language, version, size, license, languages and signature, what a script plugin asks for and the line flagging it, a pack that failed with why; and that what a pack says cannot mention anyone, link or add HTML, as it is written in code spans.
func TestPreview(t *testing.T) {
	dir := t.TempDir()
	terms := zipFolder(t, folder(t, glossary), filepath.Join(dir, "terms.zip"))
	hello := filepath.Join(dir, "hello.kandoplugin")
	root := scriptPlugin(t, hello)
	f := newFixture(t, root, nil)
	reserved := folder(t, strings.Replace(glossary, "example.terms", "kando.terms", 1))

	code, out, _ := f.run("preview", "--markdown", terms, hello, reserved)
	if code != exitFailed {
		t.Errorf("a pack failed, exit %d", code)
	}
	for _, want := range []string{
		"2 of 3 packs passed.",
		"### ` example.terms `",
		"| Kind | ` glossary ` |",
		"` en ` ` Terms `<br>` zh-Hans ` ` 术语 `",
		"| Version | ` 1.0.0 ` |",
		"| License | ` MIT ` |",
		"| Languages | ` ja `, ` en ` |",
		"a glossary of 1 terms, ` ja ` into ` en `",
		"### ` acme.hello `",
		"| Signature | ` verified `, release key ",
		"| Needs Kando | ` 0.30.0 ` or newer |",
		"permissions: ` kando.line.current `",
		"optional permissions: ` kando.toast `",
		"network: ` POST https://api.example.com /v1/… `",
		"> [!WARNING]",
		"the combination Kando flags in its install confirmation",
		"### Failed: ",
		"**failed** (reserved-id)",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("no %q in\n%s", want, out)
		}
	}
	// The summary's mention, reference, link and HTML stay inside one code span, its backticks fenced by two and its pipe escaped.
	if !strings.Contains(out, "`` Ping @octocat, see #1 and [a link](https://evil.example) <img src=x> \\| pipe `tick` ``") {
		t.Errorf("the summary is not one code span:\n%s", out)
	}
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, "@octocat") && !strings.Contains(line, "`` Ping @octocat") {
			t.Errorf("a mention outside a code span: %q", line)
		}
	}
}

// TestCode checks the code spans the preview writes what a pack says in.
func TestCode(t *testing.T) {
	for in, want := range map[string]string{
		"plain":            "` plain `",
		"a`b":              "`` a`b ``",
		"``x":              "``` ``x ```",
		"two\nlines\r\tok": "` two lines ok `",
		"":                 "(empty)",
	} {
		if got := code(in); got != want {
			t.Errorf("code(%q) = %q, want %q", in, got, want)
		}
	}
	if got := cell("a|b"); got != "` a\\|b `" {
		t.Errorf("cell = %q", got)
	}
	if got := []rune(code(strings.Repeat("x", 1000))); len(got) > previewTextMax+6 {
		t.Errorf("a long text is %d characters", len(got))
	}
}

// TestIndex checks index: the pack files of a folder listed and signed with the key in KANDO_PLUGINS_INDEX_KEY, as Kando reads it; no key, another key, no --base and an http one refused before anything is written.
func TestIndex(t *testing.T) {
	packs := t.TempDir()
	zipFolder(t, folder(t, glossary), filepath.Join(packs, "terms.zip"))
	root := scriptPlugin(t, filepath.Join(packs, "hello 1.kandoplugin"))
	key, _ := pkg.GenerateKey()
	public := key.Public().(ed25519.PublicKey)
	f := newFixture(t, root, public)
	out := filepath.Join(t.TempDir(), "index.json")

	if code, _, errOut := f.run("index", packs, "--base", "https://kando.yldm.ai/plugin/packs/", "--out", out); code != exitFailed || !strings.Contains(errOut, "KANDO_PLUGINS_INDEX_KEY is not set") {
		t.Errorf("no key = %d, %q", code, errOut)
	}
	other, _ := pkg.GenerateKey()
	f.env["KANDO_PLUGINS_INDEX_KEY"] = pkg.EncodeSecretKey(other)
	if code, _, errOut := f.run("index", packs, "--base", "https://kando.yldm.ai/plugin/packs/", "--out", out); code != exitFailed || !strings.Contains(errOut, "another key") {
		t.Errorf("another key = %d, %q", code, errOut)
	}
	f.env["KANDO_PLUGINS_INDEX_KEY"] = pkg.EncodeSecretKey(key)
	if code, _, _ := f.run("index", packs, "--out", out); code != exitUsage {
		t.Errorf("no --base = %d", code)
	}
	if code, _, errOut := f.run("index", packs, "--base", "http://kando.yldm.ai/plugin/packs/", "--out", out); code != exitFailed || !strings.Contains(errOut, "not https") {
		t.Errorf("an http --base = %d, %q", code, errOut)
	}
	if _, err := os.Stat(out); err == nil {
		t.Fatal("an index was written")
	}

	code, stdout, errOut := f.run("index", "--json", packs, "--base", "https://kando.yldm.ai/plugin/packs/", "--out", out)
	var got struct {
		Out     string        `json:"out"`
		Signer  string        `json:"signer"`
		Entries []index.Entry `json:"entries"`
	}
	if err := json.Unmarshal([]byte(stdout), &got); code != exitOK || err != nil || got.Signer != pkg.KeyID(public) || len(got.Entries) != 2 {
		t.Fatalf("index --json = %d, %q, %q, %v", code, stdout, errOut, err)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	idx, err := index.Parse(data, public)
	if err != nil || len(idx.Entries) != 2 || idx.Entries[0].ID != "acme.hello" || idx.Entries[0].URL != "https://kando.yldm.ai/plugin/packs/hello%201.kandoplugin" || idx.Entries[1].ID != "example.terms" {
		t.Fatalf("Parse = %+v, %v", idx, err)
	}
	if !idx.Generated.Equal(f.now()) {
		t.Errorf("made %v", idx.Generated)
	}
}
