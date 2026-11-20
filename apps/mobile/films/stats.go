package films

import (
	"strconv"
	"time"

	"kando/apps/mobile/learning"
	"kando/apps/mobile/locales"
	"kando/apps/mobile/look"
	"kando/apps/mobile/theme"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

// stats are the learner's day at a glance, the three tiles at the top of the tab: the minutes of films played today, the days in a row, and the words in the vocabulary, all of the phone's study as the Learn tab counts it (package learning), every computer's libraries together.
func (v *view) stats(cx *core.Scope) core.Node {
	colors := ui.ColorsOf(cx)
	s := v.s
	minutes := func() int { return int(s.Watched().Day(s.Now()) / time.Minute) }
	streak := func() int { return s.Watched().Streak(s.Now()) }
	// The vocabulary is counted again only as the learning state changes, not at every tick of the minutes watched.
	words := core.NewMemo(cx, func() int {
		n := 0
		for _, it := range s.LearnItems() {
			if it.Kind == learning.Word {
				n++
			}
		}
		return n
	})
	// The minutes are in the accent, the days in the streak's orange and the words in the stats' green, the design's colors for them. Each tile's number is drawn large with its unit after it, small; the screen reader hears them as one, with the label.
	tiles := []core.Node{
		statTile(cx, "films-stat-today", minutes, func() string { return locales.FilmsStatUnitMinutes(cx, minutes()) }, func() string { return locales.LearnMinutes(cx, minutes()) }, func() string { return locales.FilmsStatToday(cx) }, colors.Accent),
		statTile(cx, "films-stat-streak", streak, func() string { return locales.FilmsStatUnitDays(cx, streak()) }, func() string { return locales.FilmsStatDays(cx, streak()) }, func() string { return locales.FilmsStatStreak(cx) }, fixed(theme.Streak)),
		statTile(cx, "films-stat-words", words.Get, func() string { return locales.FilmsStatUnitWords(cx, words.Get()) }, func() string { return locales.FilmsStatWords(cx, words.Get()) }, func() string { return locales.FilmsStatVocabulary(cx) }, fixed(theme.StatGreen)),
	}
	gap := core.Points(8)
	if look.Tablet(cx) {
		// A tablet's fourth tile is the jobs card's place (critic U2).
		gap = 12
		tiles = append(tiles, core.Show{When: v.jobsShown, Then: v.jobsTile, Else: func(*core.Scope) core.Node {
			return core.View{Style: core.Style{FlexGrow: 1, FlexBasis: core.Pt(0)}}
		}})
	}
	return core.View{TestID: "films-stats", Style: core.Style{Direction: core.Row, Gap: gap}, Children: tiles}
}

// statTile is one of the stats: its number with its unit, over what it counts, read as one by assistive technology (spoken, the number with its unit as words).
func statTile(cx *core.Scope, testID string, number func() int, unit, spoken, label func() string, tone func() core.Color) core.Node {
	colors := ui.ColorsOf(cx)
	return core.View{TestID: testID, Role: core.RoleText, LabelFunc: func() string { return label() + ", " + spoken() },
		Style:     core.Style{FlexGrow: 1, FlexBasis: core.Pt(0), MinWidth: core.Pt(0), Padding: core.Symmetric(10, 12)},
		StyleFunc: func() core.Style { return look.Card(cx) },
		Children: []core.Node{core.View{AccessibilityElementsHidden: func() bool { return true }, Style: core.Style{Gap: 1}, Children: []core.Node{
			core.View{TestID: testID + "-value", Style: core.Style{Direction: core.Row, AlignItems: core.AlignEnd, Gap: 2}, Children: []core.Node{
				ui.Tabular(core.Text{ContentFunc: func() string { return strconv.Itoa(number()) }, MaxLines: 1, Style: core.Style{FontSize: 20, LineHeight: 24, FontWeight: look.Heavy, LetterSpacing: -0.4}, StyleFunc: ui.Colored(tone)}),
				core.Text{ContentFunc: unit, MaxLines: 1, Style: core.Style{FontSize: 12, LineHeight: 15, FontWeight: core.FontWeightSemibold, FlexShrink: 1}, StyleFunc: ui.Colored(tone)},
			}},
			core.Text{ContentFunc: label, MaxLines: 1, Style: look.Footnote, StyleFunc: ui.Colored(colors.Muted)},
		}}}}
}

// fixed is c as a color binding, for a color the same in both appearances.
func fixed(c core.Color) func() core.Color { return func() core.Color { return c } }
