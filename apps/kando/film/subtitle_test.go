package film

import (
	"image"
	"testing"

	"github.com/egoist/mygo/ui"
)

// TestSubtitleOutlineFollowsTheGlyphs draws a white line over white and checks that the outline makes it readable: dark pixels inside the line's box, among the glyphs, and none a little outside it, where a box's shadow would be.
func TestSubtitleOutlineFollowsTheGlyphs(t *testing.T) {
	line := styledLine{text: "每条河都始于高山", face: Face{Size: 30, Color: "#FFFFFF", Bold: true}}
	for _, outline := range []bool{false, true} {
		tt := ui.NewTester(func(c *ui.Context) {
			ui.Column(c).Fill().Background(ui.Hex("#FFFFFF")).AlignItems(ui.Center).Justify(ui.Center).Children(func() {
				subtitleLine(c, line, false, outline)
			})
		}, 400, 120)
		tt.Frame()
		r, ok := tt.Find(line.text)
		if !ok {
			t.Fatalf("missing the line in %q", tt.Texts())
		}
		img := tt.Image()
		scale := float32(img.Bounds().Dx()) / 400
		inside := darkPixels(img, scale, r)
		around := darkPixels(img, scale, ui.Rect{X: r.X - 8, Y: r.Y - 8, W: 4, H: r.H + 16}) + darkPixels(img, scale, ui.Rect{X: r.X + r.W + 4, Y: r.Y - 8, W: 4, H: r.H + 16})
		switch {
		case !outline && inside != 0:
			t.Fatalf("white on white without an outline has %d dark pixels", inside)
		case outline && inside < 200:
			t.Fatalf("outline left only %d dark pixels among the glyphs", inside)
		case around != 0:
			t.Fatalf("%d dark pixels outside the line's box: the outline is a box's shadow, not the glyphs'", around)
		}
	}
}

// darkPixels counts the pixels of r, in DIPs, darker than mid gray.
func darkPixels(img *image.RGBA, scale float32, r ui.Rect) int {
	n := 0
	for y := int(r.Y * scale); y < int((r.Y+r.H)*scale); y++ {
		for x := int(r.X * scale); x < int((r.X+r.W)*scale); x++ {
			if !(image.Point{X: x, Y: y}.In(img.Bounds())) {
				continue
			}
			px := img.RGBAAt(x, y)
			if int(px.R)+int(px.G)+int(px.B) < 3*100 {
				n++
			}
		}
	}
	return n
}
