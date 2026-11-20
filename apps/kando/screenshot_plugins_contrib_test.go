package main

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"kando/apps/kando/film/subtitle"
	"kando/apps/kando/locales/testlocale"
	"kando/apps/kando/plugins"
	"kando/apps/kando/plugins/script"
	"kando/apps/kando/saved"
	"kando/apps/kando/settings"

	"github.com/nsgo-dev/nsgo/nsgotest"
)

// pluginRiversFields are the fields of acme.rivers, the script plugin of the screenshots of P3b: a lookup source of English words, an LRC subtitle format, an export target of the saved lines, an annotator that draws a line under the long English words, and a key for DeepL.
const pluginRiversFields = `"capabilities": {"host": ["kando.net.credentialFetch"]},
  "contributes": {
    "lookups": [{"id": "words", "export": "lookUp", "languages": ["en"], "label": {"en": "River Words", "zh-Hans": "河流词典", "zh-Hant": "河流詞典", "ja": "川の辞書", "ko": "강 사전"}}],
    "subtitleFormats": [{"id": "lrc", "export": "exportSubtitles", "extension": "lrc", "label": {"en": "LRC", "zh-Hans": "LRC"}}],
    "exporters": [{"id": "cards", "export": "exportNotes", "takes": ["lines"], "label": {"en": "River Cards", "zh-Hans": "河流卡片", "zh-Hant": "河流卡片", "ja": "川のカード", "ko": "강 카드"}}],
    "annotators": [{"id": "long", "export": "annotate", "languages": ["en"], "label": {"en": "Long words", "zh-Hans": "长单词", "zh-Hant": "長單詞", "ja": "長い単語", "ko": "긴 단어"}}]
  },
  "credentials": [{"id": "deepl", "title": {"en": "DeepL API key", "zh-Hans": "DeepL API 密钥", "zh-Hant": "DeepL API 金鑰", "ja": "DeepL API キー", "ko": "DeepL API 키"}, "header": "Authorization", "format": "DeepL-Auth-Key {secret}", "hosts": ["api-free.deepl.com", "api.deepl.com"], "methods": ["POST"], "pathPrefixes": ["/v2/translate"]}]`

// pluginRiversAbout is acme.rivers' author, name and description, as its details, the lookup card and the Saved Lines sheet name it.
const pluginRiversAbout = `"author": "Acme", "name": {"en": "River Study Kit", "zh-Hans": "河流学习包", "zh-Hant": "河流學習包", "ja": "川の学習キット", "ko": "강 학습 키트"},
  "description": {"en": "River words, LRC subtitles and cards of saved lines.", "zh-Hans": "河流词汇、LRC 字幕和收藏句子的卡片。", "zh-Hant": "河流詞彙、LRC 字幕和收藏句子的卡片。", "ja": "川の単語、LRC 字幕、保存したセリフのカード。", "ko": "강 단어, LRC 자막, 저장한 대사 카드."}`

// pluginRiversCode is acme.rivers' code.
const pluginRiversCode = `const words = { trickle: { reading: "/ˈtrɪk(ə)l/", meaning: "n. 细流，涓涓细水；v. 慢慢地流，淌" } }
export async function lookUp(q) {
  const w = words[q.word.toLowerCase()]
  return w ? { word: q.word.toLowerCase(), reading: w.reading, meaning: w.meaning } : null
}
function stamp(ms) {
  const m = Math.floor(ms / 60000), s = Math.floor(ms / 1000) % 60, c = Math.floor(ms / 10) % 100
  return "[" + String(m).padStart(2, "0") + ":" + String(s).padStart(2, "0") + "." + String(c).padStart(2, "0") + "]"
}
export async function exportSubtitles(id, cues) { return cues.map((c) => stamp(c.startMs) + c.lines.map((l) => l.text).join(" / ")).join("\n") + "\n" }
export async function exportNotes(items) { return { sent: items.length, skipped: 0 } }
export async function annotate(lang, lines) {
  return lines.map((line) => {
    const marks = []
    for (const m of line.matchAll(/[A-Za-z]{8,}/g)) marks.push({ start: m.index, end: m.index + m[0].length, tone: "success" })
    return marks
  })
}
`

// screenshotRivers installs acme.rivers in a plugins folder of the fixture's home, a key standing in for Kando's official root, and turns it on, granted what it asks for.
func screenshotRivers(t *testing.T, f *screenshotFixture, a *app, tt *nsgotest.App, updates chan func(), key pluginsKey, folder string) {
	t.Helper()
	root := filepath.Join(f.home, folder)
	pluginsScriptInstalled(t, root, pluginScriptFolderAbout(t, key, "acme.rivers", "1.0.0", pluginRiversAbout, pluginRiversFields, pluginRiversCode))
	a.plugins.root = root
	a.plugins.script.official = key.public
	a.plugins.script.watch = script.Watch{Beat: 100 * time.Millisecond, Stall: time.Minute, Stalls: 3, Deadline: time.Minute}
	onEngine(a, a.pluginsScan)
	scriptsSettle(t, a, tt, updates, func() bool { return a.plugins.script.verified != nil && !a.plugins.inboxBusy })
	scriptsGrant(t, a, tt, updates, "acme.rivers", string(plugins.PermCredFetch))
}

// shootPluginContributions renders what a script plugin contributes (docs/plugin-system.md E10–E13) and its key: the card of a word a script plugin's lookup source answered, which names the plugin (plugins-script-lookup); 导出字幕 with the plugin's format chosen after Kando's own (plugins-script-subtitle-format); the Saved Lines sheet with the plugin's export target chosen, once it sent the lines (plugins-script-exporter); the plugin's details with its key's field and the hosts it is only sent to (plugins-script-credential); and a line with the plugin's marks under its long words (plugins-script-marks).
func shootPluginContributions(t *testing.T, f *screenshotFixture, key pluginsKey) {
	f.shoot(t, "plugins-script-lookup", func(t *testing.T, a *app, tt *nsgotest.App, updates chan func()) {
		screenshotOpen(t, a, tt, updates, "j1", protoCues[0].Start)
		screenshotLoad(tt, a)
		screenshotRivers(t, f, a, tt, updates, key, "contrib-lookup")
		onEngine(a, func() { a.LookUpWord(0, "trickle") })
		scriptsSettle(t, a, tt, updates, func() bool { return !a.vocab.lookup.busy })
		refresh(tt, a)
		if !tt.Exists("id:film-lookup-source") || a.vocab.lookup.source != "plugin:acme.rivers:words" {
			t.Fatalf("no plugin's answer: %+v; texts %q", a.vocab.lookup, tt.Texts())
		}
	})
	f.shoot(t, "plugins-script-subtitle-format", func(t *testing.T, a *app, tt *nsgotest.App, updates chan func()) {
		screenshotOpen(t, a, tt, updates, "j1", protoCues[0].Start)
		screenshotLoad(tt, a)
		screenshotRivers(t, f, a, tt, updates, key, "contrib-format")
		onEngine(a, a.OpenSubexport)
		refresh(tt, a)
		tt.Press("id:export-format-acme.rivers-lrc")
		refresh(tt, a)
		if !tt.Exists("id:export-dialog") || filepath.Ext(a.SubexportPath()) != ".lrc" {
			t.Fatalf("no plugin format: %q; texts %q", a.SubexportPath(), tt.Texts())
		}
	})
	f.shoot(t, "plugins-script-exporter", func(t *testing.T, a *app, tt *nsgotest.App, updates chan func()) {
		screenshotOpen(t, a, tt, updates, "j1", protoCues[0].Start)
		onEngine(a, func() {
			if err := os.Remove(filepath.Join(f.lib, saved.FileName)); err != nil && !os.IsNotExist(err) {
				t.Fatal(err)
			}
			a.saved.book = saved.Book{}
			for _, i := range []int{1, 3, 9} {
				a.ToggleSaveLine(i)
			}
		})
		tt.Key("escape")
		screenshotRivers(t, f, a, tt, updates, key, "contrib-exporter")
		onEngine(a, func() {
			a.SetExportTarget(pluginsContribID("acme.rivers", "cards"))
			a.ExportSaved()
		})
		scriptsSettle(t, a, tt, updates, func() bool { return a.ExportState().Finished })
		refresh(tt, a)
		if e := a.ExportState(); e.Sent != 3 || !tt.Exists("id:saved-export-target") {
			t.Fatalf("the export %+v; texts %q", e, tt.Texts())
		}
	})
	f.shoot(t, "plugins-script-credential", func(t *testing.T, a *app, tt *nsgotest.App, updates chan func()) {
		screenshotRivers(t, f, a, tt, updates, key, "contrib-credential")
		a.prefs.overMain = true
		tt = settingsOpenAt(a, tt, settings.SectionPlugins)
		tt.Press("id:settings-plugins-details-acme.rivers")
		if !hasText(tt, testlocale.Messages().SettingsPluginsCredentialHosts("api-free.deepl.com, api.deepl.com")) {
			t.Fatalf("no key's hosts; texts %q", tt.Texts())
		}
	})
	f.shoot(t, "plugins-script-marks", func(t *testing.T, a *app, tt *nsgotest.App, updates chan func()) {
		for i, job := range a.library.Jobs {
			if job.ID == "j1" {
				a.library.Jobs[i].SpokenLanguage = "en"
			}
		}
		screenshotOpen(t, a, tt, updates, "j1", protoCues[2].Start)
		screenshotLoad(tt, a)
		screenshotRivers(t, f, a, tt, updates, key, "contrib-marks")
		scriptsSettle(t, a, tt, updates, func() bool { return a.CueMarks(2) != nil })
		refresh(tt, a)
		if marks := a.CueMarks(2); !slices.ContainsFunc(marks, func(m subtitle.Mark) bool { return m.Tone == subtitle.ToneSuccess }) {
			t.Fatalf("the line's marks: %+v", marks)
		}
	})
}
