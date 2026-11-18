package film

import (
	"testing"
	"time"
)

// fakeCorrector is the fake app's correcting of lines: it records what the page asks of it. sel is 1 more than the index of the line selected, 0 for none.
type fakeCorrector struct {
	sel                   int
	editing               bool
	translation, original string
	steps                 []int
	nudges                []time.Duration
	submitted, noSpeech   bool
}

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
