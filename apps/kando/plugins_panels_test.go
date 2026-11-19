package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"kando/apps/kando/film"
	"kando/apps/kando/locales/testlocale"
	"kando/apps/kando/plugins"
	"kando/apps/kando/plugins/script"
	"kando/apps/kando/settings"

	"github.com/nsgo-dev/nsgo/nsgotest"
)

// pluginPanelsFields are the fields of acme.panels, the test plugin with panels of the player's 扩展 tab: lines, which shows the line playing as kando.line.current gives it and how many line.changed it heard, and broken, which throws as it renders.
const pluginPanelsFields = `"capabilities": {"host": ["kando.line.current"]},
  "contributes": {"panels": [
    {"id": "lines", "export": "Lines", "icon": "captions", "title": {"en": "Line Watch", "zh-Hans": "台词观察"}},
    {"id": "broken", "export": "Broken", "title": {"en": "Broken", "zh-Hans": "坏面板"}}
  ]},
  "events": ["line.changed"]`

// pluginPanelsCode is acme.panels' code: its entry module mounts the panel its view's parameters name, and nothing as it is loaded for its functions.
const pluginPanelsCode = `import { host, mount, signal, Text, View } from "nsgo"
export function Lines() {
  const said = signal("…")
  const heard = signal(0)
  async function read() {
    try { const l = await host.call("kando.line.current", {}); said.set(l ? l.original : "none") } catch (e) { said.set("refused " + e.code) }
  }
  host.on("line.changed", () => { heard.set(heard() + 1); read() })
  read()
  return <View><Text testID="panel-line">{said()}</Text><Text testID="panel-heard">{"heard " + heard()}</Text></View>
}
export function Broken() { throw new Error("broken panel") }
if (host.params.panel === "lines") mount(Lines)
if (host.params.panel === "broken") mount(Broken)
`

// panelsApp is playerFilm's library with the film talk open and loaded, the plugins of folders installed in a plugins folder of its own, key standing in for Kando's official root, and a watchdog that does not count the test's own pauses.
func panelsApp(t *testing.T, key pluginsKey, folders ...string) (*app, *nsgotest.App, chan func()) {
	t.Helper()
	a, tt, updates, item := playerFilm(t, 12, nil, nsgotest.PendingAlerts(), nsgotest.Secrets(map[string]string{}))
	root := filepath.Join(t.TempDir(), "plugins")
	pluginsScriptInstalled(t, root, folders...)
	a.plugins.root = root
	a.plugins.script.official = key.public
	a.plugins.script.watch = script.Watch{Beat: 100 * time.Millisecond, Stall: time.Minute, Stalls: 3, Deadline: time.Minute}
	onEngine(a, a.pluginsScan)
	waitUntil(t, tt, a, updates, func() bool {
		return a.plugins.scanned && !a.plugins.busy && !a.plugins.inboxBusy && a.plugins.script.verified != nil
	})
	playerOpenFilm(t, a, tt, updates, item)
	playerLoadFilm(tt, a, 12*time.Second)
	return a, tt, updates
}

// TestPluginPanels checks a script plugin's panels in the player's 扩展 tab (docs/plugin-system.md E15): the tab is there only while the plugin runs; the panel is the plugin's view, which reads the line playing through kando.line.current under the plugin's grant and hears line.changed as the line changes, while the tab is not built again; the drop-down picks the other panel, which throws and shows that it failed in the tab, the player still there, and whose failures turn the plugin off at the third; the choice is kept in settings.json; and the tab goes once the plugin is turned off.
func TestPluginPanels(t *testing.T) {
	key := newPluginsKey(t)
	a, tt, updates := panelsApp(t, key, pluginScriptFolder(t, key, "acme.panels", "1.0.0", pluginPanelsFields, pluginPanelsCode))
	m := testlocale.Messages()
	if tt.Exists("id:film-tab-extensions") {
		t.Fatal("扩展 shows before the plugin runs")
	}
	scriptsGrant(t, a, tt, updates, "acme.panels", string(plugins.PermLineCurrent))
	if !tt.Exists("id:film-tab-extensions") {
		t.Fatalf("no 扩展 tab once the plugin runs; texts %q", tt.Texts())
	}
	tt.Press("id:film-tab-extensions")
	scriptsSettle(t, a, tt, updates, func() bool { return tt.Exists("id:panel-line") && tt.Find("id:panel-line").Text == "none" })
	for _, want := range []string{"台词观察", m.FilmExtensionFrom("Anki 发送器")} {
		if !tt.Exists("id:film-extensions >> " + whole(want)) {
			t.Errorf("the tab has no %q; texts %q", want, tt.Texts())
		}
	}
	l := a.plugins.script.loaded["acme.panels"]
	if l.panel == nil || l.panel == l.host {
		t.Fatal("the panel's view has no host of its own")
	}
	// The line changes as the film plays: the panel hears it and reads the line through its grant.
	playerPlayTo(tt, a, 1500*time.Millisecond)
	scriptsSettle(t, a, tt, updates, func() bool { return tt.Find("id:panel-line").Text == "Hello." })
	playerPlayTo(tt, a, 4500*time.Millisecond)
	scriptsSettle(t, a, tt, updates, func() bool { return tt.Find("id:panel-line").Text == "Bye." })
	if heard := tt.Find("id:panel-heard").Text; heard == "heard 0" {
		t.Errorf("the panel heard no line.changed: %q", heard)
	}
	if a.plugins.script.loaded["acme.panels"].panel != l.panel {
		t.Error("the panel's view was built again as the line changed")
	}
	// The other panel, from the drop-down, throws: the tab says so, and the player is still there.
	if !tt.Exists("id:film-extension-pick") {
		t.Fatalf("no drop-down of the two panels; texts %q", tt.Texts())
	}
	broken := pluginsContribID("acme.panels", "broken")
	apply(tt, a, func() { a.SetExtension(broken) })
	scriptsSettle(t, a, tt, updates, func() bool { return tt.Exists("id:film-extension-failed") })
	if !hasText(tt, m.FilmExtensionFailed("Anki 发送器")) || !tt.Exists("id:film-video") || !tt.Exists("id:film-panel") {
		t.Fatalf("a panel that throws; texts %q", tt.Texts())
	}
	if a.plugins.panels.chosen != broken {
		t.Errorf("chose %q", a.plugins.panels.chosen)
	}
	if raw, err := os.ReadFile(a.settingsPath); err != nil || !strings.Contains(string(raw), `"pluginPanel": "`+broken+`"`) {
		t.Errorf("settings.json does not keep the panel chosen: %v\n%s", err, raw)
	}
	// Each failure, a retry's too, counts for the plugin's breaker: the third turns it off, and the tab goes with it.
	for range 2 {
		tt.Press("id:film-extension-retry")
		refresh(tt, a)
	}
	scriptsSettle(t, a, tt, updates, func() bool { return a.plugins.script.loaded["acme.panels"] == nil && !a.plugins.saving })
	if h, sick := plugins.HealthOf(a.plugins.script.health, "acme.panels", l.pack.Hash); !sick || h.Reason != plugins.ReasonFailed {
		t.Errorf("health %+v", a.plugins.script.health)
	}
	if tt.Exists("id:film-tab-extensions") || tt.Exists("id:film-extensions") || !tt.Exists("id:film-tab-subs") {
		t.Fatalf("the tab stayed with the plugin off; texts %q", tt.Texts())
	}
}

// TestPluginPanelsGoWithThePlugin checks that turning the plugin off takes its panel and the 扩展 tab away, the side panel showing the subtitles again, and turning it on again brings the panel chosen back, built anew for the plugin loaded again; and that its details list its panels among what it adds.
func TestPluginPanelsGoWithThePlugin(t *testing.T) {
	key := newPluginsKey(t)
	a, tt, updates := panelsApp(t, key, pluginScriptFolder(t, key, "acme.panels", "1.0.0", pluginPanelsFields, pluginPanelsCode))
	scriptsGrant(t, a, tt, updates, "acme.panels", string(plugins.PermLineCurrent))
	tt.Press("id:film-tab-extensions")
	scriptsSettle(t, a, tt, updates, func() bool { return tt.Exists("id:panel-line") })
	run := a.plugins.script.loaded["acme.panels"].run
	apply(tt, a, func() { a.SetPluginEnabled("acme.panels", false) })
	scriptsSettle(t, a, tt, updates, func() bool { return a.plugins.script.loaded["acme.panels"] == nil && !a.plugins.saving })
	if tt.Exists("id:film-tab-extensions") || tt.Exists("id:panel-line") || !tt.Exists("id:film-tab-subs") {
		t.Fatalf("the plugin turned off left its panel; texts %q", tt.Texts())
	}
	if a.player.filmSide != film.SideExtensions {
		t.Errorf("the side the user chose was forgotten: %q", a.player.filmSide)
	}
	scriptsGrant(t, a, tt, updates, "acme.panels")
	scriptsSettle(t, a, tt, updates, func() bool { return tt.Exists("id:panel-line") })
	l := a.plugins.script.loaded["acme.panels"]
	if l.run == run || l.panel == nil {
		t.Errorf("the panel was not built anew for the plugin loaded again: run %d, was %d", l.run, run)
	}
	// Its details list its panels among what it adds.
	m := testlocale.Messages()
	if adds := settings.PluginAdds(m, l.pack.Manifest); !slices.Contains(adds, m.SettingsPluginsAddsPanel("台词观察")) || !slices.Contains(adds, m.SettingsPluginsAddsPanel("坏面板")) {
		t.Errorf("adds %q", adds)
	}
}
