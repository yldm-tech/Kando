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
	// heroHeight is the picture's height, the design's on a phone and on a tablet's card alike; shortHero is its height in a window too short for it, as a phone's on its side, which leaves the steps room.
	heroHeight = 380
	shortHero  = 200
	// shortWindow is the height of a window under which the picture is shortHero high.
	shortWindow = 500
	// cueFor is how long each line shows; a scene lasts two lines.
	cueFor = 3500 * time.Millisecond
	// kenBurns is how long the picture takes to drift one way, then back.
	kenBurns = 22 * time.Second
)

// heroFor is the picture's height in the window cx shows.
func heroFor(cx *core.Scope) core.Points {
	if cx.Env().Window().Height < shortWindow {
		return shortHero
	}
	return heroHeight
}

// hero is the picture across the top of every step, height high: river scenes fading one into the next as they drift (Ken Burns), and a line of the sample film with its translation over it, changing every few seconds, as the player shows subtitles. It fades to the page at its foot, under the step dots. The app's icon and name and the Skip pill over it are topBar, which stays as the picture scrolls away.
func hero(cx *core.Scope, height func() core.Points, phone bool) core.Node {
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

	// The line as the player shows it: the translation, white, over the original in the subtitles' yellow.
	subtitle := core.Style{TextAlign: core.TextAlignCenter, TextShadowColor: ui.Black(0.9), TextShadowRadius: 3}
	// The sample line changes every few seconds, which assistive technology would read out each time.
	line := core.View{TestID: "onboarding-cue", AccessibilityElementsHidden: func() bool { return true },
		Style: core.Style{Position: core.Absolute, Left: core.Pt(20), Right: core.Pt(20), Top: core.Pct(50), Gap: 3, PaddingEnv: sides(phone)},
		Children: []core.Node{
			core.Text{ContentFunc: func() string { return cues[cue.Get()](cx) }, Style: subtitle.Merge(core.Style{Color: theme.OnPicture, FontSize: 20, LineHeight: 27, FontWeight: core.FontWeightBold})},
			core.Text{ContentFunc: func() string { return originals[cue.Get()] }, Style: subtitle.Merge(core.Style{Color: theme.SubtitleYellow, FontSize: 14, LineHeight: 18, Language: "en"})},
		}}

	return core.View{TestID: "onboarding-hero", Style: core.Style{FlexShrink: 0, Overflow: core.Hidden, Background: theme.PictureBlack},
		StyleFunc: func() core.Style { return core.Style{Height: core.Pt(height())} },
		Children:  []core.Node{art, shade, line}}
}

// sides keeps a phone's content clear of the safe area's sides, a notch's or a Dynamic Island's on its side; a tablet's card is in the middle of the screen, clear of them already.
func sides(phone bool) core.EnvEdges {
	if !phone {
		return core.EnvEdges{}
	}
	return core.EnvEdges{Left: core.EnvSafeArea, Right: core.EnvSafeArea}
}

// topBar is the app's icon and name and the Skip pill, over the top of the page, which stay while the picture scrolls away under them: on the picture's dark top they are white, and once the page scrolls under them (over) they sit on the page in its ink. Skip leaves the introduction; it waits while a pairing code is out (sending), which cannot be called back. On a phone it keeps clear of the status bar and the safe area's sides; a tablet's card is clear of them already.
func topBar(cx *core.Scope, skip func(), sending, over func() bool, phone bool) core.Node {
	colors := ui.ColorsOf(cx)
	env := sides(phone)
	if phone {
		env.Top = core.EnvSafeArea
	}
	onPicture := core.Style{TextShadowColor: ui.Black(0.4), TextShadowOffset: core.Point{Y: 1}, TextShadowRadius: 4}
	return core.View{TestID: "onboarding-top", Style: core.Style{Position: core.Absolute, Left: core.Pt(0), Right: core.Pt(0), Top: core.Pt(0), Direction: core.Row, AlignItems: core.AlignCenter,
		Padding: core.Edges{Top: 14, Right: 16, Bottom: 8, Left: 16}, PaddingEnv: env, Transition: core.Transition{Duration: 200 * time.Millisecond, Properties: core.TransitionBackground}},
		StyleFunc: func() core.Style {
			if over() {
				return core.Style{Background: colors.Surface()}
			}
			return core.Style{Background: core.Transparent}
		},
		Children: []core.Node{
			core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 8, FlexGrow: 1}, Children: []core.Node{
				// The design's icon is a 24-point tile in a 30-point box, its margin clear.
				core.View{Style: core.Style{Width: core.Pt(30), Height: core.Pt(30), AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter, FlexShrink: 0}, Children: []core.Node{
					core.Image{Source: brand.Icon(), ResizeMode: core.Cover, ElementsHidden: true, Style: core.Style{Width: core.Pt(24), Height: core.Pt(24), CornerRadius: 5.5, BorderCurve: core.BorderCurveContinuous}},
				}},
				core.Text{ContentFunc: func() string { return locales.AppName(cx) }, Style: core.Style{FontSize: 15, LineHeight: 20, FontWeight: core.FontWeightBold},
					StyleFunc: func() core.Style {
						if over() {
							return core.Style{Color: colors.Text()}
						}
						return onPicture.Merge(core.Style{Color: theme.OnPicture})
					}},
			}},
			// The pill is on the picture, so it is the picture's dark glass rather than one of the theme's fills; on the page the glass still reads.
			base.Button{TestID: "onboarding-skip", LabelFunc: func() string { return locales.OnboardingSkip(cx) }, OnPress: func(core.PressEvent) { skip() }, Disabled: sending,
				Style: core.Style{Height: core.Pt(34), Padding: core.Symmetric(0, 14), CornerRadius: 17, AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter,
					Background: theme.PictureGlass, BackdropFilters: []core.Filter{core.Blur(10)}, Active: &core.Style{Opacity: core.Alpha(0.7)}, Disabled: ui.Dimmed()},
				StyleFunc: func() core.Style { return core.Style{FocusVisible: ui.FocusRing(cx, 2)} },
				Children: []core.Node{core.Text{ContentFunc: func() string { return locales.OnboardingSkip(cx) }, MaxLines: 1,
					Style: core.Style{Color: theme.OnPicture, FontSize: 14, LineHeight: 18, FontWeight: core.FontWeightSemibold}}}},
		}}
}
