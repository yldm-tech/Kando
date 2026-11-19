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

// list is the films under their heading, with the library switcher and the source chips, or what shows in their place: rows of placeholders while the computers are first read, or why there are none.
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
					core.Text{TestID: "films-count", ContentFunc: func() string { return locales.FilmsCount(cx, len(v.results())) }, MaxLines: 1, Style: look.Subhead, StyleFunc: ui.Colored(colors.Muted)},
				}},
				// The switcher shows while a library is chosen even where it is the only one left, as its computer did not answer, so the list can be taken back to every library.
				core.Show{When: func() bool {
					return v.library.Get().Desktop != "" || len(catalog.Libraries(v.s.Computers(), v.entries.Get())) > 1
				}, Then: v.switcher},
				v.chips(cx),
			}}
		}},
		core.Show{When: func() bool { return !shown() && v.waiting() }, Then: func(cx *core.Scope) core.Node {
			var rows []core.Node
			for i := range 3 {
				rows = append(rows, look.Row{Index: func() int { return i }, Inset: 140,
					Leading: ui.SkeletonBlock{Width: core.Pt(112), Height: core.Pt(63), Index: i},
					Below:   ui.Skeleton{Lines: 2}})
			}
			return look.Group{TestID: "films-loading", Children: rows}
		}},
		core.Show{When: func() bool {
			listed := slices.ContainsFunc(v.s.Desktops(), func(d store.Desktop) bool { return v.s.Shelf(d.ID).Listed })
			return !shown() && !v.waiting() && (hasFilms() || listed)
		}, Then: func(cx *core.Scope) core.Node {
			return look.Empty{TestID: "films-empty",
				Icon: func() *core.SVG {
					if v.query.Get() != "" {
						return lucide.Search()
					}
					return lucide.Film()
				},
				Title: func() string {
					switch {
					case v.query.Get() != "":
						return locales.FilmsNoResults(cx, v.query.Get())
					case v.source.Get() != "":
						return locales.FilmsNoSource(cx)
					}
					return locales.FilmsEmpty(cx)
				}}
		}},
		core.Show{When: shown, Then: func(cx *core.Scope) core.Node {
			return look.Group{TestID: "films-rows", Children: []core.Node{
				core.Each(v.results, func(r catalog.Result) string { return r.Key() }, func(cx *core.Scope, r func() catalog.Result, i func() int) core.Node {
					return v.row(cx, r, i)
				}),
			}}
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
		return look.Chip{TestID: "films-source-" + id, Radio: true, Text: text, On: func() bool { return v.source.Get() == source }, OnPress: func() { v.source.Set(source) }}
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

// switcher is the button that picks the library the list shows, or every library: one of a computer, or one only the phone still knows of.
func (v *view) switcher(cx *core.Scope) core.Node {
	colors := ui.ColorsOf(cx)
	name := func() string {
		if lib := v.library.Get(); lib.Desktop != "" {
			return libraryName(cx, lib)
		}
		return locales.FilmsAllLibraries(cx)
	}
	pick := func() {
		libs := catalog.Libraries(v.s.Computers(), v.entries.GetUntracked())
		actions := []core.AlertAction{{Label: locales.FilmsAllLibraries(cx)}}
		for _, l := range libs {
			actions = append(actions, core.AlertAction{Label: libraryName(cx, l)})
		}
		actions = append(actions, core.AlertAction{Label: locales.CommonCancel(cx), Role: core.AlertCancel})
		cx.ShowAlert(core.AlertConfig{Title: locales.FilmsLibraries(cx), Sheet: !look.Tablet(cx), Cancelable: true, Actions: actions}, func(i int) {
			switch {
			case i == 0:
				v.library.Set(catalog.Scope{})
			case i > 0 && i <= len(libs):
				v.library.Set(libs[i-1])
			}
		})
	}
	return core.View{Style: core.Style{Direction: core.Row}, Children: []core.Node{
		core.View{TestID: "films-library", Role: core.RoleButton, LabelFunc: func() string { return locales.FilmsLibrary(cx, name()) }, OnPress: func(core.PressEvent) { pick() },
			Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 6, MinHeight: core.Pt(44), Padding: core.Symmetric(0, 4), FlexShrink: 1},
			StyleFunc: func() core.Style {
				return core.Style{Color: colors.Accent(), Active: &core.Style{Opacity: core.Alpha(0.6)}}
			},
			Children: []core.Node{
				ui.Icon(lucide.Library, 18, colors.Accent),
				core.Text{ContentFunc: name, MaxLines: 1, Style: look.Body.Merge(core.Style{FontWeight: core.FontWeightSemibold, FlexShrink: 1})},
				ui.Icon(lucide.ChevronDown, 16, colors.Accent),
			}},
	}}
}

// libraryName is a library as the switcher names it: with its computer's name, which a library only the phone still knows of may not have.
func libraryName(cx *core.Scope, lib catalog.Scope) string {
	if lib.Computer == "" {
		return lib.Name
	}
	return locales.FilmsLibraryOn(cx, lib.Name, lib.Computer)
}

// row is a film of the list: its poster, its title, its source with where it was left or its length, where it stands on the phone, the line a search found in it, and its options. Pressed, it opens the film's page, or plays a film on the phone from the line the search found.
func (v *view) row(cx *core.Scope, r func() catalog.Result, i func() int) core.Node {
	colors := ui.ColorsOf(cx)
	s := v.s
	key := func() string { return r().Key() }
	transfer := func() (session.Transfer, bool) { return s.Transfer(key()) }
	kept := func() (store.Film, bool) { return s.Film(key()) }
	id := r().Desktop + "-" + r().Film.ID
	return look.Row{TestID: "films-film-" + id, Index: i, Inset: 140,
		Leading: look.Poster(cx, poster(cx, s, func() catalog.Entry { return r().Entry }), 112, ""),
		Title:   func() string { return r().Film.Title },
		Detail:  func() string { return v.facts(cx, r().Entry) },
		DetailTone: func() look.Tone {
			k, ok := kept()
			t, moving := transfer()
			switch {
			case !ok || k.Done() || !moving:
				return look.ToneDefault
			case t.State == session.Failed:
				return look.ToneDanger
			}
			return look.ToneAccent
		},
		Below: core.View{Children: []core.Node{
			core.Show{When: func() bool { return r().Hit != nil }, Then: func(cx *core.Scope) core.Node {
				return core.Text{TestID: "films-hit-" + id, ContentFunc: func() string {
					h := r().Hit
					if h == nil {
						return ""
					}
					return locales.FilmsHit(cx, look.Duration(h.At), h.Text)
				}, MaxLines: 2, Style: look.Subhead, StyleFunc: ui.Colored(colors.Accent)}
			}},
			core.Show{When: func() bool {
				k, ok := kept()
				t, moving := transfer()
				return ok && !k.Done() && moving && t.State != session.Failed
			}, Then: func(cx *core.Scope) core.Node {
				// TODO(nsgo#145): ui.Progress.Label is a string only (ui/progress.go:12-19), so the bar's spoken name keeps the language it was built in; pass a LabelFunc once Progress has one https://github.com/nsgo-dev/nsgo/issues/145
				return ui.Progress{Label: locales.FilmDownload(cx), Value: func() float32 { t, _ := transfer(); return t.Fraction() * 100 }, Style: core.Style{Margin: core.Edges{Top: 6}}}
			}},
		}},
		Trailing: ui.IconButton{TestID: "films-options-" + id, Icon: lucide.Ellipsis, Variant: ui.IconButtonGhost, Size: 44, IconSize: 20,
			LabelFunc: func() string { return locales.FilmsOptions(cx, r().Film.Title) }, OnPress: func() { v.options(cx, r().Entry) }},
		OnPress: func() {
			if h := r().Hit; h != nil {
				if k, ok := kept(); ok && k.Done() {
					v.playAt(key(), h.At)
					return
				}
			}
			v.act.Film(r().Entry)
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
