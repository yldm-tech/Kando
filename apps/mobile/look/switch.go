package look

import (
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

// Switch is an on and off switch as the platform draws one: the system's own (ui.SwitchSystem), NSSwitch on macOS, UISwitch on iOS and Material 3's on Android. Label is what assistive technology reads.
type Switch struct {
	Checked  func() bool
	OnChange func(bool)
	Disabled func() bool
	Label    string
	TestID   string
}

// Render implements core.Node.
func (s Switch) Render(cx *core.Scope) core.Node {
	return ui.Switch{Checked: s.Checked, OnChange: s.OnChange, Disabled: s.Disabled, Label: s.Label, TestID: s.TestID, Variant: ui.SwitchSystem}
}
