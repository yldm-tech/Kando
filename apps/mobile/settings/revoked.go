package settings

import (
	"kando/apps/mobile/locales"
	"kando/apps/mobile/look"
	"kando/apps/mobile/session"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	"github.com/nsgo-dev/nsgo/ui"
)

// RevokedPage is what the phone shows once computer id answered that it no longer takes it, as when the phone was removed in its Settings › Phone Sync: what that means, how many of the phone's changes it did not take, and the ways on, pairing again, unpairing it, which asks first, or leaving it for now. Each leaves the page (done).
func RevokedPage(cx *core.Scope, s *session.Session, id string, pair func(), unpaired func(id string), done func()) base.NavPage {
	colors, tints := ui.ColorsOf(cx), ui.TintsOf(cx)
	name := func() string {
		if d, ok := s.Desktop(id); ok {
			return d.Name
		}
		return ""
	}
	leave := func() {
		s.DismissRevoked(id)
		done()
	}
	pending := func() int { return s.PendingChanges(id) }
	fact := func(testID string, icon func() *core.SVG, tone look.Tone, text func() string) core.Node {
		return look.Row{TestID: testID, TitleLines: 3, Leading: look.IconTile(cx, icon, tone), Title: text}
	}
	unpair := func() {
		n := name()
		cx.ShowAlert(core.AlertConfig{Title: locales.ComputersUnpairTitle(cx, n), Message: locales.ComputersUnpairMessage(cx), Sheet: !look.Tablet(cx), Cancelable: true,
			Actions: []core.AlertAction{
				{Label: locales.CommonCancel(cx), Role: core.AlertCancel},
				{Label: locales.ComputersUnpair(cx), Role: core.AlertDestructive},
			}}, func(i int) {
			if i == 1 {
				s.Unpair(id)
				unpaired(id)
				done()
			}
		})
	}
	return base.NavPage{TestID: "revoked-page", HeaderHidden: true, StyleFunc: ui.Filled(colors.Background), Children: []core.Node{
		core.Scroll{Style: core.Style{FlexGrow: 1, FlexShrink: 1}, Children: []core.Node{
			core.View{Style: core.Style{Padding: core.Edges{Top: 48, Right: 20, Left: 20, Bottom: 24}, PaddingEnv: core.EnvEdges{Top: core.EnvSafeArea, Right: core.EnvSafeArea, Left: core.EnvSafeArea},
				MaxWidth: core.Pt(560), Width: core.Pct(100), AlignSelf: core.AlignCenter, Gap: 20}, Children: []core.Node{
				core.View{Style: core.Style{Width: core.Pt(64), Height: core.Pt(64), CornerRadius: 16, BorderCurve: core.BorderCurveContinuous, AlignSelf: core.AlignCenter, AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter},
					StyleFunc: ui.Filled(tints.Danger), Children: []core.Node{ui.Icon(lucide.ShieldOff, 32, colors.Danger)}},
				core.Text{TestID: "revoked-title", ContentFunc: func() string { return locales.RevokedTitle(cx, name()) }, Role: core.RoleHeader, Style: look.Heading.Merge(core.Style{TextAlign: core.TextAlignCenter}), StyleFunc: ui.Colored(colors.Text)},
				core.Text{ContentFunc: func() string { return locales.RevokedBody(cx, name()) }, Style: look.Subhead.Merge(core.Style{TextAlign: core.TextAlignCenter}), StyleFunc: ui.Colored(colors.Muted)},
				look.Group{Children: []core.Node{
					fact("revoked-films", lucide.CircleCheck, look.ToneAccent, func() string { return locales.RevokedFilmsStay(cx) }),
					core.Show{When: func() bool { return pending() > 0 }, Then: func(cx *core.Scope) core.Node {
						return core.View{Children: []core.Node{
							core.View{Style: core.Style{Height: core.Pt(0.5), Margin: core.Edges{Left: 56}}, StyleFunc: ui.Filled(colors.Line)},
							fact("revoked-pending", lucide.CircleAlert, look.ToneDanger, func() string { return locales.RevokedPending(cx, pending()) }),
						}}
					}},
				}},
			}},
		}},
		look.ActionBar(cx,
			look.Button{TestID: "revoked-pair", Kind: look.Primary, Text: func() string { return locales.ComputersPairAgain(cx) }, OnPress: func() { leave(); pair() }},
			look.Button{TestID: "revoked-unpair", Kind: look.Danger, Text: func() string { return locales.ComputersUnpair(cx) }, OnPress: unpair},
			look.Button{TestID: "revoked-later", Kind: look.Plain, Text: func() string { return locales.RevokedLater(cx) }, OnPress: leave},
		),
	}}
}
