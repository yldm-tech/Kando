package film

import (
	"time"

	"kando/apps/kando/live"
	"kando/apps/kando/locales"
	"kando/packages/core"

	"github.com/nsgo-dev/nsgo/base"
	ns "github.com/nsgo-dev/nsgo/core"
)

// The scrubber's marks, from the prototype (template 606-618).
var (
	scrubTrack      = ns.RGBA(0xFFFFFF, 0.22)
	scrubTranslated = ns.RGBA(0x78B4FF, 0.8)
	scrubMissing    = ns.RGB(0xFFB340)
	scrubTick       = ns.RGBA(0x000000, 0.55)
	// scrubLoop is the band behind the track over the span the player loops.
	scrubLoop = ns.RGBA(0xFFD60A, 0.45)
)

// scrubberHeight is the scrubber's: the track 7 down, 5 high, and the strip of the cues 15 down, 3 high.
const scrubberHeight ns.Points = 22

// scrubber is the film's progress bar: a white fill over the track, behind it a band over the span looped, under it a strip marking each cue, blue where it has a translation and dashed orange where it has only one line, and ticks between the 片段. Dragging it moves the time shown and seeks once released; a click or the arrow keys seek at once. While the pointer is over it, a card above it shows the time there and that moment's line.
func (s *screen) scrubber(cx *ns.Scope) ns.Node {
	a := s.a
	length := live.Func(a, func() float64 { return max(a.PlayLength().Seconds(), 1) })
	cues := filmCues(cx, a)
	loop := func() (time.Duration, time.Duration, bool) { a.Track(); return a.LoopSpan() }
	width := ns.NewSignal(cx, ns.Points(0))
	hover := ns.NewSignal(cx, ns.Points(-1))
	draw := func(d *ns.Drawing, size ns.Size) {
		hi := length()
		w := float32(size.Width)
		at := func(t time.Duration) float32 { return w * float32(t.Seconds()/hi) }
		frac := float32(s.shownHead().Seconds() / hi)
		// The span looped, where the picture plays it, the subtitle delay counted, to the video's end for a loop without one.
		if from, to, ok := loop(); ok {
			delay := a.Delay()
			x1 := w
			if to > 0 {
				x1 = at(to + delay)
			}
			x0 := min(max(at(from+delay), 0), w)
			x1 = min(max(x1, x0), w)
			d.SetFillColor(scrubLoop).BeginPath().RoundRect(x0, 4, x1-x0, 11, 3).Fill()
		}
		d.SetFillColor(scrubTrack).BeginPath().RoundRect(0, 7, w, 5, 2.5).Fill()
		d.Save().BeginPath().RoundRect(0, 7, w, 5, 2.5).Clip()
		d.SetFillColor(onGlass).FillRect(0, 7, w*min(max(frac, 0), 1), 5)
		d.Restore()
		list := cues()
		for _, cue := range list {
			x0, x1 := at(cue.Start), at(cue.End)
			if x1 <= x0 {
				continue
			}
			if translation, _ := splitCue(cue.Text); translation != "" {
				d.SetFillColor(scrubTranslated).FillRect(x0, 15, x1-x0, 3)
				continue
			}
			for x := x0; x < x1; x += 6 {
				d.SetFillColor(scrubMissing).FillRect(x, 15, min(3, x1-x), 3)
			}
		}
		for i, ch := range Chapters(list, seconds(hi)) {
			if i > 0 {
				d.SetFillColor(scrubTick).FillRect(at(ch.Start)-1, 7, 2, 5)
			}
		}
	}
	// TODO(nsgo#145): give base.Slider's Label a LabelFunc twin, so the scrubber's accessible name follows the interface language as it changes; it is read once here https://github.com/nsgo-dev/nsgo/issues/145
	slider := base.Slider{
		TestID: "film-scrubber", Label: locales.FilmPlaybackPosition(cx),
		Min: func() float64 { return 0 }, Max: length,
		Value: func() base.SliderValue { return base.SliderValue{End: s.shownHead().Seconds()} },
		OnChange: func(v base.SliderValue) {
			s.held.Set(true)
			s.scrub.Set(v.End)
		},
		OnChangeEnd: live.On(a, func(v base.SliderValue) {
			s.held.Set(false)
			a.Seek(seconds(v.End))
		}),
		Style:      ns.Style{Position: ns.Absolute, Left: ns.Pt(0), Right: ns.Pt(0), Top: ns.Pt(0), Bottom: ns.Pt(0)},
		TrackStyle: ns.Style{FlexGrow: 1},
		ValueText:  func(v float64) string { return cueClock(seconds(v)) },
	}
	preview := ns.Show{When: func() bool { return hover.Get() >= 0 || s.held.Get() }, Then: func(cx *ns.Scope) ns.Node {
		where := func() (ns.Points, time.Duration) {
			w := max(width.Get(), 1)
			x := hover.Get()
			if s.held.Get() || x < 0 {
				x = w * ns.Points(s.shownHead().Seconds()/length())
			}
			x = min(max(x, 0), w)
			return x, seconds(length() * float64(x/w))
		}
		line := func() string {
			_, when := where()
			cue, ok := CueAt(cues(), when-a.Delay())
			if !ok {
				return ""
			}
			translation, original := splitCue(cue.Text)
			if translation == "" {
				return original
			}
			return translation
		}
		// A row of no width at x centers the card on it, over either side.
		return ns.View{PointerEvents: ns.PointerEventsNone,
			Style: ns.Style{Position: ns.Absolute, Bottom: ns.Pt(30), Width: ns.Pt(0), Direction: ns.Row, JustifyContent: ns.JustifyCenter},
			StyleFunc: func() ns.Style {
				x, _ := where()
				return ns.Style{Left: ns.Pt(x)}
			},
			Children: []ns.Node{ns.View{
				Style: ns.Style{FlexShrink: 0, MaxWidth: ns.Pt(280), Gap: 2, Padding: ns.Symmetric(7, 10), CornerRadius: 10, Background: ns.RGBA(0x1C1C20, 0.9),
					Shadows: []ns.BoxShadow{{Spread: 0.5, Color: ns.RGBA(0xFFFFFF, 0.2), Inset: true}, {Offset: ns.Point{Y: 8}, Blur: 24, Color: ns.RGBA(0x000000, 0.35)}}},
				Children: []ns.Node{
					ns.Text{TestID: "film-scrub-time", ContentFunc: func() string { _, when := where(); return cueClock(when) },
						Style: ns.Style{FontSize: 12, LineHeight: 16, FontWeight: ns.FontWeightBold, FontVariant: ns.FontTabularNums, Color: onGlass}},
					ns.Show{When: func() bool { return line() != "" }, Then: func(*ns.Scope) ns.Node {
						return ns.Text{ContentFunc: line, Style: ns.Style{FontSize: 12, LineHeight: 16, Color: onGlass, Opacity: ns.Alpha(0.85)}}
					}},
				},
			}}}
	}}
	return ns.View{
		Style:        ns.Style{FlexGrow: 1, MinWidth: ns.Pt(0), Height: ns.Pt(scrubberHeight)},
		OnLayout:     func(b ns.Bounds) { width.Set(b.Size.Width) },
		OnMouseMove:  func(e ns.MouseEvent) { hover.Set(e.Position.X) },
		OnMouseLeave: func(ns.MouseEvent) { hover.Set(-1) },
		Children: []ns.Node{
			ns.Canvas{TestID: "film-scrub-marks", Style: fillStyle, Draw: draw},
			slider,
			preview,
		},
	}
}

// filmCues is the open film's cues once its subtitles are read, nil before, which tells what reads it only when the subtitles are read again.
func filmCues(cx *ns.Scope, a App) func() []core.Segment {
	return ns.NewMemoFunc(cx, live.Func(a, func() []core.Segment {
		if subs := a.Subs(); subs.Key == a.OpenKey() && subs.Ready && subs.Err == "" {
			return subs.Cues
		}
		return nil
	}), sameCues).Get
}

// sameCues reports whether x and y are the same cues, the same slice of them.
func sameCues(x, y []core.Segment) bool {
	return len(x) == len(y) && (len(x) == 0 || &x[0] == &y[0])
}
