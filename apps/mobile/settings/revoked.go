package settings

import (
	"kando/apps/mobile/locales"
	"kando/apps/mobile/look"
	"kando/apps/mobile/session"
	"kando/apps/mobile/theme"

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
	// A fact is a line of what the revocation did, after a bare mark: a green check for what is done, the warning's ! for what could not be.
	fact := func(testID string, icon func() *core.SVG, done bool, text func() string) core.Node {
		return core.View{TestID: testID, Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 12, Padding: core.Symmetric(12, 16), MinHeight: core.Pt(46)}, Children: []core.Node{
			core.Icon{SVG: icon(), StyleFunc: func() core.Style {
				st := core.Style{Width: core.Pt(16), Height: core.Pt(16), FlexShrink: 0}
				if done {
					st.Color = theme.StatusOK
				} else {
					st.Color = theme.Of(cx).WarnText()
				}
				return st
			}},
			core.Text{ContentFunc: text, Style: core.Style{FontSize: 14, LineHeight: 20, FlexShrink: 1}, StyleFunc: ui.Colored(colors.Text)},
		}}
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
			// The content sits in the middle of the screen while it fits, as the design centers it.
			core.View{Style: core.Style{FlexGrow: 1, JustifyContent: core.JustifyCenter, Padding: core.Symmetric(40, 32), PaddingEnv: core.EnvEdges{Top: core.EnvSafeArea, Right: core.EnvSafeArea, Bottom: core.EnvSafeArea, Left: core.EnvSafeArea},
				MinHeight: core.Pct(100)}, Children: []core.Node{
				core.View{Style: core.Style{MaxWidth: core.Pt(340), Width: core.Pct(100), AlignSelf: core.AlignCenter, Gap: 16}, Children: []core.Node{
					// The lock is the design's glyph of about 24 points in the 72-point circle, 18 over the title.
					core.View{Style: core.Style{Width: core.Pt(72), Height: core.Pt(72), CornerRadius: 36, AlignSelf: core.AlignCenter, AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter, Margin: core.Edges{Bottom: 2}},
						StyleFunc: ui.Filled(tints.Danger), Children: []core.Node{ui.Icon(lucide.Lock, 26, colors.Danger)}},
					core.Text{TestID: "revoked-title", ContentFunc: func() string { return locales.RevokedTitle(cx, name()) }, Role: core.RoleHeader,
						Style: core.Style{FontSize: 24, LineHeight: 30, FontWeight: look.Heavy, TextAlign: core.TextAlignCenter}, StyleFunc: ui.Colored(colors.Text)},
					core.Text{ContentFunc: func() string { return locales.RevokedBody(cx, name()) }, Style: core.Style{FontSize: 15, LineHeight: 22, TextAlign: core.TextAlignCenter}, StyleFunc: ui.Colored(colors.Muted)},
					core.View{Style: core.Style{Margin: core.Edges{Top: 4}}, Children: []core.Node{look.Group{Children: []core.Node{
						fact("revoked-films", lucide.Check, true, func() string { return locales.RevokedFilmsStay(cx) }),
						core.Show{When: func() bool { return pending() > 0 }, Then: func(cx *core.Scope) core.Node {
							return core.View{Children: []core.Node{
								core.View{Style: core.Style{Height: core.Pt(0.5)}, StyleFunc: ui.Filled(colors.Line)},
								fact("revoked-pending", lucide.CircleAlert, false, func() string { return locales.RevokedPending(cx, pending()) }),
							}}
						}},
					}}}},
					core.View{Style: core.Style{Gap: 8, Margin: core.Edges{Top: 8}}, Children: []core.Node{
						look.Button{TestID: "revoked-pair", Kind: look.Primary, Text: func() string { return locales.ComputersPairAgain(cx) }, OnPress: func() { leave(); pair() }},
						look.Button{TestID: "revoked-unpair", Kind: look.Danger, Text: func() string { return locales.ComputersUnpair(cx) }, OnPress: unpair},
						// Not Now is a link in the accent at 15 and 400, 40 tall, quieter than the buttons over it, as the design sets it.
						base.Button{TestID: "revoked-later", LabelFunc: func() string { return locales.RevokedLater(cx) }, OnPress: func(core.PressEvent) { leave() },
							Style: core.Style{Height: core.Pt(40), Padding: core.Symmetric(0, 16), AlignSelf: core.AlignCenter, AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter,
								Active: &core.Style{Opacity: core.Alpha(0.6)}},
							StyleFunc: func() core.Style { return core.Style{FocusVisible: ui.FocusRing(cx, 2)} },
							Children: []core.Node{core.Text{ContentFunc: func() string { return locales.RevokedLater(cx) }, MaxLines: 1, Style: look.Subhead, StyleFunc: ui.Colored(colors.Accent)}}},
					}},
				}},
			}},
		}},
	}}
}
