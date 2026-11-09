package film

import (
	"time"

	"kando/apps/kando/locales"
	"kando/packages/core"

	"github.com/egoist/mygo/ui"
)

// The scrubber's marks, from the prototype (template 606-618).
var (
	scrubTrack      = ui.RGBA(255, 255, 255, 0.22)
	scrubTranslated = ui.RGBA(120, 180, 255, 0.8)
	scrubMissing    = ui.Hex("#FFB340")
	scrubTick       = ui.RGBA(0, 0, 0, 0.55)
)

// scrubber is the film's progress bar: a white fill over the track, under it a strip marking each cue, blue where it has a translation and dashed orange where it has only one line, and ticks between the 片段. Dragging it moves the time shown and seeks once released, since each seek opens the video again; a click or the arrow keys seek at once. While the pointer is over it, a card above it shows the time there and that moment's line.
func (s screen) scrubber(c *ui.Context) {
	length := s.PlayLength()
	value, held := s.Scrub()
	if !*held {
		*value = s.PlayHead().Seconds()
	}
	hi := length.Seconds()
	if hi <= 0 {
		hi = 1
	}
	bar := ui.SliderBase(c, value, 0, hi).Grow(1).MinWidth(0).Height(22).Label(locales.FilmPlaybackPosition())
	pressed := bar.Pressed()
	switch {
	case pressed:
		*held = true
	case *held:
		*held = false
		s.Seek(seconds(*value))
	case bar.Changed():
		s.Seek(seconds(*value))
	}
	frac := float32(*value / hi)
	cues := s.Subs().Cues
	if s.Subs().Key != s.OpenKey() {
		cues = nil
	}
	chapters := Chapters(cues, length)
	bar.Draw(func(p *ui.Painter, r ui.Rect) {
		at := func(d time.Duration) float32 { return r.X + r.W*float32(d.Seconds()/hi) }
		p.Fill(ui.Rect{X: r.X, Y: r.Y + 7, W: r.W, H: 5}, scrubTrack, 2.5)
		p.Clip(ui.Rect{X: r.X, Y: r.Y + 7, W: r.W, H: 5}, 2.5, func() {
			p.Fill(ui.Rect{X: r.X, Y: r.Y + 7, W: r.W * min(max(frac, 0), 1), H: 5}, onGlass, 0)
		})
		for _, cue := range cues {
			x0, x1 := at(cue.Start), at(cue.End)
			if x1 <= x0 {
				continue
			}
			if translation, _ := splitCue(cue.Text); translation != "" {
				p.Fill(ui.Rect{X: x0, Y: r.Y + 15, W: x1 - x0, H: 3}, scrubTranslated, 0)
				continue
			}
			for x := x0; x < x1; x += 6 {
				p.Fill(ui.Rect{X: x, Y: r.Y + 15, W: min(3, x1-x), H: 3}, scrubMissing, 0)
			}
		}
		for i, ch := range chapters {
			if i > 0 {
				p.Fill(ui.Rect{X: at(ch.Start) - 1, Y: r.Y + 7, W: 2, H: 5}, scrubTick, 0)
			}
		}
	})
	x, _, over := bar.PointerPosition()
	if !over && !pressed {
		return
	}
	w := bar.Bounds().W
	if !over || w <= 0 {
		x = frac * w
	}
	when := seconds(hi * float64(min(max(x/max(w, 1), 0), 1)))
	bar.Children(func() { s.scrubPreview(c, x, when, cues) })
}

// scrubPreview is the card above the scrubber at x: the time there and the line playing then, the translation or else the original.
func (s screen) scrubPreview(c *ui.Context, x float32, when time.Duration, cues []core.Segment) {
	line := ""
	if cue, ok := CueAt(cues, when-s.Delay()); ok {
		translation, original := splitCue(cue.Text)
		line = translation
		if line == "" {
			line = original
		}
	}
	// A row of no width at x centers the card on it, over either side.
	ui.Row(c).Absolute().Left(x).Bottom(30).Width(0).Justify(ui.Center).PassThrough().Children(func() {
		card := ui.Column(c).Shrink(0).MaxWidth(280).Gap(2).Padding(7, 10).Radius(10).Background(ui.RGBA(28, 28, 32, 0.9)).TextColor(onGlass)
		card.InsetShadow(0, 0, 0, 0.5, ui.RGBA(255, 255, 255, 0.2)).Shadow(0, 8, 24, 0, ui.RGBA(0, 0, 0, 0.35))
		card.Children(func() {
			ui.Text(c, cueClock(when)).FontSize(12).Bold().FontFeatures("tnum")
			if line != "" {
				ui.Text(c, line).FontSize(12).Opacity(0.85).Wrap()
			}
		})
	})
}

func seconds(v float64) time.Duration { return time.Duration(v * float64(time.Second)) }
