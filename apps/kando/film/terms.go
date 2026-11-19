package film

import (
	"slices"
	"strings"

	"kando/apps/kando/live"
	"kando/apps/kando/locales"
	"kando/apps/kando/theme"
	"kando/packages/core"

	"github.com/nsgo-dev/nsgo/base"
	ns "github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	"github.com/nsgo-dev/nsgo/ui"
)

// Glossary is the open film's glossary, which its translation writes names and terms by (core.Term): its own terms, its series' and every film's, as the 术语 tab lists and changes them.
type Glossary interface {
	// FilmTerms is the open film's glossary, its own terms first, each source once.
	FilmTerms() []core.Term
	// TermsWritable reports whether the open film's own terms can be changed, as for a film with a video to keep them beside.
	TermsWritable() bool
	// FilmSeries is the name of the open film's series and how many films of the library are in it, "" and 0 for a film of no series; LeaveSeries takes the film out of it.
	FilmSeries() (name string, films int)
	LeaveSeries()
	// SeriesGuess is the films of the library the open film looks like it is a series with, as their names say they are episodes of one show, the open film's name first, and the show; none when the open film is in a series or nothing is to be asked. ConfirmSeries puts them in a series named after the show; DismissSeries keeps that they are not one, so it is not asked again.
	SeriesGuess() (show string, films []string)
	ConfirmSeries()
	DismissSeries()
	// CycleTermScope moves the term for source to the next scope (the film's, its series', every film's), ToggleTermLock locks or unlocks it, and RemoveTerm takes it out of the glossary.
	CycleTermScope(source string)
	ToggleTermLock(source string)
	RemoveTerm(source string)
	// NewTermSource and NewTermTarget are the fields a term is typed in; AddTerm adds what they hold to the film's own terms, or gives the term for the same words that translation.
	NewTermSource() *string
	NewTermTarget() *string
	AddTerm()
}

// scopeColor is the pill of scope (prototype script 3010) in the palette under cx: the film's a plain glass, a series' the accent's blue and every film's green. It reads the palette, so it goes in a StyleFunc.
func scopeColor(cx *ns.Scope, scope core.TermScope) ns.Color {
	switch scope {
	case core.ScopeSeries:
		return ui.Alpha(theme.Of(cx).AccentInk(), 0.45)
	case core.ScopeGlobal:
		return ui.Alpha(theme.Of(cx).SuccessInk(), 0.35)
	}
	return ns.RGBA(0xFFFFFF, 0.12)
}

// scopeName is what the 术语 tab calls a scope in the language the interface shows under cx.
func scopeName(cx *ns.Scope, scope core.TermScope) string {
	switch scope {
	case core.ScopeSeries:
		return locales.GlossaryScopeSeries(cx)
	case core.ScopeGlobal:
		return locales.GlossaryScopeGlobal(cx)
	}
	return locales.GlossaryScopeFilm(cx)
}

// nextScope is the scope a press of a term's pill moves it to, as the app's CycleTermScope does: the film's, its series' when it is in one, every film's, and back.
func nextScope(scope core.TermScope, series bool) core.TermScope {
	switch {
	case scope == core.ScopeFilm && series:
		return core.ScopeSeries
	case scope == core.ScopeGlobal:
		return core.ScopeFilm
	}
	return core.ScopeGlobal
}

// termsTab is the open film's glossary (prototype template 956-990): what the terms do, whether the films the open film looks like a series with are one (seriesGuessCard), the film's series with the button that takes it out of it, the terms each with its scope, which a press moves on, its lock and its removal, and the fields a term is added in.
func (s *screen) termsTab(cx *ns.Scope) ns.Node {
	a := s.a
	terms := live.Func(a, a.FilmTerms)
	series := live.Func(a, func() string { name, _ := a.FilmSeries(); return name })
	inSeries := live.Func(a, func() bool { _, n := a.FilmSeries(); return n > 0 })
	seriesLine := ns.Show{When: inSeries, Then: func(cx *ns.Scope) ns.Node {
		return ns.View{TestID: "film-terms-series", Style: ns.Style{Direction: ns.Row, AlignItems: ns.AlignCenter, Gap: 8, Padding: ns.Symmetric(8, 12), CornerRadius: 12, Background: panelCard},
			Children: []ns.Node{
				ns.Text{ContentFunc: func() string { _, n := a.FilmSeries(); return locales.GlossarySeriesLine(cx, series(), n) },
					Style: ns.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: ns.Pt(0), FontSize: 12, LineHeight: 16, Color: onGlass}},
				pillButton(cx, pill{id: "film-terms-leave-series", label: func() string { return locales.GlossaryLeaveSeries(cx) }, color: func() ns.Color { return ns.RGBA(0xFFFFFF, 0.12) }, press: live.Do(a, a.LeaveSeries)}),
			}}
	}}
	guess := ns.NewMemoFunc(cx, live.Func(a, func() []string { _, names := a.SeriesGuess(); return names }), slices.Equal[[]string]).Get
	guessCard := ns.Show{When: func() bool { return len(guess()) > 1 }, Then: func(cx *ns.Scope) ns.Node {
		return s.seriesGuessCard(cx, guess)
	}}
	rows := ns.Each(terms, func(t core.Term) string { return strings.ToLower(t.Source) }, func(cx *ns.Scope, term func() core.Term, index func() int) ns.Node {
		return s.termRow(cx, term, index, inSeries)
	})
	list := ns.Show{
		When: func() bool { return len(terms()) > 0 },
		Then: func(*ns.Scope) ns.Node {
			return ns.View{TestID: "film-terms-list", Style: ns.Style{CornerRadius: 14, Background: panelCard}, Children: []ns.Node{rows}}
		},
		Else: func(cx *ns.Scope) ns.Node {
			return panelNote(func() string { return locales.GlossaryEmpty(cx) }, nil)
		},
	}
	return ns.Scroll{TestID: "film-terms", Style: ns.Style{FlexGrow: 1, MinHeight: ns.Pt(0)}, Children: []ns.Node{
		ns.View{Style: ns.Style{Padding: ns.Edges{Right: 14, Bottom: 16, Left: 14}, Gap: 10}, Children: []ns.Node{
			ns.Text{ContentFunc: func() string { return locales.GlossaryIntro(cx) }, Style: ns.Style{FontSize: 11, LineHeight: 16, Color: onGlass, Opacity: ns.Alpha(0.7)}},
			guessCard,
			seriesLine,
			list,
			s.termAdder(cx),
		}},
	}}
}

// seriesGuessCard asks whether the films of names, which look like episodes of one show, are a series (prototype template 959-966): the question, their names, what a series does and that only their names were read, and Same Series and Not a Series.
func (s *screen) seriesGuessCard(cx *ns.Scope, names func() []string) ns.Node {
	a := s.a
	return ns.View{TestID: "film-terms-guess", Role: ns.RoleGroup, LabelFunc: func() string { return locales.GlossaryGuessTitle(cx) },
		Style: ns.Style{CornerRadius: 14, Padding: ns.Symmetric(11, 12), Gap: 8},
		StyleFunc: func() ns.Style {
			return ns.Style{Background: ui.Alpha(theme.Of(cx).AccentInk(), 0.18), BorderWidth: 1, BorderColor: ui.Alpha(theme.Of(cx).AccentInk(), 0.45)}
		},
		Children: []ns.Node{
			ns.Text{ContentFunc: func() string { return locales.GlossaryGuessTitle(cx) }, Style: ns.Style{FontSize: 13, LineHeight: 17, FontWeight: ns.FontWeightBold, Color: onGlass}},
			ns.Text{TestID: "film-terms-guess-films", ContentFunc: func() string { return strings.Join(names(), " · ") }, Style: ns.Style{FontSize: 12, LineHeight: 17, Color: onGlass, Opacity: ns.Alpha(0.85)}},
			ns.Text{ContentFunc: func() string { return locales.GlossaryGuessHint(cx) }, Style: ns.Style{FontSize: 11, LineHeight: 15, Color: onGlass, Opacity: ns.Alpha(0.65)}},
			ns.View{Style: ns.Style{Direction: ns.Row, Gap: 6}, Children: []ns.Node{
				pillButton(cx, pill{id: "film-terms-guess-yes", label: func() string { return locales.GlossaryGuessYes(cx) }, color: func() ns.Color { return ns.RGBA(0xFFFFFF, 0.28) }, press: live.Do(a, a.ConfirmSeries)}),
				pillButton(cx, pill{id: "film-terms-guess-no", label: func() string { return locales.GlossaryGuessNo(cx) }, color: func() ns.Color { return ns.RGBA(0xFFFFFF, 0.14) }, press: live.Do(a, a.DismissSeries)}),
			}},
		}}
}

// termRow is a term of the 术语 tab: the words and their translation over where the term applies and whether it is locked, its scope's pill, its lock and its removal.
func (s *screen) termRow(cx *ns.Scope, term func() core.Term, index func() int, series func() bool) ns.Node {
	a := s.a
	source := func() string { return term().Source }
	meta := func() string {
		t := term()
		if t.Locked {
			return locales.GlossaryMetaLocked(cx, scopeName(cx, t.Scope))
		}
		return scopeName(cx, t.Scope)
	}
	scopeLabel := func() string {
		t := term()
		return locales.GlossaryScopeLabel(cx, scopeName(cx, t.Scope), scopeName(cx, nextScope(t.Scope, series())))
	}
	lockLabel := func() string {
		if term().Locked {
			return locales.GlossaryUnlock(cx, term().Target)
		}
		return locales.GlossaryLock(cx, term().Target)
	}
	return ns.View{TestID: "film-term", Style: ns.Style{Direction: ns.Row, AlignItems: ns.AlignCenter, Gap: 8, MinHeight: ns.Pt(50), Padding: ns.Symmetric(7, 12)},
		StyleFunc: func() ns.Style {
			if index() > 0 {
				return ns.Style{Borders: ns.Edges{Top: 1}, BorderColor: panelLine}
			}
			return ns.Style{Borders: ns.Edges{}}
		},
		Children: []ns.Node{
			ns.View{Selectable: true, Style: ns.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: ns.Pt(0), Gap: 2}, Children: []ns.Node{
				ns.Text{TestID: "film-term-pair", ContentFunc: func() string { t := term(); return t.Source + " → " + t.Target },
					Style: ns.Style{FontSize: 13, LineHeight: 17, Color: onGlass}},
				ns.Text{TestID: "film-term-meta", ContentFunc: meta, Style: ns.Style{FontSize: 11, LineHeight: 14, Color: onGlass, Opacity: ns.Alpha(0.6)}},
			}},
			pillButton(cx, pill{id: "film-term-scope", label: func() string { return scopeName(cx, term().Scope) }, name: scopeLabel, color: func() ns.Color { return scopeColor(cx, term().Scope) },
				disabled: live.Func(a, func() bool { return !a.TermsWritable() }), press: live.Do(a, func() { a.CycleTermScope(source()) })}),
			// A locked term's lock sits on a lighter disc, as the prototype marks it (template 978).
			ns.View{Style: ns.Style{CornerRadius: 14, FlexShrink: 0},
				StyleFunc: func() ns.Style {
					if term().Locked {
						return ns.Style{Background: ns.RGBA(0xFFFFFF, 0.22)}
					}
					return ns.Style{Background: ns.RGBA(0xFFFFFF, 0)}
				},
				Children: []ns.Node{ui.IconButton{TestID: "film-term-lock", LabelFunc: lockLabel, Size: 28, IconSize: 14,
					Icon: func() *ns.SVG {
						if term().Locked {
							return lucide.Lock()
						}
						return lucide.LockOpen()
					},
					Selected: func() bool { return term().Locked },
					Tint:     func() ns.Color { return onGlass },
					Disabled: live.Func(a, func() bool { return !a.TermsWritable() }),
					OnPress:  live.Do(a, func() { a.ToggleTermLock(source()) })}}},
			ui.IconButton{TestID: "film-term-remove", LabelFunc: func() string { return locales.GlossaryRemove(cx, term().Source) }, Size: 28, IconSize: 14,
				Icon: lucide.X, Tint: func() ns.Color { return ns.RGBA(0xFFFFFF, 0.7) },
				Disabled: live.Func(a, func() bool { return !a.TermsWritable() }),
				OnPress:  live.Do(a, func() { a.RemoveTerm(source()) })},
		}}
}

// termAdder is the fields a term is added in, the words as the film's lines write them and their translation, and the button that adds it; Return in either adds it too. A film with nowhere to keep its own terms, as one still downloading, cannot add any.
func (s *screen) termAdder(cx *ns.Scope) ns.Node {
	a := s.a
	add := live.Do(a, a.AddTerm)
	writable := live.Func(a, a.TermsWritable)
	field := func(id string, label, placeholder func() string, value func() *string) ns.Node {
		// TODO(nsgo#145): give core.Input (base.Input, ui.Input) LabelFunc and AccessibilityDescriptionFunc twins, so the field's accessible name follows the interface language as it changes; it is read once here, and a Dynamic would take the focus and the typed text with it https://github.com/nsgo-dev/nsgo/issues/145
		return base.Input{TestID: id, Label: label(), PlaceholderFunc: placeholder,
			Value:          live.Func(a, func() string { return *value() }),
			OnChange:       live.On(a, func(text string) { *value() = text }),
			Disabled:       func() bool { return !writable() },
			SubmitBehavior: ns.SubmitBehaviorSubmit,
			OnSubmit:       func(string) { add() },
			Style: ns.Style{FlexGrow: 1, FlexBasis: ns.Pt(0), MinWidth: ns.Pt(0), Height: ns.Pt(30), CornerRadius: 8, Background: ns.RGBA(0xFFFFFF, 0.1), FontSize: 12,
				Padding: ns.Symmetric(0, 8), Color: onGlass, PlaceholderColor: ns.RGBA(0xFFFFFF, 0.45), CaretColor: onGlass}}
	}
	ready := live.Func(a, func() bool {
		return a.TermsWritable() && strings.TrimSpace(*a.NewTermSource()) != "" && strings.TrimSpace(*a.NewTermTarget()) != ""
	})
	return ns.View{TestID: "film-terms-add", Role: ns.RoleGroup, LabelFunc: func() string { return locales.GlossaryAddGroup(cx) }, Style: ns.Style{Gap: 6}, Children: []ns.Node{
		panelHeading(locales.GlossaryAddGroup(cx)),
		ns.View{Style: ns.Style{Direction: ns.Row, Gap: 6, AlignItems: ns.AlignCenter}, Children: []ns.Node{
			field("film-term-source", func() string { return locales.GlossarySource(cx) }, func() string { return locales.GlossarySourceHint(cx) }, a.NewTermSource),
			ns.Text{Content: "→", Role: ns.RoleNone, Style: ns.Style{FontSize: 12, Color: onGlass, Opacity: ns.Alpha(0.45)}},
			field("film-term-target", func() string { return locales.GlossaryTarget(cx) }, func() string { return locales.GlossaryTargetHint(cx) }, a.NewTermTarget),
			pillButton(cx, pill{id: "film-term-add", label: func() string { return locales.GlossaryAdd(cx) }, color: func() ns.Color { return ns.RGBA(0xFFFFFF, 0.16) }, disabled: func() bool { return !ready() }, press: add}),
		}},
	}}
}

// pill is a small round-ended button of the panel: label on color, press what it does. name, when set, is what assistive technology and the tooltip say in place of the label; disabled, when set, dims it and keeps it from being pressed.
type pill struct {
	id       string
	label    func() string
	name     func() string
	color    func() ns.Color
	disabled func() bool
	press    func()
}

func pillButton(cx *ns.Scope, p pill) ns.Node {
	name := p.label
	var tooltip ns.Node
	if p.name != nil {
		name, tooltip = p.name, tip(cx, p.name)
	}
	return base.Button{TestID: p.id, LabelFunc: name, Tooltip: tooltip, Disabled: p.disabled,
		Desktop: ns.DesktopViewProps{WindowDrag: ns.WindowDragOff},
		OnPress: func(ns.PressEvent) { p.press() },
		Style: ns.Style{Height: ns.Pt(24), Padding: ns.Symmetric(0, 10), FlexShrink: 0, AlignItems: ns.AlignCenter, JustifyContent: ns.JustifyCenter, CornerRadius: 12,
			Hover: &ns.Style{Opacity: ns.Alpha(0.85)}, FocusVisible: ui.FocusRing(cx, -2), Disabled: &ns.Style{Opacity: ns.Alpha(0.4)}},
		StyleFunc: func() ns.Style { return ns.Style{Background: p.color()} },
		Children: []ns.Node{ns.Text{ContentFunc: p.label, PointerEvents: ns.PointerEventsNone, MaxLines: 1,
			Style: ns.Style{FontSize: 11, LineHeight: 14, FontWeight: ns.FontWeightSemibold, Color: onGlass}}}}
}
