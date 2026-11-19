package main

import (
	"path/filepath"
	"testing"
	"time"

	"kando/apps/kando/locales/testlocale"
	"kando/apps/kando/plugins"
	"kando/apps/kando/plugins/script"
	"kando/apps/kando/settings"
	"kando/packages/media"

	"github.com/nsgo-dev/nsgo/nsgotest"
)

// shootPluginTranslators renders a script plugin's translator (docs/plugin-system.md E14): 翻译 with acme.mt's translator chosen as the translation source, its line saying proofreading stays with the AI service (plugins-translate-source); and the card of Street Food Night Tour (j5) whose translation DeepL failed, its key refused, which names the source and opens 插件 (plugins-translate-failed).
func shootPluginTranslators(t *testing.T, f *screenshotFixture, key pluginsKey) {
	f.shoot(t, "plugins-translate-source", func(t *testing.T, a *app, tt *nsgotest.App, updates chan func()) {
		a.plugins.root = filepath.Join(f.home, "translators")
		pluginsScriptInstalled(t, a.plugins.root, pluginScriptFolder(t, key, "acme.mt", "1.0.0", pluginMTFields, pluginMTCode))
		a.plugins.script.official = key.public
		a.plugins.script.watch = script.Watch{Beat: 100 * time.Millisecond, Stall: time.Minute, Stalls: 3, Deadline: time.Minute}
		onEngine(a, a.pluginsScan)
		scriptsSettle(t, a, tt, updates, func() bool { return a.plugins.script.verified != nil && !a.plugins.inboxBusy })
		scriptsGrant(t, a, tt, updates, "acme.mt")
		onEngine(a, func() { a.SetTranslateSource("plugin:acme.mt:caps") })
		a.prefs.overMain = true
		tt = settingsOpenAt(a, tt, settings.SectionTranslate)
		if !tt.Exists("id:settings-translate-source") || !hasText(tt, testlocale.Messages().SettingsTranslateSourceHint()) || a.TranslateSource() != "plugin:acme.mt:caps" {
			t.Fatalf("no translation source chosen: %q; texts %q", a.TranslateSource(), tt.Texts())
		}
	})
	f.shoot(t, "plugins-translate-failed", func(t *testing.T, a *app, tt *nsgotest.App, updates chan func()) {
		job := f.job(t, "j5")
		job.FailKind = ""
		job.Err = &media.StageError{Stage: media.StageTranslate, Source: "DeepL", Err: plugins.NewTranslateError(plugins.TranslateAuth, "DeepL did not accept the key")}
		a.library.Upsert(job)
		queueShotScroll(t, a, tt)
		wantText(t, tt, testlocale.Messages().PrecheckSourceAuth("DeepL"))
	})
}
