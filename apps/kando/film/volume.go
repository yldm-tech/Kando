package film

import (
	"kando/apps/kando/live"
	"kando/apps/kando/locales"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
)

// volumeKnob is the diameter of the volume slider's knob; the slider is padded by half of it so the knob stays inside.
const volumeKnob core.Points = 10

// volume is the transport's sound control: a speaker that mutes and unmutes, its glyph showing how loud the film plays, and a short slider for the volume, white over the glass as the scrubber is. While muted the slider shows nothing played; dragging it unmutes.
func (s *screen) volume(cx *core.Scope) core.Node {
	a := s.a
	shown := live.Func(a, func() float64 {
		if a.Muted() {
			return 0
		}
		return a.Volume()
	})
	silent := live.Memo(cx, a, func() int {
		switch {
		case a.Muted() || a.Volume() == 0:
			return 0
		case a.Volume() < 0.5:
			return 1
		}
		return 2
	})
	label := func() string {
		if silent() == 0 {
			return locales.FilmUnmute(cx)
		}
		return locales.FilmMute(cx)
	}
	glyph := func() *core.SVG {
		switch silent() {
		case 0:
			return iconMuted
		case 1:
			return iconVolumeLow
		}
		return iconVolume
	}
	mute := glyphButton(cx, glyph, label, withKey(cx, label, "m"), 32, 30, 20, 20, "film-mute", live.Do(a, a.ToggleMute))
	draw := func(d *core.Drawing, size core.Size) {
		knob, h := float32(volumeKnob), float32(size.Height)
		w := float32(size.Width) - knob
		y := h/2 - 2
		x := knob/2 + w*float32(min(max(shown(), 0), 1))
		d.SetFillColor(scrubTrack).BeginPath().RoundRect(knob/2, y, w, 4, 2).Fill()
		d.SetFillColor(onGlass).BeginPath().RoundRect(knob/2, y, x-knob/2, 4, 2).Fill()
		d.SetFillColor(onGlass).BeginPath().RoundRect(x-knob/2, h/2-knob/2, knob, knob, knob/2).Fill()
	}
	slider := base.Slider{
		TestID: "film-volume", LabelFunc: func() string { return locales.FilmVolume(cx) },
		Min: func() float64 { return 0 }, Max: func() float64 { return 1 }, Step: func() float64 { return 0.1 },
		Value:      func() base.SliderValue { return base.SliderValue{End: shown()} },
		OnChange:   live.On(a, func(v base.SliderValue) { a.SetVolume(v.End) }),
		Style:      core.Style{Position: core.Absolute, Top: core.Pt(0), Bottom: core.Pt(0), Left: core.Pt(volumeKnob / 2), Right: core.Pt(volumeKnob / 2)},
		TrackStyle: core.Style{FlexGrow: 1},
	}
	bar := core.View{Style: core.Style{Width: core.Pt(64), Height: core.Pt(22), FlexShrink: 0}, Children: []core.Node{
		core.Canvas{Style: fillStyle, Draw: draw},
		slider,
	}}
	return row(2, mute, bar)
}
