// Package onboarding is the introduction the phone shows before anything else, and again from Settings: what Kando does, the languages the user watches and the one they want subtitles in, and pairing with the computer (the pairing form, until QR scanning, nsgo-dev/nsgo#429). Each step can be skipped; what was chosen is kept as the user leaves it.
package onboarding

import (
	"slices"

	"kando/apps/mobile/langs"
	"kando/apps/mobile/locales"
	"kando/apps/mobile/look"
	"kando/apps/mobile/pair"
	"kando/apps/mobile/session"
	"kando/apps/mobile/store"
	"kando/apps/mobile/theme"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	"github.com/nsgo-dev/nsgo/ui"
)

// The steps.
const (
	intro = iota + 1
	languages
	pairing
	steps = pairing
)

// DownloadURL is where the desktop app is, which the last step names.
const DownloadURL = "https://github.com/yldm-tech/Kando"

// Page is the introduction; done runs once the user leaves it, finished or skipped, with the introduction marked as seen. The languages are kept as they are chosen, not as it is left.
func Page(cx *core.Scope, s *session.Session, done func()) core.Node {
	colors := ui.ColorsOf(cx)
	step := core.NewSignal(cx, intro)
	// paired is the name of the computer paired at the last step, "" before.
	paired := core.NewSignal(cx, "")
	finish := func() {
		p := s.Prefs()
		p.Onboarded = true
		s.SetPrefs(p)
		done()
	}
	next := func() { step.Update(func(n int) int { return n + 1 }) }

	dots := core.View{TestID: "onboarding-steps", Role: core.RoleImage, Accessible: true, LabelFunc: func() string { return locales.OnboardingStep(cx, step.Get(), steps) },
		Style: core.Style{Direction: core.Row, Gap: 6, AlignItems: core.AlignCenter, FlexGrow: 1}}
	for i := intro; i <= steps; i++ {
		dots.Children = append(dots.Children, core.View{Style: core.Style{Height: core.Pt(6), CornerRadius: 3},
			StyleFunc: func() core.Style {
				if step.Get() == i {
					return core.Style{Width: core.Pt(22), Background: colors.Accent()}
				}
				return core.Style{Width: core.Pt(6), Background: ui.Alpha(colors.Muted(), 0.35)}
			}})
	}
	top := core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, MinHeight: core.Pt(44)}, Children: []core.Node{
		dots,
		core.Show{When: func() bool { return step.Get() < pairing }, Then: func(cx *core.Scope) core.Node {
			return look.Pill(cx, "onboarding-skip", func() string { return locales.OnboardingSkip(cx) }, finish)
		}},
	}}

	body := core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, MinHeight: core.Pt(0)}, Children: []core.Node{
		core.Show{When: func() bool { return step.Get() == intro }, Then: func(cx *core.Scope) core.Node { return introStep(cx) }},
		core.Show{When: func() bool { return step.Get() == languages }, Then: func(cx *core.Scope) core.Node { return languagesStep(cx, s) }},
		core.Show{When: func() bool { return step.Get() == pairing }, Then: func(cx *core.Scope) core.Node {
			return pairingStep(cx, s, paired, finish)
		}},
	}}
	// The last step's buttons are its own: pairing's, then the way to start.
	actions := core.Show{When: func() bool { return step.Get() < pairing }, Then: func(cx *core.Scope) core.Node {
		return look.Button{Kind: look.Primary, Text: func() string { return locales.OnboardingContinue(cx) }, TestID: "onboarding-continue", OnPress: next}
	}}
	return core.View{TestID: "onboarding", Style: core.Style{FlexGrow: 1, PaddingEnv: core.EnvAll(core.EnvSafeArea), Padding: core.Edges{Right: 24, Bottom: 16, Left: 24}},
		StyleFunc: ui.Filled(colors.Background),
		Children: []core.Node{
			core.View{Style: core.Style{FlexGrow: 1, MaxWidth: core.Pt(560), Width: core.Pct(100), AlignSelf: core.AlignCenter, Gap: 12}, Children: []core.Node{top, body, actions}},
		}}
}

// heading is a step's title and the line under it.
func heading(cx *core.Scope, title, line func() string) core.Node {
	colors := ui.ColorsOf(cx)
	return core.View{Style: core.Style{Gap: 10}, Children: []core.Node{
		core.Text{ContentFunc: title, Role: core.RoleHeader, Style: look.Display, StyleFunc: ui.Colored(colors.Text)},
		core.Text{ContentFunc: line, Style: look.Body, StyleFunc: ui.Colored(colors.Muted)},
	}}
}

// introStep says what Kando does, under its mark: a line of a film with its translation under it, as the player shows them.
func introStep(cx *core.Scope) core.Node {
	colors, tints := ui.ColorsOf(cx), ui.TintsOf(cx)
	flat := func() bool { return cx.Env().Window().Height < 500 }
	return core.View{TestID: "onboarding-intro", Style: core.Style{FlexGrow: 1, JustifyContent: core.JustifyCenter, Gap: 28}, Children: []core.Node{
		core.Show{When: func() bool { return !flat() }, Then: func(cx *core.Scope) core.Node {
			return core.View{Style: core.Style{Width: core.Pt(88), Height: core.Pt(88), CornerRadius: 22, BorderCurve: core.BorderCurveContinuous, AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter},
				StyleFunc: ui.Filled(tints.Accent), Children: []core.Node{ui.Icon(lucide.Captions, 44, colors.Accent)}}
		}},
		heading(cx, func() string { return locales.OnboardingIntroTitle(cx) }, func() string { return locales.OnboardingIntroBody(cx) }),
	}}
}

// languagesStep asks which languages the user watches and which one they want subtitles in, keeping each choice as it is made, so that leaving the introduction any way, even by the system's back, keeps it. The target stays unchosen ("", which follows the phone's language) until the user presses one; the phone's own shows as checked meanwhile.
func languagesStep(cx *core.Scope, s *session.Session) core.Node {
	colors := ui.ColorsOf(cx)
	keep := func(change func(*store.Prefs)) {
		p := s.Prefs()
		change(&p)
		s.SetPrefs(p)
	}
	var watched, into []core.Node
	for _, l := range langs.Spoken {
		watched = append(watched, look.Chip{TestID: "onboarding-spoken-" + l.Code, Text: func() string { return l.Name },
			On: func() bool { return slices.Contains(s.Prefs().Spoken, l.Code) },
			OnPress: func() {
				keep(func(p *store.Prefs) {
					if i := slices.Index(p.Spoken, l.Code); i >= 0 {
						p.Spoken = slices.Delete(slices.Clone(p.Spoken), i, i+1)
						return
					}
					p.Spoken = append(slices.Clone(p.Spoken), l.Code)
				})
			}})
	}
	for _, l := range langs.Targets {
		into = append(into, look.Chip{TestID: "onboarding-target-" + l.Code, Radio: true, Text: func() string { return l.Name },
			On: func() bool { return s.Target() == l.Code }, OnPress: func() { keep(func(p *store.Prefs) { p.Target = l.Code }) }})
	}
	return core.Scroll{TestID: "onboarding-languages", Style: core.Style{FlexGrow: 1, FlexShrink: 1}, Children: []core.Node{
		core.View{Style: core.Style{Gap: 28, Padding: core.Edges{Top: 16, Bottom: 16}}, Children: []core.Node{
			heading(cx, func() string { return locales.OnboardingLanguagesTitle(cx) }, func() string { return locales.OnboardingLanguagesBody(cx) }),
			core.View{Role: core.RoleGroup, LabelFunc: func() string { return locales.OnboardingLanguagesTitle(cx) }, Children: []core.Node{look.Chips(watched...)}},
			core.View{Style: core.Style{Gap: 12}, Children: []core.Node{
				core.Text{ContentFunc: func() string { return locales.OnboardingTarget(cx) }, Role: core.RoleHeader, Style: look.Title, StyleFunc: ui.Colored(colors.Text)},
				core.View{Role: core.RoleRadioGroup, LabelFunc: func() string { return locales.OnboardingTarget(cx) }, Children: []core.Node{look.Chips(into...)}},
			}},
		}},
	}}
}

// pairingStep pairs the phone with a computer, or lets the user go on without one; paired takes the name of the computer once it is.
func pairingStep(cx *core.Scope, s *session.Session, paired *core.Signal[string], finish func()) core.Node {
	colors := ui.ColorsOf(cx)
	flow := pair.New(cx, s, func(d store.Desktop) { paired.Set(d.Name) }, func() core.Points { return 0 })
	done := func() bool { return paired.Get() != "" }
	note := core.View{TestID: "onboarding-desktop-app", Role: core.RoleLink, LabelFunc: func() string { return locales.OnboardingNoDesktop(cx) },
		OnPress: func(core.PressEvent) { cx.OpenURL(DownloadURL, nil) }, Style: core.Style{Padding: core.Symmetric(8, 0), Active: &core.Style{Opacity: core.Alpha(0.6)}},
		Children: []core.Node{core.Text{ContentFunc: func() string { return locales.OnboardingNoDesktop(cx) }, Style: look.Caption, StyleFunc: ui.Colored(colors.Muted)}}}
	return core.View{TestID: "onboarding-pairing", Style: core.Style{FlexGrow: 1, FlexShrink: 1, Gap: 8}, Children: []core.Node{
		core.View{Style: core.Style{Padding: core.Edges{Top: 16}}, Children: []core.Node{
			heading(cx, func() string { return locales.OnboardingPairTitle(cx) }, func() string { return locales.OnboardingPairBody(cx) }),
		}},
		core.Show{When: done,
			Then: func(cx *core.Scope) core.Node {
				return core.View{TestID: "onboarding-paired", Style: core.Style{FlexGrow: 1, JustifyContent: core.JustifyCenter, AlignItems: core.AlignCenter, Gap: 16}, Children: []core.Node{
					ui.Icon(lucide.CircleCheck, 52, func() core.Color { return theme.StatusOK }),
					core.Text{ContentFunc: func() string { return locales.OnboardingPaired(cx, paired.Get()) }, Role: core.RoleHeader, LiveRegion: core.LiveRegionPolite,
						Style: look.Heading.Merge(core.Style{TextAlign: core.TextAlignCenter}), StyleFunc: ui.Colored(colors.Text)},
				}}
			},
			Else: func(cx *core.Scope) core.Node {
				return core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, Gap: 8}, Children: []core.Node{
					// The form keeps its own column's sides.
					core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, Margin: core.Edges{Right: -16, Left: -16}}, Children: []core.Node{flow.Body}},
					note,
				}}
			}},
		core.Show{When: done,
			Then: func(cx *core.Scope) core.Node {
				return look.Button{Kind: look.Primary, Text: func() string { return locales.OnboardingStart(cx) }, TestID: "onboarding-start", OnPress: finish}
			},
			Else: func(cx *core.Scope) core.Node {
				return core.View{Style: core.Style{Gap: 4}, Children: []core.Node{
					flow.Actions,
					look.Button{Kind: look.Plain, Text: func() string { return locales.OnboardingSkipPairing(cx) }, TestID: "onboarding-later", OnPress: finish},
				}}
			}},
	}}
}
