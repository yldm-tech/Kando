// Package film is a film's page: its poster, title and facts, and its download, which starts, pauses, resumes, cancels and, once whole, plays or is deleted from the phone.
package film

import (
	"context"

	"kando/apps/mobile/home"
	"kando/apps/mobile/locales"
	"kando/apps/mobile/look"
	"kando/apps/mobile/session"
	"kando/apps/mobile/store"
	"kando/packages/sync"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	"github.com/nsgo-dev/nsgo/ui"
)

// Page is film f of desktop's library, as the library listed it or as the phone keeps it; play plays it once downloaded.
func Page(cx *core.Scope, s *session.Session, desktop, library string, f sync.Film, play func(key string), back func()) base.NavPage {
	colors := ui.ColorsOf(cx)
	key := store.Key(desktop, f.ID)
	kept := func() (store.Film, bool) { return s.Film(key) }
	done := func() bool { k, ok := kept(); return ok && k.Done() }
	transfer := func() (session.Transfer, bool) { return s.Transfer(key) }
	state := func() (session.State, bool) { t, ok := transfer(); return t.State, ok }
	running := func() bool { st, ok := state(); return ok && st == session.Running }
	stopped := func() bool { st, ok := state(); return ok && st != session.Running }

	type posterSource struct{ c *sync.Client }
	remote := core.NewResource(cx, func() (posterSource, bool) {
		c := s.Client(desktop)
		_, ok := kept()
		return posterSource{c}, c != nil && f.Poster != "" && !ok
	}, func(ctx context.Context, src posterSource, _ core.ResourceInfo) ([]byte, error) {
		return s.Poster(ctx, src.c, desktop, f.ID)
	})
	poster := func() core.ImageSource {
		if k, ok := kept(); ok && k.Poster != "" {
			return home.LocalPoster(s, k)
		}
		if data, ok := remote.Get(); ok {
			return core.ImageSource{Data: data}
		}
		return core.ImageSource{}
	}
	button := func(id, text string, variant ui.ButtonVariant, icon func() *core.SVG, press func()) core.Node {
		return ui.Button{TestID: id, Text: text, Variant: variant, Size: ui.SizeLarge, Icon: icon, OnPress: press}
	}
	confirmDelete := func() {
		cx.ShowAlert(core.AlertConfig{Title: locales.FilmDeleteTitle(cx, f.Title), Actions: []core.AlertAction{
			{Label: locales.CommonCancel(cx), Role: core.AlertCancel},
			{Label: locales.FilmDeleteConfirm(cx), Role: core.AlertDestructive},
		}}, func(i int) {
			if i == 1 {
				s.Delete(key)
				back()
			}
		})
	}
	// phase is which buttons the page shows, which changes far less often than the download's progress.
	const (
		notKept = iota
		downloading
		halted
		whole
	)
	phase := core.NewMemo(cx, func() int {
		switch {
		case done():
			return whole
		case running():
			return downloading
		case stopped():
			return halted
		}
		return notKept
	})
	actions := core.Dynamic(func(cx *core.Scope) core.Node {
		var row []core.Node
		switch phase.Get() {
		case whole:
			row = []core.Node{
				button("film-play", locales.FilmPlay(cx), ui.ButtonPrimary, lucide.Play, func() { play(key) }),
				button("film-delete", locales.FilmDelete(cx), ui.ButtonSecondary, lucide.Trash, confirmDelete),
			}
		case downloading:
			row = []core.Node{
				button("film-pause", locales.FilmPause(cx), ui.ButtonPrimary, lucide.Pause, func() { s.Pause(key) }),
				button("film-cancel", locales.FilmCancel(cx), ui.ButtonSecondary, lucide.X, func() { s.Delete(key) }),
			}
		case halted:
			row = []core.Node{
				button("film-resume", locales.FilmResume(cx), ui.ButtonPrimary, lucide.Download, func() { s.Resume(key) }),
				button("film-cancel", locales.FilmCancel(cx), ui.ButtonSecondary, lucide.X, func() { s.Delete(key) }),
			}
		default:
			row = []core.Node{button("film-download", locales.FilmDownload(cx), ui.ButtonPrimary, lucide.Download, func() { s.Download(desktop, library, f) })}
		}
		return core.View{Style: core.Style{Direction: core.Row, Wrap: core.Wrap, Gap: 12}, Children: row}
	})
	progress := core.Show{When: func() bool { _, ok := transfer(); return ok }, Then: func(cx *core.Scope) core.Node {
		return core.View{Style: core.Style{Gap: 6}, Children: []core.Node{
			ui.Progress{TestID: "film-progress", Label: locales.FilmDownload(cx), Value: func() float32 { t, _ := transfer(); return t.Fraction() }},
			core.Text{TestID: "film-status", ContentFunc: func() string { return look.Status(cx, s, key) }, LiveRegion: core.LiveRegionPolite, Style: look.Caption, StyleFunc: ui.Colored(colors.Muted)},
		}}
	}}
	wide := func() bool { return cx.Env().Window().Width >= 700 }
	return ui.NavPage(cx, base.NavPage{Title: f.Title, TestID: "film-page", Children: []core.Node{
		core.Scroll{Style: core.Style{FlexGrow: 1, FlexShrink: 1}, Children: []core.Node{
			core.View{Style: core.Style{Padding: core.All(20), Gap: 16, PaddingEnv: core.EnvEdges{Bottom: core.EnvSafeArea}, Width: core.Pct(100), MaxWidth: core.Pt(720), AlignSelf: core.AlignCenter}, Children: []core.Node{
				core.View{StyleFunc: func() core.Style {
					if wide() {
						return core.Style{Width: core.Pt(420), AlignSelf: core.AlignCenter}
					}
					return core.Style{Width: core.Pct(100)}
				}, Children: []core.Node{
					core.View{Style: core.Style{Width: core.Pct(100), AspectRatio: 16.0 / 9, CornerRadius: 10, Overflow: core.Hidden}, StyleFunc: ui.Filled(colors.Inset), Children: []core.Node{
						core.Image{TestID: "film-poster", SourceFunc: poster, ResizeMode: core.Cover, ElementsHidden: true,
							Style: core.Style{Position: core.Absolute, Left: core.Pt(0), Top: core.Pt(0), Right: core.Pt(0), Bottom: core.Pt(0)}},
					}},
				}},
				core.View{Style: core.Style{Gap: 4}, Children: []core.Node{
					core.Text{TestID: "film-title", Content: f.Title, Role: core.RoleHeader, Style: core.Style{FontSize: 22, LineHeight: 28, FontWeight: core.FontWeightBold}, StyleFunc: ui.Colored(colors.Text)},
					core.Text{TestID: "film-facts", Content: look.Facts(f), Style: look.Body, StyleFunc: ui.Colored(colors.Muted)},
					core.Text{Content: library, Style: look.Caption, StyleFunc: ui.Colored(colors.Muted)},
				}},
				progress,
				actions,
			}},
		}},
	}}, ui.NavHeader{OnBack: back})
}
