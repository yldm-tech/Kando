// Package computers is a paired computer's section of the Library tab: its libraries, or why they do not show, with the way to try again or pair again.
package computers

import (
	"context"

	"kando/apps/mobile/locales"
	"kando/apps/mobile/look"
	"kando/apps/mobile/session"
	"kando/apps/mobile/store"
	"kando/packages/sync"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	"github.com/nsgo-dev/nsgo/ui"
)

// Actions are where a section leads.
type Actions struct {
	Pair    func()
	Library func(desktop string, lib sync.Library)
}

// libSource is what a computer's libraries are read for: its client, and the pull that asked.
type libSource struct {
	c   *sync.Client
	gen int
}

// Section is a paired computer's, d: its name and its libraries as it lists them, or why it does not, read again as gen changes; busy hears whether they are being read.
func Section(cx *core.Scope, s *session.Session, d func() store.Desktop, gen func() int, act Actions, busy func(bool)) core.Node {
	libs := core.NewResource(cx, func() (libSource, bool) {
		c := s.Client(d().ID)
		return libSource{c, gen()}, c != nil
	}, func(ctx context.Context, src libSource, _ core.ResourceInfo) ([]sync.Library, error) {
		return src.c.Libraries(ctx)
	})
	cx.Effect(func() { busy(libs.Loading()) })
	list := func() []sync.Library { v, _ := libs.Get(); return v }
	problem := func() session.Problem { return session.ProblemOf(libs.Err()) }
	failed := func() bool { return libs.Err() != nil && !libs.Loading() }
	// shown are the libraries the computer last listed, none after a failure, nor while it is asked again after one.
	shown := func() []sync.Library {
		if libs.Err() != nil {
			return nil
		}
		return list()
	}
	// waiting is a read with nothing to show meanwhile: the first, or one again after a failure; a pull to refresh keeps the last list, or its "No libraries", until the answer comes.
	waiting := func() bool { _, ok := libs.Get(); return libs.Loading() && (!ok || libs.Err() != nil) }
	unpairable := func() bool { p := problem(); return p == session.Unpaired || p == session.Certificate }
	placeholder := func() core.Node {
		return core.View{Style: core.Style{Padding: core.Symmetric(14, 16), MinHeight: core.Pt(52), JustifyContent: core.JustifyCenter}, Children: []core.Node{ui.Skeleton{Lines: 1}}}
	}
	return core.View{TestID: "computers-desktop-" + d().ID, Style: core.Style{Gap: 8}, Children: []core.Node{
		look.SectionTitle(cx, func() string { return d().Name }),
		look.Group{Children: []core.Node{
			core.Show{When: waiting, Then: func(cx *core.Scope) core.Node {
				return core.View{Children: []core.Node{placeholder(), placeholder()}}
			}},
			core.Show{When: failed, Then: func(cx *core.Scope) core.Node {
				return core.View{Children: []core.Node{
					look.Row{TestID: "computers-problem-" + d().ID, Inset: 56, TitleLines: 3,
						Leading: look.IconTile(cx, func() *core.SVG {
							if problem() == session.Unreachable {
								return lucide.WifiOff()
							}
							return lucide.CircleAlert()
						}, look.ToneDanger),
						Title:  func() string { return desktopProblem(cx, problem(), d().Name) },
						Detail: func() string { return d().Addr.String() }},
					core.Show{When: unpairable,
						Then: func(cx *core.Scope) core.Node {
							return look.Row{TestID: "computers-pair-again-" + d().ID, Title: func() string { return locales.ComputersPairAgain(cx) }, TitleTone: look.ToneAccent,
								Index: func() int { return 1 }, OnPress: act.Pair}
						},
						Else: func(cx *core.Scope) core.Node {
							return look.Row{TestID: "computers-retry-" + d().ID, Title: func() string { return locales.CommonRetry(cx) }, TitleTone: look.ToneAccent,
								Index: func() int { return 1 }, OnPress: libs.Refetch}
						}},
				}}
			}},
			core.Show{When: func() bool { _, ok := libs.Get(); return ok && libs.Err() == nil && len(list()) == 0 }, Then: func(cx *core.Scope) core.Node {
				return look.Row{Title: func() string { return locales.ComputersNoLibraries(cx) }, TitleTone: look.ToneMuted}
			}},
			core.Each(shown, func(l sync.Library) string { return l.ID }, func(cx *core.Scope, l func() sync.Library, i func() int) core.Node {
				return look.Row{TestID: "computers-library-" + l().ID, Index: i, Inset: 56, Chevron: true,
					Leading: look.IconTile(cx, lucide.Library, look.ToneAccent),
					Title:   func() string { return l().Name },
					Value:   func() string { return locales.ComputersFilmCount(cx, l().Films) },
					OnPress: func() { act.Library(d().ID, l()) }}
			}),
		}},
	}}
}

// desktopProblem is why a computer's libraries do not show.
func desktopProblem(cx *core.Scope, p session.Problem, name string) string {
	switch p {
	case session.Unpaired:
		return locales.ComputersUnpaired(cx, name)
	case session.Certificate:
		return locales.ComputersCertificate(cx, name)
	}
	return locales.ComputersUnreachable(cx, name)
}
