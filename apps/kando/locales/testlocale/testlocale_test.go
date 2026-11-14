package testlocale

import (
	"os"
	"testing"

	"kando/apps/kando/locales"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

func TestMain(m *testing.M) {
	Pin(locales.Default)
	os.Exit(m.Run())
}

// An app started in Environment shows Current, and Use changes it for the test that calls it only.
func TestEnvironmentStartsTheAppInCurrent(t *testing.T) {
	show := func(cx *core.Scope) core.Node {
		return core.Text{TestID: "text", ContentFunc: func() string { return locales.SettingsGeneralLanguageName(cx) }}
	}
	start := func(t *testing.T) string {
		app := nsgotest.Start(t, show, nsgotest.Environment(Environment(core.Environment{Locale: "en-US"})))
		return app.Find("id:text").Text
	}
	if got, want := start(t), locales.In(locales.Default).SettingsGeneralLanguageName(); got != want {
		t.Errorf("pinned: %q, want %q", got, want)
	}
	t.Run("ja", func(t *testing.T) {
		Use(t, "ja")
		if got := start(t); got != "日本語" || Messages().SettingsGeneralLanguageName() != "日本語" {
			t.Errorf("after Use(ja): %q, Messages %q", got, Messages().SettingsGeneralLanguageName())
		}
	})
	if Current() != locales.Default {
		t.Errorf("Use outlived its test: %s", Current())
	}
}
