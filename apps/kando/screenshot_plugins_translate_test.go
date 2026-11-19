package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"kando/apps/kando/locales/testlocale"
	"kando/apps/kando/plugins"
	"kando/apps/kando/plugins/script"
	"kando/apps/kando/settings"
	"kando/packages/media"

	"github.com/nsgo-dev/nsgo/nsgotest"
)

// shootPluginTranslators renders a script plugin's translator (docs/plugin-system.md E14): 翻译 with Kando's official DeepL plugin, packed from the repository with the scenes' key standing in for the official root, chosen as the translation source, its line saying proofreading stays with the AI service (plugins-translate-source); and the card of Street Food Night Tour (j5) whose translation DeepL failed, its key refused, which names the source and opens 插件, its actions on one row and 移除 its ×, in the scenes' language and in English and Japanese (plugins-translate-failed, -en, -ja).
func shootPluginTranslators(t *testing.T, f *screenshotFixture, key pluginsKey) {
	f.shoot(t, "plugins-translate-source", func(t *testing.T, a *app, tt *nsgotest.App, updates chan func()) {
		priv, _, err := script.DevKey(key.secret)
		if err != nil {
			t.Fatal(err)
		}
		packed := filepath.Join(f.home, "deepl-packed")
		if err := os.MkdirAll(packed, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := script.PackFolder(deeplFolder, packed, priv); err != nil {
			t.Fatal(err)
		}
		a.plugins.root = filepath.Join(f.home, "translators")
		pluginsScriptInstalled(t, a.plugins.root, packed)
		a.plugins.script.official = key.public
		a.plugins.script.watch = script.Watch{Beat: 100 * time.Millisecond, Stall: time.Minute, Stalls: 3, Deadline: time.Minute}
		onEngine(a, a.pluginsScan)
		scriptsSettle(t, a, tt, updates, func() bool { return a.plugins.script.verified != nil && !a.plugins.inboxBusy })
		scriptsGrant(t, a, tt, updates, "kando.deepl", string(plugins.PermCredFetch))
		onEngine(a, func() { a.SetTranslateSource("plugin:kando.deepl:deepl") })
		a.prefs.overMain = true
		tt = settingsOpenAt(a, tt, settings.SectionTranslate)
		if !tt.Exists("id:settings-translate-source") || !hasText(tt, "DeepL") || !hasText(tt, testlocale.Messages().SettingsTranslateSourceHint()) || a.TranslateSource() != "plugin:kando.deepl:deepl" {
			t.Fatalf("DeepL is not the translation source: %q; texts %q", a.TranslateSource(), tt.Texts())
		}
	})
	failed := func(name string) {
		f.shoot(t, name, func(t *testing.T, a *app, tt *nsgotest.App, updates chan func()) {
			job := f.job(t, "j5")
			job.FailKind = ""
			job.Err = &media.StageError{Stage: media.StageTranslate, Source: "DeepL", Err: plugins.NewTranslateError(plugins.TranslateAuth, "DeepL did not accept the key")}
			a.library.Upsert(job)
			queueShotScroll(t, a, tt)
			wantText(t, tt, testlocale.Messages().PrecheckSourceAuth("DeepL"))
			for _, id := range []string{"queue-settings", "queue-retry", "queue-remove"} {
				if !tt.Exists("id:" + id) {
					t.Fatalf("no %s; texts %q", id, tt.Texts())
				}
			}
			for _, card := range []string{"j5", "j13", "j14"} {
				queueActionsOneRow(t, tt, card)
			}
		})
	}
	failed("plugins-translate-failed")
	for _, lang := range []string{"en", "ja"} {
		t.Run(lang, func(t *testing.T) {
			testlocale.Use(t, lang)
			failed("plugins-translate-failed-" + lang)
		})
	}
}

// queueActionsOneRow fails t unless the buttons under the reason of the failed card of the job id share one row, inside the card: what fixes it (打开设置, a remedy's), 重试 and 报告….
func queueActionsOneRow(t *testing.T, tt *nsgotest.App, id string) {
	t.Helper()
	card := tt.Find("id:queue-" + id)
	if card == nil {
		t.Fatalf("no card of %s", id)
	}
	box := pageFrame(card)
	var buttons []*nsgotest.View
	var walk func(v *nsgotest.View)
	walk = func(v *nsgotest.View) {
		switch tid := v.Props.TestID; {
		case tid == "queue-settings", tid == "queue-retry", tid == "queue-report", strings.HasPrefix(tid, "queue-remedy-"):
			buttons = append(buttons, v)
			return
		}
		for _, c := range v.Children {
			walk(c)
		}
	}
	walk(card)
	if len(buttons) == 0 {
		t.Fatalf("the card of %s has no actions", id)
	}
	top := pageFrame(buttons[0]).Origin.Y
	for _, b := range buttons {
		f := pageFrame(b)
		if d := f.Origin.Y - top; d*d > 0.25 || f.Origin.X+f.Size.Width > box.Origin.X+box.Size.Width {
			t.Errorf("%s's %s is on another row or past the card: %+v, card %+v", id, b.Props.TestID, f, box)
		}
	}
}
