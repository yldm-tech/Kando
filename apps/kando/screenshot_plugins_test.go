package main

import (
	"path/filepath"
	"testing"

	"kando/apps/kando/plugins"
	"kando/apps/kando/settings"

	"github.com/nsgo-dev/nsgo/nsgotest"
)

// TestScreenshotsPlugins renders Settings › 插件 with a glossary pack and an AI service pack installed, the install confirmation of a pack over the main window, as a pack from 从文件安装… or the command line's inbox shows it, and the card of a word on the picture that a dictionary pack answered, which names the dictionary and its credit (plugins-lookup). The packs are in plugins folders of the fixture's home.
func TestScreenshotsPlugins(t *testing.T) {
	f := newScreenshotFixture(t)
	root := filepath.Join(f.home, "Library", "Application Support", "Kando", "plugins")
	pluginInstalled(t, root, pluginGlossary, pluginService)
	shown, models, turbo := settingsShotFolders(t, f)
	shootSettings(t, f, settings.SectionPlugins, "settings-plugins", shown, models, turbo, func(a *app) {
		shotCLITool(f, a, true)
		packs, issues := plugins.Scan(root)
		state, err := plugins.LoadState(root)
		// The folder is read here rather than off the UI thread, as the scene's app applies no background report; its root is set after, so the read looks at no inbox.
		a.pluginsScanned(packs, issues, state, err)
		a.plugins.root = root
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
}
