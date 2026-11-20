package settings

import (
	"testing"

	"kando/apps/kando/live"
	"kando/apps/kando/locales/testlocale"
	"kando/apps/kando/plugins"
)

// fakeStyles is the 风格预设 row's app: the style packs turned on and the one the prompt is.
type fakeStyles struct {
	live.Rev
	packs  []plugins.Pack
	chosen string
}

func (f *fakeStyles) TranslateStyles() []plugins.Pack { return f.packs }
func (f *fakeStyles) TranslateStyle() string          { return f.chosen }
func (f *fakeStyles) SetTranslateStyle(id string)     { f.chosen = id }

// stylePack is a translate-style pack of id named name.
func stylePack(id, name string) plugins.Pack {
	return plugins.Pack{Manifest: plugins.Manifest{ID: id, Name: plugins.Text{"en": name}}}
}

// TestStylesDropDownKeepsItsFocus checks that the 风格预设 drop-down is not built again as the style packs turned on change: a pack turned on and one turned off update it in place, it keeps the keyboard focus, and its open list offers 不用预设 and the packs as they are.
func TestStylesDropDownKeepsItsFocus(t *testing.T) {
	a := &fakeStyles{packs: []plugins.Pack{stylePack("example.casual", "Casual")}}
	app, _ := startPluginRow(t, a, Styles{App: a})
	msgs := testlocale.Messages()
	app.Focus("id:settings-translate-style")
	pick := app.Find("id:settings-translate-style")
	if pick == nil || !pick.Focused {
		t.Fatal("the drop-down did not take the focus")
	}
	a.packs = []plugins.Pack{stylePack("example.casual", "Casual"), stylePack("example.formal", "Formal")}
	a.Changed()
	app.Settle()
	a.packs = []plugins.Pack{stylePack("example.formal", "Formal")}
	a.Changed()
	app.Settle()
	got := app.Find("id:settings-translate-style")
	if got == nil {
		t.Fatalf("the drop-down went as the packs changed: %q", app.Texts())
	}
	if got.ID != pick.ID || !got.Focused {
		t.Fatalf("after the packs changed the drop-down is view %d, focused %v; it was view %d with the focus", got.ID, got.Focused, pick.ID)
	}
	app.Press("id:settings-translate-style")
	if !app.Exists("role:option Formal") || !app.Exists("role:option "+msgs.SettingsTranslateStyleNone()) || app.Exists("role:option Casual") {
		t.Fatalf("the open list does not offer the packs as they are: %q", app.Texts())
	}
	app.Press("role:option Formal")
	if a.chosen != "example.formal" {
		t.Fatalf("chose %q", a.chosen)
	}
}
