package film

import (
	"slices"
	"testing"
	"time"

	"kando/apps/kando/locales/testlocale"

	ns "github.com/nsgo-dev/nsgo/core"
)

// fakeExplainer is the fake app's explanation of a line and recap card: what the panel and the card show, and what they asked of the app.
type fakeExplainer struct {
	explanation Explanation
	question    string
	recap       Recap
	asks        []string
}

func (f *fakeExplainer) ask(s string)                 { f.asks = append(f.asks, s) }
func (f *fakeExplainer) Explanation() Explanation     { return f.explanation }
func (f *fakeExplainer) CloseExplain()                { f.ask("close explain") }
func (f *fakeExplainer) ExpandExplain()               { f.ask("expand") }
func (f *fakeExplainer) ExplainSettings()             { f.ask("settings") }
func (f *fakeExplainer) ExplainQuestion() *string     { return &f.question }
func (f *fakeExplainer) AskExplain()                  { f.ask("ask " + f.question); f.question = "" }
func (f *fakeExplainer) SaveExplained()               { f.ask("save explained") }
func (f *fakeExplainer) Recap() Recap                 { return f.recap }
func (f *fakeExplainer) CloseRecap()                  { f.ask("close recap") }
func (f *fakeExplainer) ContinueRecap()               { f.ask("continue") }
func (f *fakeExplainer) StartOver()                   { f.ask("start over") }
func (f *fakeExplainer) RecapJump(i int)              { f.ask("jump " + string(rune('0'+i))) }
func (f *fakeExplainer) lastAsk() string              { return f.asks[len(f.asks)-1] }
func (f *fakeExplainer) askedAll(want ...string) bool { return slices.Equal(f.asks, want) }

// TestThePanelExplainsTheLine checks the side panel's explanation of a line (design Main 1062-1083): it shows in place of the tabs' content with the line, its sections under their headings and the footnote; 展开 and ‹ 字幕 go back to the app, the field asks at Return, the save button saves the line and says once it is saved, and the questions asked show with their answers.
func TestThePanelExplainsTheLine(t *testing.T) {
	a := newFakeApp()
	app := start(t, a)
	msgs := testlocale.Messages()
	if app.Exists("id:film-explain") {
		t.Fatal("the explanation shows before a line is explained")
	}
	change(app, a, func() {
		a.side = SideExplain
		a.explanation = Explanation{Open: true, At: 70*time.Second + 400*time.Millisecond, Original: "These sweeping curves are called meanders.", Translation: "这些大弯道叫河曲。",
			Sections: []ExplainSection{{Kind: ExplainMeaning, Body: "弯道叫河曲。"}, {Kind: ExplainTerms, Body: "meander → 河曲"}}, More: true,
			Note: func(cx *ns.Scope) string { return "m1 wrote it" }}
	})
	for _, want := range []string{"1:10.4", "These sweeping curves are called meanders.", "这些大弯道叫河曲。", msgs.ExplainMeaning(), "弯道叫河曲。", msgs.ExplainTerms(), "meander → 河曲", msgs.ExplainMore(), "m1 wrote it", msgs.ExplainSave()} {
		if !app.Exists("id:film-explain >> " + whole(want)) {
			t.Fatalf("the explanation has no %q", want)
		}
	}
	if app.Exists("id:film-search") {
		t.Fatal("the subtitle list shows under the explanation")
	}
	app.Press("id:film-explain-more")
	app.Type("id:film-explain-ask", "why called?")
	app.Submit("id:film-explain-ask")
	app.Press("id:film-explain-save")
	app.Press("id:film-explain-back")
	if !a.askedAll("expand", "ask why called?", "save explained", "close explain") {
		t.Fatalf("the panel asked %q", a.asks)
	}
	change(app, a, func() {
		a.explanation.Saved = true
		a.explanation.Asked = []ExplainQA{{Question: "why called?", Answer: func(*ns.Scope) string { return "It names the curves." }}, {Question: "and sweeping?", Busy: true, Answer: func(*ns.Scope) string { return "…" }}}
	})
	for _, want := range []string{msgs.ExplainSaved(), "why called?", "It names the curves.", "and sweeping?"} {
		if !app.Exists("id:film-explain >> " + whole(want)) {
			t.Fatalf("the explanation has no %q", want)
		}
	}
	if !app.Find("id:film-explain-save").Props.Selected {
		t.Fatal("the save button of a saved line is not lit")
	}
}

// TestThePanelSaysWhyNoExplanationCame checks that while the AI service is asked the panel says so, and without one it says why and offers its settings.
func TestThePanelSaysWhyNoExplanationCame(t *testing.T) {
	a := newFakeApp()
	app := start(t, a)
	change(app, a, func() {
		a.side = SideExplain
		a.explanation = Explanation{Open: true, Original: "Hello.", Busy: true, Say: func(*ns.Scope) string { return "asking m1" }}
	})
	if !app.Exists(whole("asking m1")) || app.Exists("id:film-explain-settings") || app.Exists("id:film-explain-note") {
		t.Fatal("the panel while the line is explained")
	}
	change(app, a, func() {
		a.explanation = Explanation{Open: true, Original: "Hello.", Failed: true, NeedsAI: true, Say: func(*ns.Scope) string { return "no AI" }}
	})
	if !app.Exists(whole("no AI")) {
		t.Fatal("the panel does not say why")
	}
	app.Press("id:film-explain-settings")
	if a.lastAsk() != "settings" {
		t.Fatalf("Open Settings asked %q", a.asks)
	}
}

// TestTheRecapCardShowsWhatWasWatched checks the 前情提要 card (design Main 674-680): where the film stopped, the moments of the part watched, each playing from it at a click, while busy that they are being summed up, the footnote, and 继续, 从头看 and × going back to the app; Escape closes it too.
func TestTheRecapCardShowsWhatWasWatched(t *testing.T) {
	a := newFakeApp()
	app := start(t, a)
	msgs := testlocale.Messages()
	if app.Exists("id:film-recap") {
		t.Fatal("the card shows with no recap")
	}
	change(app, a, func() { a.recap = Recap{Open: true, At: 100 * time.Second, Busy: true} })
	for _, want := range []string{msgs.RecapLastAt("1:40"), msgs.RecapHeading(), msgs.RecapBusy(), msgs.RecapContinue("1:40"), msgs.RecapStartOver()} {
		if !app.Exists("id:film-recap >> " + whole(want)) {
			t.Fatalf("the busy card has no %q", want)
		}
	}
	change(app, a, func() {
		a.recap = Recap{Open: true, At: 100 * time.Second, Items: []RecapItem{{At: 2 * time.Second, Text: "河流始于山间。"}, {At: 70 * time.Second, Text: "弯道叫河曲。"}},
			Note: func(*ns.Scope) string { return "summed up by m1" }}
	})
	for _, want := range []string{"0:02", "河流始于山间。", "1:10", "弯道叫河曲。", "summed up by m1"} {
		if !app.Exists("id:film-recap >> " + whole(want)) {
			t.Fatalf("the card has no %q", want)
		}
	}
	if app.Exists("id:film-recap-busy") {
		t.Fatal("the card with items still says it is busy")
	}
	app.Press("id:film-recap-item >> nth=1")
	app.Press("id:film-recap-continue")
	app.Press("id:film-recap-start")
	app.Press("id:film-recap-close")
	app.Key("escape")
	if !a.askedAll("jump 1", "continue", "start over", "close recap", "close recap") {
		t.Fatalf("the card asked %q", a.asks)
	}
}
