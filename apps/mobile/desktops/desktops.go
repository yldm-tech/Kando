// Package desktops is the page of the computers the phone is paired with, each with its address, to unpair, and the way to pair another.
package desktops

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

// Page is the computers' page; pair opens the pairing page.
func Page(cx *core.Scope, s *session.Session, pair, back func()) base.NavPage {
	colors := ui.ColorsOf(cx)
	unpair := func(d store.Desktop) {
		cx.ShowAlert(core.AlertConfig{Title: locales.DesktopsUnpairTitle(cx, d.Name), Message: locales.DesktopsUnpairMessage(cx), Actions: []core.AlertAction{
			{Label: locales.CommonCancel(cx), Role: core.AlertCancel},
			{Label: locales.DesktopsUnpair(cx), Role: core.AlertDestructive},
		}}, func(i int) {
			if i == 1 {
				s.Unpair(d.ID)
			}
		})
	}
	return ui.NavPage(cx, base.NavPage{TitleFunc: func() string { return locales.DesktopsTitle(cx) }, TestID: "desktops-page", Children: []core.Node{
		core.Scroll{Style: core.Style{FlexGrow: 1, FlexShrink: 1}, Children: []core.Node{
			core.View{Style: core.Style{Padding: core.Edges{Top: 8, Bottom: 24}, PaddingEnv: core.EnvEdges{Bottom: core.EnvSafeArea}}, Children: []core.Node{
				core.Each(s.Desktops, func(d store.Desktop) string { return d.ID }, func(cx *core.Scope, d func() store.Desktop, _ func() int) core.Node {
					return look.Row{TestID: "desktops-" + d().ID,
						Leading: core.Icon{SVG: lucide.Monitor(), Style: core.Style{Width: core.Pt(22), Height: core.Pt(22)}, StyleFunc: ui.Colored(colors.Accent)},
						Title:   func() string { return d().Name },
						Detail:  func() string { return d().Addr.String() },
						Trailing: ui.Button{TestID: "desktops-unpair", Text: locales.DesktopsUnpair(cx), Variant: ui.ButtonSecondary,
							OnPress: func() { unpair(d()) }}}
				}),
				core.View{Style: core.Style{Padding: core.Symmetric(16, 16), AlignItems: core.AlignStart}, Children: []core.Node{
					ui.Button{TestID: "desktops-pair", Text: locales.DesktopsPairAnother(cx), Icon: lucide.Plus, Size: ui.SizeLarge, OnPress: pair},
				}},
			}},
		}},
	}}, ui.NavHeader{OnBack: back})
}
