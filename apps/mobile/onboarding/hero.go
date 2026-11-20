package onboarding

import (
	_ "embed"
	"time"

	"kando/apps/mobile/brand"
	"kando/apps/mobile/locales"
	"kando/apps/mobile/theme"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

// The river scenes the introduction's picture fades between, drawn from the design's art (river1–4) and cropped to the picture's shape.
var (
	//go:embed art/river1.png
	river1 []byte
	//go:embed art/river2.png
	river2 []byte
	//go:embed art/river3.png
	river3 []byte
	//go:embed art/river4.png
	river4 []byte
)

var scenes = [][]byte{river1, river2, river3, river4}

// originals are the English lines of the sample film the picture plays, in order: the original film's text, which stays as it is in every language, as a film's original line does. The translation over each is its catalog message (cues).
var originals = []string{
	"Every river begins as a trickle high in the mountains.",
	"As water flows downhill, it picks up sand and stones.",
	"Those particles act like sandpaper on the riverbed.",
	"Over thousands of years, the channel cuts deeper.",
	"That is how steep V-shaped valleys are formed.",
	"Lower down, the land flattens and the river slows.",
	"Instead of cutting down, it starts to swing sideways.",
	"These sweeping curves are called meanders.",
}

// cues are the translations of originals in the language the app shows.
var cues = []func(*core.Scope) string{
	locales.OnboardingCue1, locales.OnboardingCue2, locales.OnboardingCue3, locales.OnboardingCue4,
	locales.OnboardingCue5, locales.OnboardingCue6, locales.OnboardingCue7, locales.OnboardingCue8,
}

const (
	// heroHeight is the picture's height, the design's on a phone and on a tablet's card alike.
	heroHeight = 380
	// cueFor is how long each line shows; a scene lasts two lines.
	cueFor = 3500 * time.Millisecond
	// kenBurns is how long the picture takes to drift one way, then back.
	kenBurns = 22 * time.Second
)

// hero is the picture across the top of every step: river scenes fading one into the next as they drift (Ken Burns), the app's icon and name, the Skip pill that leaves the introduction, and a line of the sample film with its translation over it, changing every few seconds, as the player shows subtitles. It fades to the page at its foot, under the step dots.
func hero(cx *core.Scope, skip func()) core.Node {
	colors := ui.ColorsOf(cx)
	cue := core.NewSignal(cx, 0)
	var next func()
	next = func() {
		cue.Update(func(n int) int { return (n + 1) % len(originals) })
		cx.AfterFunc(cueFor, next)
	}
	cx.AfterFunc(cueFor, next)

	// The scenes drift together, from 1.02 times their size to 1.14 and a little down and left, and back, unless the user asked for less motion.
	drift := core.NewAnimatedValue(cx, 0)
	if !ui.ReduceMotion(cx) {
		cx.StartAnimation(core.ValueKeyframes(drift, core.KeyframeTiming{Duration: kenBurns, Easing: core.BezierEasing(core.EaseInOut), Iterations: core.Forever, Direction: core.PlayAlternate},
			core.Stops(0, 1)...), nil)
	}
	along := func(from, to float64) core.AnimatedNode {
		return core.Interpolate(drift, core.Interpolation{Input: []float64{0, 1}, Output: []float64{from, to}})
	}
	fill := core.Style{Position: core.Absolute, Left: core.Pt(-4), Top: core.Pt(-4), Right: core.Pt(-4), Bottom: core.Pt(-4)}
	var pictures []core.Node
	for i, scene := range scenes {
		pictures = append(pictures, core.Image{Source: core.ImageSource{Data: scene}, ResizeMode: core.Cover, ElementsHidden: true,
			Style: fill.Merge(core.Style{Transition: core.Transition{Duration: 1400 * time.Millisecond, Easing: core.EaseInOut, Properties: core.TransitionOpacity}}),
			StyleFunc: func() core.Style {
				if cue.Get()/2 == i {
					return core.Style{Opacity: core.Alpha(1)}
				}
				return core.Style{Opacity: core.Alpha(0)}
			}})
	}
	art := core.View{Style: fill, StyleFunc: func() core.Style {
		return core.Style{Animated: &core.AnimatedStyle{Scale: along(1.02, 1.14), TranslateX: along(0, -10), TranslateY: along(0, 6)}}
	}, Children: pictures}

	// The shade darkens the top for the icon and Skip, and fades the foot into the page.
	shade := core.View{Style: core.Style{Position: core.Absolute, Left: core.Pt(0), Top: core.Pt(0), Right: core.Pt(0), Bottom: core.Pt(0)}, StyleFunc: func() core.Style {
		page := colors.Surface()
		return core.Style{Gradients: []core.Gradient{{Kind: core.GradientLinear, Angle: 180, Stops: []core.ColorStop{
			{Color: ui.Black(0.25), At: core.Pct(0)}, {Color: ui.Black(0), At: core.Pct(30)}, {Color: ui.Black(0), At: core.Pct(55)},
			{Color: core.SetColor(ui.Alpha(page, 0)), At: core.Pct(70)}, {Color: page, At: core.Pct(100)},
		}}}}
	}}

	onPicture := core.Style{Color: theme.OnPicture, TextShadowColor: ui.Black(0.4), TextShadowOffset: core.Point{Y: 1}, TextShadowRadius: 4}
	top := core.View{Style: core.Style{Position: core.Absolute, Left: core.Pt(16), Right: core.Pt(16), Top: core.Pt(14), Direction: core.Row, AlignItems: core.AlignCenter, PaddingEnv: core.EnvEdges{Top: core.EnvSafeArea}},
		Children: []core.Node{
			core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 8, FlexGrow: 1}, Children: []core.Node{
				core.Image{Source: brand.Icon(), ResizeMode: core.Cover, ElementsHidden: true, Style: core.Style{Width: core.Pt(30), Height: core.Pt(30), CornerRadius: 7, BorderCurve: core.BorderCurveContinuous, FlexShrink: 0}},
				core.Text{ContentFunc: func() string { return locales.AppName(cx) }, Style: onPicture.Merge(core.Style{FontSize: 15, LineHeight: 20, FontWeight: core.FontWeightBold})},
			}},
			// The pill is on the picture, so it is the picture's dark glass rather than one of the theme's fills.
			base.Button{TestID: "onboarding-skip", LabelFunc: func() string { return locales.OnboardingSkip(cx) }, OnPress: func(core.PressEvent) { skip() },
				Style: core.Style{Height: core.Pt(34), Padding: core.Symmetric(0, 14), CornerRadius: 17, AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter,
					Background: theme.PictureGlass, BackdropFilters: []core.Filter{core.Blur(10)}, Active: &core.Style{Opacity: core.Alpha(0.7)}},
				StyleFunc: func() core.Style { return core.Style{FocusVisible: ui.FocusRing(cx, 2)} },
				Children: []core.Node{core.Text{ContentFunc: func() string { return locales.OnboardingSkip(cx) }, MaxLines: 1,
					Style: core.Style{Color: theme.OnPicture, FontSize: 14, LineHeight: 18, FontWeight: core.FontWeightSemibold}}}},
		}}

	// The line as the player shows it: the translation, white, over the original in the subtitles' yellow.
	subtitle := core.Style{TextAlign: core.TextAlignCenter, TextShadowColor: ui.Black(0.9), TextShadowRadius: 3}
	// The sample line changes every few seconds, which assistive technology would read out each time.
	line := core.View{TestID: "onboarding-cue", AccessibilityElementsHidden: func() bool { return true },
		Style: core.Style{Position: core.Absolute, Left: core.Pt(20), Right: core.Pt(20), Top: core.Pt(190), Gap: 3},
		Children: []core.Node{
			core.Text{ContentFunc: func() string { return cues[cue.Get()](cx) }, Style: subtitle.Merge(core.Style{Color: theme.OnPicture, FontSize: 20, LineHeight: 27, FontWeight: core.FontWeightBold})},
			core.Text{ContentFunc: func() string { return originals[cue.Get()] }, Style: subtitle.Merge(core.Style{Color: theme.SubtitleYellow, FontSize: 14, LineHeight: 18, Language: "en"})},
		}}

	return core.View{TestID: "onboarding-hero", Style: core.Style{Height: core.Pt(heroHeight), FlexShrink: 0, Overflow: core.Hidden, Background: theme.PictureBlack},
		Children: []core.Node{art, shade, line, top}}
}
