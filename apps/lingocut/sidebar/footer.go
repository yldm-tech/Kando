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
	ui.Row(c).Shrink(0).Margin(8).Padding(10, 12).Gap(10).AlignItems(ui.Center).Radius(11).Background(footerFill).Border(0.5, footerHairline).Label("状态").Children(func() {
		ui.Box(c).Size(8, 8).Shrink(0).Radius(4).Background(dot)
		breeze.Text(c, r.Message, breeze.TextCaption).Grow(1).MinWidth(0).Wrap()
		if flatIconButton(c, iconGear, "设置").Shrink(0).Clicked() {
			a.OpenSettings(r.Section)
		}
	})
}

// flatIconButton stands in for breeze.FlatIconButton (yldm-tech/mygo-breeze-ui#15) until a breeze release has it, with the same arguments: the bordered IconButton in its place. Replace the call with breeze.FlatIconButton then.
func flatIconButton(c *ui.Context, icon *ui.SVG, label string) ui.Element {
	return breeze.IconButton(c, icon, label)
}
