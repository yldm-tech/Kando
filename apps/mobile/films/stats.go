package films

import (
	"time"

	"kando/apps/mobile/learning"
	"kando/apps/mobile/locales"
	"kando/apps/mobile/look"

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
	// The minutes and the days take the colors the Learn tab gives them, the words the color of something gained.
	return core.View{TestID: "films-stats", Style: core.Style{Direction: core.Row, Gap: 8}, Children: []core.Node{
		statTile(cx, "films-stat-today", func() string { return locales.LearnMinutes(cx, minutes()) }, func() string { return locales.FilmsStatToday(cx) }, colors.Accent),
		statTile(cx, "films-stat-streak", func() string { return locales.FilmsStatDays(cx, streak()) }, func() string { return locales.FilmsStatStreak(cx) }, colors.Warn),
		statTile(cx, "films-stat-words", func() string { return locales.FilmsStatWords(cx, words.Get()) }, func() string { return locales.FilmsStatVocabulary(cx) }, colors.Up),
	}}
}

// statTile is one of the stats: its value, with its unit, over what it counts, read as one by assistive technology.
func statTile(cx *core.Scope, testID string, value, label func() string, tone func() core.Color) core.Node {
	colors := ui.ColorsOf(cx)
	return core.View{TestID: testID, Role: core.RoleText, LabelFunc: func() string { return label() + ", " + value() },
		Style:     core.Style{FlexGrow: 1, FlexBasis: core.Pt(0), MinWidth: core.Pt(0), Padding: core.Symmetric(10, 12), CornerRadius: 12, BorderCurve: core.BorderCurveContinuous},
		StyleFunc: func() core.Style { return core.Style{Background: colors.Surface()} },
		Children: []core.Node{core.View{AccessibilityElementsHidden: func() bool { return true }, Style: core.Style{Gap: 2}, Children: []core.Node{
			ui.Tabular(core.Text{TestID: testID + "-value", ContentFunc: value, MaxLines: 1, Style: look.Heading.Merge(core.Style{FontWeight: core.FontWeightBold}), StyleFunc: ui.Colored(tone)}),
			core.Text{ContentFunc: label, MaxLines: 1, Style: look.Caption, StyleFunc: ui.Colored(colors.Muted)},
		}}}}
}
