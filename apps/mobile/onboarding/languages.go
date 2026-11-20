package onboarding

import (
	"slices"

	"kando/apps/mobile/langs"
	"kando/apps/mobile/locales"
	"kando/apps/mobile/look"
	"kando/apps/mobile/session"
	"kando/apps/mobile/store"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	"github.com/nsgo-dev/nsgo/ui"
)

// spokenNames are the names of langs.Spoken in the language the app shows, by code, said under each language's own name.
var spokenNames = map[string]func(*core.Scope) string{
	"en": locales.OnboardingLanguageEn, "ja": locales.OnboardingLanguageJa, "ko": locales.OnboardingLanguageKo,
	"fr": locales.OnboardingLanguageFr, "es": locales.OnboardingLanguageEs, "de": locales.OnboardingLanguageDe,
}

// languagesStep asks which languages the user watches, as a grid of tiles, and which one they want subtitles in, as a row that opens the list of them, keeping each choice as it is made, so that leaving the introduction any way, even by the system's back, keeps it. The target stays unchosen ("", which follows the phone's language) until the user picks one; the phone's own shows meanwhile.
func languagesStep(cx *core.Scope, s *session.Session) core.Node {
	colors, tints := ui.ColorsOf(cx), ui.TintsOf(cx)
	keep := func(change func(*store.Prefs)) {
		p := s.Prefs()
		change(&p)
		s.SetPrefs(p)
	}
	var tiles []core.Node
	for _, l := range langs.Spoken {
		on := func() bool { return slices.Contains(s.Prefs().Spoken, l.Code) }
		name := spokenNames[l.Code]
		tiles = append(tiles, core.View{TestID: "onboarding-spoken-" + l.Code, Role: core.RoleCheckbox, Label: l.Name, Checked: on,
			OnPress: func(core.PressEvent) {
				keep(func(p *store.Prefs) {
					if i := slices.Index(p.Spoken, l.Code); i >= 0 {
						p.Spoken = slices.Delete(slices.Clone(p.Spoken), i, i+1)
						return
					}
					p.Spoken = append(slices.Clone(p.Spoken), l.Code)
				})
			},
			Style: core.Style{Height: core.Pt(58), MinWidth: core.Pt(0), Padding: core.Symmetric(0, 10), CornerRadius: 14, BorderCurve: core.BorderCurveContinuous, BorderWidth: 2,
				JustifyContent: core.JustifyCenter, Active: &core.Style{Opacity: core.Alpha(0.7)}},
			// The ring is drawn inside the tile, so a tile picked keeps its size.
			StyleFunc: func() core.Style {
				if on() {
					return core.Style{Background: tints.Accent(), BorderColor: colors.Accent()}
				}
				return core.Style{Background: colors.Inset(), BorderColor: core.Transparent}
			},
			Children: []core.Node{
				core.Text{Content: l.Name, MaxLines: 1, Style: core.Style{FontSize: 15, LineHeight: 20, FontWeight: core.FontWeightBold}, StyleFunc: ui.Colored(colors.Text)},
				core.Text{ContentFunc: func() string { return name(cx) }, MaxLines: 1, Style: look.Footnote, StyleFunc: ui.Colored(colors.Muted)},
			}})
	}
	picking := core.NewSignal(cx, false)
	return core.View{TestID: "onboarding-languages", Style: core.Style{Gap: 14}, Children: []core.Node{
		heading(cx, false, func() string { return locales.OnboardingLanguagesTitle(cx) }, func() string { return locales.OnboardingLanguagesBody(cx) }),
		core.View{Role: core.RoleGroup, LabelFunc: func() string { return locales.OnboardingLanguagesTitle(cx) },
			Style: core.Style{Display: core.DisplayGrid, GridTemplateColumns: "repeat(3, minmax(0, 1fr))", Gap: 8}, Children: tiles},
		core.View{TestID: "onboarding-target", Role: core.RoleButton, LabelFunc: func() string { return locales.OnboardingTarget(cx) }, AccessibilityValue: func() core.AccessibilityValue { return core.AccessibilityValue{Text: langs.TargetName(s.Target())} },
			OnPress: func(core.PressEvent) { picking.Set(true) },
			Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 12, Height: core.Pt(50), Padding: core.Symmetric(0, 16), CornerRadius: 14, BorderCurve: core.BorderCurveContinuous,
				Active: &core.Style{Opacity: core.Alpha(0.7)}},
			StyleFunc: ui.Filled(colors.Inset),
			Children: []core.Node{
				core.Text{ContentFunc: func() string { return locales.OnboardingTarget(cx) }, MaxLines: 1, Style: look.Body.Merge(core.Style{FlexGrow: 1, FlexShrink: 1}), StyleFunc: ui.Colored(colors.Text)},
				core.Text{ContentFunc: func() string { return langs.TargetName(s.Target()) }, MaxLines: 1, Style: look.Body.Merge(core.Style{FontWeight: core.FontWeightBold}), StyleFunc: ui.Colored(colors.Accent)},
			}},
		targetSheet(cx, s, picking),
	}}
}

// targetSheet is the list of the languages subtitles can be translated into, the one they are translated into checked, while open; picking one keeps it and closes the list. There are too many to step through by pressing the row, as the design's prototype does.
func targetSheet(cx *core.Scope, s *session.Session, open *core.Signal[bool]) core.Node {
	colors := ui.ColorsOf(cx)
	var rows []core.Node
	for i, l := range langs.Targets {
		rows = append(rows, look.Row{TestID: "onboarding-target-" + l.Code, Index: func() int { return i },
			Title:   func() string { return l.Name },
			Checked: func() bool { return s.Target() == l.Code },
			Trailing: core.Show{When: func() bool { return s.Target() == l.Code }, Then: func(*core.Scope) core.Node {
				return ui.Icon(lucide.Check, 20, colors.Accent)
			}},
			OnPress: func() {
				s.SetTarget(l.Code)
				open.Set(false)
			}})
	}
	return look.Dialog{TestID: "onboarding-target-sheet", Open: open.Get, OnClose: func() { open.Set(false) }, Title: func() string { return locales.OnboardingTarget(cx) },
		Children: []core.Node{core.Scroll{Style: core.Style{FlexShrink: 1}, Children: []core.Node{
			core.View{Role: core.RoleRadioGroup, LabelFunc: func() string { return locales.OnboardingTarget(cx) }, Children: []core.Node{look.Group{Children: rows}}},
		}}}}
}
