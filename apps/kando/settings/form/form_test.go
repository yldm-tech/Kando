package form

import (
	"testing"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
	"github.com/nsgo-dev/nsgo/ui"
)

func TestMain(m *testing.M) { nsgotest.Main(m) }

// words is the words a row says, which a test changes as another interface language or the app's state would.
type words struct{ title, subtitle *core.Signal[string] }

// start shows node, built once by build with the words of a new words, in a window on a Mac.
func start(t *testing.T, build func(w words) core.Node) (*nsgotest.App, words) {
	t.Helper()
	var w words
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		w = words{title: core.NewSignal(cx, "标题"), subtitle: core.NewSignal(cx, "说明")}
		return core.View{Children: []core.Node{build(w)}}
	}, nsgotest.Size(600, 300), nsgotest.Environment(core.Environment{OS: core.OSMacOS, Idiom: core.IdiomDesktop, AppleKeyboard: true}))
	return app, w
}

// A Row with TitleFunc and SubtitleFunc shows, names and describes itself by their words as they change, without building its trailing control again, so a drop-down in it keeps the keyboard; an empty subtitle hides.
func TestRowFollowsItsWords(t *testing.T) {
	app, w := start(t, func(w words) core.Node {
		return Row{TitleFunc: w.title.Get, SubtitleFunc: w.subtitle.Get, TestID: "row", Trailing: []core.Node{
			ui.Select[string]{TestID: "pick", Label: "pick", Options: []ui.Option[string]{{Value: "a", Label: "A"}, {Value: "b", Label: "B"}}, Value: func() string { return "a" }},
		}}
	})
	app.Focus("id:pick")
	pick := app.Find("id:pick")
	if !pick.Focused {
		t.Fatal("the drop-down did not take the keyboard")
	}
	if r := app.Find("id:row"); r.Props.Label != "标题" || r.Props.Description != "说明" || !app.Exists(`text:"标题"`) || !app.Exists(`text:"说明"`) {
		t.Fatalf("the row is named %q, described as %q: %q", r.Props.Label, r.Props.Description, app.Texts())
	}
	w.title.Set("Title")
	w.subtitle.Set("Hint")
	app.Settle()
	if r := app.Find("id:row"); r.Props.Label != "Title" || r.Props.Description != "Hint" || !app.Exists(`text:"Title"`) || !app.Exists(`text:"Hint"`) {
		t.Fatalf("after the words changed the row is named %q, described as %q: %q", r.Props.Label, r.Props.Description, app.Texts())
	}
	if got := app.Find("id:pick"); got.ID != pick.ID || !got.Focused {
		t.Fatalf("the drop-down was built again (view %d, was %d) or lost the keyboard (%v)", got.ID, pick.ID, got.Focused)
	}
	w.subtitle.Set("")
	app.Settle()
	if app.Exists(`text:"Hint"`) {
		t.Fatalf("an empty subtitle still shows: %q", app.Texts())
	}
}

// A SwitchRow with TitleFunc and SubtitleFunc shows its words as they change and names its switch by the title, without building the switch again, so it keeps the keyboard.
func TestSwitchRowFollowsItsWords(t *testing.T) {
	on := false
	app, w := start(t, func(w words) core.Node {
		return SwitchRow{TitleFunc: w.title.Get, SubtitleFunc: w.subtitle.Get, TestID: "toggle", On: func() bool { return on }, OnChange: func(v bool) { on = v }}
	})
	app.Focus("id:toggle")
	toggle := app.Find("id:toggle")
	if !toggle.Focused || toggle.Props.Label != "标题" || !app.Exists(`text:"说明"`) {
		t.Fatalf("the switch is named %q, focused %v: %q", toggle.Props.Label, toggle.Focused, app.Texts())
	}
	w.title.Set("Title")
	w.subtitle.Set("Hint")
	app.Settle()
	got := app.Find("id:toggle")
	if got.ID != toggle.ID || !got.Focused || got.Props.Label != "Title" || !app.Exists(`text:"Title"`) || !app.Exists(`text:"Hint"`) {
		t.Fatalf("after the words changed the switch is view %d (was %d), focused %v, named %q: %q", got.ID, toggle.ID, got.Focused, got.Props.Label, app.Texts())
	}
}

// Rows given plain words show them, as the settings pages, built again for another language, give them.
func TestRowsTakePlainWords(t *testing.T) {
	app, _ := start(t, func(words) core.Node {
		return core.View{Children: []core.Node{
			Row{Title: "行", Subtitle: "行的说明", TestID: "row"},
			SwitchRow{Title: "开关", Subtitle: "开关的说明", TestID: "toggle", On: func() bool { return false }, OnChange: func(bool) {}},
		}}
	})
	if r := app.Find("id:row"); r.Props.Label != "行" || r.Props.Description != "行的说明" {
		t.Fatalf("the row is named %q, described as %q", r.Props.Label, r.Props.Description)
	}
	if got := app.Find("id:toggle").Props.Label; got != "开关" {
		t.Fatalf("the switch is named %q", got)
	}
	for _, want := range []string{"行", "行的说明", "开关", "开关的说明"} {
		if !app.Exists(`text:"` + want + `"`) {
			t.Fatalf("no %q in %q", want, app.Texts())
		}
	}
}
