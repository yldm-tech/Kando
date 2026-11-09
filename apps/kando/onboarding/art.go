package onboarding

import (
	"kando/apps/kando/film/subtitle"
	"kando/apps/kando/locales"

	"github.com/egoist/mygo/ui"
)

// scene is the prototype's river landscape: its sky from top to bottom, then the sun, the far hills, the near ones and the river, in a 320 by 180 view that the panel covers, cropping its sides.
var (
	skyTop    = ui.Hex("#8DB9D2")
	skyBottom = ui.Hex("#EBDDBB")
	scene     = icon(`<svg xmlns="http://www.w3.org/2000/svg" width="320" height="180" viewBox="0 0 320 180"><circle cx="238" cy="58" r="17" fill="#FFF2CC"/><path d="M0 110 L60 70 L110 95 L170 55 L230 90 L280 65 L320 85 L320 180 L0 180Z" fill="#6A8EA1"/><path d="M0 140 Q80 110 160 130 T320 120 L320 180 L0 180Z" fill="#3E6A5B"/><path d="M150 180 C170 160 190 150 180 135 C172 124 200 118 215 112 L222 116 C210 122 188 128 196 140 C206 156 188 168 176 180Z" fill="#A9D2DE"/></svg>`)
	// shade darkens the top of the picture under the caption and its bottom under the example line.
	shade = ui.LinearGradient{Angle: 180, Stops: []ui.GradientStop{
		{Color: ui.RGBA(0, 0, 0, 0.28), At: 0},
		{Color: ui.RGBA(0, 0, 0, 0), At: 0.3},
		{Color: ui.RGBA(0, 0, 0, 0), At: 0.48},
		{Color: ui.RGBA(0, 0, 0, 0.62), At: 1},
	}}
	white = ui.Hex("#FFFFFF")
)

// art is the left panel: the landscape, the product's name over the step's caption at its top, and at its bottom a pill naming what the step is about over an example line and its translation, as a film's subtitles would show them. The window's own traffic lights stay where they are; the panel does not draw a second set.
func art(c *ui.Context, a App, step int) {
	ui.Column(c).Width(artWidth).Shrink(0).Clip().LinearGradient(ui.LinearGradient{Angle: 180, Stops: []ui.GradientStop{{Color: skyTop, At: 0}, {Color: skyBottom, At: 1}}}).Children(func() {
		ui.Image(c, scene).Absolute().Top(0).Left(0).WidthPercent(100).HeightPercent(100).Fit(ui.Cover)
		ui.Box(c).Absolute().Top(0).Left(0).Right(0).Bottom(0).PassThrough().LinearGradient(shade)
		ui.Column(c).Absolute().Left(24).Right(24).Top(52).Gap(6).TextColor(white).Children(func() {
			ui.Text(c, "KANDO").FontSize(11).Bold().LetterSpacing(1.32).Opacity(0.85)
			ui.Text(c, caption(step)).FontSize(20).Bold().FixedLineHeight(27).Wrap()
		})
		code, original, translation := demo(a.OnboardingLearning(), *a.Language())
		ui.Column(c).Absolute().Left(20).Right(20).Bottom(26).Gap(10).AlignItems(ui.Center).TextColor(white).Children(func() {
			ui.Text(c, pill(step, code, *a.Language())).FontSize(11).FontWeight(600).Padding(3, 10).Radius(10).Background(ui.RGBA(255, 255, 255, 0.18)).InsetShadow(0, 0, 0, 0.5, ui.RGBA(255, 255, 255, 0.3)).SingleLine()
			ui.Text(c, original).FontSize(13).FixedLineHeight(18).Opacity(0.9).TextAlign(ui.Center).Wrap().Lang(code)
			ui.Text(c, translation).FontSize(18).Bold().FixedLineHeight(25).TextAlign(ui.Center).Wrap()
		})
	})
}

// caption is the line over the art for step.
func caption(step int) string {
	switch step {
	case 1:
		return locales.OnboardingCaptionLanguages()
	case 2:
		if onMac {
			return locales.OnboardingCaptionSpeechMac()
		}
		return locales.OnboardingCaptionSpeechComputer()
	}
	return locales.OnboardingCaptionLink()
}

// pill names what step is about: on the first step the example's languages, original code into target.
func pill(step int, code, target string) string {
	switch step {
	case 1:
		return locales.OnboardingPillLanguages(languageName(code), targetName(target))
	case 2:
		return locales.OnboardingPillModels()
	}
	return locales.OnboardingPillLink()
}

// targetName is what the pill calls the target language target, a value the settings keep (subtitle.TargetLanguageName).
func targetName(target string) string { return subtitle.TargetLanguageName(target) }
