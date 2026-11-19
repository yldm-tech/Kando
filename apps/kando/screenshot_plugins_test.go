package main

import (
	"path/filepath"
	"testing"

	"kando/apps/kando/plugins"
	"kando/apps/kando/settings"

	"github.com/nsgo-dev/nsgo/nsgotest"
)

// TestScreenshotsPlugins renders Settings › 插件 with a glossary pack and an AI service pack installed, and the install confirmation of a pack over the main window, as a pack from 从文件安装… or the command line's inbox shows it. The packs are in the plugins folder of the fixture's home.
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
}
