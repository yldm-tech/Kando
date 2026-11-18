package tray

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"math"
)

// iconSize is the icon's side in pixels: the menu bar's 18 points at twice the pixels, which macOS scales down to the bar's height.
const iconSize = 36

// The ring the icon draws, in pixels: as the design's 16-point glyph, a circle of radius 6.6 drawn 1.6 wide, at iconSize.
const (
	ringRadius = 6.6 * iconSize / 16
	ringWidth  = 1.6 * iconSize / 16
	// trackAlpha is how opaque the whole ring is under the part that shows the progress, as the design's stroke-opacity 0.25.
	trackAlpha = 0.25
)

// samples is how many points across each pixel is sampled at, so the ring's edges are smooth.
const samples = 4

// Icon is the menu bar icon for s, as a template PNG macOS draws in the menu bar's colors: a faint ring, with an arc from the top, clockwise, over as much of it as the running job has gone. While nothing runs or the queue is paused, the ring is empty.
func Icon(s Status) []byte {
	fraction := 0.0
	if s.Running != nil && !s.Paused {
		fraction = float64(s.percent()) / 100
	}
	return ring(fraction)
}

// ring draws the ring with fraction of it, from 0 to 1, filled.
func ring(fraction float64) []byte {
	img := image.NewNRGBA(image.Rect(0, 0, iconSize, iconSize))
	c := float64(iconSize) / 2
	sweep := fraction * 2 * math.Pi
	// The arc's ends are round, as the design's stroke-linecap: its start at the top and its end where the progress reached.
	end := [2]float64{c + ringRadius*math.Sin(sweep), c - ringRadius*math.Cos(sweep)}
	start := [2]float64{c, c - ringRadius}
	for y := range iconSize {
		for x := range iconSize {
			track, arc := 0, 0
			for sy := range samples {
				for sx := range samples {
					px := float64(x) + (float64(sx)+0.5)/samples
					py := float64(y) + (float64(sy)+0.5)/samples
					dx, dy := px-c, py-c
					onRing := math.Abs(math.Hypot(dx, dy)-ringRadius) <= ringWidth/2
					if onRing {
						track++
					}
					if fraction <= 0 {
						continue
					}
					// The angle from the top, clockwise, from 0 to 2π.
					angle := math.Atan2(dx, -dy)
					if angle < 0 {
						angle += 2 * math.Pi
					}
					if onRing && angle <= sweep || near(px, py, start) || near(px, py, end) {
						arc++
					}
				}
			}
			alpha := max(trackAlpha*float64(track), float64(arc)) / (samples * samples)
			if alpha > 0 {
				img.SetNRGBA(x, y, color.NRGBA{A: uint8(math.Round(alpha * 255))})
			}
		}
	}
	var buf bytes.Buffer
	// Encoding an image in memory does not fail.
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}

// near reports whether the point (x, y) is within half the ring's width of p, the round end of the arc there.
func near(x, y float64, p [2]float64) bool {
	return math.Hypot(x-p[0], y-p[1]) <= ringWidth/2
}
