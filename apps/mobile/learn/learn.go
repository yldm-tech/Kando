// Package learn is the Learn tab: the day's goal and the streak, the review of the words and lines due, the week's study, the vocabulary and the saved lines, and their export through the share sheet. The words and lines are the ones saved on the computers' libraries and on the phone, which the session keeps on the phone (package learning), so the tab works away from the computer; what the phone changes the computers take at the next sync.
package learn

import (
	"time"

	"kando/apps/mobile/learning"
	"kando/apps/mobile/locales"
	"kando/apps/mobile/look"
	"kando/apps/mobile/session"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	"github.com/nsgo-dev/nsgo/ui"
)

// Actions are where the tab leads.
type Actions struct {
	// Review starts a review of the items due; Vocabulary and Lines open their lists; Export opens the export; Pair opens pairing.
	Review, Vocabulary, Lines, Export, Pair func()
}

// Page is the tab's root page.
func Page(cx *core.Scope, s *session.Session, act Actions) base.NavPage {
	title := func() string { return locales.LearnTitle(cx) }
	items := core.NewMemoFunc(cx, s.LearnItems, nil)
	// now is the session's clock, read again as the items change, so what is due follows a review.
	now := func() time.Time { _ = items.Get(); return s.Now() }
	due := core.NewMemoFunc(cx, func() []learning.Item { return learning.Due(items.Get(), now()) }, nil)
	empty := func() bool { return len(items.Get()) == 0 }
	pulled := core.NewSignal(cx, false)
	cx.Effect(func() {
		if pulled.Get() && !s.LearnSyncing() {
			pulled.Set(false)
		}
	})
	ref := &core.Ref{}
	body := core.Scroll{TestID: "learn-scroll", Ref: ref, Style: core.Style{FlexGrow: 1, FlexShrink: 1},
		OnRefresh:  func() { pulled.Set(true); s.SyncLearning() },
		Refreshing: pulled.Get,
		Children: []core.Node{core.View{StyleFunc: look.Column(cx), Children: []core.Node{
			status(cx, s),
			goal(cx, s),
			core.Show{When: empty, Then: func(cx *core.Scope) core.Node { return nothingSaved(cx, s, act.Pair) },
				Else: func(cx *core.Scope) core.Node { return review(cx, due.Get, act.Review) }},
			week(cx, s),
			core.Show{When: func() bool { return !empty() }, Then: func(cx *core.Scope) core.Node { return lists(cx, items.Get, act) }},
		}}}}
	return look.TabRoot(cx, base.NavPage{TestID: "learn-page", Children: []core.Node{body}}, title, nil, nil)
}

// card is a rounded card of the page.
func card(cx *core.Scope, testID string, gap core.Points, children ...core.Node) core.Node {
	colors := ui.ColorsOf(cx)
	return core.View{TestID: testID, Style: core.Style{CornerRadius: ui.RadiusCard, BorderCurve: core.BorderCurveContinuous, Padding: core.All(16), Gap: gap},
		StyleFunc: func() core.Style { return core.Style{Background: ui.Alpha(colors.Text(), 0.045)} },
		Children:  children}
}

// status says the computers could not be reached at the last sync, while the phone shows what it kept; nothing while one answered.
func status(cx *core.Scope, s *session.Session) core.Node {
	colors := ui.ColorsOf(cx)
	return core.Show{When: func() bool { p := s.LearnProblem(); return p != session.NoProblem && len(s.Desktops()) > 0 }, Then: func(cx *core.Scope) core.Node {
		return core.View{TestID: "learn-offline", Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 8, Padding: core.Symmetric(0, 4)}, Children: []core.Node{
			ui.Icon(lucide.WifiOff, 16, colors.Muted),
			core.Text{ContentFunc: func() string { return locales.LearnOffline(cx) }, Style: look.Caption.Merge(core.Style{FlexShrink: 1}), StyleFunc: ui.Colored(colors.Muted)},
		}}
	}}
}

// goal is the day's minutes watched against the goal, and the streak.
func goal(cx *core.Scope, s *session.Session) core.Node {
	colors := ui.ColorsOf(cx)
	minutes := func() int { return int(s.Watched().Day(s.Now()) / time.Minute) }
	met := func() bool { return minutes() >= s.Goal() }
	return card(cx, "learn-goal", 10,
		core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignEnd, JustifyContent: core.JustifySpaceBetween, Gap: 12}, Children: []core.Node{
			core.View{Style: core.Style{Gap: 2, FlexShrink: 1}, Children: []core.Node{
				core.Text{ContentFunc: func() string { return locales.LearnToday(cx) }, Style: look.Section, StyleFunc: ui.Colored(colors.Muted)},
				ui.Tabular(core.Text{TestID: "learn-goal-minutes", ContentFunc: func() string { return locales.LearnMinutes(cx, minutes()) }, Style: look.Display, StyleFunc: ui.Colored(colors.Accent)}),
			}},
			core.Text{TestID: "learn-streak", ContentFunc: func() string { return locales.LearnStreak(cx, s.Watched().Streak(s.Now())) }, MaxLines: 1, Style: look.Subhead.Merge(core.Style{FontWeight: core.FontWeightSemibold}), StyleFunc: ui.Colored(colors.Text)},
		}},
		// ui.Progress's label is a string, so the bar is built again as the goal or the language changes, for assistive technology to name the goal it shows.
		core.Dynamic(func(cx *core.Scope) core.Node {
			return ui.Progress{TestID: "learn-goal-bar", Label: locales.LearnGoalOf(cx, s.Goal()), Value: func() float32 { return min(float32(minutes())/float32(s.Goal()), 1) }}
		}),
		core.Text{TestID: "learn-goal-text", ContentFunc: func() string {
			if met() {
				return locales.LearnGoalMet(cx)
			}
			return locales.LearnGoalLeft(cx, s.Goal()-minutes())
		}, Style: look.Subhead, StyleFunc: ui.Colored(colors.Muted)},
	)
}

// review is the card that starts the review of the items due, or says none is.
func review(cx *core.Scope, due func() []learning.Item, start func()) core.Node {
	colors := ui.ColorsOf(cx)
	n := func() int { return len(due()) }
	return card(cx, "learn-review", 12,
		core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 12}, Children: []core.Node{
			look.IconTile(cx, lucide.Layers, look.ToneAccent),
			core.View{Style: core.Style{Gap: 2, FlexGrow: 1, FlexShrink: 1}, Children: []core.Node{
				core.Text{TestID: "learn-review-title", ContentFunc: func() string {
					if n() == 0 {
						return locales.LearnReviewNone(cx)
					}
					return locales.LearnReviewDue(cx, n())
				}, Style: look.Title, StyleFunc: ui.Colored(colors.Text)},
				core.Text{ContentFunc: func() string {
					if n() == 0 {
						return locales.LearnReviewNoneDetail(cx)
					}
					return locales.LearnReviewDueDetail(cx, max(1, (n()*2+4)/5))
				}, Style: look.Subhead, StyleFunc: ui.Colored(colors.Muted)},
			}},
		}},
		core.Show{When: func() bool { return n() > 0 }, Then: func(cx *core.Scope) core.Node {
			return look.Button{TestID: "learn-review-start", Kind: look.Primary, Text: func() string { return locales.LearnReviewStart(cx) }, OnPress: start}
		}},
	)
}

// nothingSaved is what the review's card says while nothing is saved: where words and lines come from, and the way to pair while no computer is.
func nothingSaved(cx *core.Scope, s *session.Session, pair func()) core.Node {
	colors := ui.ColorsOf(cx)
	paired := func() bool { return len(s.Desktops()) > 0 }
	return card(cx, "learn-empty", 12,
		core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 12}, Children: []core.Node{
			look.IconTile(cx, lucide.GraduationCap, look.ToneAccent),
			core.Text{ContentFunc: func() string {
				if paired() {
					return locales.LearnEmpty(cx)
				}
				return locales.LearnNoComputer(cx)
			}, Style: look.Subhead.Merge(core.Style{FlexShrink: 1}), StyleFunc: ui.Colored(colors.Text)},
		}},
		core.Show{When: func() bool { return !paired() }, Then: func(cx *core.Scope) core.Node {
			return look.Button{TestID: "learn-pair", Kind: look.Tinted, Text: func() string { return locales.SettingsPair(cx) }, OnPress: pair}
		}},
	)
}

// week is the last seven days' minutes watched as bars, today's in the accent.
func week(cx *core.Scope, s *session.Session) core.Node {
	colors := ui.ColorsOf(cx)
	const height = 56
	days := func() [7]time.Duration { return s.Watched().Week(s.Now()) }
	total := func() int {
		var sum time.Duration
		for _, d := range days() {
			sum += d
		}
		return int(sum / time.Minute)
	}
	var bars []core.Node
	for i := range 7 {
		bars = append(bars, core.View{Style: core.Style{FlexGrow: 1, FlexBasis: core.Pt(0), AlignItems: core.AlignCenter, Gap: 6}, Children: []core.Node{
			core.View{Style: core.Style{Height: core.Pt(height), Width: core.Pct(100), JustifyContent: core.JustifyEnd, AlignItems: core.AlignCenter}, Children: []core.Node{
				core.View{Style: core.Style{Width: core.Pct(60), MaxWidth: core.Pt(22), CornerRadius: 4}, StyleFunc: func() core.Style {
					top := time.Duration(0)
					for _, d := range days() {
						top = max(top, d)
					}
					h := core.Points(3)
					if top > 0 {
						h = max(h, core.Points(float64(days()[i])/float64(top)*height))
					}
					fill := ui.Alpha(colors.Accent(), 0.35)
					if i == 6 {
						fill = colors.Accent()
					}
					return core.Style{Height: core.Pt(h), Background: fill}
				}},
			}},
			core.Text{ContentFunc: func() string { return dayName(cx, learning.WeekDays(s.Now())[i]) }, MaxLines: 1, Style: look.Caption, StyleFunc: ui.Colored(colors.Muted)},
		}})
	}
	return card(cx, "learn-week", 12,
		core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, JustifyContent: core.JustifySpaceBetween}, Children: []core.Node{
			core.Text{ContentFunc: func() string { return locales.LearnWeek(cx) }, Role: core.RoleHeader, Style: look.Title, StyleFunc: ui.Colored(colors.Text)},
			ui.Tabular(core.Text{TestID: "learn-week-total", ContentFunc: func() string { return locales.LearnMinutes(cx, total()) }, Style: look.Subhead, StyleFunc: ui.Colored(colors.Muted)}),
		}},
		core.View{Role: core.RoleImage, LabelFunc: func() string { return locales.LearnWeekBars(cx, total()) }, Style: core.Style{Direction: core.Row, Gap: 4}, Children: bars},
	)
}

// dayName is a day's short name under its bar: the weekday's in the interface's language.
func dayName(cx *core.Scope, day time.Time) string {
	switch day.Weekday() {
	case time.Monday:
		return locales.LearnMon(cx)
	case time.Tuesday:
		return locales.LearnTue(cx)
	case time.Wednesday:
		return locales.LearnWed(cx)
	case time.Thursday:
		return locales.LearnThu(cx)
	case time.Friday:
		return locales.LearnFri(cx)
	case time.Saturday:
		return locales.LearnSat(cx)
	}
	return locales.LearnSun(cx)
}

// lists are the rows of the vocabulary, the saved lines and the export.
func lists(cx *core.Scope, items func() []learning.Item, act Actions) core.Node {
	count := func(k learning.Kind) func() int {
		return func() int {
			n := 0
			for _, it := range items() {
				if it.Kind == k {
					n++
				}
			}
			return n
		}
	}
	words, lines := count(learning.Word), count(learning.Line)
	return look.Group{TestID: "learn-lists", Children: []core.Node{
		look.Row{TestID: "learn-vocabulary", Inset: 56, Chevron: true,
			Leading: look.IconTile(cx, lucide.BookOpen, look.ToneAccent),
			Title:   func() string { return locales.LearnVocabulary(cx) },
			Value:   func() string { return locales.LearnWordCount(cx, words()) },
			OnPress: act.Vocabulary},
		look.Row{TestID: "learn-lines", Index: func() int { return 1 }, Inset: 56, Chevron: true,
			Leading: look.IconTile(cx, lucide.Bookmark, look.ToneAccent),
			Title:   func() string { return locales.LearnSavedLines(cx) },
			Value:   func() string { return locales.LearnLineCount(cx, lines()) },
			OnPress: act.Lines},
		look.Row{TestID: "learn-export", Index: func() int { return 2 }, Inset: 56, Chevron: true,
			Leading: look.IconTile(cx, lucide.Share, look.ToneAccent),
			Title:   func() string { return locales.LearnExport(cx) },
			OnPress: act.Export},
	}}
}
