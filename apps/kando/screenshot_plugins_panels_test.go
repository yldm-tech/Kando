package main

import (
	"path/filepath"
	"testing"
	"time"

	"kando/apps/kando/plugins"
	"kando/apps/kando/plugins/script"

	"github.com/nsgo-dev/nsgo/nsgotest"
)

// pluginStudyFields are the fields of acme.study, the script plugin of the screenshot of E15: a panel of the player's 扩展 tab that reads the line playing and lists its long words, and hears line.changed.
const pluginStudyFields = `"capabilities": {"host": ["kando.line.current"]},
  "contributes": {"panels": [{"id": "words", "export": "Words", "icon": "book-open", "title": {"en": "Long Words", "zh-Hans": "长单词", "zh-Hant": "長單詞", "ja": "長い単語", "ko": "긴 단어"}}]},
  "events": ["line.changed"]`

// pluginStudyCode is acme.study's code: its panel shows the line playing and its words of eight letters or more, each on a card.
const pluginStudyCode = `import { host, mount, signal, Text, View } from "nsgo"
export function Words() {
  const line = signal(null)
  async function read() { try { line.set(await host.call("kando.line.current", {})) } catch (e) { line.set(null) } }
  host.on("line.changed", read)
  read()
  const words = () => (line() ? line().original.match(/[A-Za-z]{8,}/g) || [] : [])
  return <View style={{ padding: 14, gap: 10 }}>
    <Text testID="study-line" style={{ color: "#ffffff", fontSize: 13, lineHeight: 19 }}>{line() ? line().original : ""}</Text>
    {words().map((w) => <View style={{ padding: 10, borderRadius: 10, backgroundColor: "rgba(255,255,255,0.08)" }}><Text testID="study-word" style={{ color: "#ffffff", fontSize: 14 }}>{w}</Text></View>)}
  </View>
}
if (host.params.panel === "words") mount(Words)
`

// shootPluginPanels renders the player's 扩展 tab with acme.study's panel showing (plugins-script-panel).
func shootPluginPanels(t *testing.T, f *screenshotFixture, key pluginsKey) {
	f.shoot(t, "plugins-script-panel", func(t *testing.T, a *app, tt *nsgotest.App, updates chan func()) {
		screenshotOpen(t, a, tt, updates, "j1", protoCues[2].Start)
		screenshotLoad(tt, a)
		root := filepath.Join(f.home, "panels")
		pluginsScriptInstalled(t, root, pluginScriptFolder(t, key, "acme.study", "1.0.0", pluginStudyFields, pluginStudyCode))
		a.plugins.root = root
		a.plugins.script.official = key.public
		a.plugins.script.watch = script.Watch{Beat: 100 * time.Millisecond, Stall: time.Minute, Stalls: 3, Deadline: time.Minute}
		onEngine(a, a.pluginsScan)
		scriptsSettle(t, a, tt, updates, func() bool { return a.plugins.script.verified != nil && !a.plugins.inboxBusy })
		scriptsGrant(t, a, tt, updates, "acme.study", string(plugins.PermLineCurrent))
		tt.Press("id:film-tab-extensions")
		scriptsSettle(t, a, tt, updates, func() bool { return tt.Exists("id:study-word") })
		refresh(tt, a)
		if !tt.Exists("id:film-extensions") || tt.Find("id:study-line").Text == "" {
			t.Fatalf("no plugin's panel; texts %q", tt.Texts())
		}
	})
}
