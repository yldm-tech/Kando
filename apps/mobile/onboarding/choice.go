package onboarding

import (
	"kando/apps/mobile/locales"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	"github.com/nsgo-dev/nsgo/ui"
)

// way is how the phone gets its films, as the user chose at the choosing step: through Kando on a computer, which makes their subtitles, straight from a NAS or cloud drive, without subtitles, or both. It is the introduction's alone and is not kept: Settings adds either later.
type way uint8

const (
	undecided way = iota
	viaComputer
	viaNAS
	viaBoth
)

// route is the introduction's steps for w: the choosing step ends them until a way is chosen, then the way's own come after it, pairing before adding a source for both.
func route(w way) []stage {
	out := []stage{intro, languages, choosing}
	switch w {
	case viaComputer:
		out = append(out, pairing)
	case viaNAS:
		out = append(out, adding)
	case viaBoth:
		out = append(out, pairing, adding)
	}
	return out
}

// dot is the step dot lit at position at of the route: the choosing step and the way's first step share the third, so a single way is three steps and both are four.
func dot(at int) int {
	if at <= 2 {
		return at + 1
	}
	return at
}

// dots is how many step dots w shows.
func dots(w way) int {
	if w == viaBoth {
		return 4
	}
	return 3
}

// choosingStep asks where the user's films are, as three cards of which one is chosen: Kando on the computer, a NAS or cloud drive, which says plainly that its films have no subtitles, or both.
func choosingStep(cx *core.Scope, chosen *core.Signal[way]) core.Node {
	options := []struct {
		w           way
		id          string
		icon        func() *core.SVG
		title, line func(*core.Scope) string
	}{
		{viaComputer, "computer", lucide.Laptop, locales.OnboardingWayComputer, locales.OnboardingWayComputerLine},
		{viaNAS, "nas", lucide.HardDrive, locales.OnboardingWayNAS, locales.OnboardingWayNASLine},
		{viaBoth, "both", lucide.Layers, locales.OnboardingWayBoth, locales.OnboardingWayBothLine},
	}
	var cards []core.Node
	for _, o := range options {
		cards = append(cards, wayCard(cx, "onboarding-way-"+o.id, o.icon, func() string { return o.title(cx) }, func() string { return o.line(cx) },
			func() bool { return chosen.Get() == o.w }, func() { chosen.Set(o.w) }))
	}
	return core.View{TestID: "onboarding-choosing", Style: core.Style{Gap: 14}, Children: []core.Node{
		heading(cx, false, func() string { return locales.OnboardingWayTitle(cx) }, func() string { return locales.OnboardingWayBody(cx) }),
		core.View{Role: core.RoleRadioGroup, LabelFunc: func() string { return locales.OnboardingWayTitle(cx) }, Style: core.Style{Gap: 8}, Children: cards},
	}}
}

// wayCard is a way's card: its icon, title and line, ringed in the accent while on, as the languages' tiles are.
func wayCard(cx *core.Scope, testID string, icon func() *core.SVG, title, line func() string, on func() bool, choose func()) core.Node {
	colors := ui.ColorsOf(cx)
	return core.View{TestID: testID, Role: core.RoleRadio, LabelFunc: title, AccessibilityDescriptionFunc: line, Checked: on, Accessible: true,
		OnPress: func(core.PressEvent) { choose() },
		Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 14, Padding: core.Symmetric(14, 14), CornerRadius: 16, BorderCurve: core.BorderCurveContinuous, BorderWidth: 2,
			Active: &core.Style{Opacity: core.Alpha(0.7)}},
		// The ring is drawn inside the card, so a card chosen keeps its size.
		StyleFunc: func() core.Style {
			if on() {
				return core.Style{Background: ui.Alpha(colors.Accent(), 0.08), BorderColor: colors.Accent()}
			}
			return core.Style{Background: colors.Background(), BorderColor: core.Transparent}
		},
		Children: []core.Node{
			core.View{Style: core.Style{Width: core.Pt(40), Height: core.Pt(40), CornerRadius: 11, BorderCurve: core.BorderCurveContinuous, FlexShrink: 0, AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter},
				StyleFunc: func() core.Style {
					if on() {
						return core.Style{Background: colors.Accent()}
					}
					return core.Style{Background: ui.Alpha(colors.Accent(), 0.12)}
				},
				Children: []core.Node{ui.Icon(icon, 22, func() core.Color {
					if on() {
						return colors.OnAccent()
					}
					return colors.Accent()
				})}},
			core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, Gap: 2}, Children: []core.Node{
				core.Text{ContentFunc: title, Style: core.Style{FontSize: 16, LineHeight: 21, FontWeight: core.FontWeightBold}, StyleFunc: ui.Colored(colors.Text)},
				core.Text{ContentFunc: line, Style: core.Style{FontSize: 13, LineHeight: 18}, StyleFunc: ui.Colored(colors.Muted)},
			}},
		}}
}
