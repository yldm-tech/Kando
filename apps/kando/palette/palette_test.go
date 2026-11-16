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
		rows := len(f.view.Films) + len(f.view.Lines) + len(f.view.Commands)
		if f.view.Ask != "" {
			rows++
		}
		f.view.Selected = min(max(f.view.Selected+ev.Move, 0), rows-1)
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
		Placeholder: "影片、命令或看过的台词…",
		Footer:      "输入以搜索",
		Lines:       []Line{{At: "0:00.4", Translation: "每条河都始于高山上的一股细流。", Original: "Every river begins as a trickle.", Film: "rivers"}},
		Commands:    []Command{{Label: "打开本地视频…", Key: "mod+o"}, {Label: "返回片库", Key: "escape"}, {Label: "设置 › AI 服务"}},
	}}
}

// start shows a's palette over a window of 1100×720 on a Mac, in testlocale's language.
func start(t *testing.T, a *fakeApp, height core.Points) *nsgotest.App {
	t.Helper()
	return startIn(t, a, height, core.Environment{OS: core.OSMacOS, Idiom: core.IdiomDesktop, AppleKeyboard: true})
}

// startIn is start in env.
func startIn(t *testing.T, a *fakeApp, height core.Points, env core.Environment) *nsgotest.App {
	t.Helper()
	env = testlocale.Environment(env)
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
	if !field.Focused || field.Input.Placeholder != "影片、命令或看过的台词…" {
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
	app.Press("0:00.4 每条河都始于高山上的一股细流。 Every river begins as a trickle. rivers")
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

// TestPaletteAskRowComesFirst checks that the row that asks the film is drawn before the others and counted first: highlighted at 0, a click on it picks 0, and the rows after it count from 1.
func TestPaletteAskRowComesFirst(t *testing.T) {
	a := newFakeApp()
	a.view.Ask = "问这部影片：“河”"
	app := start(t, a, 720)
	if got := app.Find(a.view.Ask).Style.Background; got != selected {
		t.Fatalf("the ask row's background is %v, want %v", got, selected)
	}
	if got := app.Find("0:00.4 每条河都始于高山上的一股细流。 Every river begins as a trickle. rivers").Style.Background; got == selected {
		t.Fatal("the line is highlighted with the ask row")
	}
	app.Press(a.view.Ask)
	if ev := a.last(t); ev.Pick != 0 {
		t.Fatalf("a click on the ask row: %+v", ev)
	}
	app.Press("0:00.4 每条河都始于高山上的一股细流。 Every river begins as a trickle. rivers")
	if ev := a.last(t); ev.Pick != 1 {
		t.Fatalf("a click on the line: %+v", ev)
	}
	app.Press("设置 › AI 服务")
	if ev := a.last(t); ev.Pick != 4 {
		t.Fatalf("a click on the third command: %+v", ev)
	}
	change(app, a, func() { a.view.Selected = 2 })
	if got := app.Find("打开本地视频…").Style.Background; got != selected {
		t.Fatalf("the first command's background is %v at 2, want %v", got, selected)
	}
	change(app, a, func() { a.view.Ask = "" })
	if app.Exists("问这部影片：“河”") {
		t.Fatal("the ask row stays without a question")
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
	scroll := app.Find("命令 0").Parent
	for scroll != nil && scroll.Kind != nsgotest.KindScroll {
		scroll = scroll.Parent
	}
	if scroll == nil {
		t.Fatalf("no scroll view around the rows\n%s", app.Dump())
	}
	app.Key("down")
	if scroll.Offset.Y != 0 {
		t.Fatalf("the list scrolled to %v for a row that showed already", scroll.Offset.Y)
	}
	for range 38 {
		app.Key("down")
	}
	if a.view.Selected != 39 {
		t.Fatalf("selected %d", a.view.Selected)
	}
	if scroll.Offset.Y <= 0 {
		t.Fatalf("the list did not scroll to the last row\n%s", app.Dump())
	}
	for range 39 {
		app.Key("up")
	}
	// The first row shows whole again, at the top of the list: the heading over it may stay above, as the least scroll that shows the row leaves it.
	top := core.Points(0)
	for v := app.Find("命令 0"); v != nil && v != scroll; v = v.Parent {
		top += v.Frame.Origin.Y
	}
	if a.view.Selected != 0 || scroll.Offset.Y > top || scroll.Offset.Y < top-20 {
		t.Fatalf("selected %d, the list at %v with the first row at %v, want that row at the top", a.view.Selected, scroll.Offset.Y, top)
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

// TestPaletteFilmsComeBeforeTheLines checks that the films found are drawn under their heading after the row that asks the film and before the lines, counted in that order.
func TestPaletteFilmsComeBeforeTheLines(t *testing.T) {
	a := newFakeApp()
	a.view.Ask = "问这部影片：“河”"
	a.view.Films = []string{"Rivers of Europe", "River Deltas"}
	app := start(t, a, 720)
	if !app.Exists(`text:"` + testlocale.Messages().PaletteHeadingFilms() + `"`) {
		t.Fatalf("no films heading in %q", app.Texts())
	}
	app.Press("River Deltas")
	if ev := a.last(t); ev.Pick != 2 {
		t.Fatalf("a click on the second film: %+v", ev)
	}
	app.Press("0:00.4 每条河都始于高山上的一股细流。 Every river begins as a trickle. rivers")
	if ev := a.last(t); ev.Pick != 3 {
		t.Fatalf("a click on the line after two films: %+v", ev)
	}
	change(app, a, func() { a.view.Selected = 1 })
	if got := app.Find("Rivers of Europe").Style.Background; got != selected {
		t.Fatalf("the first film's background is %v at 1, want %v", got, selected)
	}
}

// TestPaletteTellsTheScreenReaderTheHighlight checks that the rows report which is highlighted, and that with a screen reader running ↑ and ↓, which leave the keyboard in the field, have it say the row they move to, a line's both texts and its film among it.
func TestPaletteTellsTheScreenReaderTheHighlight(t *testing.T) {
	a := newFakeApp()
	line := "0:00.4 每条河都始于高山上的一股细流。 Every river begins as a trickle. rivers"
	app := startIn(t, a, 720, core.Environment{OS: core.OSMacOS, Idiom: core.IdiomDesktop, AppleKeyboard: true, ScreenReader: true})
	if !app.Find(line).Props.Selected || app.Find("打开本地视频…").Props.Selected {
		t.Fatal("the rows do not report the highlighted one")
	}
	app.Key("down")
	if !app.Find("打开本地视频…").Props.Selected || app.Find(line).Props.Selected {
		t.Fatal("the rows do not report the highlight ↓ moved")
	}
	if n := len(app.Announced); n == 0 || app.Announced[n-1] != "打开本地视频…" {
		t.Fatalf("↓ announced %q", app.Announced)
	}
	app.Key("up")
	if n := len(app.Announced); app.Announced[n-1] != line {
		t.Fatalf("↑ announced %q", app.Announced)
	}
}

// TestPaletteAnswerIsALiveRegion checks that a screen reader reads the answer as it comes, the keyboard staying in the field meanwhile.
func TestPaletteAnswerIsALiveRegion(t *testing.T) {
	a := newFakeApp()
	a.view.Answer = &Answer{Busy: true, Text: "正在问 m1…"}
	app := start(t, a, 720)
	if got := app.Find(`text:"正在问 m1…"`).Props.LiveRegion; got != core.LiveRegionPolite {
		t.Fatalf("the answer's live region is %v", got)
	}
}
