package sources

import (
	"kando/apps/mobile/locales"
	"kando/apps/mobile/look"
	"kando/apps/mobile/session"
	"kando/apps/mobile/store"

	"github.com/nsgo-dev/nsgo/core"
)

// Group is the Library's NAS group, shown while the phone has a source: a row per source, with its kind and where it is, which opens its root's page (open, with its ID).
func Group(cx *core.Scope, s *session.Session, open func(id string)) core.Node {
	return core.Show{When: func() bool { return len(s.Sources()) > 0 }, Then: func(cx *core.Scope) core.Node {
		return core.View{TestID: "films-sources", Style: core.Style{Gap: 7}, Children: []core.Node{
			look.GroupHeader(cx, func() string { return locales.SourcesGroup(cx) }),
			look.Group{Children: []core.Node{
				core.Each(s.Sources, func(src store.Source) string { return src.ID }, func(cx *core.Scope, src func() store.Source, i func() int) core.Node {
					return look.Row{TestID: "films-source-" + src().ID, Index: i, Chevron: true, TitleLines: 1,
						Leading: look.IconTile(cx, kindIcon(src().Kind), look.ToneAccent),
						Title:   func() string { return src().Name },
						Detail:  func() string { return Summary(cx, src()) },
						OnPress: func() { open(src().ID) }}
				}),
			}},
		}}
	}}
}
