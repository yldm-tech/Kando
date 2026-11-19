package film

import (
	"testing"

	"kando/apps/kando/live"
	"kando/apps/kando/locales/testlocale"
	"kando/apps/kando/theme"
	"kando/packages/core"

	ns "github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
	"github.com/nsgo-dev/nsgo/ui"
)

// fakeGlossary is the fake app's glossary: the terms the tab lists and what the tab asks of them, kept as a real app would.
type fakeGlossary struct {
	terms          []core.Term
	writable       bool
	series         string
	seriesFilms    int
	source, target string
	left           bool
	guess          []string
	guessAnswer    string
}

func (f *fakeGlossary) FilmTerms() []core.Term    { return f.terms }
func (f *fakeGlossary) TermsWritable() bool       { return f.writable }
func (f *fakeGlossary) FilmSeries() (string, int) { return f.series, f.seriesFilms }
func (f *fakeGlossary) LeaveSeries()              { f.left, f.series, f.seriesFilms = true, "", 0 }
func (f *fakeGlossary) SeriesGuess() (string, []string) {
	if len(f.guess) == 0 {
		return "", nil
	}
	return "地理课", f.guess
}
func (f *fakeGlossary) ConfirmSeries() {
	f.guessAnswer, f.series, f.seriesFilms, f.guess = "yes", "地理课", len(f.guess), nil
}
func (f *fakeGlossary) DismissSeries()           { f.guessAnswer, f.guess = "no", nil }
func (f *fakeGlossary) NewTermSource() *string   { return &f.source }
func (f *fakeGlossary) NewTermTarget() *string   { return &f.target }
func (f *fakeGlossary) RemoveTerm(source string) { f.terms = core.RemoveTerm(f.terms, source) }
func (f *fakeGlossary) CycleTermScope(source string) {
	f.edit(source, func(t *core.Term) { t.Scope = nextScope(t.Scope, f.seriesFilms > 0) })
}
func (f *fakeGlossary) ToggleTermLock(source string) {
	f.edit(source, func(t *core.Term) { t.Locked = !t.Locked })
}
func (f *fakeGlossary) AddTerm() {
	f.terms = core.SetTerm(f.terms, core.Term{Source: f.source, Target: f.target, Scope: core.ScopeFilm})
	f.source, f.target = "", ""
}

func (f *fakeGlossary) edit(source string, fn func(*core.Term)) {
	for i := range f.terms {
		if core.SameTerm(f.terms[i].Source, source) {
			fn(&f.terms[i])
		}
	}
}

// TestTermsTabChangesTheGlossary opens the 术语 tab: it says the film has no terms, adds one from its fields, moves it through the scopes the film has, locks it, removes it, and takes the film out of its series.
func TestTermsTabChangesTheGlossary(t *testing.T) {
	a := newFakeApp()
	a.writable, a.series, a.seriesFilms = true, "地理课", 3
	app := start(t, a)
	msgs := testlocale.Messages()
	app.Press("id:film-tab-terms")
	if a.side != "terms" || !app.Exists(whole(msgs.GlossaryEmpty())) {
		t.Fatalf("the terms tab: side %q texts %q", a.side, app.Texts())
	}
	if !app.Exists(whole(msgs.GlossarySeriesLine("地理课", 3))) {
		t.Fatalf("the series is not named: %q", app.Texts())
	}
	if !app.Find("id:film-term-add").Props.Disabled {
		t.Fatal("Add is on with empty fields")
	}
	app.Type("id:film-term-source", "meander")
	app.Type("id:film-term-target", "河曲")
	app.Press("id:film-term-add")
	if len(a.terms) != 1 || a.terms[0].Source != "meander" || a.terms[0].Target != "河曲" || a.source != "" {
		t.Fatalf("added %+v, fields %q %q", a.terms, a.source, a.target)
	}
	if !app.Exists(whole("meander → 河曲")) || !app.Exists(whole(msgs.GlossaryScopeFilm())) {
		t.Fatalf("the new term is not listed: %q", app.Texts())
	}
	if got, want := app.Find("id:film-term-scope").Props.Label, msgs.GlossaryScopeLabel(msgs.GlossaryScopeFilm(), msgs.GlossaryScopeSeries()); got != want {
		t.Fatalf("the scope pill is named %q, want %q", got, want)
	}
	for _, want := range []core.TermScope{core.ScopeSeries, core.ScopeGlobal, core.ScopeFilm} {
		app.Press("id:film-term-scope")
		if a.terms[0].Scope != want {
			t.Fatalf("scope %q, want %q", a.terms[0].Scope, want)
		}
	}
	app.Press("id:film-term-lock")
	if !a.terms[0].Locked || !app.Exists(whole(msgs.GlossaryMetaLocked(msgs.GlossaryScopeFilm()))) {
		t.Fatalf("lock: %+v, texts %q", a.terms, app.Texts())
	}
	if got := app.Find("id:film-term-lock").Props.Label; got != msgs.GlossaryUnlock("河曲") {
		t.Fatalf("the lock of a locked term is named %q", got)
	}
	app.Press("id:film-term-remove")
	if len(a.terms) != 0 || !app.Exists(whole(msgs.GlossaryEmpty())) {
		t.Fatalf("remove: %+v, texts %q", a.terms, app.Texts())
	}
	app.Press("id:film-terms-leave-series")
	if !a.left || app.Exists("id:film-terms-series") {
		t.Fatal("the film was not taken out of its series")
	}
}

// A film with nowhere to keep its own terms lists the others but cannot add or change any.
func TestTermsTabOfAFilmWithoutTermsFile(t *testing.T) {
	a := newFakeApp()
	a.terms = []core.Term{{Source: "delta", Target: "三角洲", Scope: core.ScopeGlobal}}
	app := start(t, a)
	app.Press("id:film-tab-terms")
	if !app.Exists(whole("delta → 三角洲")) {
		t.Fatalf("the global term is not listed: %q", app.Texts())
	}
	for _, id := range []string{"film-term-add", "film-term-scope", "film-term-lock", "film-term-remove", "film-term-source"} {
		if !app.Find("id:" + id).Props.Disabled {
			t.Errorf("%s is on for a film that cannot keep terms", id)
		}
	}
	if app.Exists("id:film-terms-series") {
		t.Fatal("a film of no series names one")
	}
}

// A term's scope pill is in the theme's colors for the dark player: a series' in the accent ink, every film's in the success ink, the film's own a plain glass.
func TestTermScopePillReadsTheTheme(t *testing.T) {
	a := newFakeApp()
	a.side, a.writable, a.series, a.seriesFilms = "terms", true, "地理课", 3
	a.terms = []core.Term{{Source: "delta", Target: "三角洲", Scope: core.ScopeSeries}}
	var k theme.Colors
	app := nsgotest.Start(t, func(cx *ns.Scope) ns.Node {
		a.Attach(cx)
		theme.Apply(cx)
		return ns.View{Style: ns.Style{FlexGrow: 1, FlexShrink: 1, AlignItems: ns.AlignStretch}, Children: []ns.Node{ns.Func(func(cx *ns.Scope) ns.Node {
			theme.Dark(cx)
			k = theme.Of(cx)
			return View{App: a, Item: live.Func(a, func() Item { return a.item })}
		})}}
	}, nsgotest.Size(1100, 720), nsgotest.Environment(testlocale.Environment(ns.Environment{OS: ns.OSMacOS, Idiom: ns.IdiomDesktop, AppleKeyboard: true})))
	for _, want := range []ns.Color{ui.Alpha(k.AccentInk(), 0.45), ui.Alpha(k.SuccessInk(), 0.35), ns.RGBA(0xFFFFFF, 0.12)} {
		if got := app.Find("id:film-term-scope").Style.Background; got != want {
			t.Fatalf("the %s pill is %#x, want %#x", a.terms[0].Scope, uint32(got), uint32(want))
		}
		app.Press("id:film-term-scope")
	}
}

// The 术语 tab asks whether the films the open film looks like a series with are one, naming them; Same Series puts them in one, which the tab then names, and Not a Series takes the question away.
func TestTermsTabAsksAboutAGuessedSeries(t *testing.T) {
	a := newFakeApp()
	a.side, a.writable = "terms", true
	a.guess = []string{"地理课 第1集", "地理课 第2集"}
	app := start(t, a)
	msgs := testlocale.Messages()
	if !app.Exists(whole(msgs.GlossaryGuessTitle())) || !app.Exists(whole("地理课 第1集 · 地理课 第2集")) {
		t.Fatalf("the question: %q", app.Texts())
	}
	app.Press("id:film-terms-guess-yes")
	if a.guessAnswer != "yes" || app.Exists("id:film-terms-guess") || !app.Exists(whole(msgs.GlossarySeriesLine("地理课", 2))) {
		t.Fatalf("after yes: answer %q, texts %q", a.guessAnswer, app.Texts())
	}
	change(app, a, func() { a.guess, a.series, a.seriesFilms = []string{"a 1", "a 2"}, "", 0 })
	app.Press("id:film-terms-guess-no")
	if a.guessAnswer != "no" || app.Exists("id:film-terms-guess") {
		t.Fatalf("after no: answer %q", a.guessAnswer)
	}
}
