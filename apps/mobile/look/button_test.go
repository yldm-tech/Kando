package look

import (
	"testing"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
	"github.com/nsgo-dev/nsgo/ui"
)

func TestMain(m *testing.M) { nsgotest.Main(m) }

// A busy primary button keeps the accent's fill, its spinner drawn in the text color on it.
func TestBusyPrimaryKeepsItsFill(t *testing.T) {
	var colors ui.Colors
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		colors = ui.ColorsOf(cx)
		return Button{TestID: "go", Text: func() string { return "Go" }, Kind: Primary, Busy: func() bool { return true }, OnPress: func() {}}
	}, nsgotest.Environment(core.Environment{OS: core.OSIOS, Idiom: core.IdiomPhone}))
	if got := app.Find("id:go").Style.Background; got != colors.Accent() {
		t.Fatalf("the busy primary button is filled with %#x, want the accent %#x", uint32(got), uint32(colors.Accent()))
	}
}
