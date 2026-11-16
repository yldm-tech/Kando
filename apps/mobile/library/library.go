// Package library is the page of a computer's library: its films with their posters, languages, lengths and sizes, and how far each is on the phone; pull to refresh reads them again. A film opens its own page.
package library

import (
	"context"

	"kando/apps/mobile/locales"
	"kando/apps/mobile/look"
	"kando/apps/mobile/session"
	"kando/apps/mobile/store"
	"kando/packages/sync"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

type filmsSource struct {
	c   *sync.Client
	gen int
}

// Page is the page of library lib of desktop; open opens a film's page.
func Page(cx *core.Scope, s *session.Session, desktop string, lib sync.Library, open func(f sync.Film), back func()) base.NavPage {
	colors := ui.ColorsOf(cx)
	gen := core.NewSignal(cx, 0)
	pulled := core.NewSignal(cx, false)
	films := core.NewResource(cx, func() (filmsSource, bool) {
		c := s.Client(desktop)
		return filmsSource{c, gen.Get()}, c != nil
	}, func(ctx context.Context, src filmsSource, _ core.ResourceInfo) ([]sync.Film, error) {
		return src.c.Films(ctx, lib.ID)
	})
	cx.Effect(func() {
		if !films.Loading() {
			pulled.Set(false)
		}
	})
	list := func() []sync.Film { v, _ := films.Get(); return v }
	name := func() string {
		if d, ok := s.Desktop(desktop); ok {
			return d.Name
		}
		return ""
	}
	note := func(cx *core.Scope, text func() string, action core.Node) core.Node {
		children := []core.Node{core.Text{ContentFunc: text, Style: look.Body.Merge(core.Style{TextAlign: core.TextAlignCenter}), StyleFunc: ui.Colored(colors.Muted)}}
		if action != nil {
			children = append(children, action)
		}
		return core.View{TestID: "library-note", Style: core.Style{Padding: core.All(32), Gap: 12, AlignItems: core.AlignCenter}, Children: children}
	}
	return ui.NavPage(cx, base.NavPage{Title: lib.Name, TestID: "library-page", Children: []core.Node{
		core.Scroll{TestID: "library-scroll", Style: core.Style{FlexGrow: 1, FlexShrink: 1},
			OnRefresh:  func() { pulled.Set(true); gen.Update(func(n int) int { return n + 1 }) },
			Refreshing: pulled.Get,
			Children: []core.Node{core.View{Style: core.Style{PaddingEnv: core.EnvEdges{Bottom: core.EnvSafeArea}, Padding: core.Edges{Top: 8, Bottom: 24}}, Children: []core.Node{
				core.Show{When: func() bool { return films.Loading() && len(list()) == 0 && !pulled.Get() }, Then: func(cx *core.Scope) core.Node {
					return core.View{Style: core.Style{Padding: core.All(32), AlignItems: core.AlignCenter}, Children: []core.Node{ui.Spinner{}}}
				}},
				core.Show{When: func() bool { return films.Err() != nil && !films.Loading() }, Then: func(cx *core.Scope) core.Node {
					return note(cx, func() string {
						switch session.ProblemOf(films.Err()) {
						case session.Gone:
							return locales.LibraryGone(cx, name())
						case session.Unpaired, session.Certificate:
							return locales.HomeUnpaired(cx, name())
						}
						return locales.HomeUnreachable(cx, name())
					}, ui.Button{TestID: "library-retry", Text: locales.CommonRetry(cx), Variant: ui.ButtonSecondary, OnPress: films.Refetch})
				}},
				core.Show{When: func() bool { _, ok := films.Get(); return ok && films.Err() == nil && len(list()) == 0 }, Then: func(cx *core.Scope) core.Node {
					return note(cx, func() string { return locales.LibraryEmpty(cx) }, nil)
				}},
				core.Each(list, func(f sync.Film) string { return f.ID }, func(cx *core.Scope, f func() sync.Film, _ func() int) core.Node {
					return filmRow(cx, s, desktop, f, open)
				}),
			}}},
		},
	}}, ui.NavHeader{OnBack: back})
}

// filmRow is a film of the list: its poster, which the computer sends, its title and facts, and where it stands on the phone.
func filmRow(cx *core.Scope, s *session.Session, desktop string, f func() sync.Film, open func(sync.Film)) core.Node {
	colors := ui.ColorsOf(cx)
	key := func() string { return store.Key(desktop, f().ID) }
	type posterSource struct {
		c  *sync.Client
		id string
	}
	poster := core.NewResource(cx, func() (posterSource, bool) {
		c := s.Client(desktop)
		return posterSource{c, f().ID}, c != nil && f().Poster != ""
	}, func(ctx context.Context, src posterSource, _ core.ResourceInfo) ([]byte, error) {
		return s.Poster(ctx, src.c, desktop, src.id)
	})
	source := func() core.ImageSource {
		if data, ok := poster.Get(); ok && len(data) > 0 {
			return core.ImageSource{Data: data}
		}
		return core.ImageSource{}
	}
	return look.Row{TestID: "library-film-" + f().ID,
		Leading: look.Poster(cx, source, 112, ""),
		Title:   func() string { return f().Title },
		Detail:  func() string { return look.Facts(f()) },
		Trailing: core.Text{TestID: "library-film-status", ContentFunc: func() string { return look.Status(cx, s, key()) },
			Style: look.Caption.Merge(core.Style{MaxWidth: core.Pt(110), TextAlign: core.TextAlignRight}), StyleFunc: ui.Colored(colors.Accent)},
		OnPress: func() { open(f()) }}
}
