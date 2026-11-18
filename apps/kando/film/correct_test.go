package film

import (
	"testing"
	"time"

	"kando/apps/kando/locales/testlocale"

	"github.com/nsgo-dev/nsgo/core"
)

// fakeCorrector is the fake app's correcting of lines: it records what the page asks of it. sel is 1 more than the index of the line selected, 0 for none.
type fakeCorrector struct {
	sel                   int
	editing               bool
	translation, original string
	steps                 []int
	nudges                []time.Duration
	submitted, noSpeech   bool
	// cand and spread are what Candidates and Spread give; picked is the candidate kept and asked what else the page asked of them.
	cand   Candidates
	spread Spread
	picked int
	asked  []string
}

func (f *fakeCorrector) Candidates() Candidates { return f.cand }
func (f *fakeCorrector) StartCandidates()       { f.asked = append(f.asked, "start") }
func (f *fakeCorrector) PickCandidate(i int)    { f.picked = i; f.cand = Candidates{} }
func (f *fakeCorrector) CloseCandidates()       { f.cand = Candidates{} }
func (f *fakeCorrector) CandidateSettings()     { f.asked = append(f.asked, "settings") }
func (f *fakeCorrector) Spread() Spread         { return f.spread }
func (f *fakeCorrector) SpreadAll()             { f.asked = append(f.asked, "all"); f.spread = Spread{} }
func (f *fakeCorrector) SpreadView()            { f.asked = append(f.asked, "view") }
func (f *fakeCorrector) SpreadOnly()            { f.asked = append(f.asked, "only"); f.spread = Spread{} }

func (f *fakeCorrector) Selected() int            { return f.sel - 1 }
func (f *fakeCorrector) Editing() bool            { return f.editing }
func (f *fakeCorrector) EditTranslation() *string { return &f.translation }
func (f *fakeCorrector) EditOriginal() *string    { return &f.original }
func (f *fakeCorrector) SelectLine(i int)         { f.sel = i + 1 }
func (f *fakeCorrector) StepSelection(d int)      { f.steps = append(f.steps, d) }
func (f *fakeCorrector) EndSelection()            { f.sel, f.editing = 0, false }
func (f *fakeCorrector) EditLine()                { f.editing = true }
func (f *fakeCorrector) CancelEdit()              { f.editing = false }
func (f *fakeCorrector) SubmitEdit()              { f.submitted, f.editing = true, false }
func (f *fakeCorrector) MarkNoSpeech()            { f.noSpeech = true }
func (f *fakeCorrector) NudgeLine(end bool, d time.Duration) {
	if end {
		d += time.Hour
	}
	f.nudges = append(f.nudges, d)
}

// TestTheSelectionBarCorrectsTheLine checks the bar over the picture while a line is selected: it shows the line's span, and its buttons step, nudge the start and the end, edit, mark as no speech and let go; editing shows the fields, which Escape puts away.
func TestTheSelectionBarCorrectsTheLine(t *testing.T) {
	a := newFakeApp()
	tt := start(t, a)
	if tt.Exists("id:film-correct-bar") {
		t.Fatal("the bar shows with no line selected")
	}
	change(tt, a, func() { a.SelectLine(0) })
	wantText := func(s string) {
		t.Helper()
		if !tt.Exists(whole(s)) {
			t.Fatalf("no %q in %q", s, tt.Texts())
		}
	}
	wantText("0:01.0 – 0:03.0")
	for _, id := range []string{"film-correct-prev", "film-correct-next", "film-correct-start-earlier", "film-correct-start-later", "film-correct-end-earlier", "film-correct-end-later", "film-correct-no-speech"} {
		tt.Press("id:" + id)
	}
	if len(a.steps) != 2 || a.steps[0] != -1 || a.steps[1] != 1 {
		t.Fatalf("steps %v", a.steps)
	}
	want := []time.Duration{-nudgeStep, nudgeStep, time.Hour - nudgeStep, time.Hour + nudgeStep}
	if len(a.nudges) != len(want) {
		t.Fatalf("nudges %v", a.nudges)
	}
	for i := range want {
		if a.nudges[i] != want[i] {
			t.Fatalf("nudges %v, want %v", a.nudges, want)
		}
	}
	if !a.noSpeech {
		t.Fatal("no speech was not asked")
	}
	tt.Press("id:film-correct-edit")
	if !tt.Exists("id:film-correct-editor") || tt.Exists("id:film-correct-bar") {
		t.Fatal("editing does not show the fields in place of the bar")
	}
	if !tt.Find("id:film-correct-translation").Focused {
		t.Fatal("the translation field does not take the keyboard")
	}
	tt.Type("id:film-correct-translation", "你好！")
	if a.translation != "你好！" {
		t.Fatalf("typed %q", a.translation)
	}
	tt.Key("escape")
	if a.editing || !tt.Exists("id:film-correct-bar") {
		t.Fatal("Escape did not put the fields away")
	}
	tt.Press("id:film-correct-edit")
	tt.Press("id:film-correct-submit")
	if !a.submitted {
		t.Fatal("the fields were not kept")
	}
	tt.Press("id:film-correct-done")
	if a.Selected() >= 0 || tt.Exists("id:film-correct-bar") {
		t.Fatal("Done did not let go of the line")
	}
}

// TestTheCandidatesShowOverTheLine checks the list of other translations of the selected line: while they are asked for, its heading says so over the line as it is; once they came, it shows the line as it is and each translation, the highlighted one selected, and a click keeps one; with none, it says why and offers the AI service's settings. The selection bar's 这句不对 asks for them.
func TestTheCandidatesShowOverTheLine(t *testing.T) {
	a := newFakeApp()
	tt := start(t, a)
	msgs := testlocale.Messages()
	change(tt, a, func() { a.SelectLine(0) })
	tt.Press("id:film-correct-candidates")
	if len(a.asked) != 1 || a.asked[0] != "start" {
		t.Fatalf("这句不对 asked %v", a.asked)
	}
	change(tt, a, func() { a.cand = Candidates{Open: true, Busy: true, Original: "Hello.", Current: "你好。"} })
	if tt.Exists("id:film-correct-bar") || !tt.Exists("id:film-candidates") {
		t.Fatal("the list does not take the bar's place")
	}
	for _, want := range []string{msgs.CorrectCandidatesBusy(), "Hello.", "你好。"} {
		if !tt.Exists(whole(want)) {
			t.Fatalf("no %q in %q", want, tt.Texts())
		}
	}
	change(tt, a, func() {
		a.cand = Candidates{Open: true, Original: "Hello.", Current: "你好。", Options: []string{"哈喽。", "嗨。"}, At: 1}
	})
	for _, want := range []string{msgs.CorrectCandidatesTitle(), msgs.CorrectCandidatesCurrent(), msgs.CorrectCandidatesOption(), "哈喽。", "嗨。"} {
		if !tt.Exists(whole(want)) {
			t.Fatalf("no %q in %q", want, tt.Texts())
		}
	}
	if !tt.Find("id:film-candidate-1").Props.Selected || tt.Find("id:film-candidate-2").Props.Selected {
		t.Fatal("the highlighted translation is not the one selected")
	}
	tt.Press("id:film-candidate-2")
	if a.picked != 2 || tt.Exists("id:film-candidates") {
		t.Fatalf("a click kept %d", a.picked)
	}
	change(tt, a, func() {
		a.cand = Candidates{Open: true, Original: "Hello.", Current: "你好。", Say: func(cx *core.Scope) string { return "no AI" }, NeedsAI: true}
	})
	if !tt.Exists(whole("no AI")) || tt.Exists("id:film-candidate-1") {
		t.Fatalf("no reason, or translations shown: %q", tt.Texts())
	}
	tt.Press("id:film-candidates-settings")
	if a.asked[len(a.asked)-1] != "settings" {
		t.Fatalf("calls %v", a.asked)
	}
}

// TestTheSpreadBarAsks checks the bar that asks about spreading a fix: it names the words, how many other lines hold them and how many of those the user edited, and its buttons change them all, show them or leave them.
func TestTheSpreadBarAsks(t *testing.T) {
	a := newFakeApp()
	tt := start(t, a)
	msgs := testlocale.Messages()
	change(tt, a, func() { a.spread = Spread{Open: true, From: "约翰", To: "琼恩", Lines: 3, Edited: 1} })
	if want := msgs.CorrectSpreadAskEdited("约翰", "琼恩", 3, 1); !tt.Exists(whole(want)) {
		t.Fatalf("no %q in %q", want, tt.Texts())
	}
	tt.Press("id:film-spread-view")
	tt.Press("id:film-spread-all")
	if len(a.asked) != 2 || a.asked[0] != "view" || a.asked[1] != "all" || tt.Exists("id:film-spread") {
		t.Fatalf("calls %v", a.asked)
	}
	change(tt, a, func() { a.spread = Spread{Open: true, From: "约翰", To: "琼恩", Lines: 2} })
	if want := msgs.CorrectSpreadAsk("约翰", "琼恩", 2); !tt.Exists(whole(want)) {
		t.Fatalf("no %q in %q", want, tt.Texts())
	}
	tt.Press("id:film-spread-only")
	if a.asked[len(a.asked)-1] != "only" || tt.Exists("id:film-spread") {
		t.Fatalf("calls %v", a.asked)
	}
}
