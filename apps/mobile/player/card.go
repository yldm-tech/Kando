package player

import (
	"kando/apps/mobile/locales"
	"kando/apps/mobile/look"
	"kando/apps/mobile/theme"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	"github.com/nsgo-dev/nsgo/ui"
)

// hiddenBlur is how far the card blurs a line the mode hides.
const hiddenBlur = 7

// card is the card of the line playing, or the last one heard: its place among the film's lines and its time, a hint, its original, whose words a tap looks up and puts in the vocabulary, the word looked up and those in the vocabulary marked, and its translation, with a chip once the learner fixed it, each blurred while the mode hides it until a tap shows it; then the actions on it: play it again, loop it, save it and have it explained.
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
	tone := toneOf(colors)
	original := core.Text{TestID: "player-card-original", PointerEvents: core.PointerEventsAuto, Style: core.Style{FontSize: 20, LineHeight: 28, FontWeight: core.FontWeightSemibold},
		StyleFunc: func() core.Style { return core.Style{Color: colors.Text()}.Merge(hidden(true)()) },
		SpansFunc: func() []core.Span {
			text := p.line(i()).Original
			if p.hides(true) {
				return []core.Span{{Content: text}}
			}
			line := i()
			ink := markInk{style: core.Style{FontSize: 20, FontWeight: core.FontWeightSemibold, Color: colors.Text()}, look: func(word string) { w.look(line, word) }, tone: tone,
				picked: w.picked(line), band: true}
			return markSpans(text, p.tokens(text), p.lineMarks(line), ink)
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
	translation := core.Text{TestID: "player-card-translation", Style: core.Style{FontSize: 15, LineHeight: 22, FlexShrink: 1},
		ContentFunc: func() string { return p.line(i()).Translation },
		StyleFunc:   func() core.Style { return core.Style{Color: ui.Alpha(colors.Text(), 0.72)}.Merge(hidden(false)()) },
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
	faint := func() core.Style { return core.Style{Color: ui.Alpha(colors.Text(), 0.55)} }
	body := core.Show{When: func() bool { return i() >= 0 },
		Then: func(cx *core.Scope) core.Node {
			return core.View{Style: core.Style{Gap: 6}, Children: []core.Node{
				core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignStart, Gap: 12}, Children: []core.Node{
					core.Text{TestID: "player-card-place", ContentFunc: func() string {
						return locales.PlayerCardPlace(cx, i()+1, len(p.list()), clock(p.line(i()).Start))
					}, MaxLines: 1, Style: mono.Merge(core.Style{FontSize: 12, LineHeight: 16, FlexShrink: 0}), StyleFunc: faint},
					// The hint takes what the place leaves on the row, one line of it, cut short where there is not room for it all: the place, as 第 1 / 1,236 句 · 0:54, keeps to one line whole.
					core.Text{TestID: "player-card-hint", ContentFunc: hint, MaxLines: 1, Style: core.Style{FontSize: 12, LineHeight: 16, FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0)}, StyleFunc: faint},
				}},
				original,
				core.Show{When: func() bool { return p.line(i()).Translation != "" }, Then: func(cx *core.Scope) core.Node {
					return core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Wrap: core.Wrap, Gap: 6}, Children: []core.Node{
						translation,
						core.Show{When: func() bool { return p.fixed(i()) && !p.hides(false) }, Then: func(cx *core.Scope) core.Node { return fixedChip(cx) }},
					}}
				}},
			}}
		},
		Else: func(cx *core.Scope) core.Node {
			return core.Text{TestID: "player-card-empty", ContentFunc: func() string { return locales.PlayerNoLines(cx) }, Style: look.Body, StyleFunc: ui.Colored(colors.Muted)}
		}}
	return core.View{TestID: "player-card", Style: core.Style{Gap: 12, Padding: core.Edges{Top: 14, Right: 16, Bottom: 12, Left: 16}, CornerRadius: 20, BorderCurve: core.BorderCurveContinuous, BorderWidth: 0.5},
		StyleFunc: func() core.Style {
			return core.Style{Background: ui.Alpha(colors.Text(), 0.08), BorderColor: ui.Alpha(colors.Text(), 0.1)}
		},
		Children: []core.Node{body, actions(cx, p, w)}}
}

// fixedChip says the translation beside it is the learner's own fix.
func fixedChip(cx *core.Scope) core.Node {
	return core.View{TestID: "player-card-fixed", Style: core.Style{Padding: core.Symmetric(1, 6), CornerRadius: 6, Background: theme.StatusOK.Faded(0.2)}, Children: []core.Node{
		core.Text{ContentFunc: func() string { return locales.PlayerFixedChip(cx) }, MaxLines: 1, Style: core.Style{FontSize: 11, LineHeight: 14, FontWeight: core.FontWeightBold, Color: theme.FixedInk}},
	}}
}

// actions are the card's actions on its line: play it again, loop it, save it and have it explained, each a small capsule with its icon beside its name.
func actions(cx *core.Scope, p *play, w *words) core.Node {
	colors := ui.ColorsOf(cx)
	none := func() bool { return p.shown.Get() < 0 }
	white := func() core.Color { return theme.PlayerInk }
	// action is a button whose fill and ink follow look, which on reports as selected.
	action := func(id string, icon func() *core.SVG, text func() string, on func() bool, look func() (fill, ink core.Color), press func()) core.Node {
		ink := func() core.Color { _, c := look(); return c }
		return base.Button{TestID: id, LabelFunc: text, Selected: on, Disabled: none, OnPress: func(core.PressEvent) { press() },
			Style: core.Style{FlexGrow: 1, FlexBasis: core.Pt(0), MinWidth: core.Pt(0), MinHeight: core.Pt(34), Direction: core.Row, Gap: 5, CornerRadius: 12,
				BorderCurve: core.BorderCurveContinuous, AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter, Padding: core.Symmetric(0, 6)},
			StyleFunc: func() core.Style {
				fill, c := look()
				return core.Style{Color: c, Background: fill, Active: &core.Style{Opacity: core.Alpha(0.7)}, Disabled: &core.Style{Opacity: core.Alpha(0.4)}}
			},
			Children: []core.Node{
				ui.IconOf(icon, 14, ink),
				core.Text{ContentFunc: text, MaxLines: 1, Style: core.Style{FontSize: 12, LineHeight: 16, FontWeight: core.FontWeightBold, FlexShrink: 1}},
			}}
	}
	plain := func() (core.Color, core.Color) { return ui.Alpha(colors.Text(), 0.12), colors.Text() }
	saved := func() bool { return p.isSaved(p.shown.Get()) }
	looped := func() bool { return p.loop.Get() >= 0 }
	return core.View{TestID: "player-actions", Style: core.Style{Direction: core.Row, Gap: 6}, Children: []core.Node{
		action("player-replay", lucide.RotateCcw, func() string { return locales.PlayerReplay(cx) }, nil, plain, p.replay),
		action("player-card-loop", lucide.Repeat, func() string { return locales.PlayerLoopLine(cx) }, looped, func() (core.Color, core.Color) {
			if looped() {
				return theme.PlayerBlue, white()
			}
			return plain()
		}, p.toggleLoop),
		action("player-save", func() *core.SVG {
			if saved() {
				return starFilled()
			}
			return lucide.Star()
		}, func() string {
			if saved() {
				return locales.PlayerSaved(cx)
			}
			return locales.PlayerSave(cx)
		}, saved, func() (core.Color, core.Color) {
			if saved() {
				return theme.SubtitleYellow.Faded(0.18), theme.SubtitleYellow
			}
			return plain()
		}, p.toggleSave),
		action("player-explain-line", sparkleFilled, func() string { return locales.PlayerExplain(cx) }, nil, func() (core.Color, core.Color) { return theme.AI.Faded(0.35), white() },
			func() { w.explainLine(p.shown.GetUntracked()) }),
	}}
}
