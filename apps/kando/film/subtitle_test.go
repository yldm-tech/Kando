package film

import (
	"image"
	"slices"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
	breeze "github.com/yldm-tech/mygo-breeze-ui/ui/breeze"
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

// TestStyleFontsArePlatformsOwn checks that the font menu offers macOS's own fonts on macOS, as the prototype has them, and a sans-serif and a serif font on Windows and Linux, where PingFang and Hiragino are not installed and would draw in the default font.
func TestStyleFontsArePlatformsOwn(t *testing.T) {
	for p, want := range map[breeze.Platform][]string{
		breeze.Mac:     {"默认", "苹方", "冬青黑体"},
		breeze.Windows: {"默认", "黑体", "宋体"},
		breeze.Linux:   {"默认", "黑体", "宋体"},
	} {
		if got := styleFonts(p); !slices.Equal(got, want) {
			t.Errorf("%s offers %q, want %q", p, got, want)
		}
	}
}

// TestFontFamilyHasAFontOnEverySystem checks that each saved family other than 默认 draws in a font of its kind on each system: its list names a font macOS has, one Windows has and one a Linux with Noto CJK has, so a family saved on one system does not fall back to the default font on another, and the menu's choices look apart everywhere.
func TestFontFamilyHasAFontOnEverySystem(t *testing.T) {
	systems := map[string]map[string][]string{
		"sans": {
			"darwin":  {"PingFang SC", "Hiragino Sans GB"},
			"windows": {"Microsoft YaHei"},
			"linux":   {"Noto Sans CJK SC"},
		},
		"serif": {
			"darwin":  {"Songti SC"},
			"windows": {"SimSun"},
			"linux":   {"Noto Serif CJK SC"},
		},
	}
	kinds := map[string]string{"苹方": "sans", "冬青黑体": "sans", "黑体": "sans", "宋体": "serif"}
	for family, kind := range kinds {
		list := strings.Split(fontFamily(family), ", ")
		for goos, fonts := range systems[kind] {
			if !slices.ContainsFunc(fonts, func(f string) bool { return slices.Contains(list, f) }) {
				t.Errorf("%s draws in %q, none of %s's %s fonts %q", family, list, goos, kind, fonts)
			}
		}
	}
	if got := fontFamily("苹方"); !strings.HasPrefix(got, "PingFang SC,") {
		t.Errorf("苹方 draws in %q, want PingFang SC first", got)
	}
	if got := fontFamily("宋体"); strings.Contains(got, "Sans") {
		t.Errorf("宋体 falls back to a sans-serif font: %q", got)
	}
	for _, family := range []string{"默认", "", "Comic Sans"} {
		if got := fontFamily(family); got != "" {
			t.Errorf("%q draws in %q, want the system's font", family, got)
		}
	}
}

// TestFontNamesFollowTheInterface checks that the style view names every saved family, the new ones too, and shows one it does not know as saved.
func TestFontNamesFollowTheInterface(t *testing.T) {
	for family, want := range map[string]string{"默认": "默认", "苹方": "苹方", "冬青黑体": "冬青黑体", "黑体": "黑体", "宋体": "宋体", "Comic Sans": "Comic Sans"} {
		if got := fontName(family); got != want {
			t.Errorf("fontName(%q) = %q, want %q", family, got, want)
		}
	}
}
