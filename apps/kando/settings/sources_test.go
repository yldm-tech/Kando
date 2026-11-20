package settings

import (
	"testing"

	"kando/apps/kando/live"
	"kando/apps/kando/locales/testlocale"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

// fakeSources is the 翻译来源 row's app: the plugins' sources, the one chosen and the one gone.
type fakeSources struct {
	live.Rev
	sources      []TranslateSource
	chosen, gone string
}

func (f *fakeSources) TranslateSources(string) []TranslateSource { return f.sources }
func (f *fakeSources) TranslateSource() string                   { return f.chosen }
func (f *fakeSources) SetTranslateSource(value string)           { f.chosen = value }
func (f *fakeSources) TranslateSourceGone() string               { return f.gone }

// startPluginRow starts an app of node alone, under the desktop and the language the tests read.
func startPluginRow(t *testing.T, state interface{ Attach(*core.Scope) }, node core.Node) *nsgotest.App {
	t.Helper()
	return nsgotest.Start(t, func(cx *core.Scope) core.Node {
		state.Attach(cx)
		return core.View{Children: []core.Node{node}}
	}, nsgotest.Size(800, 400), nsgotest.Environment(testlocale.Environment(core.Environment{OS: core.OSMacOS, Idiom: core.IdiomDesktop})))
}

// TestSourcesDropDownKeepsItsFocus checks that the 翻译来源 drop-down is not built again as the plugins' sources change: a source added and one gone update it in place, it keeps the keyboard focus, and its open list offers the AI service and the sources as they are.
func TestSourcesDropDownKeepsItsFocus(t *testing.T) {
	a := &fakeSources{sources: []TranslateSource{{Value: "plugin:a:deepl", Name: "DeepL"}}}
	app := startPluginRow(t, a, Sources{App: a})
	msgs := testlocale.Messages()
	app.Focus("id:settings-translate-source")
	pick := app.Find("id:settings-translate-source")
	if pick == nil || !pick.Focused {
		t.Fatal("the drop-down did not take the focus")
	}
	a.sources = []TranslateSource{{Value: "plugin:a:deepl", Name: "DeepL"}, {Value: "plugin:b:local", Name: "Local MT"}}
	a.Changed()
	app.Settle()
	a.sources = []TranslateSource{{Value: "plugin:b:local", Name: "Local MT"}}
	a.Changed()
	app.Settle()
	got := app.Find("id:settings-translate-source")
	if got == nil {
		t.Fatalf("the drop-down went as the sources changed: %q", app.Texts())
	}
	if got.ID != pick.ID || !got.Focused {
		t.Fatalf("after the sources changed the drop-down is view %d, focused %v; it was view %d with the focus", got.ID, got.Focused, pick.ID)
	}
	app.Press("id:settings-translate-source")
	if !app.Exists("role:option Local MT") || !app.Exists("role:option "+msgs.SettingsTranslateSourceAI()) || app.Exists("role:option DeepL") {
		t.Fatalf("the open list does not offer the sources as they are: %q", app.Texts())
	}
	app.Press("role:option Local MT")
	if a.chosen != "plugin:b:local" {
		t.Fatalf("chose %q", a.chosen)
	}
}
