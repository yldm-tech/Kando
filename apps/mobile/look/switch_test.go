package look

import (
	"testing"
	"time"

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
			return Switch{TestID: "sw", Label: func() string { return "Remind" }, Checked: on.Get, OnChange: on.Set}
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

// A switch's spoken name is bound: it follows the text its Label reads, as the interface language changes, without the switch being built again.
func TestASwitchsNameFollowsItsLabel(t *testing.T) {
	var name *core.Signal[string]
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		name = core.NewSignal(cx, "Remind")
		on := core.NewSignal(cx, true)
		return Switch{TestID: "sw", Label: name.Get, Checked: on.Get, OnChange: on.Set}
	})
	label := func() string {
		views := app.QueryMounted("id:sw")
		if len(views) != 1 {
			t.Fatalf("%d switches are mounted, want 1", len(views))
		}
		return views[0].Props.Label
	}
	if got := label(); got != "Remind" {
		t.Fatalf("the switch is read as %q, want Remind", got)
	}
	name.Set("提醒")
	app.WaitUntil(time.Second, func() bool { return label() == "提醒" })
}
