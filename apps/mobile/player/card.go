package player

import (
	"kando/apps/mobile/locales"
	"kando/apps/mobile/look"
	"kando/apps/mobile/theme"
	kcore "kando/packages/core"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	"github.com/nsgo-dev/nsgo/ui"
)

// hiddenBlur is how far the card blurs a line the mode hides.
const hiddenBlur = 7

// card is the card of the line playing, or the last one heard: its place among the film's lines and its time, a hint, its original, whose words a tap looks up, and its translation, each blurred while the mode hides it until a tap shows it; then the actions on it: play it again, loop it, save it and have it explained.
func card(cx *core.Scope, p *play, w *words) core.Node {
	colors := ui.ColorsOf(cx)
	i := func() int { return p.shown.Get() }
	hint := func() string {
		if p.hides(true) || p.hides(false) {
			return locales.PlayerHintReveal(cx)
		}
		return locales.PlayerHintWords(cx)
	}
	hidden := func(original bool) func() core.Style {
		return func() core.Style {
			if p.hides(original) {
				return core.Style{Filters: []core.Filter{core.Blur(hiddenBlur)}, Opacity: core.Alpha(0.7)}
			}
			return core.Style{}
		}
	}
	reveal := func(core.PressEvent) { p.revealed.Set(p.shown.GetUntracked()) }
	original := core.Text{TestID: "player-card-original", PointerEvents: core.PointerEventsAuto, Style: core.Style{FontSize: 20, LineHeight: 27, FontWeight: core.FontWeightSemibold},
		StyleFunc: func() core.Style { return core.Style{Color: colors.Text()}.Merge(hidden(true)()) },
		SpansFunc: func() []core.Span {
			text := p.line(i()).Original
			if p.hides(true) {
				return []core.Span{{Content: text}}
			}
			var spans []core.Span
			for _, tok := range kcore.WordTokens(text) {
				sp := core.Span{Content: tok.Text}
				if tok.Word {
					word, line := tok.Text, i()
					sp.OnPress = func(core.PressEvent) { w.look(line, word) }
					sp.Style = core.Style{TextDecoration: core.TextDecorationUnderline, TextDecorationColor: theme.SubtitleYellow.Faded(0.55)}
				}
				spans = append(spans, sp)
			}
			return spans
		},
		LabelFunc: func() string {
			if p.hides(true) {
				return locales.PlayerHiddenLine(cx)
			}
			return p.line(i()).Original
		},
		OnPress: func(e core.PressEvent) {
			if p.hides(true) {
				reveal(e)
			}
		}}
	translation := core.Text{TestID: "player-card-translation", Style: look.Body,
		ContentFunc: func() string { return p.line(i()).Translation },
		StyleFunc:   func() core.Style { return core.Style{Color: colors.Muted()}.Merge(hidden(false)()) },
		LabelFunc: func() string {
			if p.hides(false) {
				return locales.PlayerHiddenLine(cx)
			}
			return p.line(i()).Translation
		},
		OnPress: func(e core.PressEvent) {
			if p.hides(false) {
				reveal(e)
			}
		}}
	body := core.Show{When: func() bool { return i() >= 0 },
		Then: func(cx *core.Scope) core.Node {
			return core.View{Style: core.Style{Gap: 6}, Children: []core.Node{
				core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 8}, Children: []core.Node{
					ui.Tabular(core.Text{TestID: "player-card-place", ContentFunc: func() string {
						return locales.PlayerCardPlace(cx, i()+1, len(p.list()), clock(p.line(i()).Start))
					}, MaxLines: 1, Style: look.Caption.Merge(core.Style{FlexShrink: 1}), StyleFunc: ui.Colored(colors.Muted)}),
					core.View{Style: core.Style{FlexGrow: 1}},
					core.Text{ContentFunc: hint, MaxLines: 1, Style: look.Caption.Merge(core.Style{FlexShrink: 1}), StyleFunc: ui.Colored(colors.Faint)},
				}},
				original,
				core.Show{When: func() bool { return p.line(i()).Translation != "" }, Then: func(*core.Scope) core.Node { return translation }},
			}}
		},
		Else: func(cx *core.Scope) core.Node {
			return core.Text{TestID: "player-card-empty", ContentFunc: func() string { return locales.PlayerNoLines(cx) }, Style: look.Body, StyleFunc: ui.Colored(colors.Muted)}
		}}
	return core.View{TestID: "player-card", Style: core.Style{Gap: 12, Padding: core.All(14), CornerRadius: 16, BorderCurve: core.BorderCurveContinuous},
		StyleFunc: func() core.Style { return core.Style{Background: ui.Alpha(colors.Text(), 0.06)} },
		Children:  []core.Node{body, actions(cx, p, w)}}
}

// actions are the card's actions on its line: play it again, loop it, save it and have it explained.
func actions(cx *core.Scope, p *play, w *words) core.Node {
	colors := ui.ColorsOf(cx)
	none := func() bool { return p.shown.Get() < 0 }
	action := func(id string, icon func() *core.SVG, text func() string, on func() bool, tone func() core.Color, press func()) core.Node {
		ink := func() core.Color {
			if on != nil && on() {
				return tone()
			}
			return colors.Text()
		}
		var selected func() bool
		if on != nil {
			selected = on
		}
		return base.Button{TestID: id, LabelFunc: text, Selected: selected, Disabled: none, OnPress: func(core.PressEvent) { press() },
			Style: core.Style{FlexGrow: 1, FlexBasis: core.Pt(0), MinWidth: core.Pt(0), MinHeight: core.Pt(52), Gap: 3, CornerRadius: 12,
				BorderCurve: core.BorderCurveContinuous, AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter, Padding: core.Symmetric(6, 4)},
			StyleFunc: func() core.Style {
				st := core.Style{Color: ink(), Background: ui.Alpha(colors.Text(), 0.06), Active: &core.Style{Opacity: core.Alpha(0.7)}, Disabled: &core.Style{Opacity: core.Alpha(0.4)}}
				if on != nil && on() {
					st.Background = tone().Faded(0.18)
				}
				return st
			},
			Children: []core.Node{
				ui.IconOf(icon, 18, ink),
				core.Text{ContentFunc: text, MaxLines: 1, Style: look.Caption.Merge(core.Style{FontWeight: core.FontWeightSemibold})},
			}}
	}
	accent := func() core.Color { return colors.Accent() }
	yellow := func() core.Color { return theme.SubtitleYellow }
	return core.View{TestID: "player-actions", Style: core.Style{Direction: core.Row, Gap: 8}, Children: []core.Node{
		action("player-replay", lucide.RotateCcw, func() string { return locales.PlayerReplay(cx) }, nil, accent, p.replay),
		action("player-card-loop", lucide.Repeat1, func() string { return locales.PlayerLoopLine(cx) }, func() bool { return p.loop.Get() >= 0 }, accent, p.toggleLoop),
		action("player-save", lucide.Star, func() string {
			if p.isSaved(p.shown.Get()) {
				return locales.PlayerSaved(cx)
			}
			return locales.PlayerSave(cx)
		}, func() bool { return p.isSaved(p.shown.Get()) }, yellow, p.toggleSave),
		action("player-explain-line", lucide.Sparkles, func() string { return locales.PlayerExplain(cx) }, nil, func() core.Color { return theme.AI }, func() { w.explainLine(p.shown.GetUntracked()) }),
	}}
}
