package main

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"kando/apps/kando/locales/testlocale"
	"kando/apps/kando/plugins"
	"kando/apps/kando/plugins/index"
	"kando/apps/kando/settings"
)

// pluginsIndexSceneEntries are the packs the scene's directory lists: a glossary installed at an older version, a word list, a dictionary, a task, a third party's script plugin verified by Kando, and Kando's own that needs a newer Kando.
func pluginsIndexSceneEntries() []index.Entry {
	entry := func(id string, kind plugins.Kind, version, name, zh, summary, zhSummary string, size int64) index.Entry {
		publisher, _, _ := strings.Cut(id, ".")
		return index.Entry{ID: id, Kind: kind, Version: version, Publisher: publisher, License: "CC-BY-4.0", Size: size,
			Name: plugins.Text{"en": name, "zh-Hans": zh}, Summary: plugins.Text{"en": summary, "zh-Hans": zhSummary},
			SHA256: strings.Repeat("ab", 32), URL: "https://kando.yldm.ai/plugin/packs/" + id + ".zip"}
	}
	anki := entry("acme.anki", plugins.KindScript, "1.0.0", "Anki Sender", "Anki 发送器", "Sends saved lines to Anki.", "把收藏的句子发到 Anki。", 182<<10)
	anki.Source, anki.License = plugins.SourceVerified, "MIT"
	official := entry("kando.ankiconnect", plugins.KindScript, "1.2.0", "AnkiConnect", "推送到 Anki", "Send saved lines and words straight into Anki.", "把收藏的句子和生词直接发到 Anki。", 96<<10)
	official.Source, official.License, official.MinKando = plugins.SourceOfficial, "MIT", "99.0.0"
	return []index.Entry{
		anki,
		entry("example.anime-ja-zh", plugins.KindGlossary, "1.3.0", "Anime terms", "动画常用词", "Common anime terms, Japanese into Chinese.", "动画里常见的说法，日译中。", 24<<10),
		entry("example.jlpt", plugins.KindWordlist, "2026.1", "JLPT levels", "JLPT 等级", "Japanese words by JLPT level, N5 to N1.", "按 JLPT 等级（N5 到 N1）标出日语词。", 412<<10),
		entry("example.jmdict", plugins.KindDictionary, "2026.11.0", "JMdict (English)", "JMdict（英文释义）", "Japanese–English dictionary for offline lookup.", "离线查词用的日英词典。", 18<<20),
		entry("example.quiz", plugins.KindTask, "1.0.0", "Quiz me", "考考我", "Asks one question about the line.", "就这一句出一道题。", 3<<10),
		official,
	}
}

// pluginsIndexScene puts the app in the scene's state: the glossary installed at 1.2.0 in root, 浏览插件 open with the scene's directory, updated at a fixed time, read from no server.
func pluginsIndexScene(t *testing.T, a *app, root string) {
	t.Helper()
	pluginInstalled(t, root, pluginGlossary)
	packs, issues := plugins.Scan(root)
	state, err := plugins.LoadState(root)
	a.pluginsScanned(packs, issues, state, err)
	a.plugins.root = root
	p := &a.plugins.index
	p.open, p.read = true, true
	p.set(index.Index{Format: index.Format, Version: index.Version, Generated: time.Date(2026, 11, 19, 10, 0, 0, 0, time.UTC), Entries: pluginsIndexSceneEntries()},
		index.Meta{URL: index.DefaultURL, Fetched: time.Date(2026, 11, 19, 10, 30, 0, 0, time.Local)})
}

// TestScreenshotsPluginIndex renders Settings › 插件 with 浏览插件 open (plugins-index): the directory's heading with 刷新 and when it was updated, the search field and the kind filter, and six packs, a glossary with 更新 beside the version installed, a script plugin verified by Kando, one of Kando's own that needs a newer Kando, and the rest with 安装.
func TestScreenshotsPluginIndex(t *testing.T) {
	f := newScreenshotFixture(t)
	shown, models, turbo := settingsShotFolders(t, f)
	shootSettings(t, f, settings.SectionPlugins, "plugins-index", shown, models, turbo, func(a *app) {
		shotCLITool(f, a, true)
		pluginsIndexScene(t, a, filepath.Join(f.home, "Library", "Application Support", "Kando", "plugins"))
	})
}

// TestPluginIndexScene checks headless what the plugins-index scene shows, as its picture is rendered only on a platform: every pack's row, where each stands, the source of the script plugins and the line of when the list was updated.
func TestPluginIndexScene(t *testing.T) {
	a, tt, _, root := pluginsApp(t)
	m := testlocale.Messages()
	apply(tt, a, func() { pluginsIndexScene(t, a, root) })
	tt = settingsOpenAt(a, tt, settings.SectionPlugins)
	for _, e := range pluginsIndexSceneEntries() {
		if !tt.Exists("id:settings-plugins-index-row-" + e.ID) {
			t.Errorf("no row of %s; texts %q", e.ID, tt.Texts())
		}
	}
	for _, id := range []string{"settings-plugins-index-update-example.anime-ja-zh", "settings-plugins-index-install-acme.anki", "settings-plugins-index-install-example.jmdict", "settings-plugins-index-refresh", "settings-plugins-index-search", "settings-plugins-index-kind"} {
		if !tt.Exists("id:" + id) {
			t.Errorf("no %s; texts %q", id, tt.Texts())
		}
	}
	for _, s := range []string{m.SettingsPluginsIndexInstalledVersion("1.2.0"), m.SettingsPluginsIndexNeedsKando("99.0.0"), m.SettingsPluginsSourceVerified(), m.SettingsPluginsSourceOfficial(), m.SettingsPluginsIndexUpdated("2026-11-19 10:30"), m.SettingsPluginsIndexFacts("example", "2026.11.0", "18.0 MB", "CC-BY-4.0")} {
		if !hasText(tt, s) {
			t.Errorf("no %q; texts %q", s, tt.Texts())
		}
	}
	if tt.Exists("id:settings-plugins-index-install-kando.ankiconnect") || tt.Exists("id:settings-plugins-index-more") {
		t.Errorf("a plugin for a newer Kando can be installed, or the list says it shows fewer than match; texts %q", tt.Texts())
	}
}
