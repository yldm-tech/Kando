// Package learn is the Learn tab: the day's goal and the streak, the review of the words and lines due, the week's study, the vocabulary and the saved lines, and their export through the share sheet. The words and lines are the ones saved on the computers' libraries and on the phone, which the session keeps on the phone (package learning), so the tab works away from the computer; what the phone changes the computers take at the next sync.
package learn

import (
	"fmt"
	"math"
	"strconv"
	"time"

	"kando/apps/mobile/learning"
	"kando/apps/mobile/locales"
	"kando/apps/mobile/look"
	"kando/apps/mobile/session"
	"kando/apps/mobile/theme"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	"github.com/nsgo-dev/nsgo/ui"
)

// Actions are where the tab leads.
type Actions struct {
	// Review starts a review of the items due; Vocabulary and Lines open the whole lists; Pair opens pairing; Settings opens Settings, where the reminder is.
	Review, Vocabulary, Lines, Pair, Settings func()
	// Play opens the player of the film kept under key at at.
	Play func(key string, at time.Duration)
}

// inline is how many words, and how many lines, the tab lists itself; the rest are on their own pages.
const inline = 8

// Page is the tab's root page.
func Page(cx *core.Scope, s *session.Session, act Actions) base.NavPage {
	title := func() string { return locales.LearnTitle(cx) }
	items := core.NewMemoFunc(cx, s.LearnItems, nil)
	// now is the session's clock, read again as the items change, so what is due follows a review.
	now := func() time.Time { _ = items.Get(); return s.Now() }
	due := core.NewMemoFunc(cx, func() []learning.Item { return learning.Due(items.Get(), now()) }, nil)
	words := core.NewMemoFunc(cx, func() []learning.Item { return kind(items.Get(), learning.Word) }, nil)
	lines := core.NewMemoFunc(cx, func() []learning.Item { return kind(items.Get(), learning.Line) }, nil)
	pulled := core.NewSignal(cx, false)
	cx.Effect(func() {
		if pulled.Get() && !s.LearnSyncing() {
			pulled.Set(false)
		}
	})
	exporting := core.NewSignal(cx, false)
	// word is the word whose sheet shows, by its key, "" for none.
	word := core.NewSignal(cx, "")
	ref := &core.Ref{}
	column := look.Column(cx)
	body := core.Scroll{TestID: "learn-scroll", Ref: ref, Style: core.Style{FlexGrow: 1, FlexShrink: 1},
		OnRefresh:  func() { pulled.Set(true); s.SyncLearning() },
		Refreshing: pulled.Get,
		Children: []core.Node{core.View{StyleFunc: func() core.Style { st := column(); st.Gap = 30; return st }, Children: []core.Node{
			core.View{Style: core.Style{Gap: 14}, Children: []core.Node{
				status(cx, s),
				goal(cx, s),
				core.View{Style: core.Style{Gap: 20}, Children: []core.Node{
					review(cx, s, items.Get, due.Get, act),
					mode(cx, s, act.Settings),
				}},
				week(cx, s, words.Get),
			}},
			wordList(cx, s, words.Get, due.Get, word.Set, func() { exporting.Set(true) }, act.Vocabulary),
			savedLines(cx, s, lines.Get, act),
		}}}}
	return look.TabRoot(cx, base.NavPage{TestID: "learn-page", Children: []core.Node{
		body,
		ExportSheet(cx, s, exporting.Get, func() { exporting.Set(false) }),
		wordSheet(cx, s, items.Get, word, act.Play),
	}}, title, nil, nil)
}

// kind is the items of kind k.
func kind(items []learning.Item, k learning.Kind) []learning.Item {
	var out []learning.Item
	for _, it := range items {
		if it.Kind == k {
			out = append(out, it)
		}
	}
	return out
}

// card is a rounded card of the page.
func card(cx *core.Scope, testID string, style core.Style, children ...core.Node) core.Node {
	return core.View{TestID: testID, Style: style, StyleFunc: func() core.Style { return look.Card(cx) }, Children: children}
}

// white is text on the goal's blue, at alpha.
func white(cx *core.Scope, alpha float32) func() core.Style {
	colors := ui.ColorsOf(cx)
	return func() core.Style { return core.Style{Color: ui.Alpha(colors.OnAccent(), alpha)} }
}

// status is the line under the title: the words, the lines and where the learner stopped follow them from device to device; or, when the computers could not be reached at the last sync, that the phone shows what it kept.
func status(cx *core.Scope, s *session.Session) core.Node {
	colors := ui.ColorsOf(cx)
	offline := func() bool { p := s.LearnProblem(); return p != session.NoProblem && len(s.Desktops()) > 0 }
	line := func(id string, icon func() *core.SVG, text func() string) core.Node {
		return core.View{TestID: id, Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 6}, Children: []core.Node{
			ui.Icon(icon, 14, colors.Muted),
			core.Text{ContentFunc: text, Style: look.Caption.Merge(core.Style{FlexShrink: 1}), StyleFunc: ui.Colored(colors.Muted)},
		}}
	}
	return core.Show{When: offline,
		Then: func(cx *core.Scope) core.Node {
			return line("learn-offline", lucide.WifiOff, func() string { return locales.LearnOffline(cx) })
		},
		Else: func(cx *core.Scope) core.Node {
			return line("learn-sync-note", lucide.RefreshCw, func() string { return locales.LearnSyncNote(cx) })
		}}
}

// goal is the day's minutes watched against the goal, in a ring on the accent's gradient, with what is left to reach it, the streak and the week's time.
func goal(cx *core.Scope, s *session.Session) core.Node {
	colors := ui.ColorsOf(cx)
	minutes := func() int { return int(s.Watched().Day(s.Now()) / time.Minute) }
	share := func() float32 { return min(float32(minutes())/float32(max(s.Goal(), 1)), 1) }
	const ring, disc, line = 84, 68, 8
	draw := func(d *core.Drawing, size core.Size) {
		c, r := float32(size.Width)/2, float32(ring-line)/2
		on := colors.OnAccent()
		d.SetLineWidth(line).SetStrokeColor(ui.Alpha(on, 0.25)).BeginPath().Arc(c, c, r, 0, 2*math.Pi, false).Stroke()
		if f := share(); f > 0 {
			start := float32(-math.Pi / 2)
			d.SetStrokeColor(on).BeginPath().Arc(c, c, r, start, start+2*math.Pi*f, false).Stroke()
		}
	}
	dial := core.View{Style: core.Style{Width: core.Pt(ring), Height: core.Pt(ring), FlexShrink: 0, AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter}, Children: []core.Node{
		core.Canvas{TestID: "learn-goal-bar", Draw: draw, LabelFunc: func() string { return locales.LearnGoalOf(cx, s.Goal()) },
			Style: core.Style{Position: core.Absolute, Left: core.Pt(0), Top: core.Pt(0), Width: core.Pt(ring), Height: core.Pt(ring)}},
		core.View{Style: core.Style{Width: core.Pt(disc), Height: core.Pt(disc), CornerRadius: disc / 2, Background: theme.GoalDisc, AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter},
			Children: []core.Node{
				ui.Tabular(core.Text{TestID: "learn-goal-minutes", ContentFunc: func() string { return strconv.Itoa(minutes()) }, MaxLines: 1,
					Style: core.Style{FontSize: 22, LineHeight: 24, FontWeight: look.Heavy}, StyleFunc: white(cx, 1)}),
				core.Text{ContentFunc: func() string { return locales.LearnGoalOfShort(cx, s.Goal()) }, MaxLines: 1, Style: core.Style{FontSize: 10, LineHeight: 13}, StyleFunc: white(cx, 0.85)},
			}},
	}}
	week := func() time.Duration {
		var sum time.Duration
		for _, d := range s.Watched().Week(s.Now()) {
			sum += d
		}
		return sum
	}
	texts := core.View{Style: core.Style{Gap: 3, FlexGrow: 1, FlexShrink: 1}, Children: []core.Node{
		core.Text{ContentFunc: func() string { return locales.LearnToday(cx) }, Style: look.Caption, StyleFunc: white(cx, 0.85)},
		core.Text{TestID: "learn-goal-text", ContentFunc: func() string {
			if minutes() >= s.Goal() {
				return locales.LearnGoalMet(cx)
			}
			return locales.LearnGoalLeft(cx, s.Goal()-minutes())
		}, Style: core.Style{FontSize: 19, LineHeight: 25, FontWeight: look.Heavy}, StyleFunc: white(cx, 1)},
		core.Text{TestID: "learn-streak", ContentFunc: func() string {
			return locales.LearnHeroFacts(cx, s.Watched().Streak(s.Now()), duration(cx, week()))
		}, Style: look.Footnote, StyleFunc: white(cx, 0.9)},
	}}
	return core.View{TestID: "learn-goal", Children: []core.Node{dial, texts},
		Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 16, Padding: core.All(18), CornerRadius: 20, BorderCurve: core.BorderCurveContinuous,
			Gradients: []core.Gradient{{Kind: core.GradientLinear, Angle: 140, Stops: []core.ColorStop{
				{Color: theme.Accent, At: core.Pct(0)}, {Color: theme.GoalVia, At: core.Pct(60)}, {Color: theme.GoalTo, At: core.Pct(100)},
			}}},
			Shadows: []core.BoxShadow{{Offset: core.Point{Y: 14}, Blur: 30, Color: theme.GoalShadow}}}}
}

// duration writes a time watched: 42 min, or 1 h 42 min from an hour.
func duration(cx *core.Scope, d time.Duration) string {
	m := int(d / time.Minute)
	if m < 60 {
		return locales.LearnMinutes(cx, m)
	}
	return locales.LearnHoursMinutes(cx, m/60, m%60)
}

// pill is the small capsule at a card's end: the accent's, or the fill's grey with muted text where grey.
func pill(cx *core.Scope, testID string, text func() string, grey bool, press func()) core.Node {
	colors := ui.ColorsOf(cx)
	return base.Button{TestID: testID, LabelFunc: text, OnPress: func(core.PressEvent) { press() },
		Style: core.Style{Height: core.Pt(32), Padding: core.Symmetric(0, 14), CornerRadius: 16, FlexShrink: 0, AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter,
			Active: &core.Style{Opacity: core.Alpha(0.7)}},
		StyleFunc: func() core.Style {
			if grey {
				return core.Style{Background: colors.Inset(), Color: colors.Muted(), FocusVisible: ui.FocusRing(cx, -2)}
			}
			return core.Style{Background: colors.Accent(), Color: colors.OnAccent(), FocusVisible: ui.FocusRing(cx, -2)}
		},
		Children: []core.Node{core.Text{ContentFunc: text, MaxLines: 1, Style: core.Style{FontSize: 14, LineHeight: 20, FontWeight: core.FontWeightBold}}}}
}

// review is the card that starts the review of the items due, or says none is: OK puts it away until something is, and while no computer is paired and nothing is saved it leads to pairing.
func review(cx *core.Scope, s *session.Session, items, due func() []learning.Item, act Actions) core.Node {
	colors := ui.ColorsOf(cx)
	n := func() int { return len(due()) }
	unpaired := func() bool { return len(s.Desktops()) == 0 && len(items()) == 0 }
	// put is set once OK put the card away; it shows again as soon as something is due.
	put := core.NewSignal(cx, false)
	cx.Effect(func() {
		if n() > 0 {
			put.Set(false)
		}
	})
	onlyWords := func() bool {
		for _, it := range due() {
			if it.Kind != learning.Word {
				return false
			}
		}
		return true
	}
	tile := core.View{Style: core.Style{Width: core.Pt(44), Height: core.Pt(44), CornerRadius: 12, BorderCurve: core.BorderCurveContinuous, FlexShrink: 0,
		AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter, Background: ui.Alpha(theme.TileOrange, 0.14)},
		Children: []core.Node{ui.Icon(lucide.Copy, 22, func() core.Color { return theme.Streak })}}
	texts := core.View{Style: core.Style{Gap: 2, FlexGrow: 1, FlexShrink: 1}, Children: []core.Node{
		core.Text{TestID: "learn-review-title", ContentFunc: func() string {
			switch {
			case n() == 0:
				return locales.LearnReviewNone(cx)
			case onlyWords():
				return locales.LearnReviewDueWords(cx, n())
			}
			return locales.LearnReviewDue(cx, n())
		}, Style: core.Style{FontSize: 16, LineHeight: 21, FontWeight: core.FontWeightBold}, StyleFunc: ui.Colored(colors.Text)},
		core.Text{ContentFunc: func() string {
			switch {
			case unpaired():
				return locales.LearnNoComputer(cx)
			case n() == 0:
				return locales.LearnReviewNoneDetail(cx)
			}
			return locales.LearnReviewDueDetail(cx, max(1, (n()*2+4)/5))
		}, Style: look.Caption, StyleFunc: ui.Colored(colors.Muted)},
	}}
	action := core.Show{When: func() bool { return n() > 0 },
		Then: func(cx *core.Scope) core.Node {
			return pill(cx, "learn-review-start", func() string { return locales.LearnReviewStart(cx) }, false, act.Review)
		},
		Else: func(cx *core.Scope) core.Node {
			return core.Show{When: unpaired,
				Then: func(cx *core.Scope) core.Node {
					return pill(cx, "learn-pair", func() string { return locales.LearnConnect(cx) }, false, act.Pair)
				},
				Else: func(cx *core.Scope) core.Node {
					return pill(cx, "learn-review-ok", func() string { return locales.CommonOK(cx) }, true, func() { put.Set(true) })
				}}
		}}
	return core.Show{When: func() bool { return !put.Get() || n() > 0 }, Then: func(cx *core.Scope) core.Node {
		return card(cx, "learn-review", core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 14, Padding: core.Symmetric(14, 16)}, tile, texts, action)
	}}
}

// mode is the way the review goes, cards or the lines dictated, and under it when the reminder comes, which leads to Settings, while it is on.
// TODO(nsgo): shadowing, the design's third way (reading the line along with the film, recorded and compared), needs an audio recorder with levels, which NSGo lacks (audio/audio.go has only Player; core/speech.go only recognizes speech), so the review offers cards and dictation.
func mode(cx *core.Scope, s *session.Session, settings func()) core.Node {
	colors := ui.ColorsOf(cx)
	row := core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 10}, Children: []core.Node{
		core.Text{ContentFunc: func() string { return locales.LearnReviewMode(cx) }, MaxLines: 1, Style: look.Caption, StyleFunc: ui.Colored(colors.Muted)},
		// ui.Tabs' labels are strings, so the modes are built again as the language changes.
		core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1}, Children: []core.Node{core.Dynamic(func(cx *core.Scope) core.Node {
			return ui.Tabs[string]{TestID: "learn-review-mode", Label: locales.LearnReviewMode(cx), Equal: true, Size: ui.SizeLarge,
				Options: []ui.Option[string]{
					{Value: session.ReviewCards, Label: locales.LearnReviewCards(cx), TestID: "learn-review-cards"},
					{Value: session.ReviewDictation, Label: locales.LearnReviewDictation(cx), TestID: "learn-review-dictation"},
				},
				Value: s.ReviewMode, OnChange: s.SetReviewMode}
		})}},
	}}
	reminder := core.Show{When: func() bool { return s.Prefs().Remind }, Then: func(cx *core.Scope) core.Node {
		text := func() string { return locales.LearnReminderCaption(cx, fmt.Sprintf("%d:00", learning.ReminderHour)) }
		return base.Button{TestID: "learn-reminder", LabelFunc: text, OnPress: func(core.PressEvent) { settings() },
			Style:    core.Style{AlignSelf: core.AlignStart, Active: &core.Style{Opacity: core.Alpha(0.6)}},
			Children: []core.Node{core.Text{ContentFunc: text, Style: look.Footnote, StyleFunc: ui.Colored(colors.Muted)}}}
	}}
	return core.View{Style: core.Style{Gap: 10}, Children: []core.Node{row, reminder}}
}

// week is the last seven days on every device: the streak, the time watched and the words put in the vocabulary, the minutes of each day as bars, today's in the accent, and the words by how well they are known.
func week(cx *core.Scope, s *session.Session, words func() []learning.Item) core.Node {
	colors := ui.ColorsOf(cx)
	const height = 64
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
		bars = append(bars, core.View{Style: core.Style{FlexGrow: 1, FlexBasis: core.Pt(0), AlignItems: core.AlignCenter, Gap: 5}, Children: []core.Node{
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
			core.Text{ContentFunc: func() string {
				if i == 6 {
					return locales.LearnTodayShort(cx)
				}
				return dayName(cx, learning.WeekDays(s.Now())[i])
			}, MaxLines: 1, Style: core.Style{FontSize: 10, LineHeight: 12, FontWeight: dayWeight(i)}, StyleFunc: ui.Colored(colors.Muted)},
		}})
	}
	newWords := func() int {
		since := learning.WeekDays(s.Now())[0].Add(-12 * time.Hour)
		n := 0
		for _, it := range words() {
			if !it.SavedAt.Before(since) {
				n++
			}
		}
		return n
	}
	kpi := func(id string, value, unit, label func() string) core.Node {
		return core.View{TestID: id, Style: core.Style{FlexGrow: 1, FlexBasis: core.Pt(0), Gap: 1}, Children: []core.Node{
			core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignBaseline, Gap: 2}, Children: []core.Node{
				ui.Tabular(core.Text{ContentFunc: value, MaxLines: 1, Style: core.Style{FontSize: 22, LineHeight: 26, FontWeight: look.Heavy, LetterSpacing: -0.4}, StyleFunc: ui.Colored(colors.Text)}),
				core.Text{ContentFunc: unit, MaxLines: 1, Style: core.Style{FontSize: 12, LineHeight: 15, FontWeight: core.FontWeightSemibold, FlexShrink: 1}, StyleFunc: ui.Colored(colors.Muted)},
			}},
			core.Text{ContentFunc: label, MaxLines: 1, Style: look.Footnote, StyleFunc: ui.Colored(colors.Muted)},
		}}
	}
	// The week's time is in minutes under an hour, else in hours to a tenth.
	hours := func() bool { return total() >= 60 }
	streak := func() int { return s.Watched().Streak(s.Now()) }
	return card(cx, "learn-week", core.Style{Padding: core.Symmetric(14, 16), Gap: 12},
		core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, JustifyContent: core.JustifySpaceBetween}, Children: []core.Node{
			core.Text{ContentFunc: func() string { return locales.LearnWeek(cx) }, Role: core.RoleHeader, Style: core.Style{FontSize: 15, LineHeight: 20, FontWeight: core.FontWeightBold}, StyleFunc: ui.Colored(colors.Text)},
			core.Text{ContentFunc: func() string { return locales.LearnWeekAll(cx) }, Style: look.Footnote, StyleFunc: ui.Colored(colors.Muted)},
		}},
		core.View{Style: core.Style{Direction: core.Row, Gap: 8}, Children: []core.Node{
			kpi("learn-week-streak", func() string { return strconv.Itoa(streak()) }, func() string { return locales.LearnDaysUnit(cx, streak()) }, func() string { return locales.LearnStreakLabel(cx) }),
			kpi("learn-week-total", func() string {
				if hours() {
					return strconv.FormatFloat(float64(total())/60, 'f', 1, 64)
				}
				return strconv.Itoa(total())
			}, func() string {
				if hours() {
					return locales.LearnHoursUnit(cx)
				}
				return locales.LearnMinutesUnit(cx)
			}, func() string { return locales.LearnStudyTime(cx) }),
			kpi("learn-week-words", func() string { return strconv.Itoa(newWords()) }, func() string { return locales.LearnWordsUnit(cx, newWords()) }, func() string { return locales.LearnNewWords(cx) }),
		}},
		core.View{Role: core.RoleImage, LabelFunc: func() string { return locales.LearnWeekBars(cx, total()) }, Style: core.Style{Direction: core.Row, Gap: 4, Margin: core.Edges{Top: 6}}, Children: bars},
		mastery(cx, words),
	)
}

// dayWeight is the weight of a day's name under its bar: today's is bold.
func dayWeight(i int) core.FontWeight {
	if i == 6 {
		return core.FontWeightBold
	}
	return core.FontWeightNormal
}

// mastery is the words by how well they are known, as a bar in three parts with their counts under it: mastered, still being learned, and new, never yet known.
func mastery(cx *core.Scope, words func() []learning.Item) core.Node {
	colors := ui.ColorsOf(cx)
	counts := func() (mastered, learned, fresh int) {
		for _, it := range words() {
			switch {
			case it.Mastered():
				mastered++
			case it.Box == 0:
				fresh++
			default:
				learned++
			}
		}
		return
	}
	part := func(n func() int, fill func() core.Color) core.Node {
		return core.View{StyleFunc: func() core.Style {
			return core.Style{FlexGrow: float32(n()), FlexBasis: core.Pt(0), Background: fill()}
		}}
	}
	m := func() int { a, _, _ := counts(); return a }
	l := func() int { _, b, _ := counts(); return b }
	f := func() int { _, _, c := counts(); return c }
	legend := func(text func() string) core.Node {
		return core.Text{ContentFunc: text, MaxLines: 1, Style: look.Footnote, StyleFunc: ui.Colored(colors.Muted)}
	}
	return core.View{TestID: "learn-mastery", Style: core.Style{Gap: 6}, Children: []core.Node{
		core.View{Style: core.Style{Direction: core.Row, Height: core.Pt(6), CornerRadius: 3, Overflow: core.Hidden}, StyleFunc: ui.Filled(colors.Inset), Children: []core.Node{
			part(m, func() core.Color { return theme.Mastered }),
			part(l, func() core.Color { return theme.Learning }),
			part(f, colors.Inset),
		}},
		core.View{Style: core.Style{Direction: core.Row, Wrap: core.Wrap, ColumnGap: 12}, Children: []core.Node{
			legend(func() string { return locales.LearnMasteredCount(cx, m()) }),
			legend(func() string { return locales.LearnLearningCount(cx, l()) }),
			legend(func() string { return locales.LearnNewCount(cx, f()) }),
		}},
	}}
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

// sectionHead is a section's title with what it counts at its end, and trailing after that.
func sectionHead(cx *core.Scope, title, count func() string, trailing ...core.Node) core.Node {
	colors := ui.ColorsOf(cx)
	children := []core.Node{
		core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1}, Children: []core.Node{look.SectionTitle(cx, title)}},
		ui.Tabular(core.Text{ContentFunc: count, MaxLines: 1, Style: look.Caption, StyleFunc: ui.Colored(colors.Muted)}),
	}
	return core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 10}, Children: append(children, trailing...)}
}

// first is the first inline of items.
func first(items []learning.Item) []learning.Item { return items[:min(len(items), inline)] }

// more is the row at a list's end that opens the whole list, while it has more than the tab lists.
func more(cx *core.Scope, testID string, n func() int, open func()) core.Node {
	return core.Show{When: func() bool { return n() > inline }, Then: func(cx *core.Scope) core.Node {
		return look.Row{TestID: testID, Index: func() int { return 1 }, Chevron: true, Title: func() string { return locales.LearnShowAll(cx, n()) }, OnPress: open}
	}}
}

// wordList is the vocabulary, the latest saved first, in a card: how many words there are and how many are due, the export, and each word, which opens its sheet.
func wordList(cx *core.Scope, s *session.Session, words, due func() []learning.Item, open func(string), export, all func()) core.Node {
	colors := ui.ColorsOf(cx)
	dueWords := func() int {
		n := 0
		for _, it := range due() {
			if it.Kind == learning.Word {
				n++
			}
		}
		return n
	}
	exportPill := base.Button{TestID: "learn-export", LabelFunc: func() string { return locales.LearnExportShort(cx) }, OnPress: func(core.PressEvent) { export() },
		Style: core.Style{Height: core.Pt(28), Padding: core.Symmetric(0, 12), CornerRadius: 14, AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter, Active: &core.Style{Opacity: core.Alpha(0.7)}},
		StyleFunc: func() core.Style {
			return core.Style{Background: colors.Inset(), Color: colors.Accent(), FocusVisible: ui.FocusRing(cx, -2)}
		},
		Children: []core.Node{core.Text{ContentFunc: func() string { return locales.LearnExportShort(cx) }, MaxLines: 1, Style: look.Caption.Merge(core.Style{FontWeight: core.FontWeightSemibold})}}}
	list := core.Show{When: func() bool { return len(words()) == 0 },
		Then: func(cx *core.Scope) core.Node {
			return card(cx, "learn-empty", core.Style{Padding: core.Symmetric(24, 16)},
				core.Text{ContentFunc: func() string { return locales.LearnEmpty(cx) }, Style: core.Style{FontSize: 14, LineHeight: 19, TextAlign: core.TextAlignCenter}, StyleFunc: ui.Colored(colors.Muted)})
		},
		Else: func(cx *core.Scope) core.Node {
			return look.Group{TestID: "learn-words", Children: []core.Node{
				core.Each(func() []learning.Item { return first(words()) }, func(it learning.Item) string { return it.Key() }, func(cx *core.Scope, it func() learning.Item, i func() int) core.Node {
					return wordRow(cx, s, it, i, func() { open(it().Key()) })
				}),
				more(cx, "learn-vocabulary", func() int { return len(words()) }, all),
			}}
		}}
	return core.View{TestID: "learn-vocabulary-section", Style: core.Style{Gap: 12}, Children: []core.Node{
		sectionHead(cx, func() string { return locales.LearnVocabulary(cx) }, func() string { return locales.LearnWordsDue(cx, len(words()), dueWords()) }, exportPill),
		list,
	}}
}

// wordRow is a word of the list: the word and how it is said, its meaning, and at its end the film it is from and when it is next due.
func wordRow(cx *core.Scope, s *session.Session, it func() learning.Item, i func() int, open func()) core.Node {
	colors := ui.ColorsOf(cx)
	left := core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0), Gap: 2}, Children: []core.Node{
		core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignBaseline, Wrap: core.Wrap, ColumnGap: 8}, Children: []core.Node{
			core.Text{ContentFunc: func() string { return it().Text }, Style: core.Style{FontSize: 17, LineHeight: 22, FontWeight: core.FontWeightBold}, StyleFunc: ui.Colored(colors.Text)},
			core.Show{When: func() bool { return it().Reading != "" }, Then: func(cx *core.Scope) core.Node {
				return core.Text{ContentFunc: func() string { return it().Reading }, Style: look.Footnote, StyleFunc: ui.Colored(colors.Muted)}
			}},
		}},
		core.Text{ContentFunc: func() string { return it().Meaning }, MaxLines: 2, Style: look.Caption, StyleFunc: func() core.Style { return core.Style{Color: ui.Alpha(colors.Text(), 0.85)} }},
	}}
	right := core.View{Style: core.Style{MaxWidth: core.Pt(120), FlexShrink: 0, AlignItems: core.AlignEnd, Gap: 4}, Children: []core.Node{
		core.Text{ContentFunc: func() string { return it().Title }, MaxLines: 1, Style: look.Footnote, StyleFunc: ui.Colored(colors.Muted)},
		core.Show{When: func() bool { return it().IsDue(s.Now()) },
			Then: func(cx *core.Scope) core.Node { return dueBadge(cx) },
			Else: func(cx *core.Scope) core.Node {
				return core.Text{ContentFunc: func() string { return schedule(cx, it(), s.Now()) }, MaxLines: 1, Style: core.Style{FontSize: 11, LineHeight: 14}, StyleFunc: ui.Colored(colors.Muted)}
			}},
	}}
	return look.Row{TestID: "learn-word-" + it().ID, Index: i, Below: left, Trailing: right, OnPress: open}
}

// dueBadge marks a word due for review.
func dueBadge(cx *core.Scope) core.Node {
	warn := theme.Of(cx).WarnText
	return core.Text{ContentFunc: func() string { return locales.LearnDueNow(cx) }, MaxLines: 1,
		Style:     core.Style{FontSize: 11, LineHeight: 18, FontWeight: core.FontWeightBold, Padding: core.Symmetric(0, 6), CornerRadius: 9, Background: ui.Alpha(theme.BadgeOrange, 0.15)},
		StyleFunc: func() core.Style { return core.Style{Color: warn()} }}
}

// schedule is when an item not due comes back: mastered, or in so many days.
func schedule(cx *core.Scope, it learning.Item, now time.Time) string {
	if it.Mastered() {
		return locales.LearnMastered(cx)
	}
	return locales.LearnDueIn(cx, max(1, int((it.Due.Sub(now)+23*time.Hour)/(24*time.Hour))))
}

// savedLines are the lines saved, the latest first, each on its card.
func savedLines(cx *core.Scope, s *session.Session, lines func() []learning.Item, act Actions) core.Node {
	return core.View{TestID: "learn-lines", Style: core.Style{Gap: 12}, Children: []core.Node{
		sectionHead(cx, func() string { return locales.LearnSavedLines(cx) }, func() string { return locales.LearnLineCount(cx, len(lines())) }),
		core.Each(func() []learning.Item { return first(lines()) }, func(it learning.Item) string { return it.Key() }, func(cx *core.Scope, it func() learning.Item, _ func() int) core.Node {
			return lineCard(cx, s, it, act.Play)
		}),
		core.Show{When: func() bool { return len(lines()) > inline }, Then: func(cx *core.Scope) core.Node {
			return look.Group{Children: []core.Node{
				look.Row{TestID: "learn-lines-all", Chevron: true, Title: func() string { return locales.LearnShowAll(cx, len(lines())) }, OnPress: act.Lines},
			}}
		}},
	}}
}
