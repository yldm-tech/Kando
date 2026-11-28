package films

import (
	"context"
	"slices"
	"strings"
	"time"

	"kando/apps/mobile/brand"
	"kando/apps/mobile/catalog"
	"kando/apps/mobile/locales"
	"kando/apps/mobile/look"
	"kando/apps/mobile/session"
	"kando/apps/mobile/store"
	"kando/apps/mobile/theme"
	"kando/packages/sync"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	"github.com/nsgo-dev/nsgo/ui"
)

// results are the films the list shows: the library the switcher shows, narrowed to the chip's source and to what the search finds.
func (v *view) results() []catalog.Result {
	lines, _ := v.lines.Get()
	return catalog.Search(catalog.Filter(v.scoped(), nil, v.source.Get()), lines, v.searched())
}

// rows are the list's rows: the results, the films of a work among them folded into one (catalog.Fold).
func (v *view) rows() []catalog.Row { return catalog.Fold(v.results()) }

// waiting reports whether a computer is being read with nothing of it to show yet.
func (v *view) waiting() bool {
	return slices.ContainsFunc(v.s.Desktops(), func(d store.Desktop) bool { return v.s.Shelf(d.ID).Waiting() })
}

// list is the films under their heading, with the source chips, or what shows in their place: rows of placeholders while the computers are first read, or why there are none.
func (v *view) list(cx *core.Scope) core.Node {
	colors := ui.ColorsOf(cx)
	hasFilms := func() bool { return len(v.entries.Get()) > 0 }
	shown := func() bool { return len(v.results()) > 0 }
	return core.View{TestID: "films-list", Style: core.Style{Gap: 12, Margin: core.Edges{Top: 14}}, Children: []core.Node{
		core.Show{When: hasFilms, Then: func(cx *core.Scope) core.Node {
			return core.View{Style: core.Style{Gap: 10}, Children: []core.Node{
				core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 8}, Children: []core.Node{
					core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1}, Children: []core.Node{look.SectionTitle(cx, func() string {
						if v.searched() != "" {
							return locales.FilmsResults(cx)
						}
						return locales.FilmsAll(cx)
					})}},
					core.Text{TestID: "films-count", ContentFunc: func() string { return locales.FilmsCount(cx, len(v.results())) }, MaxLines: 1, Style: look.Caption, StyleFunc: ui.Colored(colors.Muted)},
				}},
				v.chips(cx),
			}}
		}},
		core.Show{When: func() bool { return !shown() && v.waiting() }, Then: func(cx *core.Scope) core.Node {
			var rows []core.Node
			for i := range 3 {
				rows = append(rows, look.Row{Index: func() int { return i },
					Leading: ui.SkeletonBlock{Width: core.Pt(112), Height: core.Pt(63), Index: i},
					Below:   ui.Skeleton{Lines: 2}})
			}
			return look.Group{TestID: "films-loading", Children: rows}
		}},
		core.Show{When: func() bool {
			listed := slices.ContainsFunc(v.s.Desktops(), func(d store.Desktop) bool { return v.s.Shelf(d.ID).Listed })
			return !shown() && !v.waiting() && (hasFilms() || listed)
		}, Then: func(cx *core.Scope) core.Node {
			// A search or a source that finds nothing says so on a card; a library with no film at all says it on the page.
			return core.Show{When: func() bool { return v.searched() != "" || v.source.Get() != "" },
				Then: func(cx *core.Scope) core.Node {
					return core.View{TestID: "films-empty", Style: core.Style{Padding: core.Symmetric(32, 16), AlignItems: core.AlignCenter}, StyleFunc: func() core.Style { return look.Card(cx) },
						Children: []core.Node{core.Text{ContentFunc: func() string {
							if q := v.searched(); q != "" {
								return locales.FilmsNoResults(cx, q)
							}
							return locales.FilmsNoSource(cx)
						}, Style: core.Style{FontSize: 14, LineHeight: 19, TextAlign: core.TextAlignCenter}, StyleFunc: ui.Colored(colors.Muted)}}}
				},
				Else: func(cx *core.Scope) core.Node {
					return look.Empty{TestID: "films-empty", Icon: lucide.Film, Title: func() string { return locales.FilmsEmpty(cx) }}
				}}
		}},
		core.Show{When: shown, Then: func(cx *core.Scope) core.Node {
			// A work's row is keyed apart from every film's (catalog.Work.Key), so a row stays a work's or a film's for as long as it shows.
			each := core.Each(v.rows, catalog.Row.Key, func(cx *core.Scope, r func() catalog.Row, i func() int) core.Node {
				if r().Work != nil {
					if look.Tablet(cx) {
						return v.workCell(cx, r)
					}
					return v.workRow(cx, r, i)
				}
				result := func() catalog.Result { return r().Result }
				if look.Tablet(cx) {
					return v.cell(cx, result)
				}
				return v.row(cx, result, i)
			})
			if look.Tablet(cx) {
				// A tablet lays the films out in three columns of cards (FP-23).
				return core.View{TestID: "films-rows", Style: core.Style{Direction: core.Row, Wrap: core.Wrap, RowGap: 22, Margin: core.Edges{Right: -8, Left: -8}}, Children: []core.Node{each}}
			}
			return core.View{TestID: "films-rows", StyleFunc: func() core.Style {
				st := look.Card(cx)
				st.Overflow = core.Hidden
				return st
			}, Children: []core.Node{each}}
		}},
		core.Show{When: func() bool { return len(v.s.Films()) > 0 }, Then: func(cx *core.Scope) core.Node {
			return core.Text{TestID: "films-used", ContentFunc: func() string { return locales.FilmsUsed(cx, look.Size(v.s.Used())) },
				Style: look.Footnote.Merge(core.Style{Padding: core.Edges{Right: 16, Left: 16}, TextAlign: core.TextAlignCenter}), StyleFunc: ui.Colored(colors.Muted)}
		}},
	}}
}

// chips narrow the list to a source, or show every source.
func (v *view) chips(cx *core.Scope) core.Node {
	chip := func(source string, text func() string) core.Node {
		id := source
		if id == "" {
			id = "all"
		}
		return look.Chip{TestID: "films-source-" + id, Radio: true, Filter: true, Text: text, On: func() bool { return v.source.Get() == source }, OnPress: func() { v.source.Set(source) }}
	}
	chips := []core.Node{chip("", func() string { return locales.FilmsSourceAll(cx) })}
	for _, src := range catalog.Sources {
		chips = append(chips, chip(src, func() string { return sourceName(cx, src) }))
	}
	side := core.Points(16)
	if look.Tablet(cx) {
		side = 32
	}
	return core.Scroll{Horizontal: true, HideIndicator: true, Style: core.Style{FlexGrow: 0, Margin: core.Edges{Right: -side, Left: -side}}, Children: []core.Node{
		core.View{Role: core.RoleRadioGroup, LabelFunc: func() string { return locales.FilmsSource(cx) },
			Style: core.Style{Direction: core.Row, Gap: 8, Padding: core.Symmetric(0, side)}, Children: chips},
	}}
}

// sourceName is a source as the chips and the rows name it.
func sourceName(cx *core.Scope, source string) string {
	switch source {
	case catalog.SourceYouTube:
		return locales.FilmsSourceYoutube(cx)
	case catalog.SourceBilibili:
		return locales.FilmsSourceBilibili(cx)
	case catalog.SourceLocal:
		return locales.FilmsSourceLocal(cx)
	}
	return locales.FilmsSourceWeb(cx)
}

// sourceMark is a source's mark before a row's meta: a 14-point tile in the source's color with its glyph, drawn alike in both appearances.
func sourceMark(source func() string) core.Node {
	fill := func() core.Color {
		switch source() {
		case catalog.SourceYouTube:
			return theme.SourceYouTube
		case catalog.SourceBilibili:
			return theme.SourceBilibili
		case catalog.SourceLocal:
			return theme.SourceLocal
		}
		return theme.SourceOther
	}
	glyph := func() *core.SVG {
		switch source() {
		case catalog.SourceYouTube:
			return markPlay
		case catalog.SourceBilibili:
			return markB
		case catalog.SourceLocal:
			return markFile
		}
		return markWeb
	}
	return core.View{AccessibilityElementsHidden: func() bool { return true }, Style: core.Style{Width: core.Pt(14), Height: core.Pt(14), CornerRadius: 4, FlexShrink: 0, AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter},
		StyleFunc: func() core.Style { return core.Style{Background: fill()} },
		Children:  []core.Node{ui.IconOf(glyph, 8, func() core.Color { return core.RGB(0xFFFFFF) })}}
}

// The source marks' glyphs, 8 points square: YouTube's play, Bilibili's B, a web page's ring and a file's square.
var (
	markPlay = core.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="8" height="8" viewBox="0 0 8 8"><path d="M2 1l5 3-5 3z" fill="currentColor"/></svg>`))
	markB    = core.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="8" height="8" viewBox="0 0 8 8" fill="none" stroke="currentColor" stroke-width="1.4" stroke-linejoin="round"><path d="M2 .9h2.3a1.5 1.5 0 0 1 0 3H2zM2 3.9h2.6a1.6 1.6 0 0 1 0 3.2H2z"/></svg>`))
	markWeb  = core.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="8" height="8" viewBox="0 0 8 8" fill="none" stroke="currentColor" stroke-width="1.3"><circle cx="4" cy="4" r="3"/><circle cx="4" cy="4" r="1" fill="currentColor"/></svg>`))
	markFile = core.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="8" height="8" viewBox="0 0 8 8" fill="none" stroke="currentColor" stroke-width="1.3"><rect x="1" y="1" width="6" height="6" rx="1"/><rect x="3" y="3" width="2" height="2" fill="currentColor" stroke="none"/></svg>`))
)

// switcher is the pill at the top of the page that picks the library the list shows, or every library: the app's icon, the library's name and a chevron.
func (v *view) switcher(cx *core.Scope) core.Node {
	colors := ui.ColorsOf(cx)
	// The pill names the library alone, as the design's does; its spoken name says its computer too.
	name := func() string {
		if lib := v.library.Get(); lib.Desktop != "" {
			return lib.Name
		}
		return locales.FilmsAllLibraries(cx)
	}
	return base.Button{TestID: "films-library", LabelFunc: func() string { return locales.FilmsLibrary(cx, LibraryName(cx, v.library)) }, OnPress: func(core.PressEvent) { v.act.PickLibrary() },
		Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 6, Height: core.Pt(32), Padding: core.Edges{Left: 4, Right: 10}, CornerRadius: 16, FlexShrink: 1, MinWidth: core.Pt(0),
			Active: &core.Style{Opacity: core.Alpha(0.6)}},
		StyleFunc: func() core.Style { return core.Style{FocusVisible: ui.FocusRing(cx, -2)} },
		Children: []core.Node{
			core.Image{Source: brand.Icon(), ResizeMode: core.Cover, ElementsHidden: true, Style: core.Style{Width: core.Pt(26), Height: core.Pt(26), CornerRadius: 6, BorderCurve: core.BorderCurveContinuous, FlexShrink: 0}},
			core.Text{ContentFunc: name, MaxLines: 1, Style: core.Style{FontSize: 13, LineHeight: 18, FontWeight: core.FontWeightSemibold, FlexShrink: 1}, StyleFunc: ui.Colored(theme.Of(cx).Subtext)},
			ui.Icon(lucide.ChevronDown, 9, colors.Muted),
		}}
}

// LibraryName is the library the Library shows, as the switcher names it, or all of them; read it in a binding.
func LibraryName(cx *core.Scope, library *core.Signal[catalog.Scope]) string {
	if lib := library.Get(); lib.Desktop != "" {
		return libraryName(cx, lib)
	}
	return locales.FilmsAllLibraries(cx)
}

// libraryName is a library as the switcher names it: with its computer's name, which a library only the phone still knows of may not have.
func libraryName(cx *core.Scope, lib catalog.Scope) string {
	if lib.Computer == "" {
		return lib.Name
	}
	return locales.FilmsLibraryOn(cx, lib.Name, lib.Computer)
}

// thumb is a film's poster with what the design draws on it: its length at its corner and, for a film left partway, how far along its foot.
func (v *view) thumb(cx *core.Scope, e func() catalog.Entry, width core.Points, radius core.Points) core.Node {
	return v.poster(cx, e, width, radius, true)
}

// poster is thumb, its length left out unless withLength, as on a work's row, which stands for more than the film it shows.
func (v *view) poster(cx *core.Scope, e func() catalog.Entry, width core.Points, radius core.Points, withLength bool) core.Node {
	colors := ui.ColorsOf(cx)
	length := func() time.Duration { return time.Duration(e().Film.DurationMS) * time.Millisecond }
	fraction := func() float32 {
		if !catalog.Resumable(e().Position, length()) {
			return 0
		}
		return min(float32(e().Position.At)/float32(length()), 1)
	}
	size := core.Style{Width: core.Pt(width), AspectRatio: 16.0 / 9}
	mark := core.Points(20)
	if width == 0 {
		size = core.Style{Width: core.Pct(100), AspectRatio: 16.0 / 9}
		mark = 32
	}
	src, blank, failed := posterOf(cx, v.s, e)
	return core.View{AccessibilityElementsHidden: func() bool { return true }, Style: size.Merge(core.Style{CornerRadius: radius, BorderCurve: core.BorderCurveContinuous, Overflow: core.Hidden, FlexShrink: 0,
		AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter}),
		StyleFunc: ui.Filled(colors.Inset),
		Children: []core.Node{
			// A film with no poster, or one the platform could not draw, shows a film's mark on the fill, as a job's thumbnail does.
			core.Show{When: blank, Then: func(cx *core.Scope) core.Node {
				return core.View{TestID: "films-poster-mark-" + e().Desktop + "-" + e().Film.ID, Children: []core.Node{ui.Icon(lucide.Film, mark, colors.Faint)}}
			}},
			core.Image{SourceFunc: src, ResizeMode: core.Cover, ElementsHidden: true, OnError: failed,
				Style: core.Style{Position: core.Absolute, Left: core.Pt(0), Top: core.Pt(0), Right: core.Pt(0), Bottom: core.Pt(0)}},
			core.Show{When: func() bool { return withLength && length() > 0 }, Then: func(cx *core.Scope) core.Node {
				return core.View{Style: core.Style{Position: core.Absolute, Right: core.Pt(4), Bottom: core.Pt(4), Height: core.Pt(16), Padding: core.Symmetric(0, 5), CornerRadius: 5, JustifyContent: core.JustifyCenter,
					Background: core.RGBA(0x000000, 0.72)},
					Children: []core.Node{ui.Tabular(core.Text{ContentFunc: func() string { return look.Duration(length()) }, MaxLines: 1,
						Style: core.Style{FontSize: 10, LineHeight: 16, FontWeight: core.FontWeightSemibold, Color: core.RGB(0xFFFFFF)}})}}
			}},
			core.Show{When: func() bool { return fraction() > 0 }, Then: func(cx *core.Scope) core.Node {
				return core.View{Style: core.Style{Position: core.Absolute, Left: core.Pt(0), Bottom: core.Pt(0), Height: core.Pt(3), Background: theme.PlayerBlue},
					StyleFunc: func() core.Style { return core.Style{Width: core.Pct(fraction() * 100)} }}
			}},
		}}
}

// meta is a film's line under its title: its source's mark, then its source, where it was left or its length, and where it stands on the phone, on one line.
func (v *view) meta(cx *core.Scope, r func() catalog.Result, tone func() core.Color) core.Node {
	return core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 5}, Children: []core.Node{
		sourceMark(func() string { return catalog.SourceOf(r().Film) }),
		core.Text{ContentFunc: func() string { return v.facts(cx, r().Entry) }, MaxLines: 1, Style: look.Footnote.Merge(core.Style{FlexShrink: 1}), StyleFunc: ui.Colored(tone)},
	}}
}

// hit is the line a search found in a film, its time and its words in the accent, on one line.
func (v *view) hit(cx *core.Scope, r func() catalog.Result, id string) core.Node {
	colors := ui.ColorsOf(cx)
	return core.Show{When: func() bool { return r().Hit != nil }, Then: func(cx *core.Scope) core.Node {
		return core.Text{TestID: "films-hit-" + id, ContentFunc: func() string {
			h := r().Hit
			if h == nil {
				return ""
			}
			return locales.FilmsHit(cx, look.Duration(h.At), h.Text)
		}, MaxLines: 1, Style: look.Footnote, StyleFunc: ui.Colored(colors.Accent)}
	}}
}

// detailTone is the color of a film's meta: muted, the accent while it downloads, danger's once its download failed.
func (v *view) detailTone(cx *core.Scope, key func() string) func() core.Color {
	colors := ui.ColorsOf(cx)
	return func() core.Color {
		k, ok := v.s.Film(key())
		t, moving := v.s.Transfer(key())
		switch {
		case !ok || k.Done() || !moving:
			return colors.Muted()
		case t.State == session.Failed:
			return colors.Danger()
		}
		return colors.Accent()
	}
}

// open is what pressing a film does: plays a film on the phone, or one streamed from its computer, storage, NAS or server, from the line the search found, or where it was left (openFilm), else opens the film's page.
func (v *view) open(r catalog.Result) {
	v.feed(v.keys)
	if h := r.Hit; h != nil {
		if k, ok := v.s.Film(r.Key()); ok && (k.Done() || (k.Streamed && v.s.StreamLink(r.Key()) != "")) || v.s.Matched(r.Key()) {
			v.playAt(r.Key(), h.At)
			return
		}
	}
	v.openFilm(r.Entry)
}

// row is a film of the list: its poster with its length and how far it was watched, its title, its source's mark with its source, where it was left or its length and where it stands on the phone, the line a search found in it, its download's bar, and its options. Pressed, it opens the film's page, or plays a film on the phone from the line the search found. While the user's storage fails the phone, a film not on it fades.
func (v *view) row(cx *core.Scope, r func() catalog.Result, i func() int) core.Node {
	colors := ui.ColorsOf(cx)
	s := v.s
	// What stands on the phone for a film of a NAS matched to it is its copy there (session.Copy).
	key := func() string { return s.Copy(r().Key()) }
	transfer := func() (session.Transfer, bool) { return s.Transfer(key()) }
	kept := func() (store.Film, bool) { return s.Film(key()) }
	id := r().Desktop + "-" + r().Film.ID
	return core.View{TestID: "films-film-" + id, Role: core.RoleButton,
		OnPress: func(core.PressEvent) { v.open(r()) },
		Style:   core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 12, Padding: core.Edges{Top: 10, Bottom: 10, Left: 12}, MinHeight: core.Pt(83)},
		StyleFunc: func() core.Style {
			st := core.Style{Active: &core.Style{Background: colors.Pressed()}, Opacity: core.Alpha(1)}
			if v.unreachable(r().Desktop, key()) {
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
			v.thumb(cx, func() catalog.Entry { return r().Entry }, 112, 9),
			core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0), Gap: 4}, Children: []core.Node{
				core.Text{ContentFunc: func() string { return r().Film.Title }, MaxLines: 2, Style: core.Style{FontSize: 15, LineHeight: 20, FontWeight: core.FontWeightSemibold}, StyleFunc: ui.Colored(colors.Text)},
				v.meta(cx, r, v.detailTone(cx, key)),
				v.hit(cx, r, id),
				core.Show{When: func() bool {
					k, ok := kept()
					t, moving := transfer()
					return ok && !k.Done() && moving && t.State != session.Failed
				}, Then: func(cx *core.Scope) core.Node {
					return ui.Progress{LabelFunc: func() string { return locales.FilmDownload(cx) }, Value: func() float32 { t, _ := transfer(); return t.Fraction() * 100 }, Style: core.Style{Margin: core.Edges{Top: 2}}}
				}},
			}},
			v.more(cx, r, id, 44),
		}}
}

// more is a film's options button: the design's grey dots in a target size points square.
func (v *view) more(cx *core.Scope, r func() catalog.Result, id string, size core.Points) core.Node {
	return ui.IconButton{TestID: "films-options-" + id, Icon: dotsIcon, Variant: ui.IconButtonGhost, Size: size, IconSize: 16, Tint: func() core.Color { return theme.TabInactive },
		LabelFunc: func() string { return locales.FilmsOptions(cx, r().Film.Title) }, OnPress: func() { v.options(cx, r().Entry) }}
}

// dotsIcon is the design's three dots, 16 by 4.
func dotsIcon() *core.SVG { return dots }

var dots = core.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 16 16" fill="currentColor"><circle cx="2" cy="8" r="2"/><circle cx="8" cy="8" r="2"/><circle cx="14" cy="8" r="2"/></svg>`))

// cell is a film of a tablet's grid: its poster with its length and how far it was watched, its title, its meta and the line a search found, and its options beside the title, at the poster's foot. Pressed, it does what a row does.
func (v *view) cell(cx *core.Scope, r func() catalog.Result) core.Node {
	colors := ui.ColorsOf(cx)
	key := func() string { return v.s.Copy(r().Key()) }
	id := r().Desktop + "-" + r().Film.ID
	return core.View{Style: core.Style{Width: core.Pct(33.333), Padding: core.Symmetric(0, 8)}, Children: []core.Node{
		core.View{TestID: "films-film-" + id, Role: core.RoleButton,
			OnPress: func(core.PressEvent) { v.open(r()) },
			Style:   core.Style{Gap: 8, Active: &core.Style{Opacity: core.Alpha(0.7)}},
			StyleFunc: func() core.Style {
				if v.unreachable(r().Desktop, key()) {
					return core.Style{Opacity: core.Alpha(0.45)}
				}
				return core.Style{Opacity: core.Alpha(1)}
			},
			Children: []core.Node{
				v.thumb(cx, func() catalog.Entry { return r().Entry }, 0, 14),
				core.View{Style: core.Style{Gap: 4}, Children: []core.Node{
					core.Text{ContentFunc: func() string { return r().Film.Title }, MaxLines: 2, Style: core.Style{FontSize: 15, LineHeight: 20, FontWeight: core.FontWeightBold, Padding: core.Edges{Right: 30}}, StyleFunc: ui.Colored(colors.Text)},
					v.meta(cx, r, v.detailTone(cx, key)),
					v.hit(cx, r, id),
				}},
			}},
		// The options sit beside the title, just under the poster (FP-23: a 40-point button 8 past the card's end, 2 over the poster's foot): a box the poster's size, over it and letting presses through, holds them at its foot. They stay out of the card's button, which assistive technology reads as one.
		core.View{PointerEvents: core.PointerEventsBoxNone, Style: core.Style{Position: core.Absolute, Left: core.Pt(8), Right: core.Pt(8), Top: core.Pt(0), AspectRatio: 16.0 / 9},
			Children: []core.Node{core.View{Style: core.Style{Position: core.Absolute, Right: core.Pt(-8), Bottom: core.Pt(-38)}, Children: []core.Node{v.more(cx, r, id, 40)}}}},
	}}
}

// facts is what a row says under a film's title: its source, then where it was left or how long it plays, that its computer keeps no video of it or is still making it, and where it stands on the phone.
func (v *view) facts(cx *core.Scope, e catalog.Entry) string {
	length := time.Duration(e.Film.DurationMS) * time.Millisecond
	when := ""
	switch {
	case catalog.Resumable(e.Position, length):
		when = locales.FilmsWatchedTo(cx, look.Duration(e.Position.At))
	case length > 0:
		when = look.Duration(length)
	}
	parts := []string{look.FilmSource(cx, v.s, e.Desktop, sourceName(cx, catalog.SourceOf(e.Film)))}
	only := ""
	switch {
	case e.Film.SubtitlesOnly:
		only = locales.FilmSubtitlesOnly(cx)
	case e.Film.Making:
		only = locales.FilmMaking(cx)
	}
	for _, p := range []string{when, only, look.Status(cx, v.s, e.Key())} {
		if p != "" {
			parts = append(parts, p)
		}
	}
	return strings.Join(parts, " · ")
}

// poster is the poster of e's film: the phone's file once it is kept, else the one its computer sends, none for a computer no longer paired.
func poster(cx *core.Scope, s *session.Session, e func() catalog.Entry) func() core.ImageSource {
	type source struct {
		c           *sync.Client
		desktop, id string
	}
	remote := core.NewResource(cx, func() (source, bool) {
		c := s.Client(e().Desktop)
		k, kept := s.Film(e().Key())
		return source{c, e().Desktop, e().Film.ID}, c != nil && e().Film.Poster != "" && (!kept || k.Poster == "")
	}, func(ctx context.Context, src source, _ core.ResourceInfo) ([]byte, error) {
		return s.Poster(ctx, src.c, src.desktop, src.id)
	})
	return func() core.ImageSource {
		if k, ok := s.Film(e().Key()); ok && k.Poster != "" {
			return look.LocalPoster(s, k)
		}
		if data, ok := remote.Get(); ok && len(data) > 0 {
			return core.ImageSource{Data: data}
		}
		return core.ImageSource{}
	}
}

// posterOf is poster with what tells a film with no poster to show: blank reports that it has none, or that the platform could not draw the one it has, which failed, an Image's OnError, records. A new source is drawn again.
func posterOf(cx *core.Scope, s *session.Session, e func() catalog.Entry) (src func() core.ImageSource, blank func() bool, failed func(error)) {
	src = poster(cx, s, e)
	broken := core.NewSignalFunc(cx, core.ImageSource{}, nil)
	blank = func() bool {
		got := src()
		return got.Empty() || broken.Get().Same(got)
	}
	failed = func(error) { broken.Set(core.Untracked(cx, src)) }
	return src, blank, failed
}
