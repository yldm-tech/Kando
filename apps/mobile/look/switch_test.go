package look

import (
	"testing"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

// A switch is the system's, an iPhone's UISwitch and an Android phone's Material 3 switch, not one drawn in parts.
func TestASwitchIsTheSystems(t *testing.T) {
	if nsgotest.Target() != "" {
		t.Skip("the platform is the target's")
	}
	for _, os := range []core.OS{core.OSIOS, core.OSAndroid} {
		app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
			on := core.NewSignal(cx, true)
			return Switch{TestID: "sw", Label: "Remind", Checked: on.Get, OnChange: on.Set}
		}, nsgotest.Environment(core.Environment{OS: os, Idiom: core.IdiomPhone}))
		// The system's switch is mounted with no frame headless, so Find, which sees only what shows, would not find it.
		views := app.QueryMounted("id:sw")
		if len(views) != 1 {
			t.Fatalf("on %v %d switches are mounted, want 1", os, len(views))
		}
		if v := views[0]; len(v.Children) != 0 {
			t.Fatalf("on %v the switch is drawn in %d parts, not the system's", os, len(v.Children))
		}
	}
}
