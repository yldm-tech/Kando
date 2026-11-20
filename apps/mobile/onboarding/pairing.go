package onboarding

import (
	"kando/apps/mobile/locales"
	"kando/apps/mobile/pair"
	"kando/apps/mobile/session"
	"kando/apps/mobile/store"
	"kando/apps/mobile/theme"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	"github.com/nsgo-dev/nsgo/ui"
)

// pairingStep pairs the phone with a computer through flow, whose button is at the page's foot, or lets the user go on without one; paired is the computer once it is. Paired, the steps stay over a green card that names the computer and, once it has listed its library, how many films it holds.
func pairingStep(cx *core.Scope, s *session.Session, flow pair.Flow, paired *core.Signal[store.Desktop]) core.Node {
	colors := ui.ColorsOf(cx)
	isPaired := func() bool { return paired.Get().ID != "" }
	note := core.View{TestID: "onboarding-desktop-app", Role: core.RoleLink, LabelFunc: func() string { return locales.OnboardingNoDesktop(cx) },
		OnPress: func(core.PressEvent) { cx.OpenURL(DownloadURL, nil) }, Style: core.Style{Padding: core.Edges{Right: 4, Left: 4}, Active: &core.Style{Opacity: core.Alpha(0.6)}},
		Children: []core.Node{core.Text{ContentFunc: func() string { return locales.OnboardingNoDesktop(cx) }, Style: core.Style{FontSize: 12, LineHeight: 17}, StyleFunc: ui.Colored(colors.Muted)}}}
	return core.View{TestID: "onboarding-pairing", Style: core.Style{Gap: 10}, Children: []core.Node{
		heading(cx, false, func() string { return locales.OnboardingPairTitle(cx) }, func() string { return locales.OnboardingPairBody(cx) }),
		core.Show{When: isPaired,
			Then: func(cx *core.Scope) core.Node {
				return core.View{Style: core.Style{Gap: 10, Margin: core.Edges{Top: 4}}, Children: []core.Node{pair.Steps(cx, pair.CardFill(cx, true)), pairedCard(cx, s, paired)}}
			},
			Else: func(cx *core.Scope) core.Node {
				// The form keeps its own column's sides.
				return core.View{Style: core.Style{Margin: core.Edges{Right: -16, Left: -16, Top: -6, Bottom: -6}}, Children: []core.Node{flow.Content}}
			}},
		note,
	}}
}

// pairedCard is the green card that says which computer the phone paired with and, once that computer has listed its library, how many films it holds.
func pairedCard(cx *core.Scope, s *session.Session, paired *core.Signal[store.Desktop]) core.Node {
	colors := ui.ColorsOf(cx)
	films := func() int {
		sh := s.Shelf(paired.Get().ID)
		if !sh.Listed {
			return -1
		}
		n := 0
		for _, lib := range sh.Listing.Libraries {
			n += len(lib.Films)
		}
		return n
	}
	return core.View{TestID: "onboarding-paired-card", Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 12, Padding: core.Symmetric(12, 14), CornerRadius: 16, BorderCurve: core.BorderCurveContinuous,
		Background: ui.Alpha(theme.StatusOK, 0.12)},
		Children: []core.Node{
			// The design's check is white on a filled green circle.
			core.View{Style: core.Style{Width: core.Pt(22), Height: core.Pt(22), CornerRadius: 11, FlexShrink: 0, AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter, Background: theme.StatusOK},
				Children: []core.Node{ui.Icon(lucide.Check, 14, func() core.Color { return theme.OnPicture })}},
			core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1}, Children: []core.Node{
				core.Text{TestID: "onboarding-paired", ContentFunc: func() string { return locales.OnboardingPaired(cx, paired.Get().Name) }, Role: core.RoleHeader, LiveRegion: core.LiveRegionPolite,
					Style: core.Style{FontSize: 15, LineHeight: 20, FontWeight: core.FontWeightBold}, StyleFunc: ui.Colored(colors.Text)},
				core.Show{When: func() bool { return films() >= 0 }, Then: func(cx *core.Scope) core.Node {
					return core.Text{TestID: "onboarding-paired-films", ContentFunc: func() string { return locales.OnboardingPairedFilms(cx, films()) },
						Style: core.Style{FontSize: 12, LineHeight: 16}, StyleFunc: ui.Colored(colors.Muted)}
				}},
			}},
		}}
}
