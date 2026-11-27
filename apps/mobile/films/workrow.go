package films

import (
	"strings"

	"kando/apps/mobile/catalog"
	"kando/apps/mobile/locales"
	"kando/apps/mobile/look"
	"kando/apps/mobile/session"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	"github.com/nsgo-dev/nsgo/ui"
)

// A work's row: the films of a show, a course or a playlist its computer shows as one card, folded into one row of the list (catalog.Fold), which opens the work's page (WorkPage). Its poster is the episode the work goes on with (catalog.GoOn); under its name, where it comes from and how many of its films the row holds; at its end how many were not watched, or a check once all were. While one of its films downloads its bar shows; while the user's storage fails the phone and none of them is on it, it fades, as a film's row does.

// workOf is the work of row r, empty for a film's row.
func workOf(r func() catalog.Row) catalog.Work {
	if w := r().Work; w != nil {
		return *w
	}
	return catalog.Work{}
}

// goOnEntry is the film a work goes on with, whose poster and progress its row shows.
func goOnEntry(w func() catalog.Work) func() catalog.Entry {
	return func() catalog.Entry { e, _ := catalog.GoOn(w()); return e }
}

// workFacts is what a work's row says under its name: where it comes from, and how many of its films the row holds, a playlist's as videos.
func workFacts(cx *core.Scope, w catalog.Work) string {
	if len(w.Films) == 0 {
		return ""
	}
	source := catalog.SourceOf(w.Films[0].Film)
	count := locales.FilmsWorkEpisodes(cx, len(w.Films))
	if source != catalog.SourceLocal {
		count = locales.FilmsWorkVideos(cx, len(w.Films))
	}
	return strings.Join([]string{sourceName(cx, source), count}, " · ")
}

// workDownload is the transfer of the first of w's films downloading, false while none is.
func (v *view) workDownload(w catalog.Work) (session.Transfer, bool) {
	for _, e := range w.Films {
		key := v.s.Copy(e.Key())
		k, ok := v.s.Film(key)
		if t, moving := v.s.Transfer(key); ok && !k.Done() && moving && t.State != session.Failed {
			return t, true
		}
	}
	return session.Transfer{}, false
}

// workUnreachable reports whether every film of w is out of the phone's reach as the storage fails it (unreachable).
func (v *view) workUnreachable(w catalog.Work) bool {
	for _, e := range w.Films {
		if !v.unreachable(e.Desktop, v.s.Copy(e.Key())) {
			return false
		}
	}
	return len(w.Films) > 0
}

// workMark is the end of a work's row: how many of its films were not watched, in the accent, or a check once all were.
func (v *view) workMark(cx *core.Scope, w func() catalog.Work, id string) core.Node {
	colors := ui.ColorsOf(cx)
	left := func() int { return catalog.Unwatched(w()) }
	return core.View{Style: core.Style{Width: core.Pt(44), AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter, FlexShrink: 0}, Children: []core.Node{
		core.Show{When: func() bool { return left() > 0 },
			Then: func(cx *core.Scope) core.Node {
				return look.Count{TestID: "films-work-unwatched-" + id, Count: left}
			},
			Else: func(cx *core.Scope) core.Node {
				return core.View{TestID: "films-work-watched-" + id, Role: core.RoleImage, LabelFunc: func() string { return locales.FilmsWorkAllWatched(cx) },
					Children: []core.Node{ui.Icon(lucide.Check, 16, colors.Muted)}}
			}},
	}}
}

// workLabel is a work's row as assistive technology reads it: its name, how many films it holds, and how many were not watched.
func workLabel(cx *core.Scope, w catalog.Work) string {
	label := locales.FilmsWorkLabel(cx, w.Title, len(w.Films))
	if n := catalog.Unwatched(w); n > 0 {
		return label + ", " + locales.FilmsWorkUnwatched(cx, n)
	}
	return label + ", " + locales.FilmsWorkAllWatched(cx)
}

// workRow is a work's row of a phone's list.
func (v *view) workRow(cx *core.Scope, r func() catalog.Row, i func() int) core.Node {
	colors := ui.ColorsOf(cx)
	w := func() catalog.Work { return workOf(r) }
	id := r().Work.Desktop + "-" + r().Work.ID
	return core.View{TestID: "films-work-" + id, Role: core.RoleButton, LabelFunc: func() string { return workLabel(cx, w()) },
		OnPress: func(core.PressEvent) { v.act.Work(w().Desktop, w().ID) },
		Style:   core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 12, Padding: core.Edges{Top: 10, Bottom: 10, Left: 12}, MinHeight: core.Pt(83)},
		StyleFunc: func() core.Style {
			st := core.Style{Active: &core.Style{Background: colors.Pressed()}, Opacity: core.Alpha(1)}
			if v.workUnreachable(w()) {
				st.Opacity = core.Alpha(0.45)
			}
			return st
		},
		Children: []core.Node{
			core.View{Style: core.Style{Position: core.Absolute, Top: core.Pt(0), Left: core.Pt(0), Right: core.Pt(0), Height: core.Pt(0.5)},
				StyleFunc: func() core.Style {
					st := core.Style{Background: colors.Line()}
					if i() <= 0 {
						st.Display = core.DisplayNone
					}
					return st
				}},
			v.poster(cx, goOnEntry(w), 112, 9, false),
			core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0), Gap: 4}, Children: v.workText(cx, w, id)},
			v.workMark(cx, w, id),
		}}
}

// workText is a work's name, what it says under it, and the bar of one of its films downloading.
func (v *view) workText(cx *core.Scope, w func() catalog.Work, id string) []core.Node {
	colors := ui.ColorsOf(cx)
	return []core.Node{
		core.Text{TestID: "films-work-title-" + id, ContentFunc: func() string { return w().Title }, MaxLines: 2, Style: core.Style{FontSize: 15, LineHeight: 20, FontWeight: core.FontWeightSemibold}, StyleFunc: ui.Colored(colors.Text)},
		core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 5}, Children: []core.Node{
			sourceMark(func() string {
				if f := w().Films; len(f) > 0 {
					return catalog.SourceOf(f[0].Film)
				}
				return ""
			}),
			core.Text{TestID: "films-work-facts-" + id, ContentFunc: func() string { return workFacts(cx, w()) }, MaxLines: 1, Style: look.Footnote.Merge(core.Style{FlexShrink: 1}), StyleFunc: ui.Colored(colors.Muted)},
		}},
		core.Show{When: func() bool { _, ok := v.workDownload(w()); return ok }, Then: func(cx *core.Scope) core.Node {
			return ui.Progress{LabelFunc: func() string { return locales.FilmDownload(cx) }, Value: func() float32 { t, _ := v.workDownload(w()); return t.Fraction() * 100 }, Style: core.Style{Margin: core.Edges{Top: 2}}}
		}},
	}
}

// workCell is a work of a tablet's grid: its poster, with how many of its films were not watched (or a check once all were) at its corner, its name and what it says under it.
func (v *view) workCell(cx *core.Scope, r func() catalog.Row) core.Node {
	colors := ui.ColorsOf(cx)
	w := func() catalog.Work { return workOf(r) }
	id := r().Work.Desktop + "-" + r().Work.ID
	return core.View{Style: core.Style{Width: core.Pct(33.333), Padding: core.Symmetric(0, 8)}, Children: []core.Node{
		core.View{TestID: "films-work-" + id, Role: core.RoleButton, LabelFunc: func() string { return workLabel(cx, w()) },
			OnPress: func(core.PressEvent) { v.act.Work(w().Desktop, w().ID) },
			Style:   core.Style{Gap: 8, Active: &core.Style{Opacity: core.Alpha(0.7)}},
			StyleFunc: func() core.Style {
				if v.workUnreachable(w()) {
					return core.Style{Opacity: core.Alpha(0.45)}
				}
				return core.Style{Opacity: core.Alpha(1)}
			},
			Children: []core.Node{
				core.View{Children: []core.Node{
					v.poster(cx, goOnEntry(w), 0, 14, false),
					core.View{Style: core.Style{Position: core.Absolute, Top: core.Pt(6), Right: core.Pt(6)}, Children: []core.Node{
						core.Show{When: func() bool { return catalog.Unwatched(w()) > 0 },
							Then: func(cx *core.Scope) core.Node {
								return look.Count{TestID: "films-work-unwatched-" + id, Count: func() int { return catalog.Unwatched(w()) }, Ring: true}
							},
							Else: func(cx *core.Scope) core.Node {
								return core.View{TestID: "films-work-watched-" + id, Role: core.RoleImage, LabelFunc: func() string { return locales.FilmsWorkAllWatched(cx) }, PointerEvents: core.PointerEventsNone,
									Style: core.Style{Width: core.Pt(20), Height: core.Pt(20), CornerRadius: 10, AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter},
									StyleFunc: func() core.Style {
										return core.Style{Background: colors.Surface(), BorderWidth: 1, BorderColor: colors.Line()}
									},
									Children: []core.Node{ui.Icon(lucide.Check, 12, colors.Muted)}}
							}},
					}},
				}},
				core.View{Style: core.Style{Gap: 4}, Children: v.workText(cx, w, id)},
			}},
	}}
}
