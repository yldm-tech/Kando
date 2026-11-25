package sources

import (
	"kando/apps/mobile/locales"
	"kando/apps/mobile/look"
	"kando/apps/mobile/session"
	"kando/apps/mobile/store"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	"github.com/nsgo-dev/nsgo/ui"
)

// SettingsPage is Settings › NAS & Servers: the sources added, each with its kind and where it is, which opens its form (edit, with its ID), the row that adds one (edit with ""), and that Kando only reads them and keeps their passwords in the phone's secure store.
func SettingsPage(cx *core.Scope, s *session.Session, edit func(id string), back func()) base.NavPage {
	colors := ui.ColorsOf(cx)
	ref := &core.Ref{}
	count := func() int { return len(s.Sources()) }
	return look.NavPage(cx, base.NavPage{TitleFunc: func() string { return locales.SourcesTitle(cx) }, TestID: "sources-page", Children: []core.Node{
		core.Scroll{Ref: ref, Style: core.Style{FlexGrow: 1, FlexShrink: 1}, Children: []core.Node{
			core.View{StyleFunc: column(cx), Children: []core.Node{
				core.View{Style: core.Style{Gap: 7}, Children: []core.Node{
					look.Group{TestID: "sources-list", Children: []core.Node{
						core.Each(s.Sources, func(src store.Source) string { return src.ID }, func(cx *core.Scope, src func() store.Source, i func() int) core.Node {
							return look.Row{TestID: "sources-source-" + src().ID, Index: i, Chevron: true, TitleLines: 1,
								Leading: look.IconTile(cx, kindIcon(src().Kind), look.ToneAccent),
								Title:   func() string { return src().Name },
								Detail:  func() string { return Summary(cx, src()) },
								OnPress: func() { edit(src().ID) }}
						}),
						look.Row{TestID: "sources-add", Index: count, TitleTone: look.ToneAccent,
							Leading: look.IconTile(cx, look.Plus, look.ToneAccent),
							Title:   func() string { return locales.SourcesAdd(cx) },
							OnPress: func() { edit("") }},
					}},
					core.Text{TestID: "sources-note", ContentFunc: func() string { return locales.SourcesNote(cx) },
						Style: look.Footnote.Merge(core.Style{Padding: core.Symmetric(0, 16)}), StyleFunc: ui.Colored(colors.Muted)},
				}},
			}},
		}},
	}}, ui.NavHeader{OnBack: back, Scroll: ref})
}

// kindIcon is a kind's mark: a server for SMB and SFTP, a cloud for WebDAV, as the computer's.
func kindIcon(kind string) func() *core.SVG {
	if kind == "webdav" {
		return lucide.Cloud
	}
	return lucide.Server
}

// column is a page's column, its groups apart as Settings' are.
func column(cx *core.Scope) func() core.Style {
	col := look.Column(cx)
	return func() core.Style {
		st := col()
		st.Gap = 23
		return st
	}
}
