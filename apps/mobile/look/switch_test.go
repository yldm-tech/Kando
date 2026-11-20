package look

import (
	"testing"

	"kando/apps/mobile/theme"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

// On Android a switch is Material 3's: a press asks for the other value, and the track is the fill's in the outline while off.
func TestASwitchOnAndroidIsMaterials(t *testing.T) {
	if nsgotest.Target() != "" {
		t.Skip("the platform is the target's")
	}
	var on *core.Signal[bool]
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		on = core.NewSignal(cx, false)
		return Switch{TestID: "sw", Label: "Remind", Checked: on.Get, OnChange: on.Set}
	}, nsgotest.Environment(core.Environment{OS: core.OSAndroid, Idiom: core.IdiomPhone}))
	track := app.Find("id:sw").Children[0]
	if track.Style.BorderColor != theme.M3Outline {
		t.Fatalf("the off switch's outline is %#x, want M3Outline", uint32(track.Style.BorderColor))
	}
	app.Press("id:sw")
	if !on.Get() {
		t.Fatal("a press on the off switch did not turn it on")
	}
}

// Elsewhere a switch is the system's, not Material's.
func TestASwitchOnIOSIsNotMaterials(t *testing.T) {
	if nsgotest.Target() != "" {
		t.Skip("the platform is the target's")
	}
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		on := core.NewSignal(cx, true)
		return Switch{TestID: "sw", Label: "Remind", Checked: on.Get, OnChange: on.Set}
	}, nsgotest.Environment(core.Environment{OS: core.OSIOS, Idiom: core.IdiomPhone}))
	// The system's switch is mounted with no frame headless, so Find, which sees only what shows, would not find it.
	views := app.QueryMounted("id:sw")
	if len(views) != 1 {
		t.Fatalf("%d switches are mounted, want 1", len(views))
	}
	if v := views[0]; len(v.Children) != 0 {
		t.Fatalf("an iPhone's switch is drawn in %d parts, not the system's", len(v.Children))
	}
}
