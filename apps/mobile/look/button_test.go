package look

import (
	"slices"
	"testing"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
	"github.com/nsgo-dev/nsgo/ui"
)

func TestMain(m *testing.M) { nsgotest.Main(m) }

// A busy primary button keeps the accent's fill, its spinner drawn in the text color on it: the ring in OnAccent over a track a quarter as opaque, not the accent, which would vanish on the fill.
func TestBusyPrimaryKeepsItsFill(t *testing.T) {
	var colors ui.Colors
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		colors = ui.ColorsOf(cx)
		return Button{TestID: "go", Text: func() string { return "Go" }, Kind: Primary, Busy: func() bool { return true }, OnPress: func() {}}
	}, nsgotest.Environment(core.Environment{OS: core.OSIOS, Idiom: core.IdiomPhone}))
	button := app.Find("id:go")
	if got := button.Style.Background; got != colors.Accent() {
		t.Fatalf("the busy primary button is filled with %#x, want the accent %#x", uint32(got), uint32(colors.Accent()))
	}
	ink := colors.OnAccent()
	if got, want := buttonStrokes(button), []core.Color{ui.Alpha(ink, 0.25), ink}; !slices.Equal(got, want) {
		t.Fatalf("the busy primary button's spinner strokes %x, want %x in the text color", got, want)
	}
}

// buttonStrokes are the colors drawn on the canvases from v down, in order: the spinner's ring, its track and then its arc.
func buttonStrokes(v *nsgotest.View) []core.Color {
	var out []core.Color
	for _, op := range v.Ops {
		if op.Color != 0 {
			out = append(out, core.Color(op.Color))
		}
	}
	for _, c := range v.Children {
		out = append(out, buttonStrokes(c)...)
	}
	return out
}
