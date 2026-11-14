package locales

import (
	"testing"

	"github.com/nsgo-dev/nsgo/core"
	desktoplocales "github.com/nsgo-dev/nsgo/desktop/locales"
	"github.com/nsgo-dev/nsgo/nsgotest"
	updaterlocales "github.com/nsgo-dev/nsgo/updater/locales"
)

// followApp shows one message three ways, and switches the app to ja when "ja" is pressed: in a binding (ContentFunc), in a region (core.Dynamic) that builds a plain Text, and as a plain Text the component built once.
func followApp(cx *core.Scope) core.Node {
	return core.View{Children: []core.Node{
		core.Text{TestID: "bound", ContentFunc: func() string { return FilmShowInFinder(cx, "macOS") }},
		core.Dynamic(func(cx *core.Scope) core.Node {
			return core.Text{TestID: "region", Content: FilmShowInFinder(cx, "macOS")}
		}),
		core.Text{TestID: "built", Content: FilmShowInFinder(cx, "macOS")},
		core.View{TestID: "ja", OnPress: func(core.PressEvent) { cx.SetLocale("ja") }, Style: core.Style{Height: core.Pt(20)}},
	}}
}

// A message says the language the app shows under its scope, which the system's locale picks among the catalogs and SetLocale forces. Read in a binding or a region it follows a switch at once; read once while a component is built it keeps the language it was built in.
func TestMessagesFollowTheLanguage(t *testing.T) {
	app := nsgotest.Start(t, followApp, nsgotest.Environment(core.Environment{Locale: "zh-Hant-TW"}))
	zhHant, ja := In("zh-Hant").FilmShowInFinder("macOS"), In("ja").FilmShowInFinder("macOS")
	if zhHant == ja || zhHant == In(Source).FilmShowInFinder("macOS") {
		t.Fatalf("zh-Hant says %q, ja %q", zhHant, ja)
	}
	for _, id := range []string{"bound", "region", "built"} {
		if got := app.Find("id:" + id).Text; got != zhHant {
			t.Errorf("%s in a zh-TW system: %q, want %q", id, got, zhHant)
		}
	}
	app.Press("id:ja")
	for _, id := range []string{"bound", "region"} {
		if got := app.Find("id:" + id).Text; got != ja {
			t.Errorf("%s after SetLocale(ja): %q, want %q", id, got, ja)
		}
	}
	if got := app.Find("id:built").Text; got != zhHant {
		t.Errorf("built after SetLocale(ja): %q; a plain Text built once was expected to keep %q", got, zhHant)
	}
}

// In matches a locale to the catalogs as the app does, and a language without a catalog says the source's messages.
func TestInMatchesTheCatalogs(t *testing.T) {
	for locale, lang := range map[string]string{"zh-TW": "zh-Hant", "zh-CN": "zh-Hans", "ja-JP": "ja", "ko-KR": "ko", "en-GB": "en", "de": "en"} {
		if got, want := In(locale).SettingsGeneralLanguageName(), In(lang).SettingsGeneralLanguageName(); got != want {
			t.Errorf("In(%s): %q, want %s's %q", locale, got, lang, want)
		}
	}
}

// The catalogs supply NSGo's own words in the languages NSGo does not translate to, so the menus and the update alerts say Kando's language.
func TestFrameworkWordsInEveryLanguage(t *testing.T) {
	for _, lang := range Interface {
		if lang == Source {
			continue
		}
		quit, later := desktoplocales.In(lang).Quit(), updaterlocales.In(lang).Later()
		if quit == desktoplocales.In(Source).Quit() || later == updaterlocales.In(Source).Later() {
			t.Errorf("%s says NSGo's words in English: %q, %q", lang, quit, later)
		}
	}
	if got := desktoplocales.In("ja").QuitApp("看懂"); got != "看懂を終了" {
		t.Errorf("ja QuitApp: %q", got)
	}
}
