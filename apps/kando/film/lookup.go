package film

import (
	"kando/apps/kando/live"
	"kando/apps/kando/locales"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

// lookupGlass is the glass of the card of a word looked up, as the selection bar's.
var lookupGlass = glassSpec{fill: core.RGBA(0x1C1C20, 0.86), blur: 24, ring: 0.2, radius: 16}

// lookupCard is the card of the word looked up on the picture (App.Lookup), under the title bar in the middle of the room the side panel leaves, clear of the lines it was clicked in: the word, how it is read and the × that closes it, what it means in the line, or that the AI service is asked, or why no meaning came, and the line itself; then Add to Vocabulary, In Vocabulary once it is there, which takes it out again, and Explain Line, or Open Settings when no AI service can answer. Escape closes it.
func (s *screen) lookupCard() core.Node {
	a, cx := s.a, s.cx
	open := live.Memo(cx, a, func() bool { return a.Lookup().Open })
	return core.Show{When: open, Then: func(cx *core.Scope) core.Node {
		l := live.Func(a, a.Lookup)
		cx.OnKeyWith("escape", core.KeyOptions{When: core.KeyAlways}, live.Do(a, a.CloseLookup))
		muted := core.RGBA(0xFFFFFF, 0.6)
		ready := func() bool { x := l(); return !x.Busy && !x.Failed }
		say := func() string {
			x := l()
			switch {
			case x.Busy:
				return locales.VocabLookupBusy(cx)
			case x.Failed && x.Say != nil:
				return x.Say(cx)
			}
			return x.Meaning
		}
		head := core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 8}, Children: []core.Node{
			core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0), Direction: core.Row, Wrap: core.Wrap, AlignItems: core.AlignBaseline, Gap: 8}, Children: []core.Node{
				core.Text{TestID: "film-lookup-word", ContentFunc: func() string { return l().Word }, Role: core.RoleHeader, Selectable: core.SelectableOn,
					Style: core.Style{FontSize: 20, LineHeight: 26, FontWeight: core.FontWeightSemibold, Color: onGlass}},
				core.Show{When: func() bool { return l().Reading != "" }, Then: func(*core.Scope) core.Node {
					return core.Text{TestID: "film-lookup-reading", ContentFunc: func() string { return l().Reading }, Style: core.Style{FontSize: 13, LineHeight: 18, Color: muted}}
				}},
			}},
			barGlyph(cx, "×", func() string { return locales.VocabLookupClose(cx) }, "escape", "film-lookup-close", live.Do(a, a.CloseLookup)),
		}}
		meaning := core.Text{TestID: "film-lookup-meaning", ContentFunc: say, Selectable: core.SelectableOn, LiveRegion: core.LiveRegionPolite,
			StyleFunc: func() core.Style {
				st := core.Style{FontSize: 14, LineHeight: 20, Color: onGlass}
				if !ready() {
					st.Color = muted
				}
				return st
			}}
		line := core.Text{TestID: "film-lookup-line", ContentFunc: func() string { return l().Line }, Selectable: core.SelectableOn,
			Style: core.Style{FontSize: 12, LineHeight: 17, Color: muted, FontStyle: core.FontStyleItalic}}
		saveLabel := func() string {
			if l().Saved {
				return locales.VocabInVocabulary(cx)
			}
			return locales.VocabAddToVocabulary(cx)
		}
		buttons := core.View{Style: core.Style{Direction: core.Row, Wrap: core.Wrap, Gap: 6, JustifyContent: core.JustifyEnd}, Children: []core.Node{
			core.Show{When: ready, Then: func(cx *core.Scope) core.Node {
				return lookupButton(cx, saveLabel, "film-lookup-save", live.Do(a, a.ToggleLookupWord), func() bool { return l().Saved })
			}},
			core.Show{When: func() bool { return !l().NeedsAI }, Then: func(cx *core.Scope) core.Node {
				return lookupButton(cx, func() string { return locales.FilmExplainLine(cx) }, "film-lookup-explain", live.Do(a, a.ExplainLookupLine), nil)
			}},
			core.Show{When: func() bool { return l().NeedsAI }, Then: func(cx *core.Scope) core.Node {
				return lookupButton(cx, func() string { return locales.VocabLookupSettings(cx) }, "film-lookup-settings", live.Do(a, a.LookupSettings), nil)
			}},
		}}
		card := core.View{TestID: "film-lookup", Role: core.RoleGroup, LabelFunc: func() string { return locales.VocabLookupCard(cx, l().Word) },
			Style:    lookupGlass.style().Merge(core.Style{Width: core.Pct(100), MaxWidth: core.Pt(420), Gap: 8, Padding: core.Symmetric(12, 14)}),
			Children: []core.Node{head, meaning, line, buttons}}
		return core.View{PointerEvents: core.PointerEventsBoxNone,
			Style: core.Style{Position: core.Absolute, Left: core.Pt(20), Top: core.Pt(72), Direction: core.Row, JustifyContent: core.JustifyCenter},
			StyleFunc: func() core.Style {
				return core.Style{Right: core.Pt(s.roomRight() + 20), Transition: roomTransition}
			},
			Children: []core.Node{card}}
	}}
}

// lookupButton is a button of the lookup card that says what it does; with on it shows as pressed while on holds.
func lookupButton(cx *core.Scope, label func() string, id string, press func(), on func() bool) core.Node {
	b := base.Button{TestID: id, LabelFunc: label,
		Desktop: core.DesktopViewProps{WindowDrag: core.WindowDragOff},
		OnPress: func(core.PressEvent) { press() },
		Style: core.Style{Height: core.Pt(28), Padding: core.Symmetric(0, 12), FlexShrink: 0, AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter, CornerRadius: 10,
			Background: core.RGBA(0xFFFFFF, 0.1),
			Hover:      &core.Style{Background: core.RGBA(0xFFFFFF, 0.18)}, Active: &core.Style{Background: core.RGBA(0xFFFFFF, 0.24)}, Selected: &core.Style{Background: core.RGBA(0x0A84FF, 0.55)}, FocusVisible: ui.FocusRing(cx, -2)},
		Children: []core.Node{core.Text{ContentFunc: label, PointerEvents: core.PointerEventsNone, Style: core.Style{FontSize: 12, LineHeight: 16, Color: onGlass}}}}
	if on != nil {
		b.Selected = on
	}
	return b
}
