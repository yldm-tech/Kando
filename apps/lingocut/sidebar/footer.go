package sidebar

import (
	"github.com/egoist/mygo/ui"
	breeze "github.com/yldm-tech/mygo-breeze-ui/ui/breeze"
)

// ReadyState is how far the app is from starting a job.
type ReadyState int

const (
	// Checking is a check still running, as the EveryAPI sign-in, which may take seconds.
	Checking ReadyState = iota
	// Ready means a job can start.
	Ready
	// Missing means something a job needs is missing; Readiness.Message names it.
	Missing
)

// Readiness is the status line at the foot of the sidebar: a dot, a message and the settings section that fixes what is missing.
type Readiness struct {
	State   ReadyState
	Message string
	// Section is the settings section the gear opens, one of the settings package's Section values; "" opens 通用.
	Section string
}

var (
	footerFill     = ui.RGBA(255, 255, 255, 0.55)
	footerHairline = ui.RGBA(0, 0, 0, 0.06)
	footerText     = ui.Hex("#3A3A3F")
	readyDot       = ui.Hex("#30D158")
	missingDot     = ui.Hex("#FF9F0A")
)

// footer is the status card pinned to the bottom of the sidebar: an 8-point dot, green when a job can start and orange when something is missing, the message saying which, and a gear that opens the settings where it is fixed.
func footer(c *ui.Context, t *ui.Theme, a App) {
	r := a.Readiness()
	dot := t.TextMuted
	switch r.State {
	case Ready:
		dot = readyDot
	case Missing:
		dot = missingDot
	}
	// The hairline is a ring outside the edge that takes no room, as the prototype's box-shadow. The message is the prototype's 11 points on 15, a point above the platform's caption, and the gear's button its 26-point square, the gear in the message's color.
	ui.Row(c).Shrink(0).Margin(8, insetEnd, 8, insetStart).Padding(10, 12).Gap(10).AlignItems(ui.Center).Radius(11).Background(footerFill).Shadow(0, 0, 0, 0.5, footerHairline).Label("状态").Children(func() {
		ui.Box(c).Size(8, 8).Shrink(0).Radius(4).Background(dot)
		breeze.Text(c, r.Message, breeze.TextCaption).FontSize(11).LineHeight(15.0 / 11).TextColor(footerText).Grow(1).MinWidth(0).Wrap()
		if breeze.FlatIconButton(c, iconGear, "设置").Size(26, 26).TextColor(footerText).Shrink(0).Clicked() {
			a.OpenSettings(r.Section)
		}
	})
}
