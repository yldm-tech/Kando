package main

import (
	"archive/zip"
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
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
	return scriptPluginCode(t, path, "")
}

// scriptPluginCode is scriptPlugin with code after kandoManifest in the plugin's entry.
func scriptPluginCode(t *testing.T, path, code string) ed25519.PublicKey {
	t.Helper()
	key, _ := signedScriptPlugin(t, path, code)
	return key.Public().(ed25519.PublicKey)
}

// signedScriptPlugin is scriptPluginCode, returning the root's secret key, which a test signs revocation lists with, and the SHA-256 of the plugin's package, in hex, as a list revokes it.
func signedScriptPlugin(t *testing.T, path, code string) (ed25519.PrivateKey, string) {
	t.Helper()
	secret, _, err := script.NewDevKey()
	if err != nil {
		t.Fatal(err)
	}
	key, _, _ := script.DevKey(secret)
	src, built := t.TempDir(), t.TempDir()
	fields := `"capabilities": {"host": ["kando.line.current"], "network": {"http": [{"host": "api.example.com", "methods": ["POST"], "pathPrefixes": ["/v1/"]}]}}`
	os.WriteFile(filepath.Join(src, plugins.ManifestName), []byte(`{"schema": 2, "id": "acme.hello", "kind": "script", "version": "0.1.0", "kando": ">=0.30.0", "license": "MIT", "name": {"en": "Hello", "ja": "こんにちは"}, `+fields+`, "optional": ["kando.toast"]}`), 0o644)
	os.WriteFile(filepath.Join(src, script.Entry), []byte("export function kandoManifest() { return {"+fields+`, "optional": ["kando.toast"]} }`+"\n"+code), 0o644)
	if err := script.PackFolder(src, built, key); err != nil {
		t.Fatal(err)
	}
	packages, _ := filepath.Glob(filepath.Join(built, "*.nsgopkg"))
	if len(packages) != 1 {
		t.Fatalf("packages %q", packages)
	}
	data, err := os.ReadFile(packages[0])
	if err != nil {
		t.Fatal(err)
	}
	zipFolder(t, built, path)
	return key, fmt.Sprintf("%x", sha256.Sum256(data))
}

// revocationList writes the revocation list root signs, sequence 1, revoking the release keys keys and the packages packages, into a file of its own and returns the file and its bytes.
func revocationList(t *testing.T, root ed25519.PrivateKey, keys, packages []string) (string, []byte) {
	t.Helper()
	list, err := pkg.Revoke(root, 1, time.Now().Add(24*time.Hour), keys, packages)
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(list)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "kando-plugin-revocations.json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return path, data
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

	// After "--" every argument is a path, even one that looks like a flag.
	code, out, _ = f.run("validate", "--json", "--", "--json")
	if err := json.Unmarshal([]byte(out), &got); code != exitFailed || err != nil || len(got.Packs) != 1 || got.Packs[0].Path != "--json" || got.Packs[0].Code != "not-found" {
		t.Errorf("validate -- --json = %d, %q, %v", code, out, err)
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
		"declares ` kando.line.current `, ` kando.toast ` but its code never calls it",
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

// TestPreviewHostCalls checks the notes the preview writes on a script plugin's host calls, read from its signed bundle as the bundler wrote it: a call of a permission it does not declare, a declared one it never calls and a call that waits outside the engine without host.call's options; and that they do not fail the pack.
func TestPreviewHostCalls(t *testing.T) {
	hello := filepath.Join(t.TempDir(), "hello.kandoplugin")
	root := scriptPluginCode(t, hello, `import { host } from "nsgo"
export async function run() {
  const line = await host.call("kando.line.current", {})
  // host.call("kando.toast", {}) in a comment is not a call
  return await host.call("kando.llm.chat", { user: String(line), maxTokens: 10 })
}
`)
	f := newFixture(t, root, nil)
	code, out, _ := f.run("preview", "--markdown", hello)
	if code != exitOK {
		t.Errorf("notes failed the pack, exit %d:\n%s", code, out)
	}
	for _, want := range []string{
		"**Host calls**",
		"calls ` kando.llm.chat ` without declaring it: Kando refuses the call with denied",
		"declares ` kando.toast ` but its code never calls it",
		"calls ` kando.llm.chat ` without host.call's options: it gives up after 30 s",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("no %q in\n%s", want, out)
		}
	}
	if strings.Contains(out, "` kando.line.current ` but") {
		t.Errorf("a called permission is noted as never called:\n%s", out)
	}
}

// TestHostNotes checks what hostNotes reads of a bundle: the calls with the permission's name written out, how many arguments each has whatever brackets, literals and comments its arguments hold, and the permissions declared, needed or optional.
func TestHostNotes(t *testing.T) {
	m := plugins.Manifest{Capabilities: &pkg.Capabilities{Host: []string{"kando.net.credentialFetch", "kando.sound.read"}}, Optional: []plugins.Perm{plugins.PermToast}}
	bundle := `var WAIT = { timeout: 150000 };
async function send(body) {
  return await import_nsgo.host.call("kando.net.credentialFetch", { url: "https://a/(", header: { "X": [1, 2] }, body: ` + "`a,${body})`" + ` /* , */ }, WAIT);
}
async function sound(id) { return await import_nsgo.host.call( 'kando.sound.read' , { saved: id }) }
const note = "kando.toast";
`
	got := hostNotes(m, bundle)
	want := []string{
		"declares ` kando.toast ` but its code never calls it: the user is asked for a permission the plugin may not need",
		"calls ` kando.sound.read ` without host.call's options: it gives up after 30 s, before Kando's own 120 s; pass {timeout: 150000}",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("notes\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	if notes := hostNotes(m, strings.ReplaceAll(bundle, "{ saved: id })", "{ saved: id }, WAIT)")+`host.call("kando.toast", {})`); len(notes) != 0 {
		t.Errorf("a plugin calling what it declares, each with options: %v", notes)
	}
}

// TestHostNotesBounds checks that hostNotes stays short and quick whatever the bundle holds: it names at most noteNamesMax permissions a line, reads at most hostCallsMax calls and then says so without claiming a declared permission is never called, takes a call whose arguments run past hostCallArgsMax to pass options, and does not note kando.line.current as unused for a plugin that hears line.changed, which Kando sends only while it is granted.
func TestHostNotesBounds(t *testing.T) {
	var b strings.Builder
	for i := range noteNamesMax + 3 {
		b.WriteString(`host.call("kando.x` + strings.Repeat("y", i) + `", {}, W);`)
	}
	notes := hostNotes(plugins.Manifest{}, b.String())
	if len(notes) != 1 || !strings.HasSuffix(notes[0], " and 3 more without declaring it: Kando refuses the call with denied") || strings.Count(notes[0], "kando.") != noteNamesMax {
		t.Errorf("names past noteNamesMax: %v", notes)
	}

	m := plugins.Manifest{Capabilities: &pkg.Capabilities{Host: []string{"kando.toast"}}}
	flood := strings.Repeat(`host.call("kando.x", (`, hostCallsMax+1) + `host.call("kando.toast", {}, W)`
	notes = hostNotes(m, flood)
	want := []string{
		"calls ` kando.x ` without declaring it: Kando refuses the call with denied",
		fmt.Sprintf("names more than %d host calls: only the first %d were read", hostCallsMax, hostCallsMax),
	}
	if strings.Join(notes, "\n") != strings.Join(want, "\n") {
		t.Errorf("past hostCallsMax:\n%s\nwant\n%s", strings.Join(notes, "\n"), strings.Join(want, "\n"))
	}

	long := `host.call("kando.llm.chat", {user: "` + strings.Repeat("a", hostCallArgsMax) + `"})`
	if notes := hostNotes(plugins.Manifest{Optional: []plugins.Perm{plugins.PermLLMChat}}, long); len(notes) != 0 {
		t.Errorf("a call cut off at hostCallArgsMax: %v", notes)
	}

	panel := plugins.Manifest{Capabilities: &pkg.Capabilities{Host: []string{"kando.line.current"}}}
	if notes := hostNotes(panel, ""); len(notes) != 1 {
		t.Errorf("kando.line.current never called nor heard: %v", notes)
	}
	panel.Events = []string{plugins.EventLineChanged}
	if notes := hostNotes(panel, ""); len(notes) != 0 {
		t.Errorf("kando.line.current of a plugin hearing line.changed: %v", notes)
	}
}

// TestRevocations checks --revocations: validate refuses, with the code revoked, a script plugin whose package or release key the list, a file or an https address, revokes, and passes the data packs beside it; index refuses to list it and writes no index. No list, an address that answers 404, keeps the packs as they are; a file that does not exist, a list another root signed and an http address fail the command.
func TestRevocations(t *testing.T) {
	dir := t.TempDir()
	terms := zipFolder(t, folder(t, glossary), filepath.Join(dir, "terms.zip"))
	hello := filepath.Join(dir, "hello.kandoplugin")
	rootKey, packageHash := signedScriptPlugin(t, hello, "")
	root := rootKey.Public().(ed25519.PublicKey)
	f := newFixture(t, root, nil)

	validate := func(args ...string) (int, []validated, string) {
		t.Helper()
		code, out, errOut := f.run(append([]string{"validate", "--json"}, args...)...)
		var got struct {
			Packs []validated `json:"packs"`
		}
		if out != "" {
			if err := json.Unmarshal([]byte(out), &got); err != nil {
				t.Fatalf("validate %q = %q, %v", args, out, err)
			}
		}
		return code, got.Packs, errOut
	}

	// A file that does not exist fails the command before any pack is checked, so a wrong path never turns the check off.
	if code, packs, errOut := validate("--revocations", filepath.Join(dir, "none.json"), terms, hello); code != exitFailed || len(packs) != 0 || !strings.Contains(errOut, "none.json") {
		t.Fatalf("missing list = %d, %+v, %q", code, packs, errOut)
	}
	code, packs, _ := validate(terms, hello)
	if code != exitOK {
		t.Fatalf("validate = %d, %+v", code, packs)
	}
	signer := packs[1].Signer

	byPackage, byPackageData := revocationList(t, rootKey, nil, []string{packageHash})
	byKey, _ := revocationList(t, rootKey, []string{signer}, nil)
	empty, emptyData := revocationList(t, rootKey, nil, nil)
	for name, list := range map[string]string{"the package": byPackage, "the release key": byKey} {
		code, packs, errOut := validate(terms, "--revocations", list, hello)
		if code != exitFailed || len(packs) != 2 || errOut != "" {
			t.Fatalf("%s revoked = %d, %+v, %q", name, code, packs, errOut)
		}
		if !packs[0].OK {
			t.Errorf("%s revoked: the glossary %+v", name, packs[0])
		}
		if p := packs[1]; p.OK || p.Code != "revoked" {
			t.Errorf("%s revoked: the script plugin %+v", name, p)
		}
	}
	if code, packs, _ := validate("--revocations", empty, hello); code != exitOK || !packs[0].OK {
		t.Errorf("an empty list = %d, %+v", code, packs)
	}

	// A list another root signed is refused before any pack is checked.
	other, _ := pkg.GenerateKey()
	forged, _ := revocationList(t, other, nil, nil)
	if code, out, errOut := f.run("validate", "--json", "--revocations", forged, terms); code != exitFailed || out != "" || !strings.Contains(errOut, "the revocation list") {
		t.Errorf("a list of another root = %d, %q, %q", code, out, errOut)
	}
	if code, _, errOut := f.run("validate", "--revocations", "http://example.com/list.json", terms); code != exitFailed || !strings.Contains(errOut, "--revocations http://example.com/list.json") || !strings.Contains(errOut, "not https") {
		t.Errorf("an http list = %d, %q", code, errOut)
	}

	// An https address: 404 is no list, a list is checked against.
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/revoked.json":
			w.Write(byPackageData)
		case "/empty.json":
			w.Write(emptyData)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	f.client = srv.Client()
	if code, packs, errOut := validate("--revocations", srv.URL+"/missing.json", hello); code != exitOK || errOut != "" || !packs[0].OK {
		t.Errorf("a 404 = %d, %+v, %q", code, packs, errOut)
	}
	if code, packs, _ := validate("--revocations", srv.URL+"/revoked.json", hello); code != exitFailed || packs[0].Code != "revoked" {
		t.Errorf("a list fetched = %d, %+v", code, packs)
	}

	// index refuses to list a pack the list revokes, and writes nothing.
	packsDir := t.TempDir()
	for _, src := range []string{terms, hello} {
		data, err := os.ReadFile(src)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(packsDir, filepath.Base(src)), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	indexKey, _ := pkg.GenerateKey()
	f.indexPublicKey = pkg.EncodePublicKey(indexKey.Public().(ed25519.PublicKey))
	f.env["KANDO_PLUGINS_INDEX_KEY"] = pkg.EncodeSecretKey(indexKey)
	out := filepath.Join(t.TempDir(), "index.json")
	if code, _, errOut := f.run("index", packsDir, "--base", "https://kando.yldm.ai/plugin/packs/", "--out", out, "--revocations", srv.URL+"/revoked.json"); code != exitFailed || !strings.Contains(errOut, "hello.kandoplugin") || !strings.Contains(errOut, "revoked") {
		t.Errorf("index of a revoked pack = %d, %q", code, errOut)
	}
	if code, _, errOut := f.run("index", packsDir, "--base", "https://kando.yldm.ai/plugin/packs/", "--out", out, "--revocations", forged); code != exitFailed || !strings.Contains(errOut, "the revocation list") {
		t.Errorf("index with a list of another root = %d, %q", code, errOut)
	}
	if _, err := os.Stat(out); err == nil {
		t.Fatal("an index of a revoked pack was written")
	}
	if code, _, errOut := f.run("index", packsDir, "--base", "https://kando.yldm.ai/plugin/packs/", "--out", out, "--revocations", srv.URL+"/empty.json"); code != exitOK {
		t.Errorf("index with an empty list = %d, %q", code, errOut)
	}
}
