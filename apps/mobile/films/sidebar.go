package films

import (
	"kando/apps/mobile/locales"
	"kando/apps/mobile/look"
	"kando/apps/mobile/session"
	"kando/apps/mobile/theme"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	"github.com/nsgo-dev/nsgo/ui"
)

// Sidebar is the paired computers' subscriptions as a tablet's sidebar lists them under the sections: their heading, with the way to add one through the Add sheet (add) while a computer takes a list link, and each subscription's mark, name and count of new videos waiting, which pressed opens its sheet (open, SubscriptionSheet). The list scrolls in the room the sidebar has; it shows while there is a subscription, or a computer to subscribe through.
func Sidebar(cx *core.Scope, s *session.Session, open func(key string), add func()) core.Node {
	colors := ui.ColorsOf(cx)
	return core.Show{When: func() bool { return len(s.Subscriptions()) > 0 || canSubscribe(s) }, Then: func(cx *core.Scope) core.Node {
		return core.View{TestID: "sidebar-subscriptions", Style: core.Style{FlexGrow: 1, FlexShrink: 1, MinHeight: core.Pt(0)}, Children: []core.Node{
			core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Padding: core.Edges{Top: 24, Right: 4, Bottom: 6, Left: 12}}, Children: []core.Node{
				core.Text{ContentFunc: func() string { return locales.FilmsSubscriptions(cx) }, Role: core.RoleHeader, MaxLines: 1,
					Style: look.Caption.Merge(core.Style{FontWeight: core.FontWeightBold, FlexGrow: 1, FlexShrink: 1}), StyleFunc: ui.Colored(colors.Muted)},
				core.Show{When: func() bool { return canSubscribe(s) }, Then: func(cx *core.Scope) core.Node {
					return ui.IconButton{TestID: "sidebar-subscriptions-add", Icon: lucide.Plus, Variant: ui.IconButtonGhost, Size: 28, IconSize: 16,
						LabelFunc: func() string { return locales.FilmsSubscriptionsAddLabel(cx) }, OnPress: add}
				}},
			}},
			core.Scroll{HideIndicator: true, Style: core.Style{FlexGrow: 1, FlexShrink: 1}, Children: []core.Node{
				core.View{Style: core.Style{Gap: 2}, Children: []core.Node{
					core.Each(s.Subscriptions, func(sub session.Subscription) string { return sub.Key() }, func(cx *core.Scope, sub func() session.Subscription, _ func() int) core.Node {
						return sidebarRow(cx, sub, open)
					}),
				}},
			}},
		}}
	}}
}

// sidebarRow is a subscription in the sidebar: its site's mark with its initial, its name, and the count of new videos waiting on it in the accent; its name in the warning's color while its list is gone or its last check failed, dimmed while paused.
func sidebarRow(cx *core.Scope, sub func() session.Subscription, open func(key string)) core.Node {
	colors := ui.ColorsOf(cx)
	waiting := func() int { return len(sub().Waiting) }
	id := sub().Desktop + "-" + sub().ID
	return core.View{TestID: "sidebar-subscription-" + id, Role: core.RoleButton, LabelFunc: func() string { return subscriptionLabel(cx, sub()) },
		OnPress: func(core.PressEvent) { open(sub().Key()) },
		Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 10, MinHeight: core.Pt(40), Padding: core.Symmetric(0, 12), CornerRadius: 10, BorderCurve: core.BorderCurveContinuous,
			FocusVisible: ui.FocusRing(cx, -2)},
		StyleFunc: func() core.Style {
			st := core.Style{Active: &core.Style{Background: colors.Pressed()}, Opacity: core.Alpha(1)}
			if sub().Paused {
				st.Opacity = core.Alpha(0.5)
			}
			return st
		},
		Children: []core.Node{
			core.View{Style: core.Style{Width: core.Pt(26), Height: core.Pt(26), CornerRadius: 13, AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter, FlexShrink: 0},
				StyleFunc: func() core.Style { return core.Style{Background: markFill(cx, sub())} },
				Children: []core.Node{core.Text{ContentFunc: func() string { return initial(sub().Name) }, MaxLines: 1,
					Style: core.Style{FontSize: 11, FontWeight: core.FontWeightBold, Color: core.RGB(0xFFFFFF)}}}},
			core.Text{ContentFunc: func() string { return sub().Name }, MaxLines: 1, Style: look.Subhead.Merge(core.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0)}),
				StyleFunc: func() core.Style {
					if trouble(sub()) {
						return core.Style{Color: theme.Of(cx).WarnText()}
					}
					return core.Style{Color: colors.Text()}
				}},
			core.Show{When: func() bool { return waiting() > 0 }, Then: func(cx *core.Scope) core.Node {
				return core.View{Style: core.Style{MinWidth: core.Pt(20), Height: core.Pt(20), Padding: core.Symmetric(0, 6), CornerRadius: 10, AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter},
					StyleFunc: func() core.Style { return core.Style{Background: colors.Accent()} },
					Children: []core.Node{ui.Tabular(core.Text{TestID: "sidebar-subscription-new-" + id, ContentFunc: func() string { return locales.FilmsSubscriptionNew(cx, waiting()) }, MaxLines: 1,
						Style: core.Style{FontSize: 11, FontWeight: core.FontWeightBold}, StyleFunc: ui.Colored(colors.OnAccent)})}}
			}},
		}}
}
