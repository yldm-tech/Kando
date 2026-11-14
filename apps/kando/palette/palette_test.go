package palette

import (
	"fmt"
	"testing"

	"kando/apps/kando/live"
	"kando/apps/kando/locales"
	"kando/apps/kando/locales/testlocale"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

func TestMain(m *testing.M) {
	testlocale.Pin(locales.Default)
	nsgotest.Main(m)
}

// fakeApp is the palette's state as the app keeps it, in plain fields, with the events the palette reported. It moves the highlight and closes the palette as the app does.
type fakeApp struct {
	live.Rev
	open   bool
	query  string
	view   View
	events []Event
}

func (f *fakeApp) PaletteOpen() bool            { return f.open }
func (f *fakeApp) PaletteQuery() *string        { return &f.query }
func (f *fakeApp) PaletteView(*core.Scope) View { return f.view }
func (f *fakeApp) PaletteEvent(ev Event) {
	f.events = append(f.events, ev)
	switch {
	case ev.Close, ev.Toggle:
		f.open = false
	case ev.Move != 0:
		f.view.Selected = min(max(f.view.Selected+ev.Move, 0), len(f.view.Lines)+len(f.view.Commands)-1)
	}
}

// last is the event the palette reported last.
func (f *fakeApp) last(t *testing.T) Event {
	t.Helper()
	if len(f.events) == 0 {
		t.Fatal("the palette reported nothing")
	}
	return f.events[len(f.events)-1]
}

func newFakeApp() *fakeApp {
	return &fakeApp{open: true, view: View{
		Placeholder: "命令或看过的台词…",
		Footer:      "输入以搜索",
		Lines:       []Line{{At: "0:00.4", Translation: "每条河都始于高山上的一股细流。", Original: "Every river begins as a trickle.", Film: "rivers"}},
		Commands:    []Command{{Label: "打开本地视频…", Key: "mod+o"}, {Label: "返回片库", Key: "escape"}, {Label: "设置 › AI 服务"}},
	}}
}

// start shows a's palette over a window of 1100×720 on a Mac, in testlocale's language.
func start(t *testing.T, a *fakeApp, height core.Points) *nsgotest.App {
	t.Helper()
	env := testlocale.Environment(core.Environment{OS: core.OSMacOS, Idiom: core.IdiomDesktop, AppleKeyboard: true})
	return nsgotest.Start(t, func(cx *core.Scope) core.Node {
		a.Attach(cx)
		return Palette{App: a}
	}, nsgotest.Size(1100, height), nsgotest.Environment(env))
}

// change runs fn on the app's state as a background report would, and settles.
func change(app *nsgotest.App, a *fakeApp, fn func()) {
	fn()
	a.Changed()
	app.Settle()
}

func TestPaletteShowsTheLinesAndCommands(t *testing.T) {
	a := newFakeApp()
	app := start(t, a, 720)
	msgs := testlocale.Messages()
	for _, want := range []string{msgs.PaletteHeadingLines(), "0:00.4", "每条河都始于高山上的一股细流。", "Every river begins as a trickle.", "rivers", msgs.PaletteHeadingCommands(), "打开本地视频…", "⌘O", "Esc", "设置 › AI 服务", "输入以搜索"} {
		if !app.Exists(`text:"` + want + `"`) {
			t.Fatalf("missing %q in %q", want, app.Texts())
		}
	}
	field := app.Find("id:palette-field")
	if !field.Focused || field.Input.Placeholder != "命令或看过的台词…" {
		t.Fatalf("field focused %v, placeholder %q", field.Focused, field.Input.Placeholder)
	}
	change(app, a, func() { a.open = false })
	if app.Exists("id:palette") {
		t.Fatal("the palette shows while closed")
	}
}

func TestPaletteReportsTheKeys(t *testing.T) {
	a := newFakeApp()
	app := start(t, a, 720)
	app.Type("id:palette-field", "河")
	if a.query != "河" || !a.last(t).Typed {
		t.Fatalf("query %q, events %+v", a.query, a.events)
	}
	app.Key("down")
	if a.last(t).Move != 1 || a.view.Selected != 1 {
		t.Fatalf("↓: events %+v, selected %d", a.events, a.view.Selected)
	}
	app.Key("up")
	if a.last(t).Move != -1 {
		t.Fatalf("↑: events %+v", a.events)
	}
	app.Key("enter")
	if !a.last(t).Submit {
		t.Fatalf("Return: events %+v", a.events)
	}
	app.Key("mod+k")
	if !a.last(t).Toggle || app.Exists("id:palette") {
		t.Fatalf("⌘K: events %+v", a.events)
	}
	change(app, a, func() { a.open = true })
	app.Key("escape")
	if !a.last(t).Close || a.open {
		t.Fatalf("Escape: events %+v", a.events)
	}
}

func TestPalettePicksTheRowClicked(t *testing.T) {
	a := newFakeApp()
	app := start(t, a, 720)
	app.Press("0:00.4 每条河都始于高山上的一股细流。 Every river begins as a trickle.")
	if ev := a.last(t); ev.Pick != 0 {
		t.Fatalf("a click on the line: %+v", ev)
	}
	app.Press("设置 › AI 服务")
	if ev := a.last(t); ev.Pick != 3 {
		t.Fatalf("a click on the third command: %+v", ev)
	}
}

func TestPaletteHighlightsTheSelectedRow(t *testing.T) {
	a := newFakeApp()
	a.view.Selected = 1
	app := start(t, a, 720)
	if got := app.Find("打开本地视频…").Style.Background; got != selected {
		t.Fatalf("the selected row's background is %v, want %v", got, selected)
	}
	change(app, a, func() { a.view.Selected = 0 })
	if got := app.Find("打开本地视频…").Style.Background; got == selected {
		t.Fatal("the row stays highlighted after the highlight moved")
	}
}

func TestPaletteShowsTheAnswer(t *testing.T) {
	a := newFakeApp()
	a.view.Answer = &Answer{Busy: true, Text: "正在问 m1…"}
	app := start(t, a, 720)
	msgs := testlocale.Messages()
	if !app.Exists(`text:"正在问 m1…"`) || !app.Exists(`text:"`+msgs.PaletteHeadingAnswer()+`"`) {
		t.Fatalf("no busy card in %q", app.Texts())
	}
	change(app, a, func() {
		a.view.Answer = &Answer{Text: "0:30 讲到河道越切越深。", Refs: []string{"0:30", "0:50"}, Note: "m1 回答"}
	})
	for _, want := range []string{"0:30 讲到河道越切越深。", "▶ 0:30", "▶ 0:50", "m1 回答"} {
		if !app.Exists(`text:"` + want + `"`) {
			t.Fatalf("missing %q in %q", want, app.Texts())
		}
	}
	app.Press(msgs.PaletteJumpTo("0:50"))
	if ev := a.last(t); ev.Ref != 1 {
		t.Fatalf("a click on the second chip: %+v", ev)
	}
	change(app, a, func() { a.view.Answer = &Answer{Text: msgs.PaletteNoAI(), Action: msgs.PaletteNoAIAction()} })
	app.Press("id:palette-action")
	if ev := a.last(t); !ev.Action {
		t.Fatalf("a click on the answer's button: %+v", ev)
	}
	change(app, a, func() { a.view.Answer = nil })
	if app.Exists(`text:"` + msgs.PaletteHeadingAnswer() + `"`) {
		t.Fatal("the answer card stays after the answer went")
	}
}

// ↓ brings the highlighted row into view when the list scrolls.
func TestPaletteRevealsTheHighlight(t *testing.T) {
	a := newFakeApp()
	a.view.Lines = nil
	a.view.Commands = nil
	for i := range 40 {
		a.view.Commands = append(a.view.Commands, Command{Label: fmt.Sprintf("命令 %d", i)})
	}
	app := start(t, a, 480)
	for range 39 {
		app.Key("down")
	}
	if a.view.Selected != 39 {
		t.Fatalf("selected %d", a.view.Selected)
	}
	row := app.Find("命令 39")
	scroll := row.Parent
	for scroll != nil && scroll.Kind != nsgotest.KindScroll {
		scroll = scroll.Parent
	}
	if scroll == nil || scroll.Offset.Y <= 0 {
		t.Fatalf("the list did not scroll to the last row\n%s", app.Dump())
	}
}

// The app hands the palette a view that follows its whole revision (live.Func), which changes on every keystroke and progress report; the field must not be built again for those, so it keeps the keyboard focus and what was typed.
func TestPaletteStaysWhileTheAppChanges(t *testing.T) {
	a := newFakeApp()
	app := start(t, a, 720)
	app.Type("id:palette-field", "河")
	field, line := app.Find("id:palette-field").ID, app.Find(`text:"0:00.4"`).ID
	a.Changed()
	app.Settle()
	if got := app.Find("id:palette-field"); got.ID != field || !got.Focused || got.Text != "河" {
		t.Fatalf("the field was built again (view %d, was %d), focused %v, holds %q", got.ID, field, got.Focused, got.Text)
	}
	if got := app.Find(`text:"0:00.4"`).ID; got != line {
		t.Fatalf("the line was built again (view %d, was %d) though it stayed the same", got, line)
	}
	change(app, a, func() {
		a.view.Lines = []Line{{At: "0:30.4", Translation: "数千年间，河道被切割得越来越深。"}}
	})
	if !app.Exists(`text:"0:30.4"`) || app.Exists(`text:"0:00.4"`) {
		t.Fatalf("the palette does not follow its lines: %q", app.Texts())
	}
}
