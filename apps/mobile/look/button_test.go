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
	// ui.Button's fill is its face, the pressable's one child.
	if got := button.Children[0].Style.Background; got != colors.Accent() {
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

// A tall button is the introduction's call to action: 56 points tall, 16 round, and a primary one lifted by a shadow in the accent.
func TestTallPrimaryIsTheIntroductionsButton(t *testing.T) {
	var colors ui.Colors
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		colors = ui.ColorsOf(cx)
		return core.View{Children: []core.Node{Button{TestID: "go", Text: func() string { return "Go" }, Kind: Primary, Tall: true, OnPress: func() {}}}}
	}, nsgotest.Environment(core.Environment{OS: core.OSIOS, Idiom: core.IdiomPhone}))
	button := app.Find("id:go")
	if got := button.Frame.Size.Height; got != 56 {
		t.Fatalf("the tall button is %v points tall, want 56", got)
	}
	if got := button.Style.CornerRadius; got != 16 {
		t.Fatalf("the tall button is %v round, want 16", got)
	}
	fx := button.Style.Effects
	if fx == nil || len(fx.Shadows) != 1 || fx.Shadows[0].Color != ui.Alpha(colors.Accent(), 0.3) || fx.Shadows[0].Offset.Y != 10 {
		t.Fatalf("the tall primary button casts %+v, want one shadow in the accent at 30%% 10 points down", fx)
	}
}

// A button's text is the design's 17pt, semibold, bold on a tall one, and a plain button's is the accent.
func TestButtonTextIsTheDesignsType(t *testing.T) {
	var colors ui.Colors
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		colors = ui.ColorsOf(cx)
		return core.View{Children: []core.Node{
			Button{TestID: "go", Text: func() string { return "Go" }, Kind: Tinted, OnPress: func() {}},
			Button{TestID: "start", Text: func() string { return "Start" }, Kind: Primary, Tall: true, OnPress: func() {}},
			Button{TestID: "later", Text: func() string { return "Later" }, Kind: Plain, OnPress: func() {}},
		}}
	}, nsgotest.Environment(core.Environment{OS: core.OSIOS, Idiom: core.IdiomPhone}))
	for _, c := range []struct {
		text   string
		weight core.FontWeight
	}{{"Go", core.FontWeightSemibold}, {"Start", core.FontWeightBold}, {"Later", core.FontWeightSemibold}} {
		if got := app.Find("text:" + c.text).TextStyle.Font; got.Size != 17 || got.Weight != c.weight {
			t.Errorf("%s is set in %v at weight %v, want 17 at %v", c.text, got.Size, got.Weight, c.weight)
		}
	}
	if got := app.Find("text:Later").TextStyle.Color; got != colors.Accent() {
		t.Errorf("the plain button's text is %#x, want the accent %#x", uint32(got), uint32(colors.Accent()))
	}
	if got := app.Find("id:later").Frame.Size.Height; got != 44 {
		t.Errorf("the plain button is %v points tall, want 44", got)
	}
}
