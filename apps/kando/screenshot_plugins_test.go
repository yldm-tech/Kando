package main

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"kando/apps/kando/film"
	"kando/apps/kando/film/subtitle"
	"kando/apps/kando/locales/testlocale"
	"kando/apps/kando/plugins"
	"kando/apps/kando/plugins/script"
	"kando/apps/kando/plugins/task"
	"kando/apps/kando/saved"
	"kando/apps/kando/settings"
	"kando/packages/core"

	"github.com/nsgo-dev/nsgo/nsgotest"
)

// pluginAnkiConnectAbout is the author, name and description of Kando's own kando.ankiconnect (plugins/official/ankiconnect), which its install confirmation shows published by Kando.
const pluginAnkiConnectAbout = `"author": "Kando", "name": {"en": "AnkiConnect", "zh-Hans": "推送到 Anki", "zh-Hant": "推送到 Anki", "ja": "Anki に送る", "ko": "Anki로 보내기"},
  "description": {"en": "Sends your saved lines and vocabulary as notes to Anki on this computer, through the AnkiConnect add-on.", "zh-Hans": "通过 AnkiConnect 插件，把收藏的句子和生词作为笔记发到这台电脑上的 Anki。", "zh-Hant": "透過 AnkiConnect 外掛，把收藏的句子和生詞作為筆記傳到這台電腦上的 Anki。", "ja": "AnkiConnect アドオンを通じて、保存したセリフと単語をこのコンピュータの Anki にノートとして送ります。", "ko": "AnkiConnect 애드온을 통해 저장한 대사와 단어를 이 컴퓨터의 Anki에 노트로 보냅니다."}`

// TestScreenshotsPlugins renders Settings › 插件 with a glossary pack, an AI service pack and a task pack installed, the install confirmation of a third party's script plugin, verified by Kando (plugins-script-install), and of Kando's own (plugins-script-install-official) and a script plugin's details with what it was granted (plugins-script-details), the rows of a script plugin that needs permissions and of one unavailable offline (plugins-script-rows), and with two dictionary packs, which move up and down, the details of one open (settings-plugins-dictionaries), the install confirmation of a pack over the main window, as a pack from 从文件安装… or the command line's inbox shows it, and of a site pack with a checkbox for each of its sites (plugins-install-site), the card of a word on the picture that a dictionary pack answered, which names the dictionary and its credit (plugins-lookup), and a Japanese line with readings over its words, a line in the color of their level under the words of a word list and a dashed one under a word in the vocabulary (plugins-marks), the card of a word of that list with its level and its rank by frequency (plugins-lookup-level), the side panel with a task pack's reply as cards (plugins-task), 翻译 with a translate-style pack's style chosen (plugins-translate-style), the Saved Lines sheet with three lines saved and an export template chosen (plugins-export-template), the sheet that exports the glossary as a pack (plugins-export-glossary), what a script plugin contributes (shootPluginContributions), a script plugin's translator chosen in 翻译 and a card its failure names (shootPluginTranslators), the details of a script plugin that adds a media source (shootPluginSources), and a script plugin's panel in the player's 扩展 tab (shootPluginPanels). The packs are in plugins folders of the fixture's home.
func TestScreenshotsPlugins(t *testing.T) {
	f := newScreenshotFixture(t)
	root := filepath.Join(f.home, "Library", "Application Support", "Kando", "plugins")
	pluginInstalled(t, root, pluginGlossary, pluginService, pluginTaskLine)
	shown, models, turbo := settingsShotFolders(t, f)
	shootSettings(t, f, settings.SectionPlugins, "settings-plugins", shown, models, turbo, func(a *app) {
		shotCLITool(f, a, true)
		packs, issues := plugins.Scan(root)
		state, err := plugins.LoadState(root)
		// The folder is read here rather than off the UI thread, as the scene's app applies no background report; its root is set after, so the read looks at no inbox.
		a.pluginsScanned(packs, issues, state, err)
		a.plugins.root = root
	})
	// The dictionaries' group, two dictionary packs with the buttons that move them up and down, and the details of the first open under its row: what it adds, its license, its credit, its id and the start of its hash, and 卸载….
	dictRoot := filepath.Join(f.home, "dictionary-packs")
	pluginInstalledWith(t, dictRoot, pluginDictionary("example.jmdict", "JMdict (English)", "ja", "en"), map[string][]byte{"words.tsv": []byte("猫\tねこ\tcat\n")})
	pluginInstalledWith(t, dictRoot, pluginDictionary("example.cedict", "CC-CEDICT", "zh", "en"), map[string][]byte{"words.tsv": []byte("猫\tmāo\tcat\n")})
	shootSettingsPressed(t, f, settings.SectionPlugins, "settings-plugins-dictionaries", shown, models, turbo, []string{"id:settings-plugins-details-example.jmdict"}, func(a *app) {
		shotCLITool(f, a, true)
		packs, issues := plugins.Scan(dictRoot)
		state, err := plugins.LoadState(dictRoot)
		// The dictionaries count as being opened already, so the scene starts no open in the background, which would build indexes the scene does not show.
		a.plugins.dicts = map[string]*pluginsDict{}
		for _, p := range packs {
			a.plugins.dicts[p.ID] = &pluginsDict{hash: p.Hash}
		}
		a.pluginsScanned(packs, issues, state, err)
		a.plugins.root = dictRoot
	})
	// 翻译 with a translate-style pack turned on and its style chosen, its prompt in the field.
	styles := filepath.Join(f.home, "styles")
	pluginInstalled(t, styles, pluginStyle)
	shootSettings(t, f, settings.SectionTranslate, "plugins-translate-style", shown, models, turbo, func(a *app) {
		packs, issues := plugins.Scan(styles)
		state, err := plugins.LoadState(styles)
		a.pluginsScanned(packs, issues, state, err)
		a.plugins.root = styles
		a.prefs.translatePrompt = packs[0].Style.Prompt
	})
	f.shoot(t, "plugins-install", func(t *testing.T, a *app, tt *nsgotest.App, updates chan func()) {
		a.plugins.root = filepath.Join(f.home, "staging")
		staged, pack, err := plugins.Stage(a.plugins.root, pluginFolder(t, pluginGlossary))
		if err != nil {
			t.Fatal(err)
		}
		a.pluginsAsk(&pluginsPending{pack: pack, staged: staged})
		refresh(tt, a)
		if !tt.Exists("id:plugins-install-confirm") {
			t.Fatalf("no confirmation; texts %q", tt.Texts())
		}
	})
	// The confirmation of a site pack, with a checkbox for each of its sites, which the user allows one by one: the first ticked, the second not yet.
	f.shoot(t, "plugins-install-site", func(t *testing.T, a *app, tt *nsgotest.App, updates chan func()) {
		a.plugins.root = filepath.Join(f.home, "staging")
		staged, pack, err := plugins.Stage(a.plugins.root, pluginFolder(t, pluginSite))
		if err != nil {
			t.Fatal(err)
		}
		a.pluginsAsk(&pluginsPending{pack: pack, staged: staged})
		a.SetPluginInstallSite("videos.example.org", true)
		refresh(tt, a)
		if !tt.Exists("id:plugins-install-confirm") || !tt.Exists("id:plugins-install-site-videos.example.org") || !tt.Exists("id:plugins-install-site-clips.example.net") {
			t.Fatalf("no confirmation with the sites; texts %q", tt.Texts())
		}
	})
	f.shoot(t, "plugins-lookup", func(t *testing.T, a *app, tt *nsgotest.App, updates chan func()) {
		screenshotOpen(t, a, tt, updates, "j1", protoCues[0].Start)
		screenshotLoad(tt, a)
		a.plugins.root = filepath.Join(f.home, "dictionaries")
		pluginsDictInstalled(t, a, tt, updates, pluginDictionary("example.rivers", "River words", "en", "zh-Hans"),
			map[string][]byte{"words.tsv": []byte("trickle\t/ˈtrɪk(ə)l/\tn. 细流，涓涓细水；v. 慢慢地流，淌\n")})
		onEngine(a, func() { a.LookUpWord(0, "trickle") })
		waitUntil(t, tt, a, updates, func() bool { return !a.vocab.lookup.busy })
		refresh(tt, a)
		if !tt.Exists("id:film-lookup-source") || a.vocab.lookup.source != "example.rivers" {
			t.Fatalf("no dictionary's answer: %+v; texts %q", a.vocab.lookup, tt.Texts())
		}
	})
	// A third party's script plugin, acme.anki, as the install confirmation shows it, certified by a key standing in for Kando's official root: published by acme and verified by Kando, what it asks for, needed and optional, that AnkiConnect's one address cannot be narrowed to an action, the lines read together with the network flagged, what Kando cannot keep a plugin from, that it stays off once installed, and its size by its version.
	key := newPluginsKey(t)
	f.shoot(t, "plugins-script-install", func(t *testing.T, a *app, tt *nsgotest.App, updates chan func()) {
		a.plugins.root = filepath.Join(f.home, "scripts")
		a.plugins.script.official = key.public
		onEngine(a, func() {
			staged, pack, err := plugins.Stage(a.plugins.root, pluginScriptFolder(t, key, "acme.anki", "1.2.0", pluginAnkiFields, pluginAnkiCode))
			v, err := pluginsVerifyStaged(a.pluginsStagedVerifier(), pack, err)
			a.pluginsAskStaged(pack, staged, v, pluginsStagedSize(pack, staged, err), err)
		})
		refresh(tt, a)
		if !tt.Exists("id:plugins-install-confirm") || !tt.Exists("id:plugins-install-risky") || !tt.Exists("id:plugins-install-unscoped") || !tt.Exists("id:plugins-install-residual") || a.plugins.pending.size <= 0 || !hasText(tt, testlocale.Messages().SettingsPluginsInstallSourceVerified("acme")) {
			t.Fatalf("no confirmation of the script plugin: %v; texts %q", a.plugins.pending.err, tt.Texts())
		}
	})
	// Kando's own plugin, kando.ankiconnect, signed under the official root with a certificate for kando.*: published by Kando.
	f.shoot(t, "plugins-script-install-official", func(t *testing.T, a *app, tt *nsgotest.App, updates chan func()) {
		a.plugins.root = filepath.Join(f.home, "official")
		a.plugins.script.official = key.public
		onEngine(a, func() {
			staged, pack, err := plugins.Stage(a.plugins.root, pluginScriptFolderAbout(t, key, "kando.ankiconnect", "1.2.0", pluginAnkiConnectAbout, pluginAnkiFields, pluginAnkiCode))
			v, err := pluginsVerifyStaged(a.pluginsStagedVerifier(), pack, err)
			a.pluginsAskStaged(pack, staged, v, pluginsStagedSize(pack, staged, err), err)
		})
		refresh(tt, a)
		if p := a.plugins.pending; p.err != nil || p.pack.Source != plugins.SourceOfficial || !hasText(tt, testlocale.Messages().SettingsPluginsInstallSourceOfficial()) {
			t.Fatalf("Kando's own plugin: %+v; texts %q", p.err, tt.Texts())
		}
	})
	// The same plugin installed, its details open on 插件: its permissions under the headings of their groups with the saved lines and the network granted, which leaves out the line playing it needs, as its row says, what its rule cannot narrow, its commands' key conflict, what Kando cannot keep a plugin from, and its settings.
	f.shoot(t, "plugins-script-details", func(t *testing.T, a *app, tt *nsgotest.App, updates chan func()) {
		root := filepath.Join(f.home, "installed")
		pluginsScriptInstalled(t, root, pluginScriptFolder(t, key, "acme.anki", "1.2.0", pluginAnkiFields, pluginAnkiCode))
		a.plugins.root = root
		a.plugins.script.official = key.public
		a.plugins.script.watch = script.Watch{Beat: 100 * time.Millisecond, Stall: time.Minute, Stalls: 3, Deadline: time.Minute}
		a.prefs.overMain = true
		onEngine(a, a.pluginsScan)
		scriptsSettle(t, a, tt, updates, func() bool { return a.plugins.script.verified != nil && !a.plugins.inboxBusy })
		scriptsGrant(t, a, tt, updates, "acme.anki", string(plugins.PermSavedRead), pluginsNetwork)
		tt = settingsOpenAt(a, tt, settings.SectionPlugins)
		tt.Press("id:settings-plugins-details-acme.anki")
		m := testlocale.Messages()
		if !tt.Exists("id:settings-plugins-grant-acme.anki-network") || !tt.Exists("id:settings-plugins-unscoped-acme.anki") || !hasText(tt, m.SettingsPluginsGroupRead()) || !hasText(tt, m.SettingsPluginsGroupNetwork()) || !hasText(tt, m.SettingsPluginsNeedsGrant()) {
			t.Fatalf("no details; texts %q", tt.Texts())
		}
	})
	// The rows of two script plugins: acme.anki, granted nothing yet, needs permissions; acme.rivers, granted its key and on, is unavailable while Kando is offline.
	f.shoot(t, "plugins-script-rows", func(t *testing.T, a *app, tt *nsgotest.App, updates chan func()) {
		root := filepath.Join(f.home, "rows")
		pluginsScriptInstalled(t, root, pluginScriptFolder(t, key, "acme.anki", "1.2.0", pluginAnkiFields, pluginAnkiCode))
		screenshotRivers(t, f, a, tt, updates, key, "rows")
		a.prefs.overMain = true
		apply(tt, a, func() { a.privacy.offline = true })
		tt = settingsOpenAt(a, tt, settings.SectionPlugins)
		m := testlocale.Messages()
		if !hasText(tt, m.SettingsPluginsNeedsGrant()) || !hasText(tt, m.SettingsPluginsOfflineUnavailable()) {
			t.Fatalf("the rows say nothing of their states; texts %q", tt.Texts())
		}
	})
	// A word task's reply on the first line's word, as the side panel shows it once the AI service answered.
	f.shoot(t, "plugins-task", func(t *testing.T, a *app, tt *nsgotest.App, updates chan func()) {
		screenshotOpen(t, a, tt, updates, "j1", protoCues[0].Start)
		screenshotLoad(tt, a)
		pack, err := plugins.Parse([]byte(pluginTaskWord))
		if err != nil {
			t.Fatal(err)
		}
		onEngine(a, func() {
			a.player.panel, a.player.filmSide = true, film.SideTask
			a.plugins.task = pluginsTask{open: true, id: pack.ID, name: pack.Name, fields: pack.Task.Reply.Fields, render: pack.Task.Render, video: a.player.film.video,
				scope: pack.Task.Scope, subject: "trickle", model: "gpt-4o-mini", spent: 412,
				items: []task.Item{{"trick-", "to flow in drops, from Middle English trikelen"}, {"-le", "a small, repeated action, as in sparkle and drizzle"}}}
		})
		refresh(tt, a)
		if !tt.Exists("id:film-task-card") {
			t.Fatalf("no task's reply; texts %q", tt.Texts())
		}
	})
	// The Saved Lines sheet with an export-template pack turned on and chosen, its drop-down beside 导出.
	f.shoot(t, "plugins-export-template", func(t *testing.T, a *app, tt *nsgotest.App, updates chan func()) {
		a.plugins.root = filepath.Join(f.home, "templates")
		pluginInstalled(t, a.plugins.root, pluginTemplate)
		packs, issues := plugins.Scan(a.plugins.root)
		state, err := plugins.LoadState(a.plugins.root)
		// Three lines of How Rivers Shape Landscapes saved, as the saved scene saves them, in a library that has none, as the scenes share its folder.
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
		onEngine(a, func() {
			a.pluginsScanned(packs, issues, state, err)
			a.plugins.exportTarget = "example.cards"
			a.OpenSavedSheet()
		})
		refresh(tt, a)
		if !tt.Exists("id:saved-export-target") || len(a.SavedEntries()) != 3 {
			t.Fatalf("no export drop-down; texts %q", tt.Texts())
		}
	})
	// The sheet that exports the glossary as a pack, over the main window, with a few global terms.
	f.shoot(t, "plugins-export-glossary", func(t *testing.T, a *app, tt *nsgotest.App, updates chan func()) {
		onEngine(a, func() {
			for _, term := range []core.Term{{Source: "meander", Target: "河曲"}, {Source: "delta", Target: "三角洲"}, {Source: "tributary", Target: "支流"}} {
				term.Scope = core.ScopeGlobal
				a.gloss.store.Set("", term)
			}
			a.pluginsExportOpen(true)
			a.SetPluginExportName("River Terms")
		})
		refresh(tt, a)
		if !tt.Exists("id:plugins-export") {
			t.Fatalf("no export sheet; texts %q", tt.Texts())
		}
	})
	// A film heard in Japanese, its first line marked by a Japanese word list chosen in 字幕外观's 逐词标注 with readings and 标出已学词 on: furigana over the words in Han characters, from the larger Japanese dictionary the list lets the app read, a thick line in the tone of their level under the words of the list, and a dashed one under 小さな, a word in the vocabulary.
	f.shoot(t, "plugins-marks", func(t *testing.T, a *app, tt *nsgotest.App, updates chan func()) {
		for i, job := range a.library.Jobs {
			if job.ID == "j1" {
				a.library.Jobs[i].SpokenLanguage = "ja"
			}
		}
		screenshotOpen(t, a, tt, updates, "j1", protoCues[0].Start)
		screenshotLoad(tt, a)
		onEngine(a, func() {
			subs := &a.player.subs
			subs.machine = slices.Clone(subs.machine)
			translation, _ := film.SplitCue(subs.machine[0].Text)
			subs.machine[0].Text = translation + "\n" + "どの川も、山の上の小さな流れから始まります。"
			a.playerCorrectSubs()
		})
		a.plugins.root = filepath.Join(f.home, "wordlists")
		pluginsDictInstalled(t, a, tt, updates, pluginWordlist("example.jlpt", "ja"), map[string][]byte{"words.tsv": []byte("川\tb1\n山\tb1\n流れ\tc1\n始まる\tc1\n")})
		onEngine(a, func() {
			a.saved.book.AddWord(saved.Word{Text: "小さな", Spoken: "ja"})
			a.SetMarkReadings(true)
			a.SetMarkWordlist("example.jlpt")
			a.SetMarkKnown(true)
		})
		waitUntil(t, tt, a, updates, func() bool { return a.plugins.marks.lines != nil })
		refresh(tt, a)
		if marks := a.CueMarks(0); !slices.ContainsFunc(marks, func(m subtitle.Mark) bool { return m.Ruby == "かわ" }) || !slices.ContainsFunc(marks, func(m subtitle.Mark) bool { return m.Tone == subtitle.ToneWarn }) || !slices.ContainsFunc(marks, func(m subtitle.Mark) bool { return m.Underline }) {
			t.Fatalf("the first line's marks: %+v", marks)
		}
	})
	// The card of 流れ, a word of a Japanese film's first line, looked up with a Japanese word list chosen in 逐词标注 that gives each word's rank by frequency: beside its reading, its level C1 in the color of the line drawn under it, with its rank, and what it means from a Japanese dictionary pack.
	f.shoot(t, "plugins-lookup-level", func(t *testing.T, a *app, tt *nsgotest.App, updates chan func()) {
		for i, job := range a.library.Jobs {
			if job.ID == "j1" {
				a.library.Jobs[i].SpokenLanguage = "ja"
			}
		}
		screenshotOpen(t, a, tt, updates, "j1", protoCues[0].Start)
		screenshotLoad(tt, a)
		onEngine(a, func() {
			subs := &a.player.subs
			subs.machine = slices.Clone(subs.machine)
			translation, _ := film.SplitCue(subs.machine[0].Text)
			subs.machine[0].Text = translation + "\n" + "どの川も、山の上の小さな流れから始まります。"
			a.playerCorrectSubs()
		})
		a.plugins.root = filepath.Join(f.home, "levels")
		pluginsDictInstalled(t, a, tt, updates, pluginWordlist("example.jlpt", "ja"), map[string][]byte{"words.tsv": []byte("川\tb1\t412\n山\tb1\t388\n流れ\tc1\t1834\n始まる\tc1\t2051\n")})
		pluginsDictInstalled(t, a, tt, updates, pluginDictionary("example.ja", "Japanese words", "ja", "zh-Hans"), map[string][]byte{"words.tsv": []byte("流れ\tながれ\t水流，流动；（事物的）趋势，进程\n")})
		onEngine(a, func() { a.SetMarkWordlist("example.jlpt") })
		waitUntil(t, tt, a, updates, func() bool { return a.plugins.marks.lines != nil })
		onEngine(a, func() { a.LookUpWord(0, "流れ") })
		waitUntil(t, tt, a, updates, func() bool { return !a.vocab.lookup.busy })
		refresh(tt, a)
		if !hasText(tt, testlocale.Messages().VocabLookupLevelRank("C1", 1834)) || a.vocab.lookup.source != "example.ja" {
			t.Fatalf("no level on the card: %+v; texts %q", a.vocab.lookup, tt.Texts())
		}
	})
	shootPluginContributions(t, f, key)
	shootPluginTranslators(t, f, key)
	shootPluginSources(t, f, key)
	shootPluginPanels(t, f, key)
}
