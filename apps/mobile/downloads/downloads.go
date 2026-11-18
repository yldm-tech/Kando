// Package downloads is the section of the Library tab that lists the films on the phone, which play offline, each with where its download stands, and the space they take.
package downloads

import (
	"kando/apps/mobile/locales"
	"kando/apps/mobile/look"
	"kando/apps/mobile/session"
	"kando/apps/mobile/store"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

// Section is the films on the phone under their heading, with the space they take; open opens a film's page. The Library tab shows it while there are any.
func Section(cx *core.Scope, s *session.Session, open func(store.Film)) core.Node {
	colors := ui.ColorsOf(cx)
	return core.View{TestID: "downloads", Style: core.Style{Gap: 8}, Children: []core.Node{
		look.SectionTitle(cx, func() string { return locales.DownloadsTitle(cx) }),
		look.Group{TestID: "downloads-list", Children: []core.Node{
			core.Each(s.Films, func(f store.Film) string { return f.Key() }, func(cx *core.Scope, f func() store.Film, i func() int) core.Node {
				return row(cx, s, f, i, open)
			}),
		}},
		core.Text{TestID: "downloads-used", ContentFunc: func() string { return locales.DownloadsUsed(cx, look.Size(s.Used())) },
			Style: look.Caption.Merge(core.Style{Padding: core.Edges{Right: 16, Left: 16}}), StyleFunc: ui.Colored(colors.Muted)},
	}}
}

// row is a film on the phone: its poster, title and where its download stands, or once whole its facts, with the bar of a download under way or paused.
func row(cx *core.Scope, s *session.Session, f func() store.Film, i func() int, open func(store.Film)) core.Node {
	transfer := func() (session.Transfer, bool) { return s.Transfer(f().Key()) }
	return look.Row{TestID: "downloads-film-" + f().Film.ID, Index: i, Inset: 140,
		Leading: look.Poster(cx, func() core.ImageSource { return look.LocalPoster(s, f()) }, 112, ""),
		Title:   func() string { return f().Film.Title },
		Detail: func() string {
			if !f().Done() {
				if st := look.Status(cx, s, f().Key()); st != "" {
					return st
				}
			}
			return look.Facts(f().Film)
		},
		DetailTone: func() look.Tone {
			t, ok := transfer()
			switch {
			case f().Done() || !ok:
				return look.ToneDefault
			case t.State == session.Failed:
				return look.ToneDanger
			}
			return look.ToneAccent
		},
		Below: core.Show{When: func() bool {
			t, ok := transfer()
			return ok && (t.State == session.Running || t.State == session.Paused)
		},
			Then: func(cx *core.Scope) core.Node {
				// TODO(nsgo#145): ui.Progress.Label is a string only (ui/progress.go:12-19), so the bar's spoken name keeps the language it was built in; pass a LabelFunc once Progress has one https://github.com/nsgo-dev/nsgo/issues/145
				return ui.Progress{Label: locales.FilmDownload(cx), Value: func() float32 { t, _ := transfer(); return t.Fraction() * 100 }, Style: core.Style{Margin: core.Edges{Top: 6}}}
			}},
		OnPress: func() { open(f()) }}
}
