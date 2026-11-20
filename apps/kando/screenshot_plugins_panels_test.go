package main

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"kando/apps/kando/locales/testlocale"
	"kando/apps/kando/plugins"
	"kando/apps/kando/plugins/script"

	"github.com/nsgo-dev/nsgo/nsgotest"
)

// pluginStudyFields are the fields of acme.study, the script plugin of the screenshot of E15: a panel of the player's 扩展 tab that reads the line playing and lists its long words, and hears line.changed.
const pluginStudyFields = `"capabilities": {"host": ["kando.line.current"]},
  "contributes": {"panels": [{"id": "words", "export": "Words", "icon": "book-open", "title": {"en": "Long Words", "zh-Hans": "长单词", "zh-Hant": "長單詞", "ja": "長い単語", "ko": "긴 단어"}}]},
  "events": ["line.changed"]`

// pluginStudyAbout is acme.study's author, name and description, as its panel names it.
const pluginStudyAbout = `"author": "Acme", "name": {"en": "Line Study", "zh-Hans": "台词精读", "zh-Hant": "台詞精讀", "ja": "セリフ精読", "ko": "대사 정독"},
  "description": {"en": "Lists the long words of the line playing.", "zh-Hans": "列出正在播放的这句里的长单词。", "zh-Hant": "列出正在播放的這句裡的長單詞。", "ja": "再生中のセリフの長い単語を並べます。", "ko": "재생 중인 대사의 긴 단어를 보여 줍니다."}`

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

// shootPluginPanels renders the player's 扩展 tab with acme.study's panel showing (plugins-script-panel), the seven tabs on one row of the bar, every label whole, in the language of the run (KANDO_SCREENSHOTS_LANG).
func shootPluginPanels(t *testing.T, f *screenshotFixture, key pluginsKey) {
	f.shoot(t, "plugins-script-panel", func(t *testing.T, a *app, tt *nsgotest.App, updates chan func()) {
		screenshotOpen(t, a, tt, updates, "j1", protoCues[2].Start)
		screenshotLoad(tt, a)
		root := filepath.Join(f.home, "panels")
		pluginsScriptInstalled(t, root, pluginScriptFolderAbout(t, key, "acme.study", "1.0.0", pluginStudyAbout, pluginStudyFields, pluginStudyCode))
		a.plugins.root = root
		a.plugins.script.official = key.public
		a.plugins.script.watch = script.Watch{Beat: 100 * time.Millisecond, Stall: time.Minute, Stalls: 3, Deadline: time.Minute}
		a.plugins.panels.watch = script.PanelWatch{Window: time.Hour, Share: 0.6, Windows: 3, Probe: time.Minute, Idle: 0.3, Retries: 3}
		onEngine(a, a.pluginsScan)
		scriptsSettle(t, a, tt, updates, func() bool { return a.plugins.script.verified != nil && !a.plugins.inboxBusy })
		scriptsGrant(t, a, tt, updates, "acme.study", string(plugins.PermLineCurrent))
		tt.Press("id:film-tab-extensions")
		scriptsSettle(t, a, tt, updates, func() bool { return tt.Exists("id:study-word") })
		refresh(tt, a)
		if !tt.Exists("id:film-extensions") || tt.Find("id:study-line").Text == "" {
			t.Fatalf("no plugin's panel; texts %q", tt.Texts())
		}
		if why := screenshotTabsOverflow(tt); why != "" {
			t.Errorf("the side panel's tabs do not fit in %s: %s", testlocale.Current(), why)
		}
	})
}

// screenshotTabsOverflow is why the side panel's seven tabs do not fit on one row of its bar, "" when they do: every tab whole within the bar, which needs no scrolling, and every label whole within its tab with its padding; 扩展 is an icon, named for the screen reader.
func screenshotTabsOverflow(tt *nsgotest.App) string {
	bar, track := tt.Find("id:film-tabs-scroll"), tt.Find("id:film-tabs")
	if bar == nil || track == nil {
		return "no tab bar"
	}
	if track.Frame.Size.Width > bar.Frame.Size.Width+0.5 {
		return fmt.Sprintf("the track is %v wide in a bar of %v", track.Frame.Size.Width, bar.Frame.Size.Width)
	}
	for _, id := range []string{"subs", "clips", "terms", "notes", "info", "extensions", "style"} {
		tab := tt.Find("id:film-tab-" + id)
		if tab == nil {
			return "no tab " + id
		}
		if f := tab.Frame; f.Origin.X < 0 || f.Origin.X+f.Size.Width > track.Frame.Size.Width+0.5 {
			return fmt.Sprintf("tab %s at %+v past the bar of %v", id, f, track.Frame.Size.Width)
		}
		if id == "extensions" {
			if tab.Props.Label != testlocale.Messages().FilmTabExtensions() {
				return fmt.Sprintf("扩展 is named %q", tab.Props.Label)
			}
			continue
		}
		for _, c := range tab.Children {
			if c.Text == "" {
				continue
			}
			if f, w := c.Frame, tab.Frame.Size.Width; f.Size.Width <= 0 || f.Origin.X < 0 || f.Origin.X+f.Size.Width > w+0.5 || w < f.Size.Width+6-0.5 {
				return fmt.Sprintf("%s: %q at %+v in a tab %v wide", id, c.Text, f, w)
			}
		}
	}
	return ""
}
