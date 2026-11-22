package film

import (
	"time"

	"kando/apps/kando/live"
	"kando/apps/kando/locales"
	"kando/apps/kando/theme"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

// Corrector is the user correcting the open film's lines, which the app keeps apart from the subtitle files the pipeline writes, so processing the film again keeps them: a line selected on the picture, its translation and its original edited, its start and end moved, or the line marked as no speech.
type Corrector interface {
	// Selected is the index of the cue selected for correcting among Subs().Cues, -1 while none is; Editing reports whether the fields that edit it show, which hold EditTranslation and EditOriginal as typed.
	Selected() int
	Editing() bool
	EditTranslation() *string
	EditOriginal() *string
	// SelectLine selects cue i and holds the film at its start; StepSelection selects the cue d after it, before for a negative d; EndSelection lets go of it.
	SelectLine(i int)
	StepSelection(d int)
	EndSelection()
	// EditLine shows the fields that edit the selected cue, CancelEdit puts them away, and SubmitEdit keeps what they hold and plays on.
	EditLine()
	CancelEdit()
	SubmitEdit()
	// NudgeLine moves the selected cue's start, or with end its end, by d.
	NudgeLine(end bool, d time.Duration)
	// MarkNoSpeech leaves the selected cue out of the film, as one heard in music or noise.
	MarkNoSpeech()
	// Candidates is the AI service's other translations of the selected cue while they show (这句不对): StartCandidates asks for them, PickCandidate keeps one, 0 being the cue as it is, CloseCandidates puts them away and CandidateSettings opens the AI service's settings when none could come.
	Candidates() Candidates
	StartCandidates()
	PickCandidate(i int)
	CloseCandidates()
	CandidateSettings()
	// Spread is the fix an edit made that the film's other cues hold too, while a bar asks about it: SpreadAll changes it in all of them, SpreadView shows them in the subtitle list and SpreadOnly leaves them.
	Spread() Spread
	SpreadAll()
	SpreadView()
	SpreadOnly()
}

// Candidates is the other translations of the selected cue as they show: Open while they do, Busy while the AI service is asked; Original and Current are the cue's original and its translation now, Options the AI service's translations, best first, and At the one highlighted, 0 for Current. Say, when set, says why there are none, and NeedsAI that the AI service's settings are the fix.
type Candidates struct {
	Open, Busy        bool
	Original, Current string
	Options           []string
	At                int
	Say               func(cx *core.Scope) string
	NeedsAI           bool
}

// Spread is the fix an edit made, From to To, while a bar asks whether to make it in the film's other cues: Lines of them hold it, Edited of those the user had corrected.
type Spread struct {
	Open          bool
	From, To      string
	Lines, Edited int
}

// nudgeStep is how far a press of a start or end button of the selection bar moves the line.
const nudgeStep = 100 * time.Millisecond

// correctGlass is the glass of the selection bar and of the fields that edit a line (prototype template 609, 631).
var correctGlass = glassSpec{theme.GlassCard, 16}

// LineClock is a time in a film as the subtitle list shows a line's start, with its tenth of a second ("0:10.4").
func LineClock(d time.Duration) string { return cueClockTenths(d) }

// correcting is the bar of the line selected for correcting, or, while it is edited, the fields that edit it, over the picture in the middle of the room the side panel leaves.
func (s *screen) correcting() core.Node {
	a, cx := s.a, s.cx
	selected := live.Memo(cx, a, func() bool { return a.Selected() >= 0 })
	editing := live.Memo(cx, a, a.Editing)
	picking := live.Memo(cx, a, func() bool { return a.Candidates().Open })
	spreading := live.Memo(cx, a, func() bool { return a.Spread().Open })
	// place lays a part out across the room the side panel leaves, at the top or the bottom of the picture.
	place := func(st core.Style, child core.Node) core.Node {
		return core.View{PointerEvents: core.PointerEventsBoxNone,
			Style: st.Merge(core.Style{Position: core.Absolute, Left: core.Pt(20), Direction: core.Row, JustifyContent: core.JustifyCenter}),
			StyleFunc: func() core.Style {
				return core.Style{Right: core.Pt(s.roomRight() + 20), Transition: roomTransition}
			},
			Children: []core.Node{child}}
	}
	// The bar sits under the title bar, clear of the selected line on the picture; the fields and the other translations, which hold that line, sit over it, above the transport. The bar that asks about spreading a fix takes the selection bar's place, as the edit let go of the line.
	return core.View{PointerEvents: core.PointerEventsBoxNone, Style: fillStyle, Children: []core.Node{
		core.Show{When: selected, Then: func(cx *core.Scope) core.Node {
			return core.View{PointerEvents: core.PointerEventsBoxNone, Style: fillStyle, Children: []core.Node{
				core.Show{When: func() bool { return !editing() && !picking() }, Then: func(cx *core.Scope) core.Node {
					return place(core.Style{Top: core.Pt(72)}, s.selectionBar(cx))
				}},
				core.Show{When: editing, Then: func(cx *core.Scope) core.Node {
					return place(core.Style{Bottom: core.Pt(96)}, s.lineEditor(cx))
				}},
				core.Show{When: picking, Then: func(cx *core.Scope) core.Node {
					return place(core.Style{Bottom: core.Pt(96)}, s.candidateList(cx))
				}},
			}}
		}},
		core.Show{When: func() bool { return spreading() && !selected() }, Then: func(cx *core.Scope) core.Node {
			return place(core.Style{Top: core.Pt(72)}, s.spreadBar(cx))
		}},
	}}
}

// selectionBar is the bar of the selected line: the line before and after it, when it shows, editing it, moving its start and end by a tenth of a second, marking it as no speech, and letting go of it, each button named with the key that does the same.
func (s *screen) selectionBar(cx *core.Scope) core.Node {
	a := s.a
	span := live.Func(a, func() string {
		i, cues := a.Selected(), a.Subs().Cues
		if i < 0 || i >= len(cues) {
			return ""
		}
		return cueClockTenths(cues[i].Start) + " – " + cueClockTenths(cues[i].End)
	})
	sep := core.View{Style: core.Style{Width: core.Pt(1), Height: core.Pt(18), Margin: core.Symmetric(0, 4), Background: core.RGBA(0xFFFFFF, 0.18)}}
	label := func(text func() string) core.Node {
		return core.Text{ContentFunc: text, Style: core.Style{FontSize: 12, LineHeight: 16, Color: core.RGBA(0xFFFFFF, 0.6), Margin: core.Symmetric(0, 2)}}
	}
	return core.View{TestID: "film-correct-bar", Role: core.RoleToolbar, LabelFunc: func() string { return locales.CorrectBar(cx) },
		Style: correctGlass.style().Merge(core.Style{Direction: core.Row, Wrap: core.Wrap, AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter, Gap: 4, Padding: core.All(5), MaxWidth: core.Pct(100)}),
		Children: []core.Node{
			barGlyph(cx, "‹", func() string { return locales.CorrectKeyPrev(cx) }, "shift+left", "film-correct-prev", live.Do(a, func() { a.StepSelection(-1) })),
			core.Text{TestID: "film-correct-span", ContentFunc: span, Style: core.Style{FontSize: 12, LineHeight: 16, Color: onGlass, FontVariant: core.FontTabularNums, Margin: core.Symmetric(0, 4)}},
			barGlyph(cx, "›", func() string { return locales.CorrectKeyNext(cx) }, "shift+right", "film-correct-next", live.Do(a, func() { a.StepSelection(1) })),
			sep,
			barButton(cx, func() string { return locales.CorrectEdit(cx) }, "enter", "film-correct-edit", live.Do(a, a.EditLine)),
			barButton(cx, func() string { return locales.CorrectCandidates(cx) }, "shift+e", "film-correct-candidates", live.Do(a, a.StartCandidates)),
			barButton(cx, func() string { return locales.CorrectExplain(cx) }, "i", "film-correct-explain", live.Do(a, func() {
				if i := a.Selected(); i >= 0 {
					a.ExplainLine(i)
				}
			})),
			sep,
			label(func() string { return locales.CorrectStart(cx) }),
			barGlyph(cx, "−", func() string { return locales.CorrectKeyStartEarlier(cx) }, "ctrl+alt+left", "film-correct-start-earlier", live.Do(a, func() { a.NudgeLine(false, -nudgeStep) })),
			barGlyph(cx, "+", func() string { return locales.CorrectKeyStartLater(cx) }, "ctrl+alt+right", "film-correct-start-later", live.Do(a, func() { a.NudgeLine(false, nudgeStep) })),
			label(func() string { return locales.CorrectEnd(cx) }),
			barGlyph(cx, "−", func() string { return locales.CorrectKeyEndEarlier(cx) }, "ctrl+alt+shift+left", "film-correct-end-earlier", live.Do(a, func() { a.NudgeLine(true, -nudgeStep) })),
			barGlyph(cx, "+", func() string { return locales.CorrectKeyEndLater(cx) }, "ctrl+alt+shift+right", "film-correct-end-later", live.Do(a, func() { a.NudgeLine(true, nudgeStep) })),
			sep,
			barButton(cx, func() string { return locales.CorrectKeyNoSpeech(cx) }, "mod+backspace", "film-correct-no-speech", live.Do(a, a.MarkNoSpeech)),
			barButton(cx, func() string { return locales.CorrectDone(cx) }, "escape", "film-correct-done", live.Do(a, a.EndSelection)),
		}}
}

// lineEditor is the fields that edit the selected line, its translation over its original as the picture shows them, the first taking the keyboard. Return in either keeps them and plays on, Escape puts them away, and Tab goes from one to the other.
func (s *screen) lineEditor(cx *core.Scope) core.Node {
	a := s.a
	submit := live.Do(a, a.SubmitEdit)
	cancel := live.Do(a, a.CancelEdit)
	cx.OnKeyWith("escape", core.KeyOptions{When: core.KeyAlways}, cancel)
	field := func(id string, label func() string, value func() *string, focus bool) core.Node {
		return core.View{Style: core.Style{Gap: 4}, Children: []core.Node{
			core.Text{ContentFunc: label, Style: core.Style{FontSize: 11, LineHeight: 14, FontWeight: core.FontWeightSemibold, Color: core.RGBA(0xFFFFFF, 0.6)}},
			base.Input{TestID: id, LabelFunc: label, AutoFocus: focus,
				Value:          live.Func(a, func() string { return *value() }),
				OnChange:       live.On(a, func(text string) { *value() = text }),
				SubmitBehavior: core.SubmitBehaviorSubmit,
				OnSubmit:       func(string) { submit() },
				Style: core.Style{Height: core.Pt(36), Padding: core.Symmetric(0, 12), CornerRadius: 10, Background: core.RGBA(0xFFFFFF, 0.12), FontSize: 16,
					BorderWidth: 1, BorderColor: core.RGBA(0xFFFFFF, 0.18), Color: onGlass, CaretColor: onGlass, PlaceholderColor: core.RGBA(0xFFFFFF, 0.45)}},
		}}
	}
	buttons := core.View{Style: core.Style{Direction: core.Row, Gap: 8, JustifyContent: core.JustifyEnd}, Children: []core.Node{
		ui.Button{TestID: "film-correct-cancel", TextFunc: func() string { return locales.CorrectCancel(cx) }, Variant: ui.ButtonSecondary, Size: ui.SizeSmall, OnPress: cancel},
		ui.Button{TestID: "film-correct-submit", TextFunc: func() string { return locales.CorrectSubmit(cx) }, Size: ui.SizeSmall, OnPress: submit},
	}}
	return core.View{TestID: "film-correct-editor", Role: core.RoleGroup, LabelFunc: func() string { return locales.CorrectKeyEdit(cx) },
		Style: correctGlass.style().Merge(core.Style{Width: core.Pct(100), MaxWidth: core.Pt(660), Gap: 8, Padding: core.Symmetric(12, 14), CornerRadius: 18}),
		Children: []core.Node{
			field("film-correct-translation", func() string { return locales.CorrectTranslation(cx) }, a.EditTranslation, true),
			field("film-correct-original", func() string { return locales.CorrectOriginal(cx) }, a.EditOriginal, false),
			buttons,
		}}
}

// barButton is a button of the selection bar that says what it does, its tooltip naming the key that does the same, when one does.
func barButton(cx *core.Scope, label func() string, key, id string, press func()) core.Node {
	tooltip := label
	if key != "" {
		tooltip = withKey(cx, label, key)
	}
	return base.Button{TestID: id, LabelFunc: label, Tooltip: tip(cx, tooltip),
		Desktop: core.DesktopViewProps{WindowDrag: core.WindowDragOff},
		OnPress: func(core.PressEvent) { press() },
		Style: core.Style{Height: core.Pt(28), Padding: core.Symmetric(0, 10), FlexShrink: 0, AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter, CornerRadius: 10,
			Hover: &core.Style{Background: core.RGBA(0xFFFFFF, 0.12)}, Active: &core.Style{Background: core.RGBA(0xFFFFFF, 0.2)}, FocusVisible: ui.FocusRing(cx, -2)},
		Children: []core.Node{core.Text{ContentFunc: label, PointerEvents: core.PointerEventsNone, Style: core.Style{FontSize: 12, LineHeight: 16, Color: onGlass}}}}
}

// barGlyph is a small button of the selection bar showing glyph, named label, its tooltip naming the key that does the same.
func barGlyph(cx *core.Scope, glyph string, label func() string, key, id string, press func()) core.Node {
	return base.Button{TestID: id, LabelFunc: label, Tooltip: tip(cx, withKey(cx, label, key)),
		Desktop: core.DesktopViewProps{WindowDrag: core.WindowDragOff},
		OnPress: func(core.PressEvent) { press() },
		Style: core.Style{Width: core.Pt(28), Height: core.Pt(28), FlexShrink: 0, AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter, CornerRadius: 10,
			Hover: &core.Style{Background: core.RGBA(0xFFFFFF, 0.12)}, Active: &core.Style{Background: core.RGBA(0xFFFFFF, 0.2)}, FocusVisible: ui.FocusRing(cx, -2)},
		Children: []core.Node{core.Text{Content: glyph, Role: core.RoleNone, PointerEvents: core.PointerEventsNone, Style: core.Style{FontSize: 15, LineHeight: 18, Color: onGlass}}}}
}
