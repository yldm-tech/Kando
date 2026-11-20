package look

import (
	"time"

	"kando/apps/mobile/theme"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

// Switch is an on and off switch as the platform draws one: the system's own on iOS and macOS (ui.SwitchSystem), Material 3's on Android, which NSGo has no switch of (M3Switch). Label is what assistive technology reads.
type Switch struct {
	Checked  func() bool
	OnChange func(bool)
	Disabled func() bool
	Label    string
	TestID   string
}

// Render implements core.Node.
func (s Switch) Render(cx *core.Scope) core.Node {
	if cx.Env().OS() == core.OSAndroid {
		return M3Switch(cx, s)
	}
	return ui.Switch{Checked: s.Checked, OnChange: s.OnChange, Disabled: s.Disabled, Label: s.Label, TestID: s.TestID, Variant: ui.SwitchSystem}
}

// The Material 3 switch's measures: a 52 × 32 track with a 2pt outline while off, its knob 24 across on and 16 off, centred 16 from the track's start off and 36 on.
const (
	m3TrackWidth  core.Points = 52
	m3TrackHeight core.Points = 32
	m3Outline     core.Points = 2
	m3Knob        core.Points = 24
	m3KnobOff                 = 16.0 / 24
	m3Travel                  = 20.0
)

// m3Timing is how the knob slides and grows, Material's standard easing over 200ms.
var m3Timing = core.TimingConfig{Duration: 200 * time.Millisecond, Easing: core.BezierEasing(core.Curve{X1: 0.2, Y1: 0, X2: 0, Y2: 1})}

// M3Switch is s in Material 3's look, as the design's Android screens draw it: on, the accent's track under a white 24pt knob at its end; off, the fill's track in an outline under a 16pt knob of the outline's color at its start. The knob slides, grows and changes color as Checked changes, in place under reduced motion.
// TODO(nsgo#937): core.SystemSwitch is macOS's and iOS's only (core/systemswitch.go:28-31) and ui.Switch has no Material look (ui/switch.go:49-107), so the app draws Android's switch on base.Switch; use the system's once core.SystemSwitch has Android's. https://github.com/nsgo-dev/nsgo/issues/937
func M3Switch(cx *core.Scope, s Switch) core.Node {
	colors := ui.ColorsOf(cx)
	shown := core.Untracked(cx, s.Checked)
	start := 0.0
	if shown {
		start = 1
	}
	t := core.NewAnimatedValue(cx, start)
	env := cx.Env()
	// A memo, since the effect's next run stops the slide it started, and Checked may read what changes without changing its value.
	value := core.NewMemo(cx, s.Checked)
	cx.Effect(func() {
		next := value.Get()
		if next == shown {
			return
		}
		shown = next
		to := 0.0
		if next {
			to = 1
		}
		if env.ReduceMotion() {
			t.Set(to)
		} else {
			ui.Timing(cx, t, to, m3Timing)
		}
	})
	track := func() core.Style {
		off, on := colors.Inset(), colors.Accent()
		return core.Style{Width: core.Pt(m3TrackWidth), Height: core.Pt(m3TrackHeight), CornerRadius: m3TrackHeight / 2, BorderWidth: m3Outline,
			Background: off, BorderColor: theme.M3Outline, Disabled: &core.Style{Opacity: core.Alpha(0.38)},
			Animated: &core.AnimatedStyle{
				Background:  core.Interpolate(t, core.Interpolation{Input: []float64{0, 1}, Colors: []core.Color{off, on}}),
				BorderColor: core.Interpolate(t, core.Interpolation{Input: []float64{0, 1}, Colors: []core.Color{theme.M3Outline, on}}),
			}}
	}
	thumb := func() core.Style {
		// Placed inside the outline: 2 in from it is 4 from the track's edge, the knob's centre 16 from its start.
		return core.Style{Position: core.Absolute, Left: core.Pt(2), Top: core.Pt(2), Width: core.Pt(m3Knob), Height: core.Pt(m3Knob), CornerRadius: m3Knob / 2,
			Background: theme.M3Outline,
			Animated: &core.AnimatedStyle{
				TranslateX: ui.Lerp(t, 0, m3Travel),
				Scale:      ui.Lerp(t, m3KnobOff, 1),
				Background: core.Interpolate(t, core.Interpolation{Input: []float64{0, 1}, Colors: []core.Color{theme.M3Outline, colors.OnAccent()}}),
			}}
	}
	// More to press on every side, without taking more room.
	return base.Switch{Checked: s.Checked, OnChange: s.OnChange, Disabled: s.Disabled, Label: s.Label, TestID: s.TestID,
		Style:          core.Style{Margin: core.All(-8), Padding: core.All(8)},
		TrackStyleFunc: track, ThumbStyleFunc: thumb}
}
