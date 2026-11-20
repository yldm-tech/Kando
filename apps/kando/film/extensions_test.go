package film

import (
	"fmt"
	"testing"

	"kando/apps/kando/locales/testlocale"
	"kando/apps/kando/plugins"

	ns "github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

// fakeExtender is the fake app's 扩展 tab: the panels of the plugins running, the one chosen, the views it built, by id, and which panels fail.
type fakeExtender struct {
	extensions []Extension
	chosen     string
	builds     []string
	fail       map[string]bool
	retries    int
}

func (f *fakeExtender) Extensions(*ns.Scope) []Extension { return f.extensions }
func (f *fakeExtender) Extension() string                { return f.chosen }
func (f *fakeExtender) SetExtension(id string)           { f.chosen = id }
func (f *fakeExtender) ExtensionView(cx *ns.Scope, id string, failed func(*ns.Scope, func()) ns.Node) ns.Node {
	f.builds = append(f.builds, id)
	if f.fail[id] {
		return failed(cx, func() { f.retries++ })
	}
	return ns.Text{TestID: "fake-extension", Content: "view " + id}
}

// TestThePanelShowsTheExtensions checks the 扩展 tab (docs/plugin-system.md E15): it is there only while a plugin running adds a panel; it shows the chosen panel's view under its title and the name of its plugin, with a drop-down of the panels once there are more than one; the view is built again only when another panel is chosen or its plugin loaded again, never as the app's state changes; a view that fails shows why with a retry; and once no plugin adds a panel the tab goes and the panel shows the subtitles.
func TestThePanelShowsTheExtensions(t *testing.T) {
	a := newFakeApp()
	app := start(t, a)
	msgs := testlocale.Messages()
	if app.Exists("id:film-tab-extensions") {
		t.Fatal("扩展 shows with no plugin's panel")
	}
	deck := Extension{ID: "plugin:acme.deck:deck", Title: "Deck", Plugin: "Acme Deck", Icon: "notebook-pen", Run: 1}
	change(app, a, func() { a.extensions = []Extension{deck} })
	if !app.Exists("id:film-tab-extensions") || app.Find("id:film-tab-extensions").Props.Label != msgs.FilmTabExtensions() {
		t.Fatalf("no 扩展 tab; texts %q", app.Texts())
	}
	app.Press("id:film-tab-extensions")
	if a.side != SideExtensions || !app.Exists("id:film-extensions") {
		t.Fatalf("the tab shows %q", a.side)
	}
	for _, want := range []string{"Deck", msgs.FilmExtensionFrom("Acme Deck"), "view " + deck.ID} {
		if !app.Exists("id:film-extensions >> " + whole(want)) {
			t.Errorf("the tab has no %q; texts %q", want, app.Texts())
		}
	}
	if app.Exists("id:film-extension-pick") {
		t.Error("a drop-down for one panel")
	}
	for range 3 {
		change(app, a, func() { a.osd = "x" + a.osd })
	}
	if len(a.builds) != 1 {
		t.Fatalf("the view was built %d times as the state changed", len(a.builds))
	}
	cards := Extension{ID: "plugin:acme.cards:cards", Title: "Cards", Plugin: "Acme Cards", Run: 2}
	change(app, a, func() { a.extensions = []Extension{deck, cards} })
	if !app.Exists("id:film-extension-pick") || len(a.builds) != 1 {
		t.Fatalf("with two panels: a drop-down %v, built %d times", app.Exists("id:film-extension-pick"), len(a.builds))
	}
	change(app, a, func() { a.SetExtension(cards.ID) })
	if !app.Exists(whole("view "+cards.ID)) || len(a.builds) != 2 || !app.Exists("id:film-extensions >> "+whole(msgs.FilmExtensionFrom("Acme Cards"))) {
		t.Fatalf("another panel chosen: built %v; texts %q", a.builds, app.Texts())
	}
	// Its plugin loaded again builds it again, now failing.
	change(app, a, func() {
		a.fail = map[string]bool{cards.ID: true}
		cards.Run = 3
		a.extensions = []Extension{deck, cards}
	})
	if !app.Exists("id:film-extension-failed") || !app.Exists(whole(msgs.FilmExtensionFailed("Acme Cards"))) || len(a.builds) != 3 {
		t.Fatalf("a failing panel: built %v; texts %q", a.builds, app.Texts())
	}
	app.Press("id:film-extension-retry")
	if a.retries != 1 {
		t.Errorf("retried %d times", a.retries)
	}
	change(app, a, func() { a.extensions = nil })
	if app.Exists("id:film-tab-extensions") || app.Exists("id:film-extensions") || !app.Exists("id:film-tab-subs") {
		t.Fatalf("with no panel left: texts %q", app.Texts())
	}
}

// TestThePanelsDropDownKeepsItsFocus checks that the drop-down of the panels is not built again as the panels change: a panel added and one gone update it in place, it keeps the keyboard focus, and its open list offers the panels as they are.
func TestThePanelsDropDownKeepsItsFocus(t *testing.T) {
	a := newFakeApp()
	app := start(t, a)
	deck := Extension{ID: "plugin:acme.deck:deck", Title: "Deck", Plugin: "Acme Deck", Run: 1}
	cards := Extension{ID: "plugin:acme.cards:cards", Title: "Cards", Plugin: "Acme Cards", Run: 2}
	change(app, a, func() { a.extensions = []Extension{deck, cards} })
	app.Press("id:film-tab-extensions")
	app.Focus("id:film-extension-pick")
	pick := app.Find("id:film-extension-pick")
	if pick == nil || !pick.Focused {
		t.Fatal("the drop-down did not take the focus")
	}
	atlas := Extension{ID: "plugin:acme.atlas:atlas", Title: "Atlas", Plugin: "Acme Atlas", Run: 3}
	change(app, a, func() { a.extensions = []Extension{deck, cards, atlas} })
	change(app, a, func() { a.extensions = []Extension{deck, atlas} })
	got := app.Find("id:film-extension-pick")
	if got == nil {
		t.Fatalf("the drop-down went as the panels changed: %q", app.Texts())
	}
	if got.ID != pick.ID || !got.Focused {
		t.Fatalf("after the panels changed the drop-down is view %d, focused %v; it was view %d with the focus", got.ID, got.Focused, pick.ID)
	}
	app.Press("id:film-extension-pick")
	if !app.Exists("role:option Atlas") || !app.Exists("role:option Deck") || app.Exists("role:option Cards") {
		t.Fatalf("the open list does not offer the panels as they are: %q", app.Texts())
	}
	app.Press("role:option Atlas")
	if a.chosen != atlas.ID {
		t.Fatalf("chose %q", a.chosen)
	}
}

// TestEveryPanelIconIsDrawn checks that each icon a panel may name (plugins.PanelIcons) has one the tab draws, and no other.
func TestEveryPanelIconIsDrawn(t *testing.T) {
	for _, name := range plugins.PanelIcons {
		if icon := extensionIcons[name]; icon == nil || icon() == nil {
			t.Errorf("no icon %q", name)
		}
	}
	if len(extensionIcons) != len(plugins.PanelIcons) {
		t.Errorf("%d icons drawn for %d names", len(extensionIcons), len(plugins.PanelIcons))
	}
}

// tabOverflow is why the side panel's tabs do not fit, "" when they do: each label lies whole within its tab, which is no narrower than it with the tab's padding, and every tab lies whole within the bar, which needs no scrolling; extension names the tab drawn as an icon, which has no label to measure.
func tabOverflow(app *nsgotest.App, ids ...string) string {
	bar, track := app.Find("id:film-tabs-scroll"), app.Find("id:film-tabs")
	if bar == nil || track == nil {
		return "no tab bar"
	}
	if track.Frame.Size.Width > bar.Frame.Size.Width+0.5 {
		return fmt.Sprintf("the track is %v wide in a bar of %v", track.Frame.Size.Width, bar.Frame.Size.Width)
	}
	for _, id := range ids {
		tab := app.Find("id:film-tab-" + id)
		if tab == nil {
			return "no tab " + id
		}
		if f := tab.Frame; f.Origin.X < 0 || f.Origin.X+f.Size.Width > track.Frame.Size.Width+0.5 {
			return fmt.Sprintf("tab %s at %+v past the bar of %v", id, f, track.Frame.Size.Width)
		}
		if id == SideExtensions {
			continue
		}
		var text *nsgotest.View
		var find func(v *nsgotest.View)
		find = func(v *nsgotest.View) {
			for _, c := range v.Children {
				if text == nil && c.Text != "" {
					text = c
				}
				find(c)
			}
		}
		find(tab)
		if text == nil {
			return "no label in " + id
		}
		f, w := text.Frame, tab.Frame.Size.Width
		if f.Size.Width <= 0 || f.Origin.X < 0 || f.Origin.X+f.Size.Width > w+0.5 || w < f.Size.Width+6-0.5 {
			return fmt.Sprintf("%s: label %q at %+v in a tab %v wide", id, text.Text, f, w)
		}
	}
	return ""
}

// TestTabLabelsFit checks that the side panel's seven tabs, 扩展 among them as an icon named for the screen reader, fit on one row of the bar in Chinese, English and Japanese, each label whole, without the bar having to scroll.
func TestTabLabelsFit(t *testing.T) {
	for _, lang := range []string{"zh-Hans", "en", "ja"} {
		t.Run(lang, func(t *testing.T) {
			testlocale.Use(t, lang)
			a := newFakeApp()
			a.extensions = []Extension{{ID: "plugin:acme.deck:deck", Title: "Deck", Plugin: "Acme", Run: 1}}
			app := start(t, a)
			if why := tabOverflow(app, "subs", "clips", "terms", SideNotes, "info", SideExtensions, "style"); why != "" {
				t.Errorf("%s\n%s", why, app.Dump())
			}
			if w := app.Find("id:film-tabs").Frame.Size.Width; w < panelWidth-28-0.5 {
				t.Errorf("the bar is %v wide in a panel of %v", w, panelWidth)
			}
			if ext := app.Find("id:film-tab-extensions"); ext.Props.Label != testlocale.Messages().FilmTabExtensions() {
				t.Errorf("扩展's accessible name is %q", ext.Props.Label)
			}
		})
	}
}
