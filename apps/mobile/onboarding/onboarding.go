// Package onboarding is the introduction the phone shows before anything else, and again from Settings: what Kando does, the languages the user watches and the one they want subtitles in, and pairing with the computer (the pairing form, until QR scanning, nsgo-dev/nsgo#429). Each step can be skipped; what was chosen is kept as the user leaves it.
package onboarding

import (
	"time"

	"kando/apps/mobile/locales"
	"kando/apps/mobile/look"
	"kando/apps/mobile/pair"
	"kando/apps/mobile/session"
	"kando/apps/mobile/store"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

// The steps.
const (
	intro = iota + 1
	languages
	pairing
	steps = pairing
)

// black is the design's 900 weight of the steps' titles.
const black core.FontWeight = 900

// DownloadURL is where the desktop app is, which the last step names.
const DownloadURL = "https://github.com/yldm-tech/Kando"

// Page is the introduction; done runs once the user leaves it, finished or skipped, with the introduction marked as seen. The languages are kept as they are chosen, not as it is left. On a phone it fills the screen; on a tablet it is a card in the middle of a dimmed screen, as the design's Pad artboards draw it.
func Page(cx *core.Scope, s *session.Session, done func()) core.Node {
	colors := ui.ColorsOf(cx)
	step := core.NewSignal(cx, intro)
	// paired is the computer paired at the last step, none before.
	paired := core.NewSignalFunc(cx, store.Desktop{}, func(a, b store.Desktop) bool { return a.ID == b.ID && a.Name == b.Name })
	finish := func() {
		p := s.Prefs()
		p.Onboarded = true
		s.SetPrefs(p)
		done()
	}
	next := func() { step.Update(func(n int) int { return n + 1 }) }
	// The pairing flow is the page's, as its form is in the step and its button at the page's foot.
	flow := pair.New(cx, s, func(d store.Desktop) { paired.Set(d) }, func() core.Points { return 16 }, true)
	isPaired := func() bool { return paired.Get().ID != "" }
	phone := !look.Tablet(cx)
	height := func() core.Points { return heroFor(cx) }

	dots := core.View{TestID: "onboarding-steps", Role: core.RoleImage, Accessible: true, LabelFunc: func() string { return locales.OnboardingStep(cx, step.Get(), steps) },
		Style: core.Style{Direction: core.Row, Gap: 6, AlignItems: core.AlignCenter}}
	for i := intro; i <= steps; i++ {
		dots.Children = append(dots.Children, core.View{Style: core.Style{Height: core.Pt(6), CornerRadius: 3, Transition: core.Transition{Duration: 300 * time.Millisecond, Easing: core.Curve{X1: 0.32, Y1: 0.72, X2: 0, Y2: 1}}},
			StyleFunc: func() core.Style {
				if step.Get() == i {
					return core.Style{Width: core.Pt(22), Background: colors.Accent()}
				}
				return core.Style{Width: core.Pt(6), Background: ui.Alpha(colors.Muted(), 0.35)}
			}})
	}

	body := core.View{Children: []core.Node{
		core.Show{When: func() bool { return step.Get() == intro }, Then: func(cx *core.Scope) core.Node {
			return core.View{TestID: "onboarding-intro", Children: []core.Node{
				heading(cx, true, func() string { return locales.OnboardingIntroTitle(cx) }, func() string { return locales.OnboardingIntroBody(cx) }),
			}}
		}},
		core.Show{When: func() bool { return step.Get() == languages }, Then: func(cx *core.Scope) core.Node { return languagesStep(cx, s) }},
		core.Show{When: func() bool { return step.Get() == pairing }, Then: func(cx *core.Scope) core.Node { return pairingStep(cx, s, flow, paired) }},
	}}
	// over is whether the page has scrolled under the top bar: the picture's dark top, which the bar's white reads on, has gone from behind it.
	over := core.NewSignal(cx, false)
	barFoot := func() core.Points {
		if !phone {
			return 56
		}
		return cx.Env().SafeArea().Top + 56
	}
	// The steps scroll under the picture, which scrolls away with them under the top bar, which stays; the button stays at the page's foot.
	content := core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, MinHeight: core.Pt(0)}, Children: []core.Node{
		core.Scroll{TestID: "onboarding-scroll", Style: core.Style{FlexGrow: 1, FlexShrink: 1},
			OnScroll: func(e core.ScrollEvent) { over.Set(e.Offset.Y > height()*0.55-barFoot()) },
			Children: []core.Node{
				hero(cx, height, phone),
				// The dots sit on the picture's faded foot.
				core.View{Style: core.Style{Gap: 18, Margin: core.Edges{Top: -40}, Padding: core.Edges{Right: 24, Bottom: 16, Left: 24}, PaddingEnv: sides(phone)}, Children: []core.Node{dots, body}},
			}},
		// The flow stays at its last stage once paired, which is no longer a code out.
		topBar(cx, finish, func() bool { return flow.Sending() && !isPaired() }, over.Get, phone),
	}}

	cta := func(text func() string, testID string, press func()) core.Node {
		return look.Button{Kind: look.Primary, Tall: true, Text: text, TestID: testID, OnPress: press}
	}
	actions := core.View{
		// The design's button is 40 points above the screen's foot, clear of a home indicator.
		StyleFunc: func() core.Style {
			return core.Style{Padding: core.Edges{Top: 8, Right: 24, Bottom: max(40, cx.Env().SafeArea().Bottom+8), Left: 24}, PaddingEnv: sides(phone)}
		},
		Children: []core.Node{
			core.Show{When: func() bool { return step.Get() < pairing }, Then: func(cx *core.Scope) core.Node {
				return cta(func() string { return locales.OnboardingContinue(cx) }, "onboarding-continue", next)
			}},
			// The last step's buttons are its own: pairing's, then the way to start. Going on without a computer waits while a code is out, as Cancel does, and gives way to Cancel while the fingerprint is compared.
			core.Show{When: func() bool { return step.Get() == pairing && isPaired() }, Then: func(cx *core.Scope) core.Node {
				return cta(func() string { return locales.OnboardingStart(cx) }, "onboarding-start", finish)
			}},
			core.Show{When: func() bool { return step.Get() == pairing && !isPaired() }, Then: func(cx *core.Scope) core.Node {
				return core.View{Style: core.Style{Gap: 4}, Children: []core.Node{
					flow.Actions,
					core.Show{When: func() bool { return !flow.Comparing() }, Then: func(cx *core.Scope) core.Node {
						return look.Button{Kind: look.Plain, Text: func() string { return locales.OnboardingSkipPairing(cx) }, TestID: "onboarding-later", Disabled: flow.Sending, OnPress: finish}
					}},
				}}
			}},
		}}

	column := []core.Node{content, actions}
	if phone {
		return core.View{TestID: "onboarding", Style: core.Style{FlexGrow: 1, FlexShrink: 1, MinHeight: core.Pt(0)}, StyleFunc: ui.Filled(colors.Surface), Children: column}
	}
	// A tablet's is the design's 480 by 780 card over the screen dimmed.
	return core.View{TestID: "onboarding", Style: core.Style{FlexGrow: 1, FlexShrink: 1, MinHeight: core.Pt(0), AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter, Padding: core.All(24)},
		StyleFunc: func() core.Style {
			return core.Style{Background: colors.Background(), Gradients: []core.Gradient{core.LinearGradient(180, ui.Black(0.35), ui.Black(0.35))}}
		},
		Children: []core.Node{
			core.View{TestID: "onboarding-card", Style: core.Style{Width: core.Pt(480), Height: core.Pt(780), MaxWidth: core.Pct(100), MaxHeight: core.Pct(100), CornerRadius: 28, BorderCurve: core.BorderCurveContinuous,
				Overflow: core.Hidden, Shadows: []core.BoxShadow{{Offset: core.Point{Y: 30}, Blur: 80, Color: ui.Black(0.3)}}},
				StyleFunc: ui.Filled(colors.Surface), Children: column},
		}}
}

// heading is a step's title and the line under it: the first step's larger, the others' tighter to leave room for their choices.
func heading(cx *core.Scope, first bool, title, line func() string) core.Node {
	colors := ui.ColorsOf(cx)
	titleStyle := core.Style{FontSize: 26, LineHeight: 34, FontWeight: black, LetterSpacing: -0.4}
	lineStyle := core.Style{FontSize: 14, LineHeight: 20}
	gap := core.Points(6)
	if first {
		titleStyle = core.Style{FontSize: 30, LineHeight: 38, FontWeight: black, LetterSpacing: -0.6}
		lineStyle = core.Style{FontSize: 16, LineHeight: 24}
		gap = 12
	}
	return core.View{Style: core.Style{Gap: gap}, Children: []core.Node{
		core.Text{ContentFunc: title, Role: core.RoleHeader, Style: titleStyle, StyleFunc: ui.Colored(colors.Text)},
		core.Text{ContentFunc: line, Style: lineStyle, StyleFunc: ui.Colored(colors.Muted)},
	}}
}
