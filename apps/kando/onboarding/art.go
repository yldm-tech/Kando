package onboarding

import (
	"kando/apps/kando/live"
	"kando/apps/kando/locales"

	"github.com/nsgo-dev/nsgo/core"
)

// scene is the prototype's river landscape: its sky from top to bottom, then the sun, the far hills, the near ones and the river, in a 320 by 180 view that the panel covers, cropping its sides. It is drawn as an image, which core.Cover scales to cover the panel.
var (
	skyTop    = core.RGB(0x8DB9D2)
	skyBottom = core.RGB(0xEBDDBB)
	scene     = []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="320" height="180" viewBox="0 0 320 180"><circle cx="238" cy="58" r="17" fill="#FFF2CC"/><path d="M0 110 L60 70 L110 95 L170 55 L230 90 L280 65 L320 85 L320 180 L0 180Z" fill="#6A8EA1"/><path d="M0 140 Q80 110 160 130 T320 120 L320 180 L0 180Z" fill="#3E6A5B"/><path d="M150 180 C170 160 190 150 180 135 C172 124 200 118 215 112 L222 116 C210 122 188 128 196 140 C206 156 188 168 176 180Z" fill="#A9D2DE"/></svg>`)
	// shade darkens the top of the picture under the caption and its bottom under the example line.
	shade = core.Gradient{Kind: core.GradientLinear, Angle: 180, Stops: []core.ColorStop{
		{Color: core.RGBA(0x000000, 0.28), At: core.Pct(0)},
		{Color: core.RGBA(0x000000, 0), At: core.Pct(30)},
		{Color: core.RGBA(0x000000, 0), At: core.Pct(48)},
		{Color: core.RGBA(0x000000, 0.62), At: core.Pct(100)},
	}}
	white = core.RGB(0xFFFFFF)
)

// art is the left panel: the landscape, the product's name over the step's caption at its top, and at its bottom a pill naming what the step is about over an example line and its translation, as a film's subtitles would show them. The window's own traffic lights stay where they are; the panel does not draw a second set. step is the step shown.
type art struct {
	a    App
	step func() int
}

// Render implements core.Node.
func (r art) Render(cx *core.Scope) core.Node {
	a := r.a
	fill := core.Style{Position: core.Absolute, Top: core.Pt(0), Left: core.Pt(0), Right: core.Pt(0), Bottom: core.Pt(0)}
	// The example follows the languages chosen and the target language, read again only when they change.
	example := live.Memo(cx, a, func() demoLine {
		code, original, translation := demo(a.OnboardingLearning(), *a.Language())
		return demoLine{code: code, original: original, translation: translation, target: *a.Language()}
	})
	caption := core.View{Style: core.Style{Position: core.Absolute, Left: core.Pt(24), Right: core.Pt(24), Top: core.Pt(52), Gap: 6, Color: white},
		Children: []core.Node{
			core.Text{Content: "KANDO", Style: core.Style{FontSize: 11, FontWeight: core.FontWeightBold, LetterSpacing: 1.32, Opacity: core.Alpha(0.85)}},
			core.Text{ContentFunc: func() string { return caption(cx, r.step()) }, Style: core.Style{FontSize: 20, FontWeight: core.FontWeightBold, LineHeight: 27}},
		}}
	pill := core.View{
		Style: core.Style{Padding: core.Symmetric(3, 10), CornerRadius: 10, Background: core.RGBA(0xFFFFFF, 0.18),
			Shadows: []core.BoxShadow{{Spread: 0.5, Color: core.RGBA(0xFFFFFF, 0.3), Inset: true}}},
		Children: []core.Node{core.Text{MaxLines: 1, EllipsizeMode: core.EllipsizeTail, Style: core.Style{FontSize: 11, FontWeight: core.FontWeightSemibold},
			ContentFunc: func() string {
				e := example()
				return pill(cx, a, r.step(), e.code, e.target)
			}}},
	}
	// TODO(nsgo#207): give core.Text the language it is written in, as MyGo's Text.Lang did, so the example line in Japanese, Korean or Chinese is drawn with that language's glyph forms rather than the interface language's https://github.com/nsgo-dev/nsgo/issues/207
	original := core.Text{ContentFunc: func() string { return example().original },
		Style: core.Style{FontSize: 13, LineHeight: 18, Opacity: core.Alpha(0.9), TextAlign: core.TextAlignCenter}}
	translation := core.Text{ContentFunc: func() string { return example().translation },
		Style: core.Style{FontSize: 18, FontWeight: core.FontWeightBold, LineHeight: 25, TextAlign: core.TextAlignCenter}}
	foot := core.View{Style: core.Style{Position: core.Absolute, Left: core.Pt(20), Right: core.Pt(20), Bottom: core.Pt(26), Gap: 10, AlignItems: core.AlignCenter, Color: white},
		Children: []core.Node{pill, original, translation}}
	return core.View{
		Style: core.Style{Width: core.Pt(artWidth), FlexShrink: 0, Overflow: core.Hidden,
			Gradients: []core.Gradient{core.LinearGradient(180, skyTop, skyBottom)}},
		Children: []core.Node{
			core.Image{Source: core.ImageSource{Data: scene}, ResizeMode: core.Cover, Style: fill},
			core.View{PointerEvents: core.PointerEventsNone, Style: fill.Merge(core.Style{Gradients: []core.Gradient{shade}})},
			caption,
			foot,
		},
	}
}

// demoLine is the art panel's example as demo gives it, with the target language it is translated into.
type demoLine struct {
	code, original, translation, target string
}

// caption is the line over the art for step, naming the machine whisper runs on as the platform calls it: 这台 Mac on macOS, 这台电脑 elsewhere.
func caption(cx *core.Scope, step int) string {
	switch step {
	case 1:
		return locales.OnboardingCaptionLanguages(cx)
	case 2:
		if onMac(cx) {
			return locales.OnboardingCaptionSpeechMac(cx)
		}
		return locales.OnboardingCaptionSpeechComputer(cx)
	}
	return locales.OnboardingCaptionLink(cx)
}

// pill names what step is about: on the first step the example's languages, original code into target, a value the settings keep, which the app names (App.OnboardingTargetName).
func pill(cx *core.Scope, a App, step int, code, target string) string {
	switch step {
	case 1:
		return locales.OnboardingPillLanguages(cx, languageName(cx, code), a.OnboardingTargetName(cx, target))
	case 2:
		return locales.OnboardingPillModels(cx)
	}
	return locales.OnboardingPillLink(cx)
}
