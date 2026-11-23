package main

import (
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"kando/apps/kando/locales/testlocale"
	"kando/apps/kando/plugins"
	"kando/apps/kando/plugins/script"
	"kando/apps/kando/settings"
	"kando/packages/core"
	"kando/packages/study"

	ns "github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
	"github.com/nsgo-dev/nsgo/tsx/pkg"
)

// The tests of the script plugins sign theirs with keys they make and throw away, never Kando's official one: an app of theirs takes a test key as its official root (pluginsScripts.official), and packs a plugin folder with it as the developer mode would (script.PackFolder), which signs it as root and release key at once.

// pluginsKey is a throwaway signing key, its secret as pkg encodes it and its public key.
type pluginsKey struct {
	secret string
	public ed25519.PublicKey
}

// newPluginsKey makes a throwaway signing key.
func newPluginsKey(t *testing.T) pluginsKey {
	t.Helper()
	secret, public, err := script.NewDevKey()
	if err != nil {
		t.Fatal(err)
	}
	return pluginsKey{secret, public}
}

// pluginAnkiFields are the fields of the test plugin acme.anki its code repeats (kandoManifest): it reads the saved lines and the line playing, may show toasts, reaches AnkiConnect on this computer, has a command with a key and one whose key Kando's own takes, two settings and hears film.opened and job.completed.
const pluginAnkiFields = `"capabilities": {
    "host": ["kando.saved.read", "kando.line.current", "kando.settings.get"],
    "network": {"http": [{"host": "127.0.0.1", "port": 8765, "schemes": ["http"], "methods": ["POST"], "paths": ["/"]}]}
  },
  "optional": ["kando.toast", "kando.lines.seen", "kando.ui.openURL"],
  "contributes": {"commands": [
    {"id": "sync", "export": "run", "key": "mod+shift+y", "title": {"en": "Send to Anki", "zh-Hans": "发到 Anki"}},
    {"id": "count", "export": "run", "key": "mod+shift+n", "title": {"en": "Count saved lines", "zh-Hans": "数一数收藏"}}
  ]},
  "settings": [
    {"key": "deck", "type": "text", "default": "Kando", "max": 100, "title": {"en": "Deck", "zh-Hans": "牌组"}},
    {"key": "audio", "type": "bool", "default": true, "title": {"en": "Attach the sound", "zh-Hans": "附上声音"}}
  ],
  "events": ["film.opened", "job.completed"]`

// pluginAnkiCode is acme.anki's code: run says what its command found, each export reaches one handler and says what it answered or the code it was refused with.
const pluginAnkiCode = `import { host, Storage } from "nsgo"
Storage.setItem("loaded", "yes")
const heard = []
host.on("film.opened", (e) => { heard.push(["film.opened", e]) })
host.on("job.completed", (e) => { heard.push(["job.completed", e]) })
async function attempt(name, data) {
  try { return await host.call(name, data || {}) } catch (e) { return "refused " + e.code }
}
export async function run(command) {
  if (command === "count") { const saved = await attempt("kando.saved.read"); return { toast: "saved " + (Array.isArray(saved) ? saved.length : saved) } }
  return { toast: "sent " + command }
}
export function saved() { return attempt("kando.saved.read") }
export function line() { return attempt("kando.line.current") }
export function seen() { return attempt("kando.lines.seen", { limit: 5 }) }
export function toast() { return attempt("kando.toast", { text: "hello" }) }
export function settings() { return attempt("kando.settings.get") }
export function events() { return heard }
export function openIt() { return attempt("kando.ui.openURL", { url: "https://ankiweb.net/decks" }) }
export async function flood() {
  for (let i = 0; i < 70; i++) { const r = await attempt("kando.settings.get"); if (typeof r === "string") return i + " " + r }
  return "never refused"
}
`

// pluginAnkiAbout is the author, name and description in the manifest of the script plugins of the tests, as pluginScriptFolder writes them.
const pluginAnkiAbout = `"author": "Acme", "name": {"en": "Anki Sender", "zh-Hans": "Anki 发送器"},
  "description": {"en": "Sends saved lines to Anki.", "zh-Hans": "把收藏的句子发到 Anki。"}`

// pluginScriptFolder packs the plugin id at version, of the manifest fields and code, with key, as a plugin folder ready to install, named as pluginAnkiAbout says.
func pluginScriptFolder(t *testing.T, key pluginsKey, id, version, fields, code string) string {
	t.Helper()
	return pluginScriptFolderAbout(t, key, id, version, pluginAnkiAbout, fields, code)
}

// pluginScriptFolderAbout is pluginScriptFolder with the manifest's author, name and description about, members of its object, as the screenshots name each plugin for what it does.
func pluginScriptFolderAbout(t *testing.T, key pluginsKey, id, version, about, fields, code string) string {
	t.Helper()
	src := t.TempDir()
	manifest := `{"schema": 2, "id": "` + id + `", "kind": "script", "version": "` + version + `", "license": "MIT", ` + about + `, ` + fields + `}`
	code = "export function kandoManifest() { return {" + fields + "} }\n" + code
	if err := os.WriteFile(filepath.Join(src, plugins.ManifestName), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, script.Entry), []byte(code), 0o644); err != nil {
		t.Fatal(err)
	}
	priv, _, err := script.DevKey(key.secret)
	if err != nil {
		t.Fatal(err)
	}
	dst := t.TempDir()
	if err := script.PackFolder(src, dst, priv); err != nil {
		t.Fatal(err)
	}
	return dst
}

// scriptsApp starts the app with its plugins folder in a new temporary folder, key standing in for Kando's official root, a watchdog that does not count the test's own pauses, and the script plugins of folders installed, and waits until it has read the folder and verified them.
func scriptsApp(t *testing.T, key pluginsKey, folders ...string) (*app, *nsgotest.App, chan func(), string) {
	t.Helper()
	a, tt, updates := newTestApp(t, &fakePipeline{}, nsgotest.PendingAlerts(), nsgotest.Secrets(map[string]string{}))
	root := filepath.Join(t.TempDir(), "plugins")
	pluginsScriptInstalled(t, root, folders...)
	a.plugins.root = root
	a.plugins.script.official = key.public
	a.plugins.script.watch = script.Watch{Beat: 100 * time.Millisecond, Stall: time.Minute, Stalls: 3, Deadline: time.Minute}
	a.pluginsScan()
	waitUntil(t, tt, a, updates, func() bool {
		return a.plugins.scanned && !a.plugins.busy && !a.plugins.inboxBusy && a.plugins.script.verified != nil
	})
	return a, tt, updates, root
}

// pluginsScriptInstalled installs the plugins of folders into root as an install the user confirmed would, leaving them off.
func pluginsScriptInstalled(t *testing.T, root string, folders ...string) {
	t.Helper()
	for _, dir := range folders {
		staged, p, err := plugins.Stage(root, dir)
		if err != nil {
			t.Fatal(err)
		}
		if err := plugins.Swap(root, staged, &p); err != nil {
			t.Fatal(err)
		}
	}
}

// scriptsSettle applies the background updates and runs what the engine was posted, as a plugin's host calls and the runner's answers are, until done holds, failing t after five seconds.
func scriptsSettle(t *testing.T, a *app, tt *nsgotest.App, updates chan func(), done func() bool) {
	t.Helper()
	scriptsSettleWithin(t, a, tt, updates, 5*time.Second, done)
}

// scriptsSettleWithin is scriptsSettle, failing t after limit.
func scriptsSettleWithin(t *testing.T, a *app, tt *nsgotest.App, updates chan func(), limit time.Duration, done func() bool) {
	t.Helper()
	deadline := time.Now().Add(limit)
	for !done() {
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for the plugins")
		}
		select {
		case fn := <-updates:
			onEngine(a, func() {
				fn()
				a.Changed()
			})
		default:
			time.Sleep(2 * time.Millisecond)
		}
		tt.Settle()
	}
	refresh(tt, a)
}

// scriptsGrant turns the plugin id on with every permission it declares granted, as the user would on its details, and waits until it runs.
func scriptsGrant(t *testing.T, a *app, tt *nsgotest.App, updates chan func(), id string, perms ...string) {
	t.Helper()
	onEngine(a, func() {
		for _, p := range perms {
			a.SetPluginGrant(id, p, true)
		}
		a.SetPluginEnabled(id, true)
	})
	scriptsSettle(t, a, tt, updates, func() bool { return a.plugins.script.loaded[id] != nil && !a.plugins.saving })
}

// scriptsCall calls the export name of the plugin id through the runner and returns what it answered.
func scriptsCall(t *testing.T, a *app, tt *nsgotest.App, updates chan func(), id, name string, args ...any) (any, error) {
	t.Helper()
	var got struct {
		v    any
		err  error
		done bool
	}
	onEngine(a, func() {
		l := a.plugins.script.loaded[id]
		if l == nil {
			got.err, got.done = errors.New("not running"), true
			return
		}
		a.pluginsRunner().Call(l.plugin, name, args, func(v any, err error) { got.v, got.err, got.done = v, err, true })
	})
	scriptsSettle(t, a, tt, updates, func() bool { return got.done })
	return got.v, got.err
}

// TestPluginScriptInstallGrantAndRun installs a script plugin from Settings › 插件 as a user would: its confirmation says where it comes from, what it asks for, needed and optional, flags reading the lines together with the network and says it stays off; installed, it is off with nothing granted; granted a permission and turned on on its details, it runs, and its handlers answer what it was granted and refuse the rest.
func TestPluginScriptInstallGrantAndRun(t *testing.T) {
	key := newPluginsKey(t)
	a, tt, updates, root := scriptsApp(t, key)
	m := testlocale.Messages()
	tt = settingsOpenAt(a, tt, settings.SectionPlugins)
	onEngine(a, func() {
		a.InstallPlugins([]string{pluginScriptFolder(t, key, "acme.anki", "1.0.0", pluginAnkiFields, pluginAnkiCode)})
	})
	waitUntil(t, tt, a, updates, func() bool { return a.plugins.pending != nil && !a.plugins.busy })
	if p := a.plugins.pending; p.err != nil {
		t.Fatalf("staged with %v", p.err)
	}
	for _, s := range []string{m.SettingsPluginsInstallSourceVerified("acme"), m.SettingsPluginsWants(), m.SettingsPluginsWantsOptional(), m.SettingsPluginsPermSavedRead(), m.SettingsPluginsPermToast(), m.SettingsPluginsScriptStaysOff()} {
		if !tt.Exists("text:" + s) {
			t.Errorf("the confirmation does not say %q; texts %q", s, tt.Texts())
		}
	}
	if !tt.Exists("id:plugins-install-risky") {
		t.Error("reading the lines with the network is not flagged")
	}
	// Another publisher than kando, under the official root, is verified, named by its id's publisher, never Kando's own nor its manifest's author.
	if tt.Exists("text:"+m.SettingsPluginsInstallSourceOfficial()) || hasText(tt, m.SettingsPluginsInstallSourceVerified("Acme")) {
		t.Errorf("acme.anki reads as Kando's own or by its author; texts %q", tt.Texts())
	}
	tt.Press("id:plugins-install-ok")
	waitUntil(t, tt, a, updates, func() bool {
		return a.plugins.pending == nil && !a.plugins.busy && !a.plugins.saving && len(a.pluginsScriptPacks()) == 1 && a.pluginsScriptPacks()[0].Signer != ""
	})
	p := a.pluginsScriptPacks()[0]
	if a.plugins.state.EnabledPack(p) || len(a.plugins.state.Effective(p, false, false).Perms) > 1 || p.Source != plugins.SourceVerified {
		t.Fatalf("installed %+v, state %+v", p, a.plugins.state)
	}
	tt.Press("id:settings-plugins-details-acme.anki")
	for _, s := range []string{"settings-plugins-grant-acme.anki-kando.saved.read", "settings-plugins-grant-acme.anki-network", "settings-plugins-setting-acme.anki-deck", "settings-plugins-risky-acme.anki"} {
		if !tt.Exists("id:" + s) {
			t.Errorf("the details have no %s; texts %q", s, tt.Texts())
		}
	}
	tt.Press("id:settings-plugins-grant-acme.anki-kando.saved.read")
	waitUntil(t, tt, a, updates, func() bool { return !a.plugins.saving })
	tt.Press("id:settings-plugins-switch-acme.anki")
	scriptsSettle(t, a, tt, updates, func() bool { return a.plugins.script.loaded["acme.anki"] != nil && !a.plugins.saving })
	state, err := plugins.LoadState(root)
	if err != nil || !state.EnabledPack(p) || state.Format != 2 {
		t.Fatalf("state.json %+v, %v", state, err)
	}
	onEngine(a, func() {
		book, _ := study.Load(a.snap().LibraryDir)
		book.Lines = append(book.Lines, study.Line{ID: "l1", Video: "/videos/talk.mp4", Original: "hello", Film: "Talk"})
		a.saved.book = book
	})
	got, err := scriptsCall(t, a, tt, updates, "acme.anki", "saved")
	if lines, ok := got.([]any); err != nil || !ok {
		t.Errorf("saved.read: %v, %v", got, err)
	} else if line := lines[0].(map[string]any); line["original"] != "hello" || strings.Contains(fmt.Sprint(line), "/videos") || !strings.HasPrefix(line["film"].(string), "f_") {
		t.Errorf("a saved line reads %v", line)
	}
	// What it was not granted its sandbox refuses before any handler hears it (core's access, ERR_ACCESS_DENIED); a handler refuses with "denied" what the plugin may no longer do since it loaded.
	for _, name := range []string{"toast", "line"} {
		if got, _ := scriptsCall(t, a, tt, updates, "acme.anki", name); !strings.Contains(strings.ToLower(fmt.Sprint(got)), "denied") {
			t.Errorf("%s without its grant: %v", name, got)
		}
	}
	if got, _ := scriptsCall(t, a, tt, updates, "acme.anki", "settings"); fmt.Sprint(got) != "map[audio:true deck:Kando]" {
		t.Errorf("settings.get: %v", got)
	}
	onEngine(a, func() { a.SetPluginValue("acme.anki", "deck", "Mining") })
	if got, _ := scriptsCall(t, a, tt, updates, "acme.anki", "settings"); fmt.Sprint(got) != "map[audio:true deck:Mining]" {
		t.Errorf("settings.get after a change: %v", got)
	}
	// Each plugin has a host of its own.
	if l := a.plugins.script.loaded["acme.anki"]; l.host == nil {
		t.Fatal("no host")
	}
}

// TestPluginScriptRefusals checks the plugins the install refuses, each saying why: one whose manifest asks for more than its code says, one under kando. that the developer key signed, one older than a build accepted, and one signed by a root Kando does not trust.
func TestPluginScriptRefusals(t *testing.T) {
	key := newPluginsKey(t)
	a, tt, updates, root := scriptsApp(t, key)
	m := testlocale.Messages()
	refused := func(dir string, want string) {
		t.Helper()
		onEngine(a, func() { a.InstallPlugins([]string{dir}) })
		waitUntil(t, tt, a, updates, func() bool { return a.plugins.pending != nil && !a.plugins.busy })
		if p := a.plugins.pending; p.err == nil || pluginsErrorText(m, p.err) != want {
			t.Errorf("staged %s: %v, want %q", dir, p.err, want)
		}
		if !tt.Exists("id:plugins-install-failed") {
			t.Errorf("no failure; texts %q", tt.Texts())
		}
		onEngine(a, a.CancelPluginInstall)
		refresh(tt, a)
	}
	// The manifest asks for words.read, which the code does not say.
	tampered := pluginScriptFolder(t, key, "acme.anki", "1.0.0", pluginAnkiFields, pluginAnkiCode)
	data, _ := os.ReadFile(filepath.Join(tampered, plugins.ManifestName))
	os.WriteFile(filepath.Join(tampered, plugins.ManifestName), []byte(strings.Replace(string(data), `"kando.saved.read",`, `"kando.saved.read", "kando.words.read",`, 1)), 0o644)
	refused(tampered, m.PluginsErrTampered())
	dev := newPluginsKey(t)
	onEngine(a, func() {
		a.plugins.script.developer, a.plugins.script.dev, a.plugins.script.devRead = true, dev.public, true
		a.plugins.script.trust = nil
	})
	refused(pluginScriptFolder(t, dev, "kando.anki", "1.0.0", pluginAnkiFields, pluginAnkiCode), m.PluginsErrScope())
	refused(pluginScriptFolder(t, newPluginsKey(t), "acme.other", "1.0.0", pluginAnkiFields, pluginAnkiCode), m.PluginsErrUntrusted())
	// A build was accepted that is newer than any this test packs.
	os.MkdirAll(script.TrustDir(root), 0o755)
	os.WriteFile(filepath.Join(script.TrustDir(root), "trust.json"), []byte(`{"builds": {"`+script.AppIDPrefix+`acme.anki": 99999999999}}`), 0o600)
	onEngine(a, func() { a.plugins.script.trust = nil })
	refused(pluginScriptFolder(t, key, "acme.anki", "1.0.0", pluginAnkiFields, pluginAnkiCode), m.PluginsErrRollback())
}

// TestPluginScriptHungIsTurnedOff checks the self-healing of docs/plugin-system.md 6.6: a call left in flight when Kando stopped turns its plugin off as hung at the next start, which its row says.
func TestPluginScriptHungIsTurnedOff(t *testing.T) {
	key := newPluginsKey(t)
	dir := pluginScriptFolder(t, key, "acme.anki", "1.0.0", pluginAnkiFields, pluginAnkiCode)
	root := filepath.Join(t.TempDir(), "plugins")
	staged, p, err := plugins.Stage(root, dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := plugins.Swap(root, staged, &p); err != nil {
		t.Fatal(err)
	}
	signer := pluginsSignerOf(t, key, root, p)
	state := plugins.State{Plugins: []plugins.PluginState{{ID: p.ID, Hash: p.Hash, Signer: signer, Enabled: true, Grant: plugins.Grant{Perms: []plugins.Perm{plugins.PermSavedRead}}}}}
	if err := state.Save(root); err != nil {
		t.Fatal(err)
	}
	if err := plugins.MarkInFlight(root, p.ID, p.Hash); err != nil {
		t.Fatal(err)
	}
	a, tt, updates := newTestApp(t, &fakePipeline{}, nsgotest.PendingAlerts())
	a.plugins.root, a.plugins.script.official = root, key.public
	a.pluginsScan()
	scriptsSettle(t, a, tt, updates, func() bool { return a.plugins.script.verified != nil && !a.plugins.saving })
	if a.plugins.script.loaded[p.ID] != nil || a.plugins.state.Enabled(p.ID) {
		t.Fatal("the hung plugin runs")
	}
	if h, ok := plugins.HealthOf(a.plugins.script.health, p.ID, p.Hash); !ok || h.Reason != plugins.ReasonHung {
		t.Fatalf("health %+v", a.plugins.script.health)
	}
	tt = settingsOpenAt(a, tt, settings.SectionPlugins)
	wantText(t, tt, testlocale.Messages().SettingsPluginsStoppedHung())
}

// TestPluginScriptBackgroundHogIsTurnedOff checks that a plugin whose timers keep the window busy outside its calls, which no call's watchdog watches, is turned off by the runner's background watchdog as slow: it no longer runs, health.json records why, which keeps it from loading again, and the audit log has a line for it. The test's clock stands still but for what the test lets go by, which runs the plugin's interval once a millisecond, each run a loop well under tsx's budget, so the window is busy with it nearly all the time the test runs.
func TestPluginScriptBackgroundHogIsTurnedOff(t *testing.T) {
	key := newPluginsKey(t)
	a, tt, updates, root := scriptsApp(t, key, pluginScriptFolder(t, key, "acme.anki", "1.0.0", pluginAnkiFields, pluginAnkiCode+"\nsetInterval(() => { let x = 0; for (let i = 0; i < 200000; i++) x += i }, 1)\n"))
	w := script.DefaultWatch
	w.Stall = time.Minute
	w.BgBeat, w.Burst, w.TotalBurst = 50*time.Millisecond, time.Second, 0
	a.plugins.script.watch = w
	scriptsGrant(t, a, tt, updates, "acme.anki")
	p := a.pluginsScriptPacks()[0]
	for deadline := time.Now().Add(20 * time.Second); a.plugins.script.loaded["acme.anki"] != nil; {
		if time.Now().After(deadline) {
			t.Fatal("the plugin keeping the window busy still runs")
		}
		tt.Advance(20 * time.Millisecond)
	}
	if h, ok := plugins.HealthOf(a.plugins.script.health, p.ID, p.Hash); !ok || h.Reason != plugins.ReasonSlow {
		t.Fatalf("health %+v", a.plugins.script.health)
	}
	if a.plugins.state.Enabled(p.ID) {
		t.Error("its switch is still on")
	}
	pluginsAuditHas(t, a, tt, updates, root, plugins.AuditStopped+" acme.anki "+plugins.ReasonSlow)
	scriptsSettle(t, a, tt, updates, func() bool {
		hs, _ := plugins.LoadHealth(root)
		h, ok := plugins.HealthOf(hs, p.ID, p.Hash)
		return ok && h.Reason == plugins.ReasonSlow
	})
}

// pluginsSignerOf is the ID of the release key that signed the plugin p, as Kando verifies it.
func pluginsSignerOf(t *testing.T, key pluginsKey, root string, p plugins.Pack) string {
	t.Helper()
	roots := script.Roots{Official: key.public}
	trust, err := script.OpenTrust(t.TempDir(), roots)
	if err != nil {
		t.Fatal(err)
	}
	v, err := script.Verify(trust, roots, p)
	if err != nil {
		t.Fatal(err)
	}
	return v.Signer
}

// TestPluginScriptRevoked checks that a plugin whose package a revocation list names is refused as it is read, says so, and does not run.
func TestPluginScriptRevoked(t *testing.T) {
	key := newPluginsKey(t)
	dir := pluginScriptFolder(t, key, "acme.anki", "1.0.0", pluginAnkiFields, pluginAnkiCode)
	data, _ := os.ReadFile(filepath.Join(dir, plugins.ManifestName))
	var m plugins.Manifest
	json.Unmarshal(data, &m)
	priv, _, _ := script.DevKey(key.secret)
	list, err := pkgRevoke(priv, m.Script.SHA256)
	if err != nil {
		t.Fatal(err)
	}
	a, tt, updates, root := scriptsApp(t, key, dir)
	trust, err := script.OpenTrust(root, script.Roots{Official: key.public})
	if err != nil {
		t.Fatal(err)
	}
	if err := trust.UpdateRevocations(list); err != nil {
		t.Fatal(err)
	}
	onEngine(a, func() {
		a.plugins.script.trust = nil
		a.pluginsScan()
	})
	scriptsSettle(t, a, tt, updates, func() bool { return len(a.plugins.script.issues) == 1 && !a.plugins.inboxBusy })
	if !errors.Is(a.plugins.script.issues[0].Err, script.ErrRevoked) {
		t.Fatalf("issues %+v", a.plugins.script.issues)
	}
	tt = settingsOpenAt(a, tt, settings.SectionPlugins)
	wantText(t, tt, testlocale.Messages().PluginsErrRevoked())
}

// TestPluginScriptCommandsAndQuota checks the commands a plugin adds: under a menu of its name in the shortcut table, with its key unless one of Kando's own has it, which the details then say; running one shows what it returns as a toast once it may show toasts; and a plugin's host calls past sixty a minute are refused.
func TestPluginScriptCommandsAndQuota(t *testing.T) {
	key := newPluginsKey(t)
	a, tt, updates, _ := scriptsApp(t, key, pluginScriptFolder(t, key, "acme.anki", "1.0.0", pluginAnkiFields, pluginAnkiCode))
	scriptsGrant(t, a, tt, updates, "acme.anki", string(plugins.PermSavedRead), string(plugins.PermToast))
	var rows []shortcut
	onEngine(a, func() { rows = a.shortcuts(a.cx) })
	var mine []shortcut
	for _, r := range rows {
		if r.Group == pluginsCommandGroup+"acme.anki" {
			mine = append(mine, r)
		}
	}
	if len(mine) != 2 || mine[0].Key != "mod+shift+y" || mine[1].Key != "" || mine[0].Label != "发到 Anki" {
		t.Fatalf("the plugin's rows %+v", mine)
	}
	if title := groupTitle(a.cx, mine[0].Group); title != "Anki 发送器" {
		t.Errorf("its menu is called %q", title)
	}
	tt = settingsOpenAt(a, tt, settings.SectionPlugins)
	tt.Press("id:settings-plugins-details-acme.anki")
	if !tt.Exists("id:settings-plugins-conflicts-acme.anki") {
		t.Errorf("the conflict is not said; texts %q", tt.Texts())
	}
	onEngine(a, mine[1].Run)
	scriptsSettle(t, a, tt, updates, func() bool { return tt.Exists("text:saved 0") })
	got, err := scriptsCall(t, a, tt, updates, "acme.anki", "flood")
	if err != nil || !strings.HasSuffix(fmt.Sprint(got), "refused quota") {
		t.Errorf("seventy calls: %v, %v", got, err)
	}
}

// TestPluginScriptUpdateKeepsTheGrant checks the update rules of docs/plugin-system.md 6.2: an update of the same signer that asks for no more keeps the switch and the grant; one that asks for more keeps what both allow and is flagged until the user looks at its grant again; a developer plugin cannot take the id of one that is not.
func TestPluginScriptUpdateKeepsTheGrant(t *testing.T) {
	key := newPluginsKey(t)
	a, tt, updates, _ := scriptsApp(t, key, pluginScriptFolder(t, key, "acme.anki", "1.0.0", pluginAnkiFields, pluginAnkiCode))
	scriptsGrant(t, a, tt, updates, "acme.anki", string(plugins.PermSavedRead))
	stage := func(dir string) *pluginsPending {
		t.Helper()
		onEngine(a, func() { a.InstallPlugins([]string{dir}) })
		waitUntil(t, tt, a, updates, func() bool { return a.plugins.pending != nil && !a.plugins.busy })
		return a.plugins.pending
	}
	install := func(dir, version string) {
		t.Helper()
		if p := stage(dir); p.err != nil {
			t.Fatal(p.err)
		}
		onEngine(a, a.ConfirmPluginInstall)
		scriptsSettle(t, a, tt, updates, func() bool {
			packs := a.pluginsScriptPacks()
			return a.plugins.pending == nil && !a.plugins.busy && !a.plugins.saving && !a.plugins.inboxBusy && len(packs) == 1 && packs[0].Signer != "" && packs[0].Version == version
		})
	}
	entry := func() plugins.PluginState { e, _ := a.plugins.state.Entry("acme.anki"); return e }
	install(pluginScriptFolder(t, key, "acme.anki", "1.0.1", pluginAnkiFields, pluginAnkiCode), "1.0.1")
	if e := entry(); !e.Enabled || !e.Grant.Has(plugins.PermSavedRead) || e.OldHash != "" {
		t.Errorf("the same asks: %+v", e)
	}
	more := strings.Replace(pluginAnkiFields, `"kando.settings.get"]`, `"kando.settings.get", "kando.words.read"]`, 1)
	install(pluginScriptFolder(t, key, "acme.anki", "1.1.0", more, pluginAnkiCode), "1.1.0")
	if e := entry(); !e.Enabled || !e.Grant.Has(plugins.PermSavedRead) || e.Grant.Has(plugins.PermWordsRead) || e.OldHash == "" {
		t.Errorf("asking for more: %+v", e)
	}
	tt = settingsOpenAt(a, tt, settings.SectionPlugins)
	tt.Press("id:settings-plugins-details-acme.anki")
	if !tt.Exists("id:settings-plugins-updated-acme.anki") {
		t.Errorf("the update is not flagged; texts %q", tt.Texts())
	}
	dev := newPluginsKey(t)
	onEngine(a, func() {
		a.plugins.script.developer, a.plugins.script.dev, a.plugins.script.devRead = true, dev.public, true
		a.plugins.script.trust = nil
	})
	if p := stage(pluginScriptFolder(t, dev, "acme.anki", "2.0.0", pluginAnkiFields, pluginAnkiCode)); !errors.Is(p.err, errPluginsDevID) {
		t.Errorf("a developer plugin in place of an official one: %v", p.err)
	}
	onEngine(a, a.CancelPluginInstall)
}

// TestPluginScriptUninstallForgetsItsStorage checks that uninstalling a script plugin clears what it stored, its settings and its grant.
func TestPluginScriptUninstallForgetsItsStorage(t *testing.T) {
	key := newPluginsKey(t)
	a, tt, updates, root := scriptsApp(t, key, pluginScriptFolder(t, key, "acme.anki", "1.0.0", pluginAnkiFields, pluginAnkiCode))
	scriptsGrant(t, a, tt, updates, "acme.anki")
	signer := a.pluginsScriptPacks()[0].Signer
	keys := func() []string {
		var out []string
		onEngine(a, func() {
			inner := a.cx.Child()
			inner.Run(func() {
				inner.ProvideStorage(script.Partition("acme.anki", signer), 0)
				out = inner.Storage().Keys()
			})
			inner.Dispose()
		})
		return out
	}
	if got := keys(); !slices.Contains(got, "loaded") {
		t.Fatalf("the plugin stored %q", got)
	}
	onEngine(a, func() { a.SetPluginValue("acme.anki", "deck", "Mining") })
	onEngine(a, func() { a.UninstallPlugin("acme.anki") })
	scriptsSettle(t, a, tt, updates, func() bool { return !a.plugins.busy && len(a.pluginsScriptPacks()) == 0 && !a.plugins.inboxBusy })
	if got := keys(); len(got) != 0 {
		t.Errorf("left stored %q", got)
	}
	if _, ok := a.plugins.state.Entry("acme.anki"); ok {
		t.Error("its grant is left")
	}
	time.Sleep(50 * time.Millisecond)
	if v, _ := plugins.LoadValues(root, "acme.anki"); len(v) != 0 {
		t.Errorf("its settings are left: %v", v)
	}
}

// TestPluginScriptForgetsALegacySecret checks that a script plugin installed before Kando refused secret settings is still listed and runs, without a field for its secret, and that what the user saved under that secret is removed as the plugin is uninstalled, or updated by the same signer to a version that cannot declare it.
func TestPluginScriptForgetsALegacySecret(t *testing.T) {
	legacy := strings.Replace(pluginAnkiFields, `"settings": [`, `"settings": [
    {"key": "token", "type": "secret", "title": {"en": "Token", "zh-Hans": "令牌"}},`, 1)
	for _, how := range []string{"uninstall", "update"} {
		t.Run(how, func(t *testing.T) {
			key := newPluginsKey(t)
			a, tt, updates, root := scriptsApp(t, key)
			// Stage refuses it now, so it is put in place as an install of Kando 0.38 left it.
			if err := os.CopyFS(filepath.Join(root, string(plugins.KindScript), "acme.anki"), os.DirFS(pluginScriptFolder(t, key, "acme.anki", "1.0.0", legacy, pluginAnkiCode))); err != nil {
				t.Fatal(err)
			}
			onEngine(a, a.pluginsScan)
			scriptsSettle(t, a, tt, updates, func() bool {
				packs := a.pluginsScriptPacks()
				return !a.plugins.busy && !a.plugins.inboxBusy && len(packs) == 1 && packs[0].Signer != ""
			})
			scriptsGrant(t, a, tt, updates, "acme.anki")
			if got, err := scriptsCall(t, a, tt, updates, "acme.anki", "settings"); err != nil || !strings.Contains(fmt.Sprint(got), "deck") || strings.Contains(fmt.Sprint(got), "token") {
				t.Errorf("kando.settings.get answers %v, %v", got, err)
			}
			signer := a.pluginsScriptPacks()[0].Signer
			onEngine(a, func() { a.SetPluginSecret("acme.anki", "token", "s3cret") })
			scriptsSettle(t, a, tt, updates, func() bool { return a.PluginSecretKept("acme.anki", "token") })
			tt = settingsOpenAt(a, tt, settings.SectionPlugins)
			tt.Press("id:settings-plugins-details-acme.anki")
			if !tt.Exists("id:settings-plugins-setting-acme.anki-deck") || tt.Exists("id:settings-plugins-setting-acme.anki-token") {
				t.Errorf("the settings drawn; texts %q", tt.Texts())
			}
			var secrets ns.Secrets
			onEngine(a, func() { secrets = a.pluginsHostSecrets("acme.anki", signer) })
			if how == "uninstall" {
				onEngine(a, func() { a.UninstallPlugin("acme.anki") })
				scriptsSettle(t, a, tt, updates, func() bool { return !a.plugins.busy && len(a.pluginsScriptPacks()) == 0 && !a.plugins.inboxBusy })
			} else {
				update := pluginScriptFolder(t, key, "acme.anki", "1.0.1", pluginAnkiFields, pluginAnkiCode)
				onEngine(a, func() { a.InstallPlugins([]string{update}) })
				waitUntil(t, tt, a, updates, func() bool { return a.plugins.pending != nil && !a.plugins.busy })
				if err := a.plugins.pending.err; err != nil {
					t.Fatal(err)
				}
				onEngine(a, a.ConfirmPluginInstall)
				scriptsSettle(t, a, tt, updates, func() bool {
					packs := a.pluginsScriptPacks()
					return a.plugins.pending == nil && !a.plugins.busy && !a.plugins.inboxBusy && len(packs) == 1 && packs[0].Version == "1.0.1"
				})
			}
			scriptsSettle(t, a, tt, updates, func() bool { _, ok, _ := secrets.Get("token"); return !ok })
		})
	}
}

// TestPluginScriptsCloseAsTheAppQuits checks that the plugins running are closed by quitStarting, while the app's scope is there.
func TestPluginScriptsCloseAsTheAppQuits(t *testing.T) {
	key := newPluginsKey(t)
	a, tt, updates, _ := scriptsApp(t, key, pluginScriptFolder(t, key, "acme.anki", "1.0.0", pluginAnkiFields, pluginAnkiCode))
	scriptsGrant(t, a, tt, updates, "acme.anki")
	l := a.plugins.script.loaded["acme.anki"]
	onEngine(a, func() {
		for _, h := range a.areas() {
			if h.quitStarting != nil {
				h.quitStarting()
			}
		}
	})
	if !l.plugin.Closed() || len(a.plugins.script.loaded) != 0 {
		t.Fatal("a plugin is still running")
	}
}

// pkgRevoke is a revocation list of the root priv naming the package sha, for an hour.
func pkgRevoke(priv ed25519.PrivateKey, sha string) (*pkg.Revocations, error) {
	return pkg.Revoke(priv, 1, time.Now().Add(time.Hour), nil, []string{sha})
}

// TestPluginScriptHearsEvents checks the events a plugin hears: job.completed once a job completes, not for the jobs complete as the app started, and film.opened, each with the film's id for this plugin and scalars alone.
func TestPluginScriptHearsEvents(t *testing.T) {
	key := newPluginsKey(t)
	a, tt, updates, _ := scriptsApp(t, key, pluginScriptFolder(t, key, "acme.anki", "1.0.0", pluginAnkiFields, pluginAnkiCode))
	scriptsGrant(t, a, tt, updates, "acme.anki")
	onEngine(a, func() {
		a.library.Jobs = []core.Job{{ID: "done", Stage: core.StageComplete, VideoPath: "/videos/done.mp4"}, {ID: "j1", Stage: core.StageTranscribing, VideoPath: "/videos/talk.mp4"}}
		a.pluginsJobsCompleted()
		a.library.Jobs[1].Stage = core.StageComplete
		a.pluginsJobsCompleted()
		a.pluginsSend("film.opened", func(l *pluginsScript) map[string]any {
			return map[string]any{"film": a.pluginsFilmID(l, "/videos/talk.mp4"), "spoken": "ja"}
		})
	})
	// The events reach the plugin as the engine runs what was posted.
	refresh(tt, a)
	got, err := scriptsCall(t, a, tt, updates, "acme.anki", "events")
	heard, ok := got.([]any)
	if err != nil || !ok || len(heard) != 2 {
		t.Fatalf("heard %v, %v", got, err)
	}
	job, film := heard[0].([]any), heard[1].([]any)
	data := job[1].(map[string]any)
	if job[0] != "job.completed" || data["stage"] != "complete" || film[0] != "film.opened" || film[1].(map[string]any)["film"] != data["film"] || strings.Contains(fmt.Sprint(heard), "/videos") {
		t.Errorf("heard %v", heard)
	}
}

// TestPluginScriptFollowsTheGates checks that a plugin running is loaded again without the network once Kando goes offline, and without what reads the library while it is locked, as tsx holds a plugin to what it was granted as it loaded.
func TestPluginScriptFollowsTheGates(t *testing.T) {
	key := newPluginsKey(t)
	a, tt, updates, _ := scriptsApp(t, key, pluginScriptFolder(t, key, "acme.anki", "1.0.0", pluginAnkiFields, pluginAnkiCode))
	scriptsGrant(t, a, tt, updates, "acme.anki", string(plugins.PermSavedRead), pluginsNetwork)
	if l := a.plugins.script.loaded["acme.anki"]; len(l.grant.Network) != 1 {
		t.Fatalf("loaded with %+v", l.grant)
	}
	onEngine(a, func() { a.privacy.offline = true })
	refresh(tt, a)
	scriptsSettle(t, a, tt, updates, func() bool {
		l := a.plugins.script.loaded["acme.anki"]
		return l != nil && len(l.grant.Network) == 0
	})
	if l := a.plugins.script.loaded["acme.anki"]; !l.grant.Has(plugins.PermSavedRead) {
		t.Errorf("offline it lost %+v", l.grant)
	}
}

// TestPluginScriptsAtMostTwelve checks that no more than twelve script plugins can be on at once, which the page says.
func TestPluginScriptsAtMostTwelve(t *testing.T) {
	a, tt, _ := newTestApp(t, &fakePipeline{})
	a.plugins.root = t.TempDir()
	for i := range 13 {
		p := plugins.Pack{Manifest: plugins.Manifest{ID: fmt.Sprintf("acme.p%d", i), Kind: plugins.KindScript}, Hash: fmt.Sprintf("h%d", i), Signer: "k"}
		a.plugins.packs = append(a.plugins.packs, p)
		if i < 12 {
			a.plugins.state.Set(plugins.PluginState{ID: p.ID, Hash: p.Hash, Signer: "k", Enabled: true})
		}
	}
	onEngine(a, func() { a.SetPluginEnabled("acme.p12", true) })
	if a.plugins.state.EnabledPack(a.plugins.packs[12]) || !errors.Is(a.plugins.err, errPluginsTooMany) {
		t.Fatalf("a thirteenth turned on: %v", a.plugins.err)
	}
	if a.PluginsErr() != testlocale.Messages().PluginsErrTooManyScripts() {
		t.Errorf("the page says %q", a.PluginsErr())
	}
	refresh(tt, a)
}

// TestPluginScriptSupersededVerificationKeepsTheMarks checks that a verification started while the first one, which read health.json and the in-flight marks off the disk, still ran does not lose them: the plugin a call hung is turned off all the same.
func TestPluginScriptSupersededVerificationKeepsTheMarks(t *testing.T) {
	key := newPluginsKey(t)
	root := filepath.Join(t.TempDir(), "plugins")
	pluginsScriptInstalled(t, root, pluginScriptFolder(t, key, "acme.anki", "1.0.0", pluginAnkiFields, pluginAnkiCode))
	packs, _ := plugins.Scan(root)
	p := packs[0]
	signer := pluginsSignerOf(t, key, root, p)
	state := plugins.State{Plugins: []plugins.PluginState{{ID: p.ID, Hash: p.Hash, Signer: signer, Enabled: true}}}
	if err := state.Save(root); err != nil {
		t.Fatal(err)
	}
	if err := plugins.MarkInFlight(root, p.ID, p.Hash); err != nil {
		t.Fatal(err)
	}
	a, tt, updates := newTestApp(t, &fakePipeline{}, nsgotest.PendingAlerts())
	a.plugins.root, a.plugins.script.official = root, key.public
	a.pluginsScan()
	waitUntil(t, tt, a, updates, func() bool { return a.plugins.scanned })
	// The first verification runs; a second one starts before it reports.
	onEngine(a, a.pluginsVerify)
	scriptsSettle(t, a, tt, updates, func() bool {
		return a.plugins.script.verified != nil && !a.plugins.saving && len(a.plugins.script.health) == 1
	})
	if a.plugins.script.loaded[p.ID] != nil || a.plugins.state.EnabledPack(a.pluginsScriptPacks()[0]) {
		t.Fatal("the hung plugin runs")
	}
	if len(a.plugins.script.filmKey) != 32 {
		t.Error("the films' secret was dropped")
	}
}

// TestPluginScriptSettingsOfAPluginNotRunning checks that a plugin turned off shows the settings it kept, and that changing one keeps the others.
func TestPluginScriptSettingsOfAPluginNotRunning(t *testing.T) {
	key := newPluginsKey(t)
	a, tt, updates, root := scriptsApp(t, key, pluginScriptFolder(t, key, "acme.anki", "1.0.0", pluginAnkiFields, pluginAnkiCode))
	// Its settings, none yet, are read once it is verified, as every plugin's.
	scriptsSettle(t, a, tt, updates, func() bool { _, ok := a.plugins.script.values["acme.anki"]; return ok })
	if err := plugins.SaveValues(root, "acme.anki", plugins.Values{"deck": "Mining"}); err != nil {
		t.Fatal(err)
	}
	onEngine(a, func() {
		delete(a.plugins.script.values, "acme.anki")
		a.pluginsVerify()
	})
	scriptsSettle(t, a, tt, updates, func() bool { _, ok := a.plugins.script.values["acme.anki"]; return ok })
	if v := a.PluginValue("acme.anki", "deck"); v != "Mining" {
		t.Fatalf("deck reads %v", v)
	}
	onEngine(a, func() { a.SetPluginValue("acme.anki", "audio", false) })
	scriptsSettle(t, a, tt, updates, func() bool {
		v, _ := plugins.LoadValues(root, "acme.anki")
		return v["audio"] == false
	})
	if v, _ := plugins.LoadValues(root, "acme.anki"); v["deck"] != "Mining" {
		t.Errorf("the file holds %v", v)
	}
}

// TestPluginScriptBadgeFollowsTheVerification checks that a script plugin's row, listed before its signature is verified, shows where it comes from once it is.
func TestPluginScriptBadgeFollowsTheVerification(t *testing.T) {
	key := newPluginsKey(t)
	root := filepath.Join(t.TempDir(), "plugins")
	pluginsScriptInstalled(t, root, pluginScriptFolder(t, key, "acme.anki", "1.0.0", pluginAnkiFields, pluginAnkiCode))
	a, tt, updates := newTestApp(t, &fakePipeline{}, nsgotest.PendingAlerts())
	a.plugins.root, a.plugins.script.official = root, key.public
	a.pluginsScan()
	waitUntil(t, tt, a, updates, func() bool { return a.plugins.scanned })
	m := testlocale.Messages()
	tt = settingsOpenAt(a, tt, settings.SectionPlugins)
	if a.plugins.script.verified != nil || !hasText(tt, m.SettingsPluginsSourceLocal()) {
		t.Fatalf("before the verification; texts %q", tt.Texts())
	}
	scriptsSettle(t, a, tt, updates, func() bool { return a.plugins.script.verified != nil })
	if !hasText(tt, m.SettingsPluginsSourceVerified()) || hasText(tt, m.SettingsPluginsSourceLocal()) {
		t.Errorf("after it; texts %q", tt.Texts())
	}
}

// TestPluginScriptOpenURLWaitsForTheUser checks that a plugin waiting for the user to answer whether a page opens is not held to its call's deadline nor blamed for the time: with the alert open well past the deadline, it still runs, and hears the answer.
func TestPluginScriptOpenURLWaitsForTheUser(t *testing.T) {
	key := newPluginsKey(t)
	a, tt, updates, _ := scriptsApp(t, key, pluginScriptFolder(t, key, "acme.anki", "1.0.0", pluginAnkiFields, pluginAnkiCode))
	a.plugins.script.watch = script.Watch{Beat: 20 * time.Millisecond, Stall: 50 * time.Millisecond, Stalls: 2, Deadline: 300 * time.Millisecond}
	scriptsGrant(t, a, tt, updates, "acme.anki", string(plugins.PermOpenURL))
	var got struct {
		v    any
		done bool
	}
	onEngine(a, func() {
		a.pluginsRunner().Call(a.plugins.script.loaded["acme.anki"].plugin, "openIt", nil, func(v any, err error) { got.v, got.done = v, true })
	})
	scriptsSettle(t, a, tt, updates, func() bool { return len(tt.Alerts()) == 1 })
	// Five deadlines go by with the alert open, the test's own pauses between steps counting as the window's.
	for end := time.Now().Add(1500 * time.Millisecond); time.Now().Before(end); {
		time.Sleep(60 * time.Millisecond)
		tt.Settle()
	}
	if a.plugins.script.loaded["acme.anki"] == nil || got.done {
		t.Fatalf("stopped while the user had not answered: %+v, health %+v", got, a.plugins.script.health)
	}
	tt.Answer(0)
	scriptsSettle(t, a, tt, updates, func() bool { return got.done })
	if got.v != false || a.plugins.script.loaded["acme.anki"] == nil {
		t.Errorf("answered %v", got.v)
	}
}

// TestPluginScriptUninstallOfOneNotVerified checks that uninstalling a plugin whose signature does not verify now, as after its signer was revoked, still clears what it stored, under the signer state.json recorded.
func TestPluginScriptUninstallOfOneNotVerified(t *testing.T) {
	key := newPluginsKey(t)
	a, tt, updates, _ := scriptsApp(t, key, pluginScriptFolder(t, key, "acme.anki", "1.0.0", pluginAnkiFields, pluginAnkiCode))
	scriptsGrant(t, a, tt, updates, "acme.anki")
	signer := a.pluginsScriptPacks()[0].Signer
	stored := func() []string {
		var out []string
		onEngine(a, func() {
			inner := a.cx.Child()
			inner.Run(func() {
				inner.ProvideStorage(script.Partition("acme.anki", signer), 0)
				out = inner.Storage().Keys()
			})
			inner.Dispose()
		})
		return out
	}
	if len(stored()) == 0 {
		t.Fatal("the plugin stored nothing")
	}
	onEngine(a, func() {
		a.plugins.script.official = newPluginsKey(t).public
		a.plugins.script.trust = nil
		a.pluginsScan()
	})
	scriptsSettle(t, a, tt, updates, func() bool { return len(a.plugins.script.issues) == 1 && !a.plugins.inboxBusy })
	if a.pluginsScriptPacks()[0].Signer != "" {
		t.Fatal("still verified")
	}
	onEngine(a, func() { a.UninstallPlugin("acme.anki") })
	scriptsSettle(t, a, tt, updates, func() bool { return !a.plugins.busy && len(a.pluginsScriptPacks()) == 0 && !a.plugins.inboxBusy })
	if got := stored(); len(got) != 0 {
		t.Errorf("left stored %q", got)
	}
}

// TestPluginsArgIsClamped checks that a number a plugin sends is clamped before it becomes an int, whatever its size.
func TestPluginsArgIsClamped(t *testing.T) {
	for _, tt := range []struct {
		v    any
		want int
	}{{1e300, 500}, {-1e300, 20}, {float64(7), 7}, {"x", 20}, {math.NaN(), 20}, {math.Inf(1), 500}} {
		c := &ns.HostCall{Data: map[string]any{"limit": tt.v}}
		if got := pluginsArg(c, "limit", 20, 500); got != tt.want {
			t.Errorf("%v gave %d, want %d", tt.v, got, tt.want)
		}
	}
}

// TestPluginScriptGrantPage checks what the install confirmation and the details say of a script plugin's grant (docs/plugin-system.md 6.7, 7.1–7.3): the confirmation gives its size by its version, that AnkiConnect's one address cannot be narrowed to an action, and what Kando cannot keep a plugin from; the details list its permissions under the headings of their groups, the same two lines with them; its row says it needs permissions until what it needs is granted, and that it is unavailable offline while it needs the network; and the audit log keeps the install, the grants and a confirmation cancelled.
func TestPluginScriptGrantPage(t *testing.T) {
	key := newPluginsKey(t)
	a, tt, updates, root := scriptsApp(t, key)
	m := testlocale.Messages()
	tt = settingsOpenAt(a, tt, settings.SectionPlugins)
	folder := pluginScriptFolder(t, key, "acme.anki", "1.0.0", pluginAnkiFields, pluginAnkiCode)
	// A confirmation cancelled is its answer, which the audit log keeps.
	onEngine(a, func() { a.InstallPlugins([]string{folder}) })
	waitUntil(t, tt, a, updates, func() bool { return a.plugins.pending != nil && !a.plugins.busy })
	tt.Press("id:plugins-install-cancel")
	waitUntil(t, tt, a, updates, func() bool { return a.plugins.pending == nil })
	onEngine(a, func() { a.InstallPlugins([]string{folder}) })
	waitUntil(t, tt, a, updates, func() bool { return a.plugins.pending != nil && !a.plugins.busy })
	if p := a.plugins.pending; p.err != nil || p.size <= 0 {
		t.Fatalf("staged %d bytes, %v", p.size, p.err)
	}
	version := m.SettingsPluginsVersionAuthor("1.0.0", "Acme") + " · "
	if !slices.ContainsFunc(tt.Texts(), func(s string) bool { return strings.HasPrefix(s, version) && strings.HasSuffix(s, "KB") }) {
		t.Errorf("the confirmation gives no size by the version; texts %q", tt.Texts())
	}
	for _, s := range []string{m.SettingsPluginsUnscoped("127.0.0.1:8765"), m.SettingsPluginsResidual()} {
		if !hasText(tt, s) {
			t.Errorf("the confirmation does not say %q; texts %q", s, tt.Texts())
		}
	}
	// The single address line is under the network it asks for, before what it asks for optionally.
	if texts := tt.Texts(); !(slices.Index(texts, "· "+m.SettingsPluginsPermNetwork(`POST http://127.0.0.1:8765 /`))+1 == slices.Index(texts, m.SettingsPluginsUnscoped("127.0.0.1:8765")) && slices.Index(texts, m.SettingsPluginsUnscoped("127.0.0.1:8765")) < slices.Index(texts, m.SettingsPluginsWantsOptional())) {
		t.Errorf("the single address line is not under the network; texts %q", texts)
	}
	// It keeps no key, so no line says other programs can read its keys.
	if tt.Exists("id:plugins-install-shared-keys") {
		t.Error("a plugin without keys says other programs read them")
	}
	tt.Press("id:plugins-install-ok")
	waitUntil(t, tt, a, updates, func() bool {
		return a.plugins.pending == nil && !a.plugins.busy && !a.plugins.saving && len(a.pluginsScriptPacks()) == 1 && a.pluginsScriptPacks()[0].Signer != ""
	})
	refresh(tt, a)
	if !hasText(tt, m.SettingsPluginsNeedsGrant()) {
		t.Errorf("the row of a plugin granted nothing does not say it needs permissions; texts %q", tt.Texts())
	}
	tt.Press("id:settings-plugins-details-acme.anki")
	// It reads the saved lines and the line playing, may show a toast and reaches the network and opens pages; it never asks the AI service.
	for group, want := range map[plugins.PermGroup]bool{plugins.GroupRead: true, plugins.GroupChange: true, plugins.GroupNetwork: true, plugins.GroupAI: false} {
		if got := tt.Exists("id:settings-plugins-group-acme.anki-" + strconv.Itoa(int(group))); got != want {
			t.Errorf("the heading of group %d shows %v", group, got)
		}
	}
	for _, id := range []string{"settings-plugins-unscoped-acme.anki", "settings-plugins-residual-acme.anki"} {
		if !tt.Exists("id:" + id) {
			t.Errorf("the details have no %s; texts %q", id, tt.Texts())
		}
	}
	// The network's switch is under the network's heading, after the saved lines' under the reading one.
	texts := tt.Texts()
	at := func(s string) int { return slices.Index(texts, s) }
	if r, n, s, w := at(m.SettingsPluginsGroupRead()), at(m.SettingsPluginsGroupNetwork()), at(m.SettingsPluginsPermSavedRead()), at(m.SettingsPluginsPermNetwork(`POST http://127.0.0.1:8765 /`)); r < 0 || !(r < s && s < n && n < w) {
		t.Errorf("headings and permissions out of order (%d %d %d %d): %q", r, s, n, w, texts)
	}
	// What the rule cannot narrow is under the network's switch, before the optional page it may open.
	if w, u, o := at(m.SettingsPluginsPermNetwork(`POST http://127.0.0.1:8765 /`)), at(m.SettingsPluginsUnscoped("127.0.0.1:8765")), at(m.SettingsPluginsPermOpenURL()); !(w < u && u < o) {
		t.Errorf("the single address line is not under the network (%d %d %d): %q", w, u, o, texts)
	}
	for _, perm := range []string{string(plugins.PermSavedRead), string(plugins.PermLineCurrent)} {
		tt.Press("id:settings-plugins-grant-acme.anki-" + perm)
		waitUntil(t, tt, a, updates, func() bool { return !a.plugins.saving })
	}
	if !hasText(tt, m.SettingsPluginsNeedsGrant()) {
		t.Error("the row of a plugin not granted its network does not say it needs permissions")
	}
	tt.Press("id:settings-plugins-grant-acme.anki-" + pluginsNetwork)
	waitUntil(t, tt, a, updates, func() bool { return !a.plugins.saving })
	if hasText(tt, m.SettingsPluginsNeedsGrant()) || !hasText(tt, m.SettingsPluginsVersionAuthor("1.0.0", "Acme")) {
		t.Errorf("granted what it needs, the row still says it needs permissions; texts %q", tt.Texts())
	}
	tt.Press("id:settings-plugins-switch-acme.anki")
	scriptsSettle(t, a, tt, updates, func() bool { return a.plugins.script.loaded["acme.anki"] != nil && !a.plugins.saving })
	apply(tt, a, func() { a.privacy.offline = true })
	if !hasText(tt, m.SettingsPluginsOfflineUnavailable()) {
		t.Errorf("offline, the row of a plugin that needs the network says nothing; texts %q", tt.Texts())
	}
	apply(tt, a, func() { a.privacy.offline = false })
	if hasText(tt, m.SettingsPluginsOfflineUnavailable()) {
		t.Error("back online, the row still says it is unavailable")
	}
	scriptsSettle(t, a, tt, updates, func() bool {
		data, _ := os.ReadFile(plugins.AuditPath(root))
		log := string(data)
		return strings.Contains(log, "cancel acme.anki 1.0.0") && strings.Contains(log, "install acme.anki 1.0.0 script verified") && strings.Contains(log, "grant acme.anki kando.saved.read kando.line.current network:127.0.0.1")
	})
}

// TestPluginScriptKeysSharedWithOtherPrograms checks that the details of a plugin that keeps a key say other programs can read it where the system's secure store is open to them, as on Windows and Linux, and nowhere else.
func TestPluginScriptKeysSharedWithOtherPrograms(t *testing.T) {
	key := newPluginsKey(t)
	a, tt, _, _ := scriptsApp(t, key, pluginScriptFolder(t, key, "acme.rivers", "1.0.0", pluginRiversFields, pluginRiversCode))
	m := testlocale.Messages()
	tt = settingsOpenAt(a, tt, settings.SectionPlugins)
	for _, shared := range []bool{true, false} {
		was := pluginsKeysShared
		pluginsKeysShared = shared
		t.Cleanup(func() { pluginsKeysShared = was })
		var view *settings.PluginScript
		onEngine(a, func() { view = a.pluginsScriptView(m, a.pluginsScriptPacks()[0], false) })
		if view.SharedKeys != shared {
			t.Errorf("shared %v: the view says %v", shared, view.SharedKeys)
		}
	}
	pluginsKeysShared = true
	refresh(tt, a)
	tt.Press("id:settings-plugins-details-acme.rivers")
	if !tt.Exists("id:settings-plugins-shared-keys-acme.rivers") || !hasText(tt, m.SettingsPluginsSharedKeys()) {
		t.Errorf("no line says other programs read its key; texts %q", tt.Texts())
	}
}

// TestPluginScriptInstallSaysKeysAreShared checks that the install confirmation of a plugin that keeps a key says other programs can read it where the system's secure store is open to them, and does not say so where it is not.
func TestPluginScriptInstallSaysKeysAreShared(t *testing.T) {
	key := newPluginsKey(t)
	a, tt, updates, _ := scriptsApp(t, key)
	tt = settingsOpenAt(a, tt, settings.SectionPlugins)
	folder := pluginScriptFolder(t, key, "acme.rivers", "1.0.0", pluginRiversFields, pluginRiversCode)
	was := pluginsKeysShared
	t.Cleanup(func() { pluginsKeysShared = was })
	for _, shared := range []bool{true, false} {
		pluginsKeysShared = shared
		onEngine(a, func() { a.InstallPlugins([]string{folder}) })
		waitUntil(t, tt, a, updates, func() bool { return a.plugins.pending != nil && !a.plugins.busy })
		if p := a.plugins.pending; p.err != nil {
			t.Fatalf("staging acme.rivers: %v", p.err)
		}
		if got := tt.Exists("id:plugins-install-shared-keys"); got != shared {
			t.Errorf("shared %v: the confirmation's line about other programs reading its keys shows %v; texts %q", shared, got, tt.Texts())
		}
		tt.Press("id:plugins-install-cancel")
		waitUntil(t, tt, a, updates, func() bool { return a.plugins.pending == nil })
	}
}
