package film

import (
	"strconv"

	"kando/apps/kando/live"
	"kando/apps/kando/locales"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

// candidateGlass is the glass of the list of other translations, a little denser than the selection bar's (prototype template 599).
var candidateGlass = glassSpec{fill: core.RGBA(0x1C1C20, 0.82), blur: 28, ring: 0.2, radius: 18}

// candidateList is the AI service's other translations of the selected line (这句不对): a heading, the line's original, then the line as it is and each translation, the one highlighted ringed, and a line naming the keys. A click keeps a translation, as ↑↓ and Return do (the app's keys). While the AI service is asked the line as it is shows alone; when no translation came, why, with the AI service's settings when they are the fix.
func (s *screen) candidateList(cx *core.Scope) core.Node {
	a := s.a
	c := live.Func(a, a.Candidates)
	options := live.Func(a, func() []int {
		cand := a.Candidates()
		if cand.Busy || cand.Say != nil {
			return []int{0}
		}
		out := make([]int, len(cand.Options)+1)
		for i := range out {
			out[i] = i
		}
		return out
	})
	heading := func() string {
		if c().Busy {
			return locales.CorrectCandidatesBusy(cx)
		}
		return locales.CorrectCandidatesTitle(cx)
	}
	small := core.Style{FontSize: 12, LineHeight: 16, Color: core.RGBA(0xFFFFFF, 0.75)}
	row := func(cx *core.Scope, i func() int, _ func() int) core.Node {
		text := func() string {
			cand, k := c(), i()
			if k == 0 {
				if cand.Current == "" {
					return locales.CorrectUntranslated(cx)
				}
				return cand.Current
			}
			if k-1 < len(cand.Options) {
				return cand.Options[k-1]
			}
			return ""
		}
		label := func() string {
			if i() == 0 {
				return locales.CorrectCandidatesCurrent(cx)
			}
			return locales.CorrectCandidatesOption(cx)
		}
		on := func() bool { cand := c(); return cand.At == i() && !cand.Busy && cand.Say == nil }
		return base.Button{TestID: "film-candidate-" + strconv.Itoa(i()), Role: core.RoleOption, LabelFunc: func() string { return label() + " " + text() },
			Selected: on,
			Desktop:  core.DesktopViewProps{WindowDrag: core.WindowDragOff},
			OnPress:  live.On(a, func(core.PressEvent) { a.PickCandidate(i()) }),
			StyleFunc: func() core.Style {
				st := core.Style{Gap: 3, Padding: core.Symmetric(9, 12), CornerRadius: 12, Background: core.RGBA(0xFFFFFF, 0.05),
					Hover: &core.Style{Background: core.RGBA(0xFFFFFF, 0.1)}, FocusVisible: ui.FocusRing(cx, -2)}
				if on() {
					st.Background = core.RGBA(0xFFFFFF, 0.16)
					st.Shadows = []core.BoxShadow{{Spread: 1.5, Color: onGlass, Inset: true}}
				}
				return st
			},
			Children: []core.Node{
				core.Text{ContentFunc: label, PointerEvents: core.PointerEventsNone, Style: core.Style{FontSize: 11, LineHeight: 14, Color: core.RGBA(0xFFFFFF, 0.7)}},
				core.Text{ContentFunc: text, PointerEvents: core.PointerEventsNone, Style: core.Style{FontSize: 16, LineHeight: 22, Color: onGlass}},
			}}
	}
	return core.View{TestID: "film-candidates", Role: core.RoleListbox, LabelFunc: func() string { return locales.CorrectCandidatesLabel(cx) },
		Style: candidateGlass.style().Merge(core.Style{Width: core.Pct(100), MaxWidth: core.Pt(600), Gap: 6, Padding: core.All(12)}),
		Children: []core.Node{
			core.View{Style: core.Style{Direction: core.Row, Gap: 8, AlignItems: core.AlignCenter, Padding: core.Edges{Left: 4, Right: 4, Bottom: 2}}, Children: []core.Node{
				core.Text{TestID: "film-candidates-heading", ContentFunc: heading, Style: small.Merge(core.Style{FlexGrow: 1, FlexShrink: 1})},
				core.Text{ContentFunc: func() string { return c().Original }, Style: small.Merge(core.Style{FlexShrink: 1})},
			}},
			core.Each(options, func(i int) int { return i }, func(cx *core.Scope, item func() int, index func() int) core.Node { return row(cx, item, index) }),
			core.Show{When: func() bool { return c().Say != nil }, Then: func(cx *core.Scope) core.Node {
				return core.View{Style: core.Style{Direction: core.Row, Gap: 8, AlignItems: core.AlignCenter, Padding: core.Symmetric(2, 4)}, Children: []core.Node{
					core.Text{TestID: "film-candidates-say", ContentFunc: func() string {
						if say := c().Say; say != nil {
							return say(cx)
						}
						return ""
					}, Style: core.Style{FontSize: 13, LineHeight: 18, Color: onGlass, FlexGrow: 1, FlexShrink: 1}},
					core.Show{When: func() bool { return c().NeedsAI }, Then: func(cx *core.Scope) core.Node {
						return barButton(cx, func() string { return locales.CorrectCandidatesSettings(cx) }, "", "film-candidates-settings", live.Do(a, a.CandidateSettings))
					}},
				}}
			}},
			core.Text{ContentFunc: func() string {
				return locales.CorrectCandidatesHint(cx, cx.KeyLabel("up")+cx.KeyLabel("down"), cx.KeyLabel("enter"), cx.KeyLabel("escape"))
			}, Style: core.Style{FontSize: 11, LineHeight: 14, Color: core.RGBA(0xFFFFFF, 0.6), Padding: core.Edges{Left: 4, Right: 4, Top: 2}}},
		}}
}

// spreadBar asks whether to make the fix an edit made in the film's other lines that hold it: in all of them (⌘↩), showing them in the subtitle list first (⇧⌘↩), or in the edited line alone (Escape).
func (s *screen) spreadBar(cx *core.Scope) core.Node {
	a := s.a
	sp := live.Func(a, a.Spread)
	text := func() string {
		v := sp()
		if v.Edited > 0 {
			return locales.CorrectSpreadAskEdited(cx, v.From, v.To, v.Lines, v.Edited)
		}
		return locales.CorrectSpreadAsk(cx, v.From, v.To, v.Lines)
	}
	return core.View{TestID: "film-spread", Role: core.RoleStatus,
		Style: correctGlass.style().Merge(core.Style{Direction: core.Row, Wrap: core.Wrap, AlignItems: core.AlignCenter, Gap: 8, Padding: core.Edges{Left: 14, Right: 8, Top: 8, Bottom: 8}, MaxWidth: core.Pt(680)}),
		Children: []core.Node{
			core.Text{TestID: "film-spread-text", ContentFunc: text, Style: core.Style{FontSize: 13, LineHeight: 18, Color: onGlass, FlexGrow: 1, FlexShrink: 1, FlexBasis: core.Pt(240)}},
			barButton(cx, func() string { return locales.CorrectSpreadAll(cx) }, "mod+enter", "film-spread-all", live.Do(a, a.SpreadAll)),
			barButton(cx, func() string { return locales.CorrectSpreadView(cx) }, "mod+shift+enter", "film-spread-view", live.Do(a, a.SpreadView)),
			barButton(cx, func() string { return locales.CorrectSpreadOnlyButton(cx) }, "escape", "film-spread-only", live.Do(a, a.SpreadOnly)),
		}}
}
