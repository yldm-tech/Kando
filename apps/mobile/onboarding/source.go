package onboarding

import (
	"kando/apps/mobile/locales"
	"kando/apps/mobile/sources"
	"kando/apps/mobile/store"
	"kando/apps/mobile/theme"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	"github.com/nsgo-dev/nsgo/ui"
)

// addingStep adds a NAS or cloud drive through form, the form Settings › NAS & Servers adds one with, whose Save is at the page's foot, or lets the user go on without one; added is the source once it is saved, which the step then names on a green card in the form's place.
func addingStep(cx *core.Scope, form sources.Form, added *core.Signal[store.Source]) core.Node {
	return core.View{TestID: "onboarding-adding", Style: core.Style{Gap: 14}, Children: []core.Node{
		heading(cx, false, func() string { return locales.OnboardingAddTitle(cx) }, func() string { return locales.OnboardingAddBody(cx) }),
		core.Show{When: func() bool { return added.Get().ID != "" },
			Then: func(cx *core.Scope) core.Node { return addedCard(cx, added) },
			Else: func(*core.Scope) core.Node { return form.Content }},
	}}
}

// addedCard is the green card that names the source added and where it is, as the Library's NAS group will list it.
func addedCard(cx *core.Scope, added *core.Signal[store.Source]) core.Node {
	colors := ui.ColorsOf(cx)
	return core.View{TestID: "onboarding-added-card", Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 12, Padding: core.Symmetric(12, 14), CornerRadius: 16, BorderCurve: core.BorderCurveContinuous,
		Background: ui.Alpha(theme.StatusOK, 0.12)},
		Children: []core.Node{
			core.View{Style: core.Style{Width: core.Pt(22), Height: core.Pt(22), CornerRadius: 11, FlexShrink: 0, AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter, Background: theme.StatusOK},
				Children: []core.Node{ui.Icon(lucide.Check, 14, func() core.Color { return theme.OnPicture })}},
			core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1}, Children: []core.Node{
				core.Text{TestID: "onboarding-added", ContentFunc: func() string { return locales.OnboardingAdded(cx, added.Get().Name) }, Role: core.RoleHeader, LiveRegion: core.LiveRegionPolite,
					Style: core.Style{FontSize: 15, LineHeight: 20, FontWeight: core.FontWeightBold}, StyleFunc: ui.Colored(colors.Text)},
				core.Text{TestID: "onboarding-added-place", ContentFunc: func() string { return sources.Summary(cx, added.Get()) },
					Style: core.Style{FontSize: 12, LineHeight: 16}, StyleFunc: ui.Colored(colors.Muted)},
			}},
		}}
}
