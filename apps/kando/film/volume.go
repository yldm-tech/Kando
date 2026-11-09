package film

import (
	"github.com/egoist/mygo/ui"
	breeze "github.com/yldm-tech/mygo-breeze-ui/ui/breeze"
)

// volumeKnob is the diameter of the volume slider's knob; the slider is padded by half of it so the knob stays inside.
const volumeKnob = 10

// volume is the transport's sound control: a speaker that mutes and unmutes, its glyph showing how loud the film plays, and a short slider for the volume, white over the glass as the scrubber is. While muted the slider shows nothing played; dragging it unmutes.
func (s screen) volume(c *ui.Context) {
	muted, volume := s.Muted(), s.Volume()
	shown := volume
	if muted {
		shown = 0
	}
	ui.Row(c).Gap(2).AlignItems(ui.Center).Shrink(0).Children(func() {
		glyph, label := iconVolume, "静音"
		switch {
		case muted || volume == 0:
			glyph, label = iconMuted, "取消静音"
		case volume < 0.5:
			glyph = iconVolumeLow
		}
		c.Key("film-mute")
		if glyphButton(c, glyph, label, 32, 30, 20, 20).Tooltip(label + " " + breeze.ShortcutText(c, 0, ui.KeyM)).Clicked() {
			s.ToggleMute()
		}
		c.Key("film-volume")
		value := shown
		bar := ui.SliderBase(c, &value, 0, 1).Step(0.1).Width(64).Height(22).PaddingX(volumeKnob / 2).Label("音量")
		if bar.Changed() {
			s.SetVolume(value)
		}
		bar.Draw(func(p *ui.Painter, r ui.Rect) {
			track := ui.Rect{X: r.X + volumeKnob/2, Y: r.Y + r.H/2 - 2, W: r.W - volumeKnob, H: 4}
			p.Fill(track, scrubTrack, 2)
			x := track.X + track.W*float32(min(max(value, 0), 1))
			p.Fill(ui.Rect{X: track.X, Y: track.Y, W: x - track.X, H: track.H}, onGlass, 2)
			p.Fill(ui.Rect{X: x - volumeKnob/2, Y: r.Y + r.H/2 - volumeKnob/2, W: volumeKnob, H: volumeKnob}, onGlass, volumeKnob/2)
		})
	})
}
