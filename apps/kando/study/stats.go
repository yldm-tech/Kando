package study

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"kando/apps/kando/desktop"
	"kando/apps/kando/live"
	"kando/apps/kando/locales"
	"kando/apps/kando/saved"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

// StatsApp is what the study stats dialog reads of the study record and the clicks it reports, through live.Func and live.Do as App's.
type StatsApp interface {
	live.State
	// StudyStats is what the study records of the unlocked libraries say of the learner's study now, all of them together; StatsLibraries is those libraries' names, and StatsLocked how many libraries it leaves out as they are locked.
	StudyStats() saved.Stats
	StatsLibraries() []string
	StatsLocked() int
	// StatsTab is the tab shown, TabStats or TabExport; SetStatsTab shows another.
	StatsTab() string
	SetStatsTab(tab string)
	// VocabOptions is what the export writes, which SetVocabOptions changes; VocabPreview is its first three records as the file holds them, and VocabSummary what it holds.
	VocabOptions() saved.VocabOptions
	SetVocabOptions(o saved.VocabOptions)
	VocabPreview() string
	VocabSummary() VocabSummary
	// ExportVocab asks for a folder and writes the export there; VocabExported is the file it wrote or why it failed, and RevealVocabExport shows the file in the file manager.
	ExportVocab()
	VocabExported() VocabExported
	RevealVocabExport()
	// CloseStats closes the dialog.
	CloseStats()
}

// The dialog's tabs: the stats, and the export of the vocabulary.
const (
	TabStats  = "stats"
	TabExport = "export"
)

// VocabSummary is what the export holds: its words, the saved lines with them when it holds those too, and how many films they come from.
type VocabSummary struct {
	Words, Lines, Films int
}

// VocabExported is what the last export did: the File it wrote, Shown as the dialog says it, or Err, why it failed; the zero value before one.
type VocabExported struct {
	File, Shown, Err string
}

// The dialog's measures.
const (
	statsWidth  core.Points = 580
	statsHeight core.Points = 640
	// chartHeight is the height of the tallest bar of the chart.
	chartHeight core.Points = 90
)

// StatsDialog is the study stats dialog, the 学习 menu's Study Stats and Export Vocabulary…, at two tabs. Stats, of the unlocked libraries together, which it names: the days in a row studied and the longest run, the time films played in the last seven days against the seven before, the words and the saved lines and how many of each are due, a bar a day of the last seven days' minutes, on this computer and on the other devices, and how well the words and lines are known. Export Vocabulary: the format, Anki's notes, CSV or tab-separated text, what goes with each word, a preview of the first records, what the export holds and Export…, which asks for a folder. Done, Escape and closing it run CloseStats.
type StatsDialog struct {
	App StatsApp
}

// Render implements core.Node.
func (d StatsDialog) Render(cx *core.Scope) core.Node {
	a := d.App
	colors := ui.ColorsOf(cx)
	tab := live.Func(a, a.StatsTab)
	title := func() string { return locales.VocabStatsTitle(cx) }
	tabs := core.Dynamic(func(cx *core.Scope) core.Node {
		// TODO(nsgo#145): give ui.Option and ui.Tabs a LabelFunc twin so the tabs' names follow the interface language without building them again, which takes the keyboard focus off them; then drop this Dynamic https://github.com/nsgo-dev/nsgo/issues/145
		m := locales.For(cx)
		return ui.Tabs[string]{
			Options: []ui.Option[string]{
				{Value: TabStats, Label: m.VocabTabStats(), TestID: "stats-tab-stats"},
				{Value: TabExport, Label: m.VocabTabExport(), TestID: "stats-tab-export"},
			},
			Value:    tab,
			OnChange: live.On(a, a.SetStatsTab),
			Label:    m.VocabStatsTitle(),
			TestID:   "stats-tabs",
		}
	})
	head := core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 12, FlexShrink: 0}, Children: []core.Node{
		core.View{Role: core.RoleHeader, AccessibilityLevel: func() int { return 1 }, LabelFunc: title, Style: core.Style{FlexGrow: 1}, Children: []core.Node{
			core.Text{ContentFunc: title, Style: ui.TypeTitle, StyleFunc: ui.Colored(colors.Text)},
		}},
		tabs,
	}}
	body := core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, MinHeight: core.Pt(0)}, Children: []core.Node{
		core.Show{When: func() bool { return tab() != TabExport }, Then: func(cx *core.Scope) core.Node { return statsTab(cx, a) },
			Else: func(cx *core.Scope) core.Node { return exportTab(cx, a) }},
	}}
	summary := live.Func(a, a.VocabSummary)
	exported := live.Func(a, a.VocabExported)
	footer := core.View{Style: core.Style{Direction: core.Row, Gap: 8, AlignItems: core.AlignCenter, FlexShrink: 0}, Children: []core.Node{
		core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0)}, Children: []core.Node{
			core.Show{When: func() bool { return tab() == TabExport }, Then: func(cx *core.Scope) core.Node {
				text := func() string {
					e := exported()
					switch {
					case e.Err != "":
						return locales.VocabExportFailed(cx, e.Err)
					case e.File != "":
						return locales.VocabExported(cx, e.Shown)
					}
					s := summary()
					return locales.VocabSummary(cx, s.Words, s.Lines, s.Films)
				}
				return core.Text{TestID: "stats-export-status", ContentFunc: text, Style: ui.TypeCaption, StyleFunc: ui.Colored(colors.Muted)}
			}},
		}},
		core.Show{When: func() bool { return tab() == TabExport && exported().File != "" }, Then: func(cx *core.Scope) core.Node {
			return textButton(func(cx *core.Scope) string { return locales.FilmShowInFinder(cx, desktop.Platform(cx)) }, ui.Button{Variant: ui.ButtonSecondary, TestID: "stats-export-reveal", OnPress: live.Do(a, a.RevealVocabExport)})
		}},
		core.Show{When: func() bool { return tab() == TabExport }, Then: func(cx *core.Scope) core.Node {
			return textButton(locales.VocabExport, ui.Button{Variant: ui.ButtonSecondary, TestID: "stats-export", OnPress: live.Do(a, a.ExportVocab),
				Disabled: func() bool { s := summary(); return s.Words+s.Lines == 0 }})
		}},
		textButton(locales.VocabDone, ui.Button{TestID: "stats-done", OnPress: live.Do(a, a.CloseStats)}),
	}}
	// TODO(nsgo#145): give base.Dialog (ui.Dialog) LabelFunc and AccessibilityDescriptionFunc twins, so the dialog's accessible name follows the interface language as it changes while it is open; it is read as the dialog opens https://github.com/nsgo-dev/nsgo/issues/145
	return ui.Dialog{
		TestID: "stats",
		Label:  locales.VocabStatsTitle(cx),
		Open:   func() bool { return true },
		Bare:   true,
		Style:  core.Style{Width: core.Pt(statsWidth), Height: core.Pt(statsHeight)},
		OnOpenChange: func(open bool, _ base.Reason) {
			if !open {
				a.CloseStats()
				a.Changed()
			}
		},
		Children: []core.Node{core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, MinHeight: core.Pt(0), Gap: 16, Padding: core.All(20)}, Children: []core.Node{head, body, footer}}},
	}
}

// statsTab is the Stats tab: four cards of numbers, the chart of the last seven days and the bar of how well the words and lines are known.
func statsTab(cx *core.Scope, a StatsApp) core.Node {
	colors := ui.ColorsOf(cx)
	stats := live.Func(a, a.StudyStats)
	week := func() string {
		s := stats()
		diff := s.Week - s.LastWeek
		switch {
		case diff.Round(time.Minute) == 0:
			return locales.VocabWeekSame(cx)
		case diff > 0:
			return locales.VocabWeekMore(cx, Duration(cx, diff))
		}
		return locales.VocabWeekLess(cx, Duration(cx, -diff))
	}
	cards := core.View{Style: core.Style{Direction: core.Row, Wrap: core.Wrap, Gap: 10}, Children: []core.Node{
		kpi(cx, "stats-streak", func() string { return locales.VocabStreak(cx) }, func() string { return locales.VocabDays(cx, stats().Streak) }, func() string { return locales.VocabLongest(cx, stats().Longest) }),
		kpi(cx, "stats-week", func() string { return locales.VocabWeek(cx) }, func() string { return Duration(cx, stats().Week) }, week),
		kpi(cx, "stats-words", func() string { return locales.VocabWords(cx) }, func() string { return locales.VocabWordCount(cx, stats().Words) }, func() string { return locales.VocabWordsDue(cx, stats().WordsDue) }),
		kpi(cx, "stats-lines", func() string { return locales.VocabLines(cx) }, func() string { return locales.VocabLineCount(cx, stats().Lines) }, func() string { return locales.VocabLinesDue(cx, stats().LinesDue) }),
	}}
	section := func(label func() string) core.Node {
		return core.Text{ContentFunc: label, Role: core.RoleHeader, Style: ui.TypeLabel, StyleFunc: ui.Colored(colors.Text)}
	}
	return core.Scroll{Style: core.Style{FlexGrow: 1, MinHeight: core.Pt(0)}, Children: []core.Node{
		core.View{Style: core.Style{Gap: 18, Padding: core.Edges{Bottom: 4}}, Children: []core.Node{
			libraries(cx, a),
			cards,
			core.View{Style: core.Style{Gap: 8}, Children: []core.Node{section(func() string { return locales.VocabChartTitle(cx) }), chart(cx, stats)}},
			core.View{Style: core.Style{Gap: 8}, Children: []core.Node{section(func() string { return locales.VocabMastery(cx) }), mastery(cx, stats)}},
		}},
	}}
}

// libraries says which libraries the stats count, and how many locked ones they leave out when there are any.
func libraries(cx *core.Scope, a StatsApp) core.Node {
	colors := ui.ColorsOf(cx)
	names := live.Func(a, a.StatsLibraries)
	locked := live.Func(a, a.StatsLocked)
	return core.View{Style: core.Style{Gap: 2}, Children: []core.Node{
		core.Text{TestID: "stats-libraries", ContentFunc: func() string {
			return locales.VocabStatsLibraries(cx, strings.Join(names(), locales.VocabLibrarySeparator(cx)))
		}, Style: ui.TypeCaption, StyleFunc: ui.Colored(colors.Muted)},
		core.Show{When: func() bool { return locked() > 0 }, Then: func(cx *core.Scope) core.Node {
			return core.Text{TestID: "stats-locked", ContentFunc: func() string { return locales.VocabStatsLocked(cx, locked()) }, Style: ui.TypeCaption, StyleFunc: ui.Colored(colors.Muted)}
		}},
	}}
}

// Duration is a time studied as the dialog writes it: whole minutes under an hour, else hours to a tenth.
func Duration(cx *core.Scope, d time.Duration) string {
	if d < time.Hour {
		return locales.VocabMinutes(cx, int(d.Round(time.Minute)/time.Minute))
	}
	return locales.VocabHours(cx, strconv.FormatFloat(d.Hours(), 'f', 1, 64))
}

// kpi is a card of the Stats tab: its title, its number large and a line under it.
func kpi(cx *core.Scope, id string, title, value, detail func() string) core.Node {
	colors := ui.ColorsOf(cx)
	return core.View{TestID: id, Role: core.RoleGroup, LabelFunc: func() string { return title() + " " + value() + " " + detail() },
		Style:     core.Style{FlexGrow: 1, FlexBasis: core.Pt(240), Gap: 2, Padding: core.Symmetric(12, 14), CornerRadius: 10, BorderWidth: 1},
		StyleFunc: func() core.Style { return core.Style{Background: colors.Inset(), BorderColor: colors.Border()} },
		Children: []core.Node{
			core.Text{ContentFunc: title, Role: core.RoleNone, Style: ui.TypeCaption, StyleFunc: ui.Colored(colors.Muted)},
			core.Text{ContentFunc: value, Role: core.RoleNone, Style: ui.TypeHeading, StyleFunc: ui.Colored(colors.Text)},
			core.Text{ContentFunc: detail, Role: core.RoleNone, Style: ui.TypeCaption, StyleFunc: ui.Colored(colors.Muted)},
		}}
}

// chart is a bar a day of the last seven days, today last, each the minutes films played on this computer under those on the other devices, the tallest day chartHeight high, with the day under it, and the legend under them.
func chart(cx *core.Scope, stats func() saved.Stats) core.Node {
	colors := ui.ColorsOf(cx)
	tallest := func() time.Duration {
		most := time.Minute
		for _, d := range stats().Days {
			most = max(most, d.Here+d.Elsewhere)
		}
		return most
	}
	height := func(d time.Duration) core.Points {
		return core.Points(float64(chartHeight) * float64(d) / float64(tallest()))
	}
	var bars []core.Node
	for i := range 7 {
		day := func() saved.DayTime { return stats().Days[i] }
		bars = append(bars, core.View{TestID: fmt.Sprintf("stats-day-%d", i), Role: core.RoleImage,
			LabelFunc: func() string { return dayLabel(day().Day) + " " + Duration(cx, day().Here+day().Elsewhere) },
			Style:     core.Style{FlexGrow: 1, AlignItems: core.AlignCenter, Gap: 4},
			Children: []core.Node{
				core.View{Style: core.Style{Height: core.Pt(chartHeight), Width: core.Pct(100), MaxWidth: core.Pt(36), JustifyContent: core.JustifyEnd}, Children: []core.Node{
					core.View{StyleFunc: func() core.Style {
						return core.Style{Height: core.Pt(height(day().Elsewhere)), Background: colors.Accent(), Opacity: core.Alpha(elsewhere), CornerRadius: 3}
					}},
					core.View{StyleFunc: func() core.Style {
						return core.Style{Height: core.Pt(height(day().Here)), Background: colors.Accent(), CornerRadius: 3}
					}},
				}},
				core.Text{ContentFunc: func() string { return dayLabel(day().Day) }, Role: core.RoleNone, Style: ui.TypeCaption, StyleFunc: ui.Colored(colors.Muted)},
			}})
	}
	legend := core.View{Style: core.Style{Direction: core.Row, Gap: 14}, Children: []core.Node{
		swatch(cx, colors.Accent, 1, func() string { return locales.VocabThisComputer(cx) }),
		swatch(cx, colors.Accent, elsewhere, func() string { return locales.VocabOtherDevices(cx) }),
	}}
	return core.View{Style: core.Style{Gap: 8}, Children: []core.Node{
		core.View{Style: core.Style{Direction: core.Row, Gap: 8, AlignItems: core.AlignEnd}, Children: bars},
		legend,
	}}
}

// dayLabel is a day of the chart, "2026-11-16", as it is written under its bar: its month and day.
func dayLabel(day string) string {
	t, err := time.Parse(time.DateOnly, day)
	if err != nil {
		return day
	}
	return fmt.Sprintf("%d/%d", t.Month(), t.Day())
}

// elsewhere is how opaque the parts of the other devices and of what is new are drawn, in the accent color, to tell them from this computer's and from what is being learned.
const elsewhere = 0.35

// swatch is a key of a legend: a square of color, at opacity, and what it stands for.
func swatch(cx *core.Scope, color func() core.Color, opacity float32, label func() string) core.Node {
	colors := ui.ColorsOf(cx)
	return core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 6}, Children: []core.Node{
		core.View{Style: core.Style{Width: core.Pt(10), Height: core.Pt(10), CornerRadius: 2, Opacity: core.Alpha(opacity)}, StyleFunc: func() core.Style { return core.Style{Background: color()} }},
		core.Text{ContentFunc: label, Style: ui.TypeCaption, StyleFunc: ui.Colored(colors.Muted)},
	}}
}

// mastery is the bar of how well the words and the saved lines are known, mastered, learning and new, each as long as its share, with their counts and what mastered means under it.
func mastery(cx *core.Scope, stats func() saved.Stats) core.Node {
	colors := ui.ColorsOf(cx)
	part := func(n func(saved.Stats) int, color func() core.Color, opacity float32) core.Node {
		return core.View{StyleFunc: func() core.Style {
			return core.Style{FlexGrow: float32(n(stats())), Background: color(), Opacity: core.Alpha(opacity)}
		}}
	}
	mastered := func(s saved.Stats) int { return s.Mastered }
	learning := func(s saved.Stats) int { return s.Learning }
	fresh := func(s saved.Stats) int { return s.New }
	bar := core.View{TestID: "stats-mastery", Style: core.Style{Direction: core.Row, Height: core.Pt(10), CornerRadius: 5, Overflow: core.Hidden},
		StyleFunc: func() core.Style { return core.Style{Background: colors.Track()} },
		Children:  []core.Node{part(mastered, colors.Up, 1), part(learning, colors.Accent, 1), part(fresh, colors.Accent, elsewhere)}}
	return core.View{Style: core.Style{Gap: 8}, Children: []core.Node{
		bar,
		core.View{Style: core.Style{Direction: core.Row, Wrap: core.Wrap, Gap: 14}, Children: []core.Node{
			swatch(cx, colors.Up, 1, func() string { return locales.VocabMastered(cx, stats().Mastered) }),
			swatch(cx, colors.Accent, 1, func() string { return locales.VocabLearning(cx, stats().Learning) }),
			swatch(cx, colors.Accent, elsewhere, func() string { return locales.VocabNew(cx, stats().New) }),
		}},
		core.Text{ContentFunc: func() string { return locales.VocabMasteryNote(cx) }, Style: ui.TypeCaption, StyleFunc: ui.Colored(colors.Muted)},
	}}
}

// exportTab is the Export Vocabulary tab: what it is for, the format, what goes with each word, and the preview.
func exportTab(cx *core.Scope, a StatsApp) core.Node {
	colors := ui.ColorsOf(cx)
	options := live.Func(a, a.VocabOptions)
	format := core.Dynamic(func(cx *core.Scope) core.Node {
		// TODO(nsgo#145): give ui.Option and ui.Tabs a LabelFunc twin so the formats' names follow the interface language without building them again, which takes the keyboard focus off them; then drop this Dynamic https://github.com/nsgo-dev/nsgo/issues/145
		m := locales.For(cx)
		return ui.Tabs[string]{
			Options: []ui.Option[string]{
				{Value: saved.VocabAnki, Label: m.VocabFormatAnki(), TestID: "stats-format-anki"},
				{Value: saved.VocabCSV, Label: m.VocabFormatCSV(), TestID: "stats-format-csv"},
				{Value: saved.VocabTSV, Label: m.VocabFormatTSV(), TestID: "stats-format-tsv"},
			},
			Value: func() string { return options().Format },
			OnChange: live.On(a, func(f string) {
				o := a.VocabOptions()
				o.Format = f
				a.SetVocabOptions(o)
			}),
			Label:  m.VocabFormat(),
			TestID: "stats-format",
		}
	})
	check := func(id string, label func(*core.Scope) string, on func(saved.VocabOptions) bool, change func(o *saved.VocabOptions, on bool)) core.Node {
		return core.Dynamic(func(cx *core.Scope) core.Node {
			// TODO(nsgo#145): give ui.Checkbox a TextFunc twin so its text follows the interface language without building it again; then drop this Dynamic https://github.com/nsgo-dev/nsgo/issues/145
			return ui.Checkbox{TestID: id, Text: label(cx),
				Checked: func() bool { return on(options()) },
				OnChange: live.On(a, func(v bool) {
					o := a.VocabOptions()
					change(&o, v)
					a.SetVocabOptions(o)
				})}
		})
	}
	preview := live.Func(a, a.VocabPreview)
	return core.View{Style: core.Style{FlexGrow: 1, MinHeight: core.Pt(0), Gap: 14}, Children: []core.Node{
		core.Text{ContentFunc: func() string { return locales.VocabExportIntro(cx) }, Style: ui.TypeBody, StyleFunc: ui.Colored(colors.Muted)},
		core.View{Style: core.Style{Gap: 6}, Children: []core.Node{
			core.Text{ContentFunc: func() string { return locales.VocabFormat(cx) }, Style: ui.TypeLabel, StyleFunc: ui.Colored(colors.Text)},
			format,
		}},
		core.View{Style: core.Style{Gap: 6}, Children: []core.Node{
			check("stats-with-sentence", locales.VocabWithSentence, func(o saved.VocabOptions) bool { return o.Context }, func(o *saved.VocabOptions, on bool) { o.Context = on }),
			check("stats-with-translation", locales.VocabWithTranslation, func(o saved.VocabOptions) bool { return o.Translation }, func(o *saved.VocabOptions, on bool) { o.Translation = on }),
			check("stats-with-lines", locales.VocabWithLines, func(o saved.VocabOptions) bool { return o.Lines }, func(o *saved.VocabOptions, on bool) { o.Lines = on }),
		}},
		core.View{Style: core.Style{Gap: 6, FlexGrow: 1, MinHeight: core.Pt(0)}, Children: []core.Node{
			core.Text{ContentFunc: func() string { return locales.VocabPreview(cx) }, Style: ui.TypeLabel, StyleFunc: ui.Colored(colors.Text)},
			core.Scroll{Style: core.Style{FlexGrow: 1, MinHeight: core.Pt(60), CornerRadius: 8, BorderWidth: 1},
				StyleFunc: func() core.Style { return core.Style{Background: colors.Inset(), BorderColor: colors.Border()} },
				Children: []core.Node{core.View{Style: core.Style{Padding: core.Symmetric(8, 10)}, Children: []core.Node{
					core.Show{When: func() bool { return preview() != "" },
						Then: func(cx *core.Scope) core.Node {
							return core.Text{TestID: "stats-preview", ContentFunc: preview, Selectable: core.SelectableOn, Style: ui.TypeCode, StyleFunc: ui.Colored(colors.Text)}
						},
						Else: func(cx *core.Scope) core.Node {
							return core.Text{TestID: "stats-preview-empty", ContentFunc: func() string { return locales.VocabExportEmpty(cx) }, Style: ui.TypeBody, StyleFunc: ui.Colored(colors.Muted)}
						}},
				}}}},
		}},
	}}
}
