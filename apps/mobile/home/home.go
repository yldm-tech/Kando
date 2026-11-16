// Package home is the app's first page: the films downloaded to the phone, which play offline, with the space they take, and under each paired computer its libraries, which pull to refresh reads again. With nothing paired and nothing downloaded it offers to pair.
package home

import (
	"context"
	"maps"
	"path/filepath"
	"slices"

	"kando/apps/mobile/locales"
	"kando/apps/mobile/look"
	"kando/apps/mobile/session"
	"kando/apps/mobile/store"
	kcore "kando/packages/core"
	"kando/packages/sync"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	"github.com/nsgo-dev/nsgo/ui"
)

// Actions are where the page leads.
type Actions struct {
	Pair, Desktops func()
	Library        func(desktop string, lib sync.Library)
	Film           func(f store.Film)
}

// Page is the home page.
func Page(cx *core.Scope, s *session.Session, act Actions) base.NavPage {
	empty := func() bool { return len(s.Desktops()) == 0 && len(s.Films()) == 0 }
	// gen counts the pulls to refresh, which the computers' sections read their libraries again on; loading is which of them are still reading.
	gen := core.NewSignal(cx, 0)
	loading := core.NewSignalFunc(cx, map[string]bool{}, nil)
	pulled := core.NewSignal(cx, false)
	cx.Effect(func() {
		if !pulled.Get() {
			return
		}
		busy := loading.Get()
		if !slices.ContainsFunc(s.Desktops(), func(d store.Desktop) bool { return busy[d.ID] }) {
			pulled.Set(false)
		}
	})
	content := core.Scroll{TestID: "home-scroll", Style: core.Style{FlexGrow: 1, FlexShrink: 1},
		OnRefresh:  func() { pulled.Set(true); gen.Update(func(n int) int { return n + 1 }) },
		Refreshing: pulled.Get,
		Children: []core.Node{core.View{Style: core.Style{PaddingEnv: core.EnvEdges{Bottom: core.EnvSafeArea}, Padding: core.Edges{Bottom: 24}}, Children: []core.Node{
			downloads(cx, s, act),
			core.Each(s.Desktops, func(d store.Desktop) string { return d.ID }, func(cx *core.Scope, d func() store.Desktop, _ func() int) core.Node {
				return computer(cx, s, d, gen.Get, act, func(busy bool) {
					id := d().ID
					loading.Update(func(m map[string]bool) map[string]bool { m = maps.Clone(m); m[id] = busy; return m })
				})
			}),
		}}},
	}
	body := core.Show{When: s.Loaded, Else: func(cx *core.Scope) core.Node {
		return core.View{Style: core.Style{FlexGrow: 1, AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter}, Children: []core.Node{ui.Spinner{}}}
	}, Then: func(cx *core.Scope) core.Node {
		return core.Show{When: empty, Then: func(cx *core.Scope) core.Node { return welcome(cx, act.Pair) }, Else: func(*core.Scope) core.Node { return content }}
	}}
	return ui.NavPage(cx, base.NavPage{TitleFunc: func() string { return locales.AppName(cx) }, TestID: "home-page", Children: []core.Node{body}},
		ui.NavHeader{Trailing: []core.Node{ui.IconButton{TestID: "home-desktops", Icon: lucide.Monitor, LabelFunc: func() string { return locales.DesktopsTitle(cx) }, Variant: ui.IconButtonGhost, OnPress: act.Desktops}}})
}

// welcome is the page before anything is paired or downloaded.
func welcome(cx *core.Scope, pair func()) core.Node {
	colors := ui.ColorsOf(cx)
	return core.View{TestID: "home-welcome", Style: core.Style{FlexGrow: 1, AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter, Gap: 12, Padding: core.All(32)}, Children: []core.Node{
		core.Icon{SVG: lucide.Monitor(), Style: core.Style{Width: core.Pt(44), Height: core.Pt(44)}, StyleFunc: ui.Colored(colors.Muted)},
		core.Text{ContentFunc: func() string { return locales.HomeEmptyTitle(cx) }, Role: core.RoleHeader, Style: look.Title.Merge(core.Style{TextAlign: core.TextAlignCenter}), StyleFunc: ui.Colored(colors.Text)},
		core.Text{ContentFunc: func() string { return locales.HomeEmptyBody(cx) }, Style: look.Body.Merge(core.Style{TextAlign: core.TextAlignCenter, MaxWidth: core.Pt(420)}), StyleFunc: ui.Colored(colors.Muted)},
		core.View{Style: core.Style{Height: core.Pt(8)}},
		ui.Button{TestID: "home-pair", Text: locales.HomePair(cx), Size: ui.SizeLarge, OnPress: pair},
	}}
}

// downloads is the section of the films on the phone, with the space they take, shown while there are any.
func downloads(cx *core.Scope, s *session.Session, act Actions) core.Node {
	return core.Show{When: func() bool { return len(s.Films()) > 0 }, Then: func(cx *core.Scope) core.Node {
		return core.View{TestID: "home-downloads", Children: []core.Node{
			look.SectionTitle(cx, func() string { return locales.HomeDownloads(cx) }, func() string { return locales.HomeSpaceUsed(cx, look.Size(s.Used())) }, "home-downloads-title"),
			core.Each(s.Films, func(f store.Film) string { return f.Key() }, func(cx *core.Scope, f func() store.Film, _ func() int) core.Node {
				return look.Row{TestID: "home-film-" + f().Film.ID,
					Leading: look.Poster(cx, func() core.ImageSource { return LocalPoster(s, f()) }, 96, ""),
					Title:   func() string { return f().Film.Title },
					Detail: func() string {
						if st := look.Status(cx, s, f().Key()); st != "" && !f().Done() {
							return st
						}
						return look.Facts(f().Film)
					},
					OnPress: func() { act.Film(f()) }}
			}),
		}}
	}}
}

// LocalPoster is the poster of a film kept on the phone, none until it is downloaded.
func LocalPoster(s *session.Session, f store.Film) core.ImageSource {
	if f.Poster == "" {
		return core.ImageSource{}
	}
	return core.Src(kcore.FileURL(filepath.Join(s.Store().FilmDir(f.Desktop, f.Film.ID), f.Poster)))
}

// libSource is what a computer's libraries are read for: its client, and the pull that asked.
type libSource struct {
	c   *sync.Client
	gen int
}

// computer is a paired computer's section: its name, and its libraries as it lists them, or why it does not.
func computer(cx *core.Scope, s *session.Session, d func() store.Desktop, gen func() int, act Actions, busy func(bool)) core.Node {
	colors := ui.ColorsOf(cx)
	libs := core.NewResource(cx, func() (libSource, bool) {
		c := s.Client(d().ID)
		return libSource{c, gen()}, c != nil
	}, func(ctx context.Context, src libSource, _ core.ResourceInfo) ([]sync.Library, error) {
		return src.c.Libraries(ctx)
	})
	cx.Effect(func() { busy(libs.Loading()) })
	list := func() []sync.Library { v, _ := libs.Get(); return v }
	problem := func() session.Problem { return session.ProblemOf(libs.Err()) }
	return core.View{TestID: "home-desktop-" + d().ID, Children: []core.Node{
		look.SectionTitle(cx, func() string { return d().Name }, nil, ""),
		core.Show{When: func() bool { return libs.Loading() && len(list()) == 0 }, Then: func(cx *core.Scope) core.Node {
			return core.View{Style: core.Style{Padding: core.Symmetric(12, 16), AlignItems: core.AlignStart}, Children: []core.Node{ui.Spinner{}}}
		}},
		core.Show{When: func() bool { return libs.Err() != nil && !libs.Loading() }, Then: func(cx *core.Scope) core.Node {
			return core.View{TestID: "home-desktop-problem", Style: core.Style{Padding: core.Symmetric(8, 16), Gap: 10, AlignItems: core.AlignStart}, Children: []core.Node{
				core.Text{ContentFunc: func() string { return desktopProblem(cx, problem(), d().Name) }, Style: look.Body, StyleFunc: ui.Colored(colors.Muted)},
				core.Show{When: func() bool { p := problem(); return p == session.Unpaired || p == session.Certificate },
					Then: func(cx *core.Scope) core.Node {
						return ui.Button{TestID: "home-pair-again", Text: locales.HomePairAgain(cx), Variant: ui.ButtonSecondary, OnPress: act.Pair}
					},
					Else: func(cx *core.Scope) core.Node {
						return ui.Button{TestID: "home-retry", Text: locales.CommonRetry(cx), Variant: ui.ButtonSecondary, OnPress: libs.Refetch}
					}},
			}}
		}},
		core.Show{When: func() bool { _, ok := libs.Get(); return ok && libs.Err() == nil && len(list()) == 0 }, Then: func(cx *core.Scope) core.Node {
			return core.View{Style: core.Style{Padding: core.Symmetric(8, 16)}, Children: []core.Node{
				core.Text{ContentFunc: func() string { return locales.HomeNoLibraries(cx) }, Style: look.Body, StyleFunc: ui.Colored(colors.Muted)}}}
		}},
		core.Each(list, func(l sync.Library) string { return l.ID }, func(cx *core.Scope, l func() sync.Library, _ func() int) core.Node {
			return look.Row{TestID: "home-library-" + l().ID,
				Leading:  core.Icon{SVG: lucide.Library(), Style: core.Style{Width: core.Pt(22), Height: core.Pt(22)}, StyleFunc: ui.Colored(colors.Accent)},
				Title:    func() string { return l().Name },
				Detail:   func() string { return locales.HomeFilmCount(cx, l().Films) },
				Trailing: core.Icon{SVG: lucide.ChevronRight(), Style: core.Style{Width: core.Pt(16), Height: core.Pt(16)}, StyleFunc: ui.Colored(colors.Faint)},
				OnPress:  func() { act.Library(d().ID, l()) }}
		}),
	}}
}

// desktopProblem is why a computer's libraries do not show.
func desktopProblem(cx *core.Scope, p session.Problem, name string) string {
	switch p {
	case session.Unpaired:
		return locales.HomeUnpaired(cx, name)
	case session.Certificate:
		return locales.HomeCertificate(cx, name)
	}
	return locales.HomeUnreachable(cx, name)
}
