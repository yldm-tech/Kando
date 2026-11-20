package films

import (
	"context"
	"slices"
	"strings"
	"time"

	"kando/apps/mobile/catalog"
	"kando/apps/mobile/locales"
	"kando/apps/mobile/look"
	"kando/apps/mobile/session"
	"kando/apps/mobile/store"
	"kando/apps/mobile/theme"
	"kando/packages/sync"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	"github.com/nsgo-dev/nsgo/ui"
)

// results are the films the list shows: the library the switcher shows, narrowed to the chip's source and to what the search finds.
func (v *view) results() []catalog.Result {
	lines, _ := v.lines.Get()
	return catalog.Search(catalog.Filter(v.scoped(), nil, v.source.Get()), lines, v.query.Get())
}

// waiting reports whether a computer is being read with nothing of it to show yet.
func (v *view) waiting() bool {
	return slices.ContainsFunc(v.s.Desktops(), func(d store.Desktop) bool { return v.s.Shelf(d.ID).Waiting() })
}

// narrowed reports whether the list is narrowed to what a search finds or to a source, which can leave nothing of films there are.
func (v *view) narrowed() bool { return v.query.Get() != "" || v.source.Get() != "" }

// list is the films under their heading, with the source chips, or what shows in their place: rows of placeholders while the computers are first read, why there are none, or that the search or the source found none, in a card under the chips. A tablet shows them in a grid of three.
func (v *view) list(cx *core.Scope) core.Node {
	colors := ui.ColorsOf(cx)
	hasFilms := func() bool { return len(v.entries.Get()) > 0 }
	shown := func() bool { return len(v.results()) > 0 }
	return core.View{TestID: "films-list", Style: core.Style{Gap: 12}, Children: []core.Node{
		core.Show{When: hasFilms, Then: func(cx *core.Scope) core.Node {
			return core.View{Style: core.Style{Gap: 12}, Children: []core.Node{
				core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 8}, Children: []core.Node{
					core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1}, Children: []core.Node{look.SectionTitle(cx, func() string {
						if v.query.Get() != "" {
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
				rows = append(rows, look.Row{Index: func() int { return i }, Padding: &rowPadding,
					Leading: ui.SkeletonBlock{Width: core.Pt(112), Height: core.Pt(63), Index: i},
					Below:   ui.Skeleton{Lines: 2}})
			}
			return look.Group{TestID: "films-loading", Children: rows}
		}},
		core.Show{When: func() bool { return !shown() && !v.waiting() && hasFilms() && v.narrowed() }, Then: func(cx *core.Scope) core.Node {
			return look.Group{TestID: "films-empty", Children: []core.Node{
				core.Text{ContentFunc: func() string {
					if v.query.Get() != "" {
						return locales.FilmsNoResults(cx, v.query.Get())
					}
					return locales.FilmsNoSource(cx)
				}, Style: core.Style{FontSize: 14, LineHeight: 19, TextAlign: core.TextAlignCenter, Padding: core.Symmetric(32, 16)}, StyleFunc: ui.Colored(colors.Muted)},
			}}
		}},
		core.Show{When: func() bool {
			listed := slices.ContainsFunc(v.s.Desktops(), func(d store.Desktop) bool { return v.s.Shelf(d.ID).Listed })
			return !shown() && !v.waiting() && !(hasFilms() && v.narrowed()) && (hasFilms() || listed)
		}, Then: func(cx *core.Scope) core.Node {
			return look.Empty{TestID: "films-empty", Icon: lucide.Film, Title: func() string { return locales.FilmsEmpty(cx) }}
		}},
		core.Show{When: shown, Then: func(cx *core.Scope) core.Node {
			each := core.Each(v.results, func(r catalog.Result) string { return r.Key() }, func(cx *core.Scope, r func() catalog.Result, i func() int) core.Node {
				if look.Tablet(cx) {
					return v.cell(cx, r)
				}
				return v.row(cx, r, i)
			})
			if look.Tablet(cx) {
				// Three columns 18 apart: each cell is a third of the row with 9 on either side, the row 9 wider on either side than the column.
				return core.View{TestID: "films-rows", Style: core.Style{Direction: core.Row, Wrap: core.Wrap, RowGap: 22, Margin: core.Edges{Right: -9, Left: -9}}, Children: []core.Node{each}}
			}
			return look.Group{TestID: "films-rows", Children: []core.Node{each}}
		}},
		core.Show{When: func() bool { return len(v.s.Films()) > 0 }, Then: func(cx *core.Scope) core.Node {
			return core.Text{TestID: "films-used", ContentFunc: func() string { return locales.FilmsUsed(cx, look.Size(v.s.Used())) },
				Style: look.Caption.Merge(core.Style{Padding: core.Edges{Right: 16, Left: 16}}), StyleFunc: ui.Colored(colors.Muted)}
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
	return core.Scroll{Horizontal: true, HideIndicator: true, Style: core.Style{FlexGrow: 0, Margin: core.Edges{Right: -16, Left: -16}}, Children: []core.Node{
		core.View{Role: core.RoleRadioGroup, LabelFunc: func() string { return locales.FilmsSource(cx) },
			Style: core.Style{Direction: core.Row, Gap: 8, Padding: core.Symmetric(0, 16)}, Children: chips},
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

// sourceMark is the color and the glyph of a source's mark: a sign, not a word, so it is not translated.
func sourceMark(source string) (core.Color, string) {
	switch source {
	case catalog.SourceYouTube:
		return theme.SourceYouTube, "▶"
	case catalog.SourceBilibili:
		return theme.SourceBilibili, "B"
	case catalog.SourceLocal:
		return theme.SourceLocal, "▣"
	}
	return theme.SourceOther, "◎"
}

// mark is the small square of a film's source before what its row says of it, which the text names too, so the screen reader skips it.
func mark(cx *core.Scope, source func() string) core.Node {
	colors := ui.ColorsOf(cx)
	return core.View{AccessibilityElementsHidden: func() bool { return true }, Style: core.Style{Width: core.Pt(14), Height: core.Pt(14), CornerRadius: 4, FlexShrink: 0, AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter},
		StyleFunc: func() core.Style { fill, _ := sourceMark(source()); return core.Style{Background: fill} },
		Children: []core.Node{core.Text{ContentFunc: func() string { _, glyph := sourceMark(source()); return glyph }, MaxLines: 1,
			Style: core.Style{FontSize: 8, LineHeight: 11, FontWeight: look.Heavy}, StyleFunc: ui.Colored(colors.OnAccent)}}}
}

// rowPadding is the room around a film's row: its poster 12 in from the card's edge, its options button at the card's end.
var rowPadding = core.Edges{Top: 10, Bottom: 10, Left: 12}

// freshFor is how long a film stays new after its computer added it: one not yet watched anywhere wears the new badge on its poster until then.
const freshFor = 72 * time.Hour

// fresh reports whether e's film is new: added in the last freshFor and not watched anywhere yet.
func (v *view) fresh(e catalog.Entry) bool {
	return e.Position.At == 0 && !e.Added.IsZero() && v.s.Now().Sub(e.Added) < freshFor
}

// thumb is e's poster with what the design draws over it: its length at its bottom corner, how much of it was watched along its bottom, and the new badge at its top corner. A tablet's grid draws them larger.
func (v *view) thumb(cx *core.Scope, e func() catalog.Entry, width, radius core.Points, large bool) core.Node {
	colors := ui.ColorsOf(cx)
	length := func() time.Duration { return time.Duration(e().Film.DurationMS) * time.Millisecond }
	pill, text, bar := core.Points(5), core.Style{FontSize: 10, LineHeight: 16, FontWeight: core.FontWeightSemibold}, core.Points(3)
	if large {
		pill, text, bar = 6, core.Style{FontSize: 11, LineHeight: 18, FontWeight: core.FontWeightSemibold}, 4
	}
	badge := text
	badge.FontWeight = core.FontWeightBold
	hidden := func() bool { return true }
	return look.Poster(cx, poster(cx, v.s, e), width, radius, "",
		core.Show{When: func() bool { return length() > 0 }, Then: func(cx *core.Scope) core.Node {
			return core.View{AccessibilityElementsHidden: hidden, Style: core.Style{Position: core.Absolute, Right: core.Pt(4), Bottom: core.Pt(4), Padding: core.Symmetric(0, 5), CornerRadius: pill, Background: theme.PosterPill},
				Children: []core.Node{ui.Tabular(core.Text{ContentFunc: func() string { return look.Duration(length()) }, MaxLines: 1, Style: text, StyleFunc: ui.Colored(colors.OnAccent)})}}
		}},
		core.Show{When: func() bool { return e().Position.At > 0 && length() > 0 }, Then: func(cx *core.Scope) core.Node {
			return core.View{Style: core.Style{Position: core.Absolute, Left: core.Pt(0), Bottom: core.Pt(0), Height: core.Pt(bar), Background: theme.PlayerBlue},
				StyleFunc: func() core.Style {
					return core.Style{Width: core.Pct(float32(min(1, float64(e().Position.At)/float64(length()))) * 100)}
				}}
		}},
		core.Show{When: func() bool { return v.fresh(e()) }, Then: func(cx *core.Scope) core.Node {
			return core.View{AccessibilityElementsHidden: hidden, Style: core.Style{Position: core.Absolute, Left: core.Pt(5), Top: core.Pt(5), Padding: core.Symmetric(0, 6), CornerRadius: pill},
				StyleFunc: ui.Filled(colors.Accent),
				Children:  []core.Node{core.Text{ContentFunc: func() string { return locales.FilmsNew(cx) }, MaxLines: 1, Style: badge, StyleFunc: ui.Colored(colors.OnAccent)}}}
		}})
}

// meta is the line under a film's title: its source's mark, then its facts on one line, in the accent while it downloads and in danger's color once its download failed.
func (v *view) meta(cx *core.Scope, r func() catalog.Result) core.Node {
	colors := ui.ColorsOf(cx)
	key := func() string { return r().Key() }
	tone := func() core.Color {
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
	return core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 5, Margin: core.Edges{Top: 4}}, Children: []core.Node{
		mark(cx, func() string { return catalog.SourceOf(r().Film) }),
		core.Text{ContentFunc: func() string { return v.facts(cx, r().Entry) }, MaxLines: 1, Style: look.Footnote.Merge(core.Style{FlexShrink: 1}), StyleFunc: ui.Colored(tone)},
	}}
}

// hit is the line a search found in a film, under its facts: where it starts and its translation, or the line itself where it has none.
func (v *view) hit(cx *core.Scope, r func() catalog.Result, id string) core.Node {
	colors := ui.ColorsOf(cx)
	return core.Show{When: func() bool { return r().Hit != nil }, Then: func(cx *core.Scope) core.Node {
		return core.Text{TestID: "films-hit-" + id, ContentFunc: func() string {
			h := r().Hit
			if h == nil {
				return ""
			}
			text := h.Text
			if h.Translation != "" {
				text = h.Translation
			}
			return locales.FilmsHit(cx, look.Duration(h.At), text)
		}, MaxLines: 1, Style: look.Footnote.Merge(core.Style{Margin: core.Edges{Top: 3}}), StyleFunc: ui.Colored(colors.Accent)}
	}}
}

// more is the button of a film's options, the system's grey dots in a 44-point target.
func (v *view) more(cx *core.Scope, r func() catalog.Result, id string, size core.Points) core.Node {
	return ui.IconButton{TestID: "films-options-" + id, Icon: moreIcon, Variant: ui.IconButtonGhost, Size: size, IconSize: 16, Tint: func() core.Color { return theme.TabInactive },
		LabelFunc: func() string { return locales.FilmsOptions(cx, r().Film.Title) }, OnPress: func() { v.options(r().Entry) }}
}

// open plays the film of r, from the line the search found in it where there is one, once it is on the phone, and opens its page while it is not, where it is downloaded.
func (v *view) open(r catalog.Result) {
	if k, ok := v.s.Film(r.Key()); ok && k.Done() && r.Hit != nil {
		v.playAt(r.Key(), r.Hit.At)
		return
	}
	v.resume(r.Entry)
}

// row is a film of the list: its poster, its title, its source's mark with what it says of it, the line a search found in it, and its options. Pressed, it plays the film once it is on the phone, else opens its page.
func (v *view) row(cx *core.Scope, r func() catalog.Result, i func() int) core.Node {
	id := r().Desktop + "-" + r().Film.ID
	return look.Row{TestID: "films-film-" + id, Index: i, Padding: &rowPadding, TitleLines: 2,
		TitleStyle: core.Style{FontSize: 15, LineHeight: 20, FontWeight: core.FontWeightSemibold},
		Leading:    v.thumb(cx, func() catalog.Entry { return r().Entry }, 112, 9, false),
		Title:      func() string { return r().Film.Title },
		Below:      core.View{Children: []core.Node{v.meta(cx, r), v.hit(cx, r, id)}},
		Trailing:   v.more(cx, r, id, 44),
		OnPress:    func() { v.open(r()) }}
}

// cell is a film of a tablet's grid: its poster the width of a third of the row, its title with its options at its end, and the line of its source and facts under it.
func (v *view) cell(cx *core.Scope, r func() catalog.Result) core.Node {
	colors := ui.ColorsOf(cx)
	id := r().Desktop + "-" + r().Film.ID
	return core.View{Style: core.Style{Width: core.Pct(100.0 / 3), Padding: core.Symmetric(0, 9)}, Children: []core.Node{
		core.View{TestID: "films-film-" + id, Role: core.RoleButton, OnPress: func(core.PressEvent) { v.open(r()) }, LabelFunc: func() string { return r().Film.Title },
			Style: core.Style{Gap: 10, Active: &core.Style{Opacity: core.Alpha(0.7)}},
			Children: []core.Node{
				v.thumb(cx, func() catalog.Entry { return r().Entry }, 0, 14, true),
				core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignStart}, Children: []core.Node{
					core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0)}, Children: []core.Node{
						core.Text{ContentFunc: func() string { return r().Film.Title }, MaxLines: 2, Style: core.Style{FontSize: 15, LineHeight: 20, FontWeight: core.FontWeightBold}, StyleFunc: ui.Colored(colors.Text)},
						v.meta(cx, r),
						v.hit(cx, r, id),
					}},
					core.View{Style: core.Style{Margin: core.Edges{Top: -10, Right: -8}}, Children: []core.Node{v.more(cx, r, id, 40)}},
				}},
			}},
	}}
}

// facts is what a row says under a film's title: its source, then where it was left or how long it plays, that its computer keeps no video of it, and where it stands on the phone.
func (v *view) facts(cx *core.Scope, e catalog.Entry) string {
	length := time.Duration(e.Film.DurationMS) * time.Millisecond
	when := ""
	switch {
	case catalog.Resumable(e.Position, length):
		when = locales.FilmsWatchedTo(cx, look.Duration(e.Position.At))
	case length > 0:
		when = look.Duration(length)
	}
	parts := []string{sourceName(cx, catalog.SourceOf(e.Film))}
	only := ""
	if e.Film.SubtitlesOnly {
		only = locales.FilmSubtitlesOnly(cx)
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
		_, kept := s.Film(e().Key())
		return source{c, e().Desktop, e().Film.ID}, c != nil && e().Film.Poster != "" && !kept
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
