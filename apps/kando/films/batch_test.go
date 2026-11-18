package films

import (
	"slices"
	"testing"

	"kando/apps/kando/locales/testlocale"

	ns "github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

// labelsOf is the labels of the popup menu open in app, "" for a separator.
func labelsOf(app *nsgotest.App) []string {
	out := make([]string, len(app.PopupMenu))
	for i, it := range app.PopupMenu {
		out[i] = it.Label
	}
	return out
}

// TestSelectPressReadsTheModifiers ⌘-clicks and ⇧-clicks the posters and the list's rows: ⌘ selects a film or leaves it out and ⇧ selects up to it on a Mac, where Ctrl is a plain click that opens the film; elsewhere Ctrl is what ⌘ is on a Mac.
func TestSelectPressReadsTheModifiers(t *testing.T) {
	for _, mac := range []bool{true, false} {
		for _, view := range []string{ViewGrid, ViewList} {
			env := testEnvironment()
			if !mac {
				env.OS, env.AppleKeyboard = ns.OSWindows, false
			}
			a := newFakeApp()
			a.view = view
			app := nsgotest.Start(t, func(cx *ns.Scope) ns.Node {
				a.Attach(cx)
				return ns.View{Style: ns.Style{FlexGrow: 1}, Children: []ns.Node{View{App: a}}}
			}, nsgotest.Size(1068, 782), nsgotest.Environment(env))
			rivers, algebra := "id:film-done", "id:film-algebra"
			if view == ViewList {
				rivers, algebra = "id:films-list >> "+whole("Rivers of Europe"), "id:films-list >> "+whole("Linear Algebra")
			}
			command := ns.Modifiers{Platform: true}
			if !mac {
				command = ns.Modifiers{Control: true}
			}
			app.PressWith(algebra, command)
			if !slices.Equal(a.selection.Keys, []string{"algebra"}) {
				t.Fatalf("mac %v, view %v: the command click selected %v", mac, view, a.selection.Keys)
			}
			app.PressWith(rivers, ns.Modifiers{Shift: true})
			if a.selection.Len() != 2 || !a.selection.Has("done") {
				t.Fatalf("mac %v, view %v: ⇧-click selected %v", mac, view, a.selection.Keys)
			}
			app.PressWith(algebra, command)
			if !slices.Equal(a.selection.Keys, []string{"done"}) {
				t.Fatalf("mac %v, view %v: a second command click left %v selected", mac, view, a.selection.Keys)
			}
			if mac {
				// A plain click: a poster opens its film and lets go of the selection, a row selects itself alone.
				a.calls = nil
				app.PressWith(algebra, ns.Modifiers{Control: true})
				if view == ViewGrid && (!slices.Equal(a.calls, []string{"open algebra"}) || a.selection.Len() != 0) {
					t.Fatalf("Ctrl-click on a Mac's poster made %q and left %v selected", a.calls, a.selection.Keys)
				}
				if view == ViewList && !slices.Equal(a.selection.Keys, []string{"algebra"}) {
					t.Fatalf("Ctrl-click on a Mac's row selected %v, want it alone", a.selection.Keys)
				}
			}
		}
	}
}

// TestPostersSelectedRingAndShareAMenu checks the posters of a selection: each selected is ringed in the accent, a right click on one of them offers the selection's menu, which acts on them all, a right click on another film offers its own menu and lets go of the selection, and a click opens the film it is on and lets go of the selection.
func TestPostersSelectedRingAndShareAMenu(t *testing.T) {
	a := newFakeApp()
	app := start(t, a)
	msgs := testlocale.Messages()
	ring := func(key string) float32 {
		v := app.Find("id:film-" + key)
		if !v.Props.Selected {
			return 0
		}
		return float32(v.Style.Outline.Width)
	}
	if ring("done") != 0 || ring("algebra") != 0 {
		t.Fatal("a poster is ringed with nothing selected")
	}
	change(app, a, func() { a.selection.Only("done"); a.selection.Toggle("algebra") })
	if ring("done") == 0 || ring("algebra") == 0 {
		t.Fatalf("the selected posters are not ringed: %v %v", ring("done"), ring("algebra"))
	}
	app.RightClick("id:film-done")
	menu := labelsOf(app)
	want := []string{msgs.FilmsSelectedCount(2), "", msgs.FilmsMarkWatched(), msgs.FilmsReprocess(), msgs.FilmsTranslateAgain(), "", msgs.FilmsRemoveFromLibrary()}
	if !slices.Equal(menu, want) {
		t.Fatalf("the selection's menu %q, want %q", menu, want)
	}
	if !app.PopupMenu[0].Disabled {
		t.Fatal("the count at the top of the menu can be chosen")
	}
	app.ChoosePopup(msgs.FilmsRemoveFromLibrary())
	wantCalls(t, a, "ask-remove done,algebra")
	if !app.Exists("id:remove-confirm") || !app.Exists(whole(msgs.FilmsRemoveConfirmMany(2))) {
		t.Fatalf("the confirmation does not say how many films go: %q", app.Texts())
	}
	app.Press("id:remove-confirm-trash")
	a.calls = nil
	// ⋯ on a selected poster is the same menu.
	app.Press("id:film-more >> nth=0")
	if got := labelsOf(app); len(got) == 0 || got[0] != msgs.FilmsSelectedCount(2) {
		t.Fatalf("⋯ on a selected poster offers %q", got)
	}
	app.ClosePopup()
	change(app, a, func() { a.selection.Only("algebra") })
	app.RightClick("id:film-done")
	if got := labelsOf(app); got[0] != msgs.FilmsPlay() || a.selection.Len() != 0 {
		t.Fatalf("a right click outside the selection offers %q and keeps %v selected", got, a.selection.Keys)
	}
	app.ClosePopup()
	change(app, a, func() { a.selection.All([]string{"done", "algebra"}) })
	app.Press("id:film-algebra")
	wantCalls(t, a, "open algebra")
	if a.selection.Len() != 0 {
		t.Fatalf("a click on a poster kept %v selected", a.selection.Keys)
	}
}

// TestSelectAllAndEscapeOnThePage checks ⌘A, which selects every done film the page shows, in the posters and in the list, and Escape, which selects none, while the search field keeps both for its text.
func TestSelectAllAndEscapeOnThePage(t *testing.T) {
	a := newFakeApp()
	app := start(t, a)
	app.Key("mod+a")
	if got := a.selection.Keys; len(got) != 2 || !a.selection.Has("done") || !a.selection.Has("algebra") {
		t.Fatalf("⌘A selected %v, want the two done films", got)
	}
	app.Key("escape")
	if a.selection.Len() != 0 {
		t.Fatalf("Escape kept %v selected", a.selection.Keys)
	}
	app.Press("id:films-view-list")
	app.Key("mod+a")
	if a.selection.Len() != 2 {
		t.Fatalf("⌘A in the list selected %v", a.selection.Keys)
	}
	change(app, a, func() { a.selection.Clear() })
	app.Focus("id:films-link")
	app.Key("mod+a")
	if a.selection.Len() != 0 {
		t.Fatalf("⌘A in the link field selected %v", a.selection.Keys)
	}
}

// TestListSelectsSeveralRows checks the list with several films selected: a right click on one of them offers the selection's menu, ⇧↓ grows the selection from the row selected, and a click selects its row alone again.
func TestListSelectsSeveralRows(t *testing.T) {
	a := newFakeApp()
	a.view = ViewList
	app := start(t, a)
	msgs := testlocale.Messages()
	rivers, algebra := "id:films-list >> "+whole("Rivers of Europe"), "id:films-list >> "+whole("Linear Algebra")
	app.Press(rivers)
	if !slices.Equal(a.selection.Keys, []string{"done"}) {
		t.Fatalf("a click selected %v", a.selection.Keys)
	}
	app.Key("shift+down")
	if a.selection.Len() != 2 {
		t.Fatalf("⇧↓ from Rivers of Europe selected %v", a.selection.Keys)
	}
	app.RightClick(algebra)
	if got := labelsOf(app); len(got) == 0 || got[0] != msgs.FilmsSelectedCount(2) {
		t.Fatalf("a right click on a selected row offers %q", got)
	}
	app.ClosePopup()
	app.Press(algebra)
	if !slices.Equal(a.selection.Keys, []string{"algebra"}) {
		t.Fatalf("a click on a selected row selected %v, want it alone", a.selection.Keys)
	}
}
