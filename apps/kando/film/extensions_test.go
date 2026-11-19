package film

import (
	"testing"

	"kando/apps/kando/locales/testlocale"
	"kando/apps/kando/plugins"

	ns "github.com/nsgo-dev/nsgo/core"
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
	if !app.Exists("id:film-tab-extensions") || !app.Exists("id:film-tabs >> "+whole(msgs.FilmTabExtensions())) {
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
